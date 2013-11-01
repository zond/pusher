package hub

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHubRecive(t *testing.T) {
	l, hub := StartServer()
	defer l.Close()

	// Create new session using ws
	send, recv := Connect("")

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

func TestHubReciveInternal(t *testing.T) {
	l, hub := StartServer()
	defer l.Close()

	// Create new session not using ws
	_, send, recv := hub.InternalPipe("")

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
