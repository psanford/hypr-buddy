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
	"strconv"
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

var (
	OpenWindowEvt  = "openwindow"
	CloseWindowEvt = "closewindow"
)

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
	mux.HandleFunc("/debug", s.handleDebugState)
	mux.HandleFunc("/debug/state", s.handleDebugState)
	mux.HandleFunc("/toggle-stack", s.handleToggleStack)
	mux.HandleFunc("/focus", s.handleFocus)
	mux.HandleFunc("/unhide-all", s.handleUnhideAll)
	mux.HandleFunc("/toggle-bling", s.handleToggleBlingMode)

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

	// unhide any previously hidden windows
	s.unhideAll()

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

			if evt.Name == OpenWindowEvt {
				parts := strings.SplitN(evt.Data, ",", 2)
				s.handleWindowOpen("0x" + parts[0])

			} else if evt.Name == CloseWindowEvt {
				s.handleWindowClose("0x" + evt.Data)
				//closewindow>>cdb5bd0

			}

			// log.Printf("window evt: %s %s", evt.Name, evt.Data)
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

	hiddenName := hiddenWSName(wsInfo.ID)

	if wsState.Layout == LayoutSingleWindow {
		windowOrder := make([]string, 0, 10)

		wsWindows := make([]hyprctl.Window, 0, 10)
		for _, w := range allWindows {
			if w.Workspace.ID != wsInfo.ID {
				continue
			}

			windowOrder = append(windowOrder, w.Address)
			wsWindows = append(wsWindows, w)
		}

		// move all the windows except the master to the shadow workspace
		for _, w := range wsWindows[1:] {
			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %s,address:%s", hiddenName, w.Address))
		}

		wsState.WindowOrder = windowOrder

	} else {
		for _, w := range allWindows {
			if w.Workspace.Name != hiddenName {
				continue
			}

			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", wsInfo.ID, w.Address))
		}
		s.moveWindowsToOrder(c, wsInfo, wsState.WindowOrder)
	}
}

func (s *server) handleUnhideAll(w http.ResponseWriter, r *http.Request) {
	s.unhideAll()
}

func (s *server) unhideAll() {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	sort.Sort(WindowSort(allWindows))

	workspaces, err := c.Workspaces()
	if err != nil {
		panic(err)
	}

	for _, ws := range workspaces {
		if ws.ID < 0 {
			continue
		}
		wsState := s.getWSStateByID(ws.ID)

		if wsState.Layout == LayoutSingleWindow {
			wsState.Layout = LayoutPrimaryWithStack
		}

		hiddenName := hiddenWSName(ws.ID)

		var didMove bool
		for _, w := range allWindows {
			if w.Workspace.Name != hiddenName {
				continue
			}

			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", ws.ID, w.Address))
			didMove = true
		}
		if didMove && len(wsState.WindowOrder) > 0 {
			s.moveWindowsToOrder(c, &ws, wsState.WindowOrder)
		}
	}
}

func (s *server) handleToggleBlingMode(w http.ResponseWriter, r *http.Request) {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	opt, err := c.GetOption("animations:enabled")
	if err != nil {
		panic(err)
	}

	animationsEnabled := opt.Int == 1

	type Opt struct {
		Name string
		Val  string
	}

	var cmds []Opt
	if animationsEnabled {
		cmds = []Opt{
			{"animations:enabled", "no"},
			{"general:gaps_in", "0"},
			{"general:gaps_out", "0"},
			{"decoration:rounding", "0"},
		}
	} else {
		cmds = []Opt{
			{"animations:enabled", "yes"},
			{"general:gaps_in", "5"},
			{"general:gaps_out", "20"},
			{"decoration:rounding", "10"},
		}
	}

	for _, cmd := range cmds {
		c.SetOption(cmd.Name, cmd.Val)
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
		startIdx := -100
		for j := 0; j < len(wsWindows); j++ {
			win := wsWindows[j]
			if win.Address == addr {
				log.Printf("move %s (%s) to position %d; cur=%d", addr, win.Class, i, j)
				startIdx = j
				break
			}
		}
		moveAmt := i - startIdx

		if moveAmt > 0 {
			log.Printf("moveAmt %d > 0; this should not happen i=%d j=%d addr=%s", moveAmt, i, startIdx, addr)
			return
		}

		moveAmt *= -1

		for n := 0; n < moveAmt; n++ {
			c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", addr))
			c.DispatchRaw("layoutmsg swapprev")

			log.Printf("swap %d %d", startIdx-n, startIdx-n-1)
			wsWindows[startIdx-n], wsWindows[startIdx-n-1] = wsWindows[startIdx-n-1], wsWindows[startIdx-n]
		}
	}

	if len(desiredOrder) > 0 {
		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", desiredOrder[0]))
	}
}

