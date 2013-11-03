package main

import (
	"flag"
	"fmt"
	"github.com/zond/pusher/hub"
	"runtime"
	"time"
)

func testMsg(channel string) hub.Message {
	return hub.Message{
		Type: hub.TypeMessage,
		URI:  channel,
		Data: []string{"Hello world"},
	}
}

func main() {
	c := flag.String("chan", "/foo", "What channel to communicate on")
	t := flag.Int("timer", 5, "time between messures")
	flag.Parse()
	fmt.Printf("Processes: %d\n", runtime.GOMAXPROCS(-1))

	// Create new session using ws
	send, recv := hub.Connect("")

	// Subscribe to chan
	send <- hub.Message{
		Type: hub.TypeSubscribe,
		URI:  *c,
		Id:   "myfooack",
	}
	recv.Next(hub.TypeAck)

	// Main benchmark loop
	sent := 0
	received := 0
	ticker := time.Tick(time.Second * time.Duration(*t))
	for {
		select {
		case send <- testMsg(*c):
			sent++
			//fmt.Printf("message sent\n")
		case msg, ok := <-recv:
			if ok && msg.Type == hub.TypeMessage && msg.URI == *c {
				received++
				//fmt.Printf("Got message: %v\n", msg)
			}
		case <-ticker:
			fmt.Printf("Messages '%s' sent: %d/s Received: %d/s lost: %d/s\n", *c, sent/(*t), received/(*t), (sent/(*t))-(received/(*t)))
			sent = 0
			received = 0
		}

	}
}
