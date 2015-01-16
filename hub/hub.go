package hub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
)

const (
	defaultBufferSize         = 128
	defaultHeartbeat          = time.Second * 60
	defaultHeartbeatGracetime = time.Second * 5
	defaultSessionTimeout     = time.Second * 180
	idLength                  = 16
	defaultLoglevel           = 1
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
	heartbeat          time.Duration
	heartbeatGracetime time.Duration
	sessionTimeout     time.Duration
	sessions           map[string]*Session
	subscriptions      map[string]map[string]*Session
	subscribers        map[string]map[string]bool
	lock               *sync.RWMutex
	loglevel           int
	bufferSize         int
	logger             *log.Logger
	authorizer         Authorizer
}

func NewServer() *Server {
	return &Server{
		heartbeat:          defaultHeartbeat,
		heartbeatGracetime: defaultHeartbeatGracetime,
		bufferSize:         defaultBufferSize,
		sessionTimeout:     defaultSessionTimeout,
		loglevel:           defaultLoglevel,
		sessions:           map[string]*Session{},
		subscriptions:      map[string]map[string]*Session{},
		subscribers:        map[string]map[string]bool{},
		lock:               &sync.RWMutex{},
		logger:             log.New(os.Stdout, "pusher: ", 0),
	}
}

type PusherSessionStats map[string]int
type PusherStats struct {
	Sessions      map[string]PusherSessionStats            `json:"sessions"`
	Subscriptions map[string]map[string]PusherSessionStats `json:"subscriptions"`
	Subscribers   []string                                 `json:"subscribers"`
}

func (self *Server) Stats() PusherStats {
	stats := PusherStats{
		Sessions:      map[string]PusherSessionStats{},
		Subscriptions: map[string]map[string]PusherSessionStats{},
		Subscribers:   []string{},
	}
	for k, session := range self.sessions {
		stats.Sessions[k] = PusherSessionStats{}
		stats.Sessions[k]["output"] = len(session.output)
		stats.Sessions[k]["input"] = len(session.input)
		stats.Sessions[k]["connections"] = int(session.connections)
	}
	for subscription := range self.subscriptions {
		stats.Subscriptions[subscription] = map[string]PusherSessionStats{}
		for k, session := range self.subscriptions[subscription] {
			stats.Subscriptions[subscription][k] = PusherSessionStats{}
			stats.Subscriptions[subscription][k]["output"] = len(session.output)
			stats.Subscriptions[subscription][k]["input"] = len(session.input)
			stats.Subscriptions[subscription][k]["connections"] = int(session.connections)
		}
	}
	for subscriber := range self.subscribers {
		stats.Subscribers = append(stats.Subscribers, subscriber)
	}
	return stats
}

func (self *Server) Loglevel(i int) *Server {
	self.loglevel = i
	return self
}

func (self *Server) Logger(l *log.Logger) *Server {
	self.logger = l
	return self
}

func (self *Server) Fatalf(fmt string, i ...interface{}) {
	self.logger.Printf("FATAL: "+fmt, i...)
}

func (self *Server) Errorf(fmt string, i ...interface{}) {
	if self.loglevel > 0 {
		self.logger.Printf("ERROR: "+fmt, i...)
	}
}

func (self *Server) Infof(fmt string, i ...interface{}) {
	if self.loglevel > 1 {
		self.logger.Printf("INFO: "+fmt, i...)
	}
}

