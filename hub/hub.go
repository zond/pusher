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
	defaultBufferSize     = 1024
	defaultHeartbeat      = time.Second * 5
	defaultSessionTimeout = time.Second * 30
	idLength              = 16
	bufLength             = 4096
	defaultLoglevel       = 1
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Authorizer func(uri, token string, write bool) (authorized bool, err error)

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
	loglevel       int
	bufferSize     int
	logger         *log.Logger
	authorizer     Authorizer
}

func NewServer() *Server {
	return &Server{
		heartbeat:      defaultHeartbeat,
		bufferSize:     defaultBufferSize,
		sessionTimeout: defaultSessionTimeout,
		loglevel:       defaultLoglevel,
		sessions:       map[string]*Session{},
		subscriptions:  map[string]map[string]*Session{},
		subscribers:    map[string]map[string]bool{},
		lock:           &sync.RWMutex{},
		logger:         log.New(os.Stdout, "pusher: ", 0),
		authorizer: func(uri, token string, write bool) (bool, error) {
			return true, nil
		},
	}
}

func (self *Server) Logger(l *log.Logger) *Server {
	self.logger = l
	return self
}

func (self *Server) Fatalf(fmt string, i ...interface{}) {
	self.logger.Printf(fmt, i...)
}

func (self *Server) Errorf(fmt string, i ...interface{}) {
	if self.loglevel > 0 {
		self.logger.Printf(fmt, i...)
	}
}

func (self *Server) Infof(fmt string, i ...interface{}) {
	if self.loglevel > 1 {
		self.logger.Printf(fmt, i...)
	}
}

