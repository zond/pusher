package hub

import (
	"encoding/json"
	"fmt"
	socknet "github.com/lindroth/socknet/lib"
	"net"
	"net/http"
	"time"
)

func StartServer() (net.Listener, *Server) {
	hub := NewServer()
	l, err := net.Listen("tcp", ":2233")
	if err != nil {
		panic(err)
	}
	go http.Serve(l, hub)
	return l, hub
}

type IncommingMessage <-chan Message
type OutgoingMessage chan<- Message

func (in IncommingMessage) Next(msg_type MessageType) Message {
	for {
		select {
		case m, ok := <-in:
			if !ok {
				panic(fmt.Errorf("Input closed"))
			}
			if m.Type == msg_type {
				return m
			}
		case <-time.After(time.Second * 10):
			panic(fmt.Errorf("timeout"))
		}
	}
	return Message{}
}

type pipe struct {
	send   chan Message
	recive chan Message
}

func (hub *Server) InternalPipe(session_id string) (*Session, OutgoingMessage, IncommingMessage) {
	session := hub.GetSession(session_id)
	p := &pipe{
		send:   make(chan Message),
		recive: make(chan Message),
	}
	go session.Handle(p)

	return session, p.send, p.recive
}

func (self *pipe) Read(p []byte) (n int, err error) {
	var buf []byte
	buf, err = json.Marshal(<-self.send)
	if err != nil {
		panic(err)
	}
	return copy(p, buf), nil
}
func (self *pipe) Write(p []byte) (n int, err error) {
	m := &Message{}
	err = json.Unmarshal(p, &m)
	if err != nil {
		panic(err)
	}
	self.recive <- *m
	return len(p), nil
}
func (self *pipe) Close() error {
	close(self.recive)
	return nil
}

func Connect(session_id string) (OutgoingMessage, IncommingMessage) {
	input := make(chan Message)
	output := make(chan Message)
	ws := socknet.Socknet{}
	ws_input, ws_output, err := ws.Connect("http://localhost/2233", "ws://localhost:2233/", nil)
	if err != nil {
		panic(err)
	}

	/* Unmarshal incomming messages, and forward */
	go func() {
		for o := range ws_output {
			m := &Message{}
			err := json.Unmarshal([]byte(o), &m)
			if err != nil {
				panic(err)
			}
			output <- *m
		}
	}()

	/* Marshal outgount messages, and forward */
	go func() {
		for m := range input {
			encoded, err := json.Marshal(m)
			if err != nil {
				panic(err)
			}
			ws_input <- string(encoded)
		}
	}()
	return input, output
}
