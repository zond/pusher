package hub

import (
	"code.google.com/p/go.net/websocket"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	defaultOutputBuffer   = 1024
	defaultHeartbeat      = time.Second * 5
	defaultSessionTimeout = time.Second * 30
	idLength              = 16
	bufLength             = 4096
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func prettify(obj interface{}) string {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

type Server struct {
	heartbeat      time.Duration
	sessionTimeout time.Duration
	sessions       map[string]*Session
	lock           *sync.RWMutex
	outputBuffer   int
	logger         *log.Logger
}

func NewServer() *Server {
	return &Server{
		heartbeat:      defaultHeartbeat,
		sessionTimeout: defaultSessionTimeout,
		sessions:       map[string]*Session{},
		lock:           &sync.RWMutex{},
		logger:         log.New(os.Stdout, "pusher: ", 0),
	}
}

func (self *Server) randomId() string {
	buf := make([]byte, idLength)
	for index, _ := range buf {
		buf[index] = byte(rand.Int31())
	}
	return (base64.URLEncoding.EncodeToString(buf))
}

func (self *Server) removeSession(id string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	delete(self.sessions, id)
}

func (self *Server) getSession(id string) (result *Session, err error) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if id == "" {
		result = &Session{
			output: make(chan Message, defaultOutputBuffer),
			id:     self.randomId(),
			server: self,
		}
		result.cleanupTimer = time.AfterFunc(self.sessionTimeout, result.remove)
		self.sessions[id] = result
	} else {
		result = self.sessions[id]
		if result == nil {
			err = fmt.Errorf("No session with id %v", id)
		}
	}
	return
}

func (self *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.logger.Printf("%v\t%v\t%v\t%v", time.Now(), r.Method, r.URL, r.RemoteAddr)
	session, err := self.getSession(r.URL.Query().Get("session_id"))
	if err != nil {
		w.WriteHeader(417)
		fmt.Fprintf(w, "%v", err)
		return
	}
	websocket.Handler(session.Handle).ServeHTTP(w, r)
}

type MessageType string

const (
	TypeError     = "Error"
	TypeHeartbeat = "Heartbeat"
	TypeWelcome   = "Welcome"
)

type Welcome struct {
	Heartbeat      time.Duration
	SessionTimeout time.Duration
	Id             string
}

type Message struct {
	Type MessageType
	Data interface{} `json:",omitempty"`
}

type Session struct {
	ws           *websocket.Conn
	id           string
	input        chan Message
	output       chan Message
	closing      chan struct{}
	server       *Server
	cleanupTimer *time.Timer
}

func (self *Session) parseMessage(b []byte) (result Message, err error) {
	err = json.Unmarshal(b, &result)
	return
}

func (self *Session) readLoop() {
	defer self.ws.Close()
	buf := make([]byte, bufLength)
	n, err := self.ws.Read(buf)
	for err == nil {
		if message, err := self.parseMessage(buf[:n]); err == nil {
			self.input <- message
		} else {
			self.Send(Message{Type: TypeError, Data: err})
		}
		n, err = self.ws.Read(buf)
	}
	close(self.input)
}

func (self *Session) writeLoop() {
	defer self.ws.Close()
	var message Message
	var err error
	var n int
	var encoded []byte
	for {
		select {
		case message = <-self.output:
			if encoded, err = json.Marshal(message); err == nil {
				if n, err = self.ws.Write(encoded); err != nil {
					self.server.logger.Printf("Error sending %s on %+v: %v", encoded, self.ws, err)
					return
				} else if n != len(encoded) {
					self.server.logger.Printf("Unable to send all of %s on %+v: only sent %v bytes", encoded, self.ws, n)
					return
				}
			} else {
				self.server.logger.Printf("Unable to JSON marshal %+v: %v", message, err)
				return
			}
		case <-self.closing:
			return
		}
	}
}

func (self *Session) heartbeatLoop() {
	for {
		select {
		case <-self.closing:
			return
		case <-time.After(self.server.heartbeat / 2):
			self.Send(Message{Type: TypeHeartbeat})
		}
	}
}

func (self *Session) Send(message Message) {
	self.output <- message
}

func (self *Session) handleMessage(message Message) {
	switch message.Type {
	case TypeHeartbeat:
	default:
		self.server.logger.Printf("Got message %+v", message)
	}
}

func (self *Session) remove() {
	self.server.removeSession(self.id)
}

func (self *Session) Handle(ws *websocket.Conn) {
	self.server.logger.Printf("%v\t%v\t%v\t%v\t[ws]", time.Now(), ws.Request().Method, ws.Request().URL, ws.Request().RemoteAddr)

	self.ws = ws
	defer self.ws.Close()

	self.cleanupTimer.Stop()
	defer func() {
		self.cleanupTimer = time.AfterFunc(self.server.sessionTimeout, self.remove)
	}()

	self.input = make(chan Message)

	self.closing = make(chan struct{})
	defer close(self.closing)

	go self.readLoop()
	go self.writeLoop()
	go self.heartbeatLoop()

	self.Send(Message{
		Type: TypeWelcome,
		Data: &Welcome{
			Heartbeat:      self.server.heartbeat / time.Millisecond,
			SessionTimeout: self.server.sessionTimeout / time.Millisecond,
			Id:             self.id,
		},
	})
	var message Message
	var ok bool
	for {
		select {
		case message, ok = <-self.input:
			if !ok {
				return
			}
			self.handleMessage(message)
		case <-time.After(self.server.heartbeat):
			return
		}
	}
}
