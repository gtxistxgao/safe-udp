package main

import (
	"context"
	"fmt"
	"github.com/gtxistxgao/safe-udp/server/controller"
	"log"
)

/*

every package received from the port will be send to channel 1

channel 1: received package

10 workers will process the package, put it into struct and push to channel 2

channel 2: processed received package into object

1 worker will get the object and put them into a min heap

1 worker will get the object:
  - if top one is the one we want
    - pop it
    - write file
  - if top one is not the one we want, wait.
    - send notice to client asking for that package

*/

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	defer cancel()

	c := controller.New(ctx, "8888")
	go c.Run()

	select {
	case <-ctx.Done():
		fmt.Println("full process cancelled")
	}
}
