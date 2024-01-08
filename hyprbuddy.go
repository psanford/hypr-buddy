package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/psanford/hypr-buddy/client"
	"github.com/psanford/hypr-buddy/hyprctl"
	"github.com/psanford/hypr-buddy/server"
)

var doGotoNextWorkspace = flag.Bool("ws-next", false, "goto next workspace")
var doGotoPrevWorkspace = flag.Bool("ws-prev", false, "goto next workspace")

var doMasterGrow = flag.Bool("master-grow", false, "grow master region")
var doMasterShrink = flag.Bool("master-shrink", false, "shrink master region")
var doToggleStack = flag.Bool("toggle-stack", false, "toggle stacked windows")
var runDaemon = flag.Bool("daemon", false, "run daemon")
var doPing = flag.Bool("ping", false, "ping daemon")

var doFocusNext = flag.Bool("focus-next", false, "focus next window")
var doFocusPrev = flag.Bool("focus-prev", false, "focus prev window")
var doUnhideAll = flag.Bool("unhide-all", false, "reset all hidden windows")

const wsMax = 10
const wsMin = 1

func main() {
	flag.Parse()

	if *runDaemon {
		ctx := context.Background()
		server.New().Serve(ctx)
	} else if *doGotoNextWorkspace {
		gotoNextWS(1)
	} else if *doGotoPrevWorkspace {
		gotoNextWS(-1)
	} else if *doMasterGrow {
		masterGrow(0.05)
	} else if *doMasterShrink {
		masterGrow(-0.05)
	} else if *doPing {
		err := client.NewClient().Ping()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")

	} else if *doToggleStack {
		err := client.NewClient().ToggleStack()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")
	} else if *doFocusNext {
		err := client.NewClient().FocusNext()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")
	} else if *doFocusPrev {
		err := client.NewClient().FocusPrev()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")
	} else if *doUnhideAll {
		err := client.NewClient().UnhideAll()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")
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
