package main

import (
	"fmt"
	"os"

	"github.com/nitrous-io/nsync/nsync"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println(fmt.Sprintf("usage: %s <nitrous-token> <path-to-watch> <container-slug> <remote-path>", os.Args[0]))
		os.Exit(1)
	}

	token, localPath, containerSlug, remotePath := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

	fmt.Println(token, localPath, containerSlug, remotePath)

	localPath = "/Users/jihoon/workspace/nsync-test-141337"
	containerSlug = "nsync-test-141337"
	remotePath = "code"

	ns := nsync.NewNsync(token, localPath, containerSlug, remotePath)
	ns.Start()
}