func (s *server) handleFocus(w http.ResponseWriter, r *http.Request) {
	lgr := logmiddleware.LgrFromContext(r.Context())
	n := 1
	nStr := r.FormValue("n")
	if nStr != "" {
		var err error
		n, err = strconv.Atoi(nStr)
		if err != nil {
			lgr.Error("invalid non-numeric n value", "n", nStr)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Bad request invalid non-numeric n parameter")
			return
		}
	}

	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	wsState := s.getWSStateByID(wsInfo.ID)

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	sort.Sort(WindowSort(allWindows))

	windowsByID := make(map[string]hyprctl.Window)
	windowNames := make(map[string]string)
	for _, w := range allWindows {
		windowNames[w.Address] = fmt.Sprintf("[%s/%s]", w.InitialClass, w.InitialTitle)
		windowsByID[w.Address] = w
	}

	hiddenName := hiddenWSName(wsInfo.ID)

	if wsState.Layout == LayoutPrimaryWithStack {
		cmd := "layoutmsg cyclenext"
		if n < 0 {
			cmd = "layoutmsg cycleprev"
		}
		log.Printf("Multi layout, cmd: %s", cmd)
		c.DispatchRaw(cmd)
	} else {
		if len(wsState.WindowOrder) < 2 {
			log.Printf("window order < 2, nothing to toggle")
			return
		}

		oldMaster := wsState.WindowOrder[0]

		var newMaster string
		newOrder := make([]string, len(wsState.WindowOrder))
		if n < 0 { // cycle prev
			newMaster = wsState.WindowOrder[len(wsState.WindowOrder)-1]

			// before:
			// [a, b, c, d, e]
			// after:
			// [e, a, b, c, d]

			newOrder[0] = newMaster
			newOrder[1] = oldMaster
			if len(wsState.WindowOrder) > 2 {
				copy(newOrder[2:], wsState.WindowOrder[1:len(wsState.WindowOrder)-1])
			}

		} else { // cycle next
			newMaster = wsState.WindowOrder[1]

			// before:
			// [a, b, c, d, e]
			// after:
			// [b, c, d, e, a]

			copy(newOrder, wsState.WindowOrder[1:])
			newOrder[len(newOrder)-1] = oldMaster
		}

		wsState.WindowOrder = newOrder

		c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", wsInfo.ID, newMaster))

		c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %s,address:%s", hiddenName, oldMaster))
	}
}

func (s *server) handleWindowOpen(id string) {
	log.Printf("evt window open %s", id)
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	wsState := s.getWSStateByID(wsInfo.ID)

	if wsState.Layout != LayoutSingleWindow {
		return
	}

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	sort.Sort(WindowSort(allWindows))

	for _, w := range allWindows {
		if w.Address == id {
			if w.Floating {
				// floating windows are not part of the stack
				return
			}
		}
	}

	if len(wsState.WindowOrder) > 0 {
		oldMaster := wsState.WindowOrder[0]
		hiddenName := hiddenWSName(wsInfo.ID)
		c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %s,address:%s", hiddenName, oldMaster))
	}

	wsState.WindowOrder = append([]string{id}, wsState.WindowOrder...)
}

func (s *server) handleWindowClose(id string) {
	log.Printf("evt window close %s", id)
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	wsState := s.getWSStateByID(wsInfo.ID)

	if wsState.Layout != LayoutSingleWindow {
		return
	}

	if len(wsState.WindowOrder) < 2 {
		return
	}

	if wsState.WindowOrder[0] != id {
		return
	}

	wsState.WindowOrder = wsState.WindowOrder[1:]

	c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", wsInfo.ID, wsState.WindowOrder[0]))
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

func hiddenWSName(id int64) string {
	return fmt.Sprintf("special:hidden-%d", id)
}
