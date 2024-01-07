package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/psanford/hypr-buddy/client"
	"github.com/psanford/hypr-buddy/config"
	"github.com/psanford/hypr-buddy/hyprctl"
	"github.com/psanford/logmiddleware"
)

type server struct {
	windowEvt chan HyprEvent
	userEvt   chan string

	handler http.Handler

	spaces []*WorkspaceDesiredState
}

type WorkspaceDesiredState struct {
	ID     int
	Layout LayoutMode

	WindowOrder []string
}

type LayoutMode int

const (
	LayoutPrimaryWithStack LayoutMode = iota
	LayoutSingleWindow
)

func New() *server {
	s := &server{
		windowEvt: make(chan HyprEvent),
		userEvt:   make(chan string),
		spaces:    make([]*WorkspaceDesiredState, 10), // 1 - 10
	}

	for i := 0; i < len(s.spaces); i++ {
		s.spaces[i] = &WorkspaceDesiredState{
			ID: i + 1,
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/debug/state", s.handleDebugState)
	mux.HandleFunc("/toggleStack", s.handleToggleStack)

	s.handler = logmiddleware.New(mux)

	return s
}

type HyprEvent struct {
	Name string
	Data string
}

func (s *server) Serve(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	go func() {
		err := s.acceptEventsFromHypr(ctx)
		log.Fatal(err)
		cancel()
	}()

	go func() {
		err := s.acceptUserEvents(ctx)
		log.Fatal(err)
		cancel()
	}()

OUTER:
	for {
		select {
		case evt, ok := <-s.windowEvt:
			if !ok {
				break OUTER
			}
			log.Printf("window evt: %s %s", evt.Name, evt.Data)
		case evt := <-s.userEvt:
			log.Printf("user evt: %s", evt)
		case <-ctx.Done():
			log.Printf("ctx done: %s", ctx.Err())
			break OUTER
		}
	}
}

func (s *server) acceptUserEvents(parentCtx context.Context) error {
	sockPath := config.SocketPath()

	_, err := os.Stat(sockPath)
	if err == nil {
		c := client.NewClientWithTimeout(sockPath, 1*time.Second)
		err = c.Ping()
		if err == nil {
			return errors.New("Existing server already running")
		}

		os.Remove(sockPath)
	}

	log.Printf("listen at %s", sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	os.Chmod(config.SocketPath(), 0700)

	return http.Serve(l, s.handler)
}

func (s *server) acceptEventsFromHypr(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return errors.New("HYPRLAND_INSTANCE_SIGNATURE not set")
	}

	path := fmt.Sprintf("/tmp/hypr/%s/.socket2.sock", sig)

	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	r := bufio.NewReader(conn)
	for {
		b, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}

		line := string(b)
		line = strings.TrimSpace(line)

		parts := strings.SplitN(line, ">>", 2)
		if len(parts) < 2 {
			return fmt.Errorf("malformatted event line: <%s>", b)
		}

		event, data := parts[0], parts[1]
		evt := HyprEvent{
			Name: event,
			Data: data,
		}

		select {
		case s.windowEvt <- evt:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

func (s *server) handleDebugState(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	if r.FormValue("p") != "" {
		enc.SetIndent("", "  ")
	}
	enc.Encode(s.spaces)
}

func (s *server) handleToggleStack(w http.ResponseWriter, r *http.Request) {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	wsState := s.getWSStateByID(wsInfo.ID)

	if wsState.Layout == LayoutSingleWindow {
		wsState.Layout = LayoutPrimaryWithStack
	} else {
		wsState.Layout = LayoutSingleWindow
	}

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	sort.Sort(WindowSort(allWindows))

	hiddenName := hiddenWSName(wsInfo)

	if wsState.Layout == LayoutSingleWindow {
		windowOrder := make([]string, 0, 10)

		otherWindows := make([]hyprctl.Window, 0, len(allWindows))
		for _, w := range allWindows {
			if w.Workspace.ID != wsInfo.ID {
				continue
			}

			windowOrder = append(windowOrder, w.Address)

			if w.Address != wsInfo.LastWindow { // LastWindow is the focused window
				otherWindows = append(otherWindows, w)
				if !w.Floating {
					c.DispatchRaw(fmt.Sprintf("togglefloating address:%s", w.Address))
				}
			}
		}

		for _, w := range otherWindows {
			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %s,address:%s", hiddenName, w.Address))
		}

		wsState.WindowOrder = windowOrder

	} else {
		for _, w := range allWindows {
			if w.Workspace.Name != hiddenName {
				continue
			}

			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", wsInfo.ID, w.Address))
			c.DispatchRaw(fmt.Sprintf("togglefloating address:%s", w.Address))
		}
		s.moveWindowsToOrder(c, wsInfo, wsState.WindowOrder)
	}
}

func (s *server) moveWindowsToOrder(c *hyprctl.Client, wsInfo *hyprctl.Workspace, desiredOrder []string) {
	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}
	sort.Sort(WindowSort(allWindows))

	wsWindows := make([]hyprctl.Window, 0, 10)
	for _, w := range allWindows {
		if w.Workspace.ID != wsInfo.ID {
			continue
		}

		wsWindows = append(wsWindows, w)
	}

	for i, addr := range desiredOrder {
		startIdx := -1
		for j, win := range wsWindows {
			if win.Address == addr {
				startIdx = j
			}
		}
		moveAmt := i - startIdx

		if moveAmt < 0 {
			log.Printf("moveAmt %d < 0; this should not happen", moveAmt)
			return
		}

		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", addr))
		for n := 0; i < moveAmt; n++ {
			c.DispatchRaw("layoutmsg swapprev")
		}
	}

	if len(desiredOrder) > 0 {
		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", desiredOrder[0]))
	}
}

func (s *server) handleFocusNext(w http.ResponseWriter, r *http.Request) {
}

func (s *server) handleFocusPrev(w http.ResponseWriter, r *http.Request) {
}

func (s *server) getWSStateByID(id int64) *WorkspaceDesiredState {
	return s.spaces[int(id)-1]
}

// Sorts windows by Workspace and then by order on a workspace
// left to right and then top to bottom
type WindowSort []hyprctl.Window

func (w WindowSort) Len() int {
	return len(w)
}

func (w WindowSort) Less(i, j int) bool {
	a := w[i]
	b := w[j]

	if a.Workspace.ID != b.Workspace.ID {
		return a.Workspace.ID < b.Workspace.ID
	}

	if a.At[0] != b.At[0] {
		return a.At[0] < b.At[0]
	}

	return a.At[1] < b.At[1]
}

func (w WindowSort) Swap(i, j int) {
	w[i], w[j] = w[j], w[i]
}

func hiddenWSName(ws *hyprctl.Workspace) string {
	return fmt.Sprintf("special:hidden-%d", ws.ID)
}
