package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/psanford/hypr-buddy/hyprctl"
)

var doGotoNextWorkspace = flag.Bool("ws-next", false, "goto next workspace")
var doGotoPrevWorkspace = flag.Bool("ws-prev", false, "goto next workspace")

const wsCount = 10

func main() {
	flag.Parse()

	if *doGotoNextWorkspace {
		gotoNextWS(1)
	} else if *doGotoPrevWorkspace {
		gotoNextWS(-1)
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

	nextID := (wsInfo.ID + n) % wsCount
	if nextID < 1 {
		nextID = 1
	}

	err = c.DispatchRaw(fmt.Sprintf("workspace %d", nextID))
	if err != nil {
		panic(err)
	}
}
