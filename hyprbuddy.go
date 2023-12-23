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
var doToggleStack = flag.Bool("toggle-stack", false, "toggle stacked windows")

const wsMax = 10
const wsMin = 1

func main() {
	flag.Parse()

	if *doGotoNextWorkspace {
		gotoNextWS(1)
	} else if *doGotoPrevWorkspace {
		gotoNextWS(-1)
	} else if *doToggleStack {
		toggleStack()
	} else {
		flag.PrintDefaults()
		os.Exit(1)
	}
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

	hiddenName := fmt.Sprintf("special:hidden-%d", wsInfo.ID)

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
