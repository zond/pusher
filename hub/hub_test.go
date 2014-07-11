package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHubRecive(t *testing.T) {
	l, hub := StartServer()
	defer l.Close()

	// Create new session using ws
	send, recv := Connect("", "http://localhost/2233", "ws://localhost:2233/")

	// Expect an
	input_welcome := recv.Next(TypeWelcome)
	assert.Equal(t, input_welcome.Welcome.Heartbeat, 60000, "Expected a heartbeat setting")
	assert.Equal(t, input_welcome.Welcome.SessionTimeout, 180000, "Expected a heartbeat setting")

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

func TestHubReciveInternal(t *testing.T) {
	l, hub := StartServer()
	defer l.Close()

	// Create new session not using ws
	_, send, recv := hub.InternalPipe("")

	// Expect an
	input_welcome := recv.Next(TypeWelcome)
	assert.NotEqual(t, input_welcome.Welcome.Heartbeat, 0, "Expected a heartbeat setting")
	assert.NotEqual(t, input_welcome.Welcome.SessionTimeout, 0, "Expected a heartbeat setting")

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
	assert.Equal(t, message.Data, []string{"Hello world"}, "Expected hello world")
}
