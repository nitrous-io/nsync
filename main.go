package main

import (
	"fmt"
	"os"

	"github.com/nitrous-io/nsync/nsync"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println(fmt.Sprintf("usage: %s <nitrous-token> <nitrous-folder-path> <container-slug> <remote-path>", os.Args[0]))
		os.Exit(1)
	}

	ns := nsync.NewNsync(os.Args[1], os.Args[2], os.Args[3], os.Args[4])
	ns.Start()
}
