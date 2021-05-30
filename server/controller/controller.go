package controller

import (
	"context"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/util"
	"github.com/gtxistxgao/safe-udp/server/user"
	"log"
	"net"
)

type Controller struct {
	ctx      context.Context
	listener *net.TCPListener
	userMap  map[string]*user.User
}

func New(ctx context.Context, port string) *Controller {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", "localhost:"+port)
	checkError(err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)

	c := &Controller{
		ctx:      ctx,
		listener: listener,
		userMap:  make(map[string]*user.User),
	}

	return c
}

func (c *Controller) Run() {
	userChan := make(chan *user.User, consts.MaxUserLimit)
	ready := make(chan *bool, consts.MaxUserLimit)

	go c.userGreeter(ready, userChan)
	go c.userManager(ready, userChan)

	// Let's start!
	for i := 0; i < consts.MaxUserLimit; i++ {
		ready <- util.BoolPtr(true)
	}

	select {
	case <-c.ctx.Done():
		fmt.Println("Controller cancelled")
	}
}

func (c *Controller) userGreeter(ready chan *bool, userChan chan *user.User) {
	for {
		signal := <-ready
		if signal == nil {
			break
		}

		conn, err := c.listener.Accept()
		if err != nil {
			log.Printf("error: %s", err)
			continue
		}

		newUser := user.New(conn)
		log.Println("New user joined")
		c.userMap[conn.RemoteAddr().String()] = newUser
		userChan <- newUser
	}

	select {
	case <-c.ctx.Done():
		for len(ready) > 0 {
			<-ready
		}

		ready <- nil
		fmt.Println("userGreeter cancelled")
	}
}

func (c *Controller) userManager(ready chan *bool, userChan chan *user.User) {
	for {
		newUser := <-userChan
		if newUser == nil {
			break
		}

		log.Println("User started")
		newUser.Start()
		ready <- util.BoolPtr(true)
	}

	select {
	case <-c.ctx.Done():
		for len(ready) > 0 {
			<-ready
		}

		ready <- nil
		fmt.Println("userManager cancelled")
	}
}

func checkError(err error) {
	if err != nil {
		log.Fatal("Fatal error: ", err)
	}
}
