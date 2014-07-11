package client

import (
	"time"
	"github.com/zond/pusher/hub"
)

type Client struct {
	idIncr   int
	id       string
	outgoing hub.OutgoingMessage
	incoming hub.IncomingMessage
}

func (self *Client) getNextId() string {
	self.idIncr++
	return self.id + string(self.idIncr)
}

func (self *Client) Connect(origin, location string) {
	self.outgoing, self.incoming = hub.Connect("", origin, location)
	welcome := self.incoming.Next(hub.TypeWelcome)
	self.id = welcome.Welcome.Id
	go func() {
		for {
			self.outgoing <- hub.Message{Type: hub.TypeHeartbeat}
			time.Sleep(time.Millisecond * welcome.Welcome.Heartbeat)
		}
	}()
}
func (self *Client) Authorize(uri, token string) (err error) {
	self.outgoing <- hub.Message{Type: hub.TypeAuthorize, URI: uri, Token: token, Write: true, Id: self.getNextId()}
	self.incoming.Next(hub.TypeAck)
	return
}

func (self *Client) Subscribe(uri string) (err error) {
	self.outgoing <- hub.Message{Type: hub.TypeSubscribe, URI: uri, Id: self.getNextId()}
	self.incoming.Next(hub.TypeAck)
	return
}

func (self *Client) Unsubscribe(uri string) (err error) {
	self.outgoing <- hub.Message{Type: hub.TypeUnsubscribe, URI: uri, Id: self.getNextId()}
	self.incoming.Next(hub.TypeAck)
	return
}

func (self *Client) Next(msgType hub.MessageType) hub.Message {
	return self.incoming.Next(msgType)
}