func (self *Server) Debugf(f string, i ...interface{}) {
	if self.loglevel > 2 {
		self.logger.Printf("DEBUG: "+f, i...)
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

	self.Infof("%v\t-\t%v\t%v\t%v\t[subscribe]", time.Now(), uri, sess.RemoteAddr, sess.id)
}

func (self *Server) Emit(message Message) {
	self.lock.RLock()
	defer self.lock.RUnlock()

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
	self.Infof("%v\t-\t%v\t%v\t-\t[unsubscribe]", time.Now(), uri, id)
}

func (self *Server) randomId() string {
	buf := make([]byte, idLength)
	for index, _ := range buf {
		buf[index] = byte(rand.Int31())
	}
	return (base64.URLEncoding.EncodeToString(buf))
}

func (self *Server) Close() {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, sess := range self.sessions {
		close(sess.input)
	}
}

func (self *Server) removeSession(id string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	delete(self.sessions, id)

	for uri, _ := range self.subscribers[id] {
		self.removeSubscription(id, uri, false)
	}
	self.Infof("%v\t-\t[cleanup]\t%v", time.Now(), id)
}

func (self *Server) GetSession(id string) (result *Session) {
	self.lock.Lock()
	defer self.lock.Unlock()

	// if the id is not found (because the server restarted, or the client gave the initial empty id) we just create a new id and insert a new session
	if result = self.sessions[id]; result == nil {
		result = &Session{
			output:         make(chan Message, self.bufferSize),
			input:          make(chan Message),
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
	websocket.Handler(self.GetSession(r.URL.Query().Get("session_id")).handleWS).ServeHTTP(w, r)
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
	Heartbeat          time.Duration
	HeartbeatGracetime time.Duration
	SessionTimeout     time.Duration
	Id                 string
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
	id             string
	RemoteAddr     string
	input          chan Message
	output         chan Message
	server         *Server
	connections    int32
	cleanupTimer   *time.Timer
	authorizations map[string]bool
	lock           *sync.RWMutex
}

func (self *Session) parseMessage(b []byte) (result Message, err error) {
	err = json.Unmarshal(b, &result)
	return
}

type MessagePipe interface {
	Close() error
	ReceiveMessage() (*Message, error)
	SendMessage(*Message) error
}

type wsWrapper struct {
	*websocket.Conn
}

func (self *wsWrapper) ReceiveMessage() (result *Message, err error) {
	result = &Message{}
	err = websocket.JSON.Receive(self.Conn, result)
	return
}

func (self *wsWrapper) SendMessage(m *Message) (err error) {
	err = websocket.JSON.Send(self.Conn, m)
	return
}
func (self *Session) readLoop(closing chan struct{}, ws MessagePipe) {
	defer self.kill(closing)
	var err error
	var message *Message
	for message, err = ws.ReceiveMessage(); err == nil; message, err = ws.ReceiveMessage() {
		self.input <- *message
		self.server.Debugf("%v\t%v\t%v\t%v\t%v\t[received from socket]", time.Now(), message.Type, message.URI, self.RemoteAddr, self.id)
	}
	if err != nil && err != io.EOF {
		self.server.Errorf("%v\t%v\t%v\t[%v]", time.Now(), self.RemoteAddr, self.id, err)
	}
}

func (self *Session) writeLoop(closing chan struct{}, ws MessagePipe) {
	defer self.kill(closing)
	var message Message
	var err error
	for {
		select {
		case message = <-self.output:
			if err = ws.SendMessage(&message); err != nil {
				self.server.Fatalf("Error sending %v on %+v: %v", message, ws, err)
				return
			}
			self.server.Debugf("%v\t%v\t%v\t%v\t%v\t[sent to socket]", time.Now(), message.Type, message.URI, self.RemoteAddr, self.id)
		case <-closing:
			return
		}
	}
}

func (self *Session) heartbeatLoop(closing chan struct{}) {
	for {
		select {
		case <-closing:
			return
		case <-time.After(self.server.heartbeat / 2):
			self.send(Message{Type: TypeHeartbeat})
		}
	}
}

func (self *Session) authorized(uri string, wantWrite bool) bool {
	if self.server.authorizer == nil {
		return true
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	hasWrite, found := self.authorizations[uri]
	if !found {
		return false
	}
	return !wantWrite || hasWrite
}

func (self *Session) send(message Message) {
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
			self.server.Debugf("%v\t%v\t%v\t[unauthorized emit]", time.Now(), message.URI, self.RemoteAddr)
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
			self.server.Debugf("%v\t%v\t%v\t[unauthorized subscribe]", time.Now(), message.URI, self.RemoteAddr)
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
		self.server.Debugf("%v\t%v\t%v\t[authorizing %#v/%v]", time.Now(), message.URI, self.RemoteAddr, message.Token, message.Write)
		if self.server.authorizer != nil {
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
		}
		self.lock.Lock()
		defer self.lock.Unlock()
		self.authorizations[message.URI] = (self.authorizations[message.URI] || message.Write)
		self.server.Infof("%v\t-\t[authorize]\t%v\t%v\t%v", time.Now(), self.RemoteAddr, self.id, message.URI)
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

func (self *Session) kill(closing chan struct{}) {

	self.lock.Lock()
	defer self.lock.Unlock()

	select {
	case _ = <-closing:
	default:
		close(closing)
	}
}
func (self *Session) terminate(closing chan struct{}, ws MessagePipe) {
	self.lock.Lock()
	defer self.lock.Unlock()

	select {
	case _ = <-closing:
	default:
		close(closing)
	}

	self.server.Infof("%v\t-\t[disconnect]\t%v\t%v", time.Now(), self.RemoteAddr, self.id)

	ws.Close()
	self.cleanupTimer.Stop()
	if atomic.AddInt32(&self.connections, -1) == 0 {
		self.cleanupTimer = time.AfterFunc(self.server.sessionTimeout, self.remove)
	}
}

func (self *Session) handleWS(ws *websocket.Conn) {
	self.RemoteAddr = ws.Request().RemoteAddr
	self.Handle(&wsWrapper{ws})
}

/*
 * Handle works as the session main-loop listening on events
 * on a websocket or other source that implements io.ReadWriteCloser.
 */
func (self *Session) Handle(ws MessagePipe) {
	self.server.Infof("%v\t-\t[connect]\t%v\t%v", time.Now(), self.RemoteAddr, self.id)

	closing := make(chan struct{})
	defer self.terminate(closing, ws)
	atomic.AddInt32(&self.connections, 1)

	self.cleanupTimer.Stop()

	go self.readLoop(closing, ws)
	go self.writeLoop(closing, ws)
	go self.heartbeatLoop(closing)

	self.send(Message{
		Type: TypeWelcome,
		Welcome: &Welcome{
			Heartbeat:          self.server.heartbeat / time.Millisecond,
			HeartbeatGracetime: self.server.heartbeatGracetime / time.Millisecond,
			SessionTimeout:     self.server.sessionTimeout / time.Millisecond,
			Id:                 self.id,
		},
	})

	var message Message
	var ok bool
	for {
		select {
		case _ = <-closing:
			return
		case message, ok = <-self.input:
			if !ok {
				return
			}
			self.handleMessage(message)
		case <-time.After(self.server.heartbeat + self.server.heartbeatGracetime):
			return
		}
	}
}