func (self *Server) Debugf(fmt string, i ...interface{}) {
	if self.loglevel > 2 {
		self.logger.Printf(fmt, i...)
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

	self.Infof("%v\t[ws]\t%v\t%v\t%v\t[subscribe]", time.Now(), uri, sess.RemoteAddr, sess.id)
}

func (self *Server) Emit(message Message) {
	self.lock.Lock()
	defer self.lock.Unlock()

	sent := 0
	for _, sess := range self.subscriptions[message.URI] {
		sess.send(message)
		sent++
	}
	self.Debugf("%v\t%v\t[emitted] x %v", time.Now(), message.URI, sent)
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
	self.Infof("%v\t[ws]\t%v\t%v\t-\t[unsubscribe]", time.Now(), uri, id)
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
	self.Infof("%v\t[ws]\t[cleanup]\t%v", time.Now(), id)
}

func (self *Server) getSession(id string) (result *Session) {
	self.lock.Lock()
	defer self.lock.Unlock()

	// if the id is not found (because the server restarted, or the client gave the initial empty id) we just create a new id and insert a new session
	if result = self.sessions[id]; result == nil {
		result = &Session{
			output:         make(chan Message, self.bufferSize),
			id:             self.randomId(),
			server:         self,
			authorizations: map[string]bool{},
			lock:           &sync.RWMutex{},
		}
		result.cleanupTimer = &time.Timer{}
		self.sessions[result.id] = result
	}
	return
}

func (self *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.Infof("%v\t%v\t%v\t%v", time.Now(), r.Method, r.URL, r.RemoteAddr)
	websocket.Handler(self.getSession(r.URL.Query().Get("session_id")).handle).ServeHTTP(w, r)
}

type MessageType string
type ErrorType string

const (
	TypeError       = "Error"
	TypeHeartbeat   = "Heartbeat"
	TypeWelcome     = "Welcome"
	TypeSubscribe   = "Subscribe"
	TypeUnsubscribe = "Unsubscribe"
	TypeMessage     = "Message"
	TypeAuthorize   = "Authorize"
	TypeAck         = "Ack"
)

const (
	TypeJSONError          = "JSONError"
	TypeAuthorizationError = "AuthorizationError"
	TypeSyntaxError        = "SyntaxError"
)

type Welcome struct {
	Heartbeat      time.Duration
	SessionTimeout time.Duration
	Id             string
}

type Error struct {
	Message string
	Type    ErrorType
}

type Message struct {
	Type    MessageType
	Id      string      `json:",omitempty"`
	Welcome *Welcome    `json:",omitempty"`
	Error   *Error      `json:",omitempty"`
	Data    interface{} `json:",omitempty"`
	URI     string      `json:",omitempty"`
	Token   string      `json:",omitempty"`
	Write   bool        `json:",omitempty"`
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
	defer self.terminate()
	buf := make([]byte, bufLength)
	n, err := self.ws.Read(buf)
	for err == nil {
		if message, err := self.parseMessage(buf[:n]); err == nil {
			self.input <- message
			self.server.Debugf("%v\t%v\t%v\t%v\t[received from socket]", time.Now(), message.URI, self.RemoteAddr, self.id)
		} else {
			self.send(Message{
				Type: TypeError,
				Error: &Error{
					Message: err.Error(),
					Type:    TypeJSONError,
				},
				Data: string(buf[:n])})
		}
		n, err = self.ws.Read(buf)
	}
}

func (self *Session) writeLoop() {
	defer self.terminate()
	var message Message
	var err error
	var n int
	var encoded []byte
	for {
		select {
		case message = <-self.output:
			if encoded, err = json.Marshal(message); err == nil {
				if n, err = self.ws.Write(encoded); err != nil {
					self.server.Fatalf("Error sending %s on %+v: %v", encoded, self.ws, err)
					return
				} else if n != len(encoded) {
					self.server.Fatalf("Unable to send all of %s on %+v: only sent %v bytes", encoded, self.ws, n)
					return
				}
			} else {
				self.server.Fatalf("Unable to JSON marshal %+v: %v", message, err)
				return
			}
			self.server.Debugf("%v\t%v\t%v\t%v\t[sent to socket]", time.Now(), message.URI, self.RemoteAddr, self.id)
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

func (self *Session) authorized(uri string, wantWrite bool) bool {
	self.lock.RLock()
	defer self.lock.RUnlock()
	hasWrite, found := self.authorizations[uri]
	if !found {
		return false
	}
	return !wantWrite || hasWrite
}

func (self *Session) send(message Message) {
	self.pushOutput(message)
}

func (self *Session) pushOutput(message Message) {
	select {
	case self.output <- message:
	default:
		self.server.Errorf("Unable to send %+v to %+v, output buffer full", message, self)
	}
}

func (self *Session) handleMessage(message Message) {
	switch message.Type {
	case TypeHeartbeat:
	case TypeMessage:
		if !self.authorized(message.URI, true) {
			self.send(Message{
				Type: TypeError,
				Id:   message.Id,
				Error: &Error{
					Message: fmt.Sprintf("%v not authorized for writing to %v", self.id, message.URI),
					Type:    TypeAuthorizationError,
				},
				Data: message,
			})
			return
		}
		self.server.Emit(message)
	case TypeUnsubscribe:
		self.server.removeSubscription(self.id, message.URI, true)
	case TypeSubscribe:
		if !self.authorized(message.URI, false) {
			self.send(Message{
				Type: TypeError,
				Id:   message.Id,
				Error: &Error{
					Message: fmt.Sprintf("%v not authorized for subscribing to %v", self.id, message.URI),
					Type:    TypeAuthorizationError,
				},
				Data: message,
			})
			return
		}
		self.server.addSubscription(self, message.URI)
	case TypeAuthorize:
		ok, err := self.server.authorizer(message.URI, message.Token, message.Write)
		if err != nil {
			self.send(Message{
				Type: TypeError,
				Id:   message.Id,
				Error: &Error{
					Message: err.Error(),
					Type:    TypeAuthorizationError,
				}, Data: message,
			})
			return
		}
		if !ok {
			self.send(Message{
				Type: TypeError,
				Id:   message.Id,
				Error: &Error{
					Message: fmt.Sprintf("%v does not provide authorization for %v", message.Token, message.URI),
					Type:    TypeAuthorizationError,
				}, Data: message,
			})
			return
		}
		self.lock.Lock()
		defer self.lock.Unlock()
		self.authorizations[message.URI] = (self.authorizations[message.URI] || message.Write)
		self.server.Infof("%v\t[ws]\t[authorize]\t%v\t%v\t%v", time.Now(), self.RemoteAddr, self.id, message.URI)
	default:
		self.send(Message{
			Type: TypeError,
			Id:   message.Id,
			Error: &Error{
				Message: fmt.Sprintf("Unknown message type %#v", message.Type),
				Type:    TypeSyntaxError,
			},
			Data: message,
		})
		return
	}
	if message.Id != "" {
		self.send(Message{
			Type: TypeAck,
			Id:   message.Id,
		})
	}

}

func (self *Session) remove() {
	self.server.removeSession(self.id)
}

func (self *Session) terminate() {
	self.ws.Close()
	select {
	case _ = <-self.closing:
	default:
		close(self.closing)
	}
	self.server.Infof("%v\t[ws]\t[disconnect]\t%v\t%v", time.Now(), self.RemoteAddr, self.id)
	self.cleanupTimer.Stop()
	self.cleanupTimer = time.AfterFunc(self.server.sessionTimeout, self.remove)
}

func (self *Session) handle(ws *websocket.Conn) {
	self.server.Infof("%v\t[ws]\t[connect]\t%v\t%v", time.Now(), ws.Request().RemoteAddr, self.id)

	defer self.terminate()

	self.ws = ws
	self.RemoteAddr = self.ws.Request().RemoteAddr
	self.cleanupTimer.Stop()
	self.input = make(chan Message)
	self.closing = make(chan struct{})

	go self.readLoop()
	go self.writeLoop()
	go self.heartbeatLoop()

	self.send(Message{
		Type: TypeWelcome,
		Welcome: &Welcome{
			Heartbeat:      self.server.heartbeat / time.Millisecond,
			SessionTimeout: self.server.sessionTimeout / time.Millisecond,
			Id:             self.id,
		},
	})
	var message Message
	for {
		select {
		case _ = <-self.closing:
			return
		case message = <-self.input:
			self.handleMessage(message)
		case <-time.After(self.server.heartbeat):
			return
		}
	}
}
