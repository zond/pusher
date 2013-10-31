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

type Authorizer func(uri, token string) (authorized bool, err error)

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
	subscriptions  map[string]map[string]*Session
	subscribers    map[string]map[string]bool
	lock           *sync.RWMutex
	outputBuffer   int
	logger         *log.Logger
	authorizer     Authorizer
}

func NewServer() *Server {
	return &Server{
		heartbeat:      defaultHeartbeat,
		sessionTimeout: defaultSessionTimeout,
		sessions:       map[string]*Session{},
		subscriptions:  map[string]map[string]*Session{},
		subscribers:    map[string]map[string]bool{},
		lock:           &sync.RWMutex{},
		logger:         log.New(os.Stdout, "pusher: ", 0),
		authorizer: func(uri, token string) (bool, error) {
			return true, nil
		},
	}
}

func (self *Server) Authorizer(f Authorizer) *Server {
	self.authorizer = f
	return self
}

func (self *Server) addSubscription(sess *Session, uri string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if _, found := self.subscriptions[uri]; !found {
		self.subscriptions[uri] = map[string]*Session{}
	}
	self.subscriptions[uri][sess.id] = sess

	if _, found := self.subscribers[sess.id]; !found {
		self.subscribers[sess.id] = map[string]bool{}
	}
	self.subscribers[sess.id][uri] = true

	self.logger.Printf("%v\t[ws]\t%v\t%v\t%v\t[subscribe]", time.Now(), uri, sess.RemoteAddr, sess.id)
}

func (self *Server) Emit(message Message) {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, sess := range self.subscriptions[message.URI] {
		sess.send(message)
	}
}

func (self *Server) removeSubscription(id, uri string, withLocking bool) {
	if withLocking {
		self.lock.Lock()
		defer self.lock.Unlock()
	}

	delete(self.subscriptions[uri], id)
	if len(self.subscriptions[uri]) == 0 {
		delete(self.subscriptions, uri)
	}

	delete(self.subscribers[id], uri)
	if len(self.subscribers[id]) == 0 {
		delete(self.subscribers, id)
	}
	self.logger.Printf("%v\t[ws]\t%v\t%v\t-\t[unsubscribe]", time.Now(), uri, id)
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

	for uri, _ := range self.subscribers[id] {
		self.removeSubscription(id, uri, false)
	}
}

func (self *Server) getSession(id string) (result *Session) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if id == "" {
		id = self.randomId()
	}
	if result = self.sessions[id]; result == nil {
		result = &Session{
			output:         make(chan Message, defaultOutputBuffer),
			id:             self.randomId(),
			server:         self,
			authorizations: map[string]bool{},
			lock:           &sync.RWMutex{},
		}
		result.cleanupTimer = &time.Timer{}
		self.sessions[id] = result
	}
	return
}

func (self *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.logger.Printf("%v\t%v\t%v\t%v", time.Now(), r.Method, r.URL, r.RemoteAddr)
	websocket.Handler(self.getSession(r.URL.Query().Get("session_id")).handle).ServeHTTP(w, r)
}

type MessageType string

const (
	TypeError       = "Error"
	TypeHeartbeat   = "Heartbeat"
	TypeWelcome     = "Welcome"
	TypeSubscribe   = "Subscribe"
	TypeUnsubscribe = "Unsubscribe"
	TypeMessage     = "Message"
	TypeAuthorize   = "Authorize"
)

type Welcome struct {
	Heartbeat      time.Duration
	SessionTimeout time.Duration
	id             string
}

type Message struct {
	Type    MessageType
	Welcome *Welcome    `json:",omitempty"`
	Error   string      `json:",omitempty"`
	Data    interface{} `json:",omitempty"`
	URI     string      `json:",omitempty"`
	Token   string      `json:",omitempty"`
}

type Session struct {
	ws             *websocket.Conn
	id             string
	RemoteAddr     string
	input          chan Message
	output         chan Message
	closing        chan struct{}
	server         *Server
	cleanupTimer   *time.Timer
	authorizations map[string]bool
	lock           *sync.RWMutex
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
			self.send(Message{Type: TypeError, Error: err.Error(), Data: string(buf[:n])})
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
			self.send(Message{Type: TypeHeartbeat})
		}
	}
}

func (self *Session) authorized(auth string) bool {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.authorizations[auth]
}

func (self *Session) send(message Message) {
	self.output <- message
}

func (self *Session) handleMessage(message Message) {
	switch message.Type {
	case TypeHeartbeat:
	case TypeMessage:
		if !self.authorized(message.URI) {
			self.send(Message{Type: TypeError, Error: fmt.Sprintf("%v not authorized for %v", self.id, message.URI), Data: message})
			return
		}
		self.server.Emit(message)
	case TypeUnsubscribe:
		self.server.removeSubscription(self.id, message.URI, true)
	case TypeSubscribe:
		if !self.authorized(message.URI) {
			self.send(Message{Type: TypeError, Error: fmt.Sprintf("%v not authorized for %v", self.id, message.URI), Data: message})
			return
		}
		self.server.addSubscription(self, message.URI)
	case TypeAuthorize:
		ok, err := self.server.authorizer(message.URI, message.Token)
		if err != nil {
			self.send(Message{Type: TypeError, Error: err.Error(), Data: message})
			return
		}
		if ok {
			self.lock.Lock()
			defer self.lock.Unlock()
			self.authorizations[message.URI] = true
		}
	default:
		self.send(Message{Type: TypeError, Error: fmt.Sprintf("Unknown message type %#v", message.Type), Data: message})
	}
}

func (self *Session) remove() {
	self.server.removeSession(self.id)
}

func (self *Session) handle(ws *websocket.Conn) {
	self.server.logger.Printf("%v\t[ws]\t[connect]\t%v\t%v", time.Now(), ws.Request().RemoteAddr, self.id)

	self.ws = ws
	defer self.ws.Close()
	self.RemoteAddr = self.ws.Request().RemoteAddr

	self.cleanupTimer.Stop()
	defer func() {
		self.server.logger.Printf("%v\t[ws]\t[disconnect]\t%v\t%v", time.Now(), ws.Request().RemoteAddr, self.id)
		self.cleanupTimer = time.AfterFunc(self.server.sessionTimeout, self.remove)
	}()

	self.input = make(chan Message)

	self.closing = make(chan struct{})
	defer close(self.closing)

	go self.readLoop()
	go self.writeLoop()
	go self.heartbeatLoop()

	self.send(Message{
		Type: TypeWelcome,
		Welcome: &Welcome{
			Heartbeat:      self.server.heartbeat / time.Millisecond,
			SessionTimeout: self.server.sessionTimeout / time.Millisecond,
			id:             self.id,
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
