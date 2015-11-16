package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/nitrous-io/nsync/nsync"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println(fmt.Sprintf("usage: %s <nitrous-token> <nitrous-folder-path> <container-slug> <remote-path>", os.Args[0]))
		os.Exit(1)
	}


  stopCh := make(chan struct{})
	ns := nsync.NewNsync(os.Args[1], os.Args[2], os.Args[3], os.Args[4], stopCh)
	go ns.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	fmt.Println("listening signal....")

  select {
  case s := <-c:
	  fmt.Println("Got signal:", s)
    stopCh <- struct{}{}
    <-stopCh
  }
}
