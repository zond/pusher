package hub

import (
	"encoding/json"
	"fmt"
	socknet "github.com/lindroth/socknet/lib"
	"github.com/stretchr/testify/assert"
	"net"
	"net/http"
	"testing"
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

func Connect() (OutgoingMessage, IncommingMessage) {
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

func TestHubRecive(t *testing.T) {
	l, hub := StartServer()
	defer l.Close()

	send, recv := Connect()

	// Expect an
	input_welcome := recv.Next(TypeWelcome)
	assert.Equal(t, input_welcome.Welcome.Heartbeat, 5000, "Expected a hartbeat setting")
	assert.Equal(t, input_welcome.Welcome.SessionTimeout, 30000, "Expected a hartbeat setting")

	send <- Message{
		Type: TypeSubscribe,
		URI:  "/foo",
		Id:   "myfooack",
	}
	ack := recv.Next(TypeAck)
	assert.Equal(t, ack.Id, "myfooack", "Expected ack")

	hub.Emit(Message{
		Type: TypeMessage,
		URI:  "/foo",
		Data: []string{"Hello world"},
	})

	message := recv.Next(TypeMessage)
	assert.Equal(t, message.Data, []interface{}{"Hello world"}, "Expected hello world")
}
