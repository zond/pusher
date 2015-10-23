package main

import (
	"flag"
	"fmt"
	"runtime"
	"time"

	"github.com/soundtrackyourbrand/pusher/hub"
)

func testMsg(channel string) hub.Message {
	return hub.Message{
		Type: hub.TypeMessage,
		URI:  channel,
		Data: []string{"Hello world"},
		Id:   "myack",
	}
}

func main() {
	to := flag.String("to", "", "What channel to send to")
	from := flag.String("from", "", "What channel to receive from")
	t := flag.Int("timer", 5, "time between messures")
	flag.Parse()
	fmt.Printf("Processes: %d\n", runtime.GOMAXPROCS(-1))

	// Create new session using ws
	send, recv := hub.Connect("")

	// Subscribe to chan
	if *from != "" {
		send <- hub.Message{
			Type: hub.TypeSubscribe,
			URI:  *from,
			Id:   "myfooack",
		}
		recv.Next(hub.TypeAck)
	}

	// Main benchmark loop
	sent := 0
	received := 0
	ack := 0
	ticker := time.Tick(time.Second * time.Duration(*t))
	if *to != "" {
		go func() {
			for {
				send <- testMsg(*to)
				sent++
				// Only have 100 msg in output pipe, before requireing acks
				for sent > ack+100 {
					time.Sleep(time.Nanosecond)
				}
			}
		}()
	}

	for {
		select {
		case msg, ok := <-recv:
			if !ok {
				break
			}

			switch msg.Type {
			case hub.TypeMessage:
				if msg.URI == *from {
					received++
				}
			case hub.TypeAck:
				if msg.Id == "myack" {
					ack++
				}
			}
		case <-ticker:
			fmt.Printf("Messages sent: %d/s Received: %d/s ack: %d lost: %d/s\n", sent/(*t), received/(*t), ack/(*t), (sent/(*t))-(ack/(*t)))
			sent = 0
			received = 0
			ack = 0
		}

	}
}
