package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/psanford/hypr-buddy/hyprctl"
)

var doGotoNextWorkspace = flag.Bool("ws-next", false, "goto next workspace")
var doGotoPrevWorkspace = flag.Bool("ws-prev", false, "goto next workspace")

// var doFocusNext = flag.Bool("focus-next", false, "focus next window")
// var doFocusPrev = flag.Bool("focus-prev", false, "focus prev window")
var doMasterGrow = flag.Bool("master-grow", false, "grow master region")
var doMasterShrink = flag.Bool("master-shrink", false, "shrink master region")
var doToggleStack = flag.Bool("toggle-stack", false, "toggle stacked windows")
var runDaemon = flag.Bool("daemon", false, "run daemon")

const wsMax = 10
const wsMin = 1

func main() {
	flag.Parse()

	if *runDaemon {
		newServer().Serve()
	} else if *doGotoNextWorkspace {
		gotoNextWS(1)
	} else if *doGotoPrevWorkspace {
		gotoNextWS(-1)
	} else if *doToggleStack {
		toggleStack()
		// } else if *doFocusNext {
		// 	focusNext(1)
		// } else if *doFocusPrev {
		// 	focusNext(-1)
	} else if *doMasterGrow {
		masterGrow(0.05)
	} else if *doMasterShrink {
		masterGrow(-0.05)
	} else {
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func masterGrow(n float64) {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	windows := activeWorkspaceWindows(c)

	master := windows[0]
	width := master.Size[0]

	monitors, err := c.Monitors()
	if err != nil {
		panic(err)
	}

	var monitorWidth float64
	for _, m := range monitors {
		if m.ID == master.Monitor {
			monitorWidth = float64(m.Width) / m.Scale
			break
		}
	}

	curRatio := float64(width) / float64(monitorWidth)

	newRatio := curRatio + n

	c.DispatchRaw(fmt.Sprintf("layoutmsg mfact %.02f", newRatio))
	c.DispatchRaw("forcerendererreload")
}

func gotoNextWS(n int64) {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	nextID := wsInfo.ID + n
	if nextID > wsMax {
		nextID = wsMin
	}
	if nextID < wsMin {
		nextID = wsMax
	}

	err = c.DispatchRaw(fmt.Sprintf("workspace %d", nextID))
	if err != nil {
		panic(err)
	}
}

func activeWorkspaceWindows(c *hyprctl.Client) []hyprctl.Window {
	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	var wsWindows []hyprctl.Window
	for _, w := range allWindows {
		if w.Workspace.ID == wsInfo.ID {
			wsWindows = append(wsWindows, w)
		}
	}
	sort.Sort(WindowSort(wsWindows))

	return wsWindows
}

func focusNext(n int) {
}

func hiddenWSName(ws *hyprctl.Workspace) string {
	return fmt.Sprintf("special:hidden-%d", ws.ID)
}

func toggleStack() {
	c, err := hyprctl.New()
	if err != nil {
		panic(err)
	}

	wsInfo, err := c.ActiveWorkspace()
	if err != nil {
		panic(err)
	}

	allWindows, err := c.Windows()
	if err != nil {
		panic(err)
	}

	// starting layout
	// |---|---|
	// |   | A |
	// |   |---|
	// | M | B |
	// |   |---|
	// |   | C |
	// |---|---|

	sort.Sort(WindowSort(allWindows))

	hiddenName := hiddenWSName(wsInfo)

	var count int
	var masterWindow hyprctl.Window
	otherWindows := make([]hyprctl.Window, 0, len(allWindows))
	for _, w := range allWindows {
		if w.Workspace.ID != wsInfo.ID {
			continue
		}

		if w.Address == wsInfo.LastWindow {
			masterWindow = w
		} else {
			count++
			otherWindows = append(otherWindows, w)
			if !w.Floating {
				c.DispatchRaw(fmt.Sprintf("togglefloating address:%s", w.Address))
			}
		}
	}

	for _, w := range otherWindows {
		c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %s,address:%s", hiddenName, w.Address))
	}

	if count == 0 {
		matchedWindows := make([]hyprctl.Window, 0, 10)
		for _, w := range allWindows {
			if w.Workspace.Name != hiddenName {
				continue
			}

			c.DispatchRaw(fmt.Sprintf("movetoworkspacesilent %d,address:%s", wsInfo.ID, w.Address))
			matchedWindows = append(matchedWindows, w)
		}

		sort.Sort(sort.Reverse(WindowSort(matchedWindows)))

		for _, w := range matchedWindows {
			c.DispatchRaw(fmt.Sprintf("togglefloating address:%s", w.Address))
		}

		// we're now in this configuration:

		// |---|---|
		// |   | A |
		// |   |---|
		// | B | C |
		// |   |---|
		// |   | M |
		// |---|---|

		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", masterWindow.Address))
		c.DispatchRaw("layoutmsg swapwithmaster")

		// |---|---|
		// |   | A |
		// |   |---|
		// | M | C |
		// |   |---|
		// |   | B |
		// |---|---|

		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", masterWindow.Address))
		c.DispatchRaw("layoutmsg cycleprev")
		c.DispatchRaw("layoutmsg swapprev")
		c.DispatchRaw(fmt.Sprintf("focuswindow address:%s", masterWindow.Address))

		// |---|---|
		// |   | A |
		// |   |---|
		// | M | B |
		// |   |---|
		// |   | C |
		// |---|---|
	}
}

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
