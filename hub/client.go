package hub

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	socknet "github.com/lindroth/socknet/lib"
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

type IncomingMessage <-chan Message
type OutgoingMessage chan<- Message

func (in IncomingMessage) Next(msg_type MessageType) Message {
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
	send    chan Message
	receive chan Message
}

func (self *pipe) ReceiveMessage() (result *Message, err error) {
	m := <-self.send
	result = &m
	return
}

func (self *pipe) SendMessage(m *Message) (err error) {
	self.receive <- *m
	return
}

func (hub *Server) InternalPipe(session_id string) (*Session, OutgoingMessage, IncomingMessage) {
	session := hub.GetSession(session_id)
	p := &pipe{
		send:    make(chan Message, 100),
		receive: make(chan Message, 100),
	}
	go session.Handle(p)

	return session, p.send, p.receive
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
	self.receive <- *m
	return len(p), nil
}
func (self *pipe) Close() error {
	close(self.receive)
	return nil
}

func Connect(session_id, origin, location string) (OutgoingMessage, IncomingMessage) {
	input := make(chan Message)
	output := make(chan Message)
	ws := socknet.Socknet{}
	ws_input, ws_output, err := ws.Connect(origin, location, nil)
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
		defer close(ws_input)
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
