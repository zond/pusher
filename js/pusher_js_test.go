package js

import (
	"bytes"
	socknet "github.com/lindroth/socknet/lib"
	"github.com/robertkrimen/otto"
	"github.com/zond/pusher/hub"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func must2(i interface{}, err error) {
	if err != nil {
		panic(err)
	}
}

type ottoSocket struct {
	o      *otto.Otto
	socket *socknet.Socknet
	obj    *otto.Object
	input  chan<- string
	output <-chan string
}

func (self *ottoSocket) listen() {
	for s := range self.output {
		event, err := self.o.Run("new Object()")
		if err != nil {
			panic(err)
		}
		eventObj := event.Object()
		must(eventObj.Set("data", s))
		must2(self.obj.Call("onmessage", event))
	}
}

func (self *ottoSocket) connect(url string) {
	var err error
	self.input, self.output, err = self.socket.Connect(url, url, nil)
	if err != nil {
		val, err2 := otto.ToValue(err)
		if err2 != nil {
			panic(err2)
		}
		_, err2 = self.obj.Call("onerror", val)
		if err2 != nil {
			panic(err2)
		}
		return
	}
	must(self.obj.Set("readyState", 1))
	go self.listen()
	must2(self.obj.Call("onopen"))
}

func newOttoSocket(call otto.FunctionCall) (result otto.Value) {
	var err error
	result, err = call.Otto.Run("new Object()")
	if err != nil {
		panic(err)
	}
	resultObj := result.Object()
	socket := &ottoSocket{
		o:      call.Otto,
		socket: &socknet.Socknet{},
		obj:    resultObj,
	}
	must(resultObj.Set("readyState", 0))
	must(resultObj.Set("onerror", func(call otto.FunctionCall) (result otto.Value) {
		return
	}))
	must(resultObj.Set("onopen", func(call otto.FunctionCall) (result otto.Value) {
		return
	}))
	must(resultObj.Set("onclose", func(call otto.FunctionCall) (result otto.Value) {
		return
	}))
	must(resultObj.Set("onmessage", func(call otto.FunctionCall) (result otto.Value) {
		return
	}))
	must(resultObj.Set("close", func(call otto.FunctionCall) (result otto.Value) {
		resultObj.Set("readyState", 2)
		close(socket.input)
		resultObj.Set("readyState", 3)
		return
	}))
	must(resultObj.Set("send", func(call otto.FunctionCall) (result otto.Value) {
		socket.input <- call.ArgumentList[0].String()
		return
	}))
	go socket.connect(call.ArgumentList[0].String())

	return result
}

func setTimeout(call otto.FunctionCall) (result otto.Value) {
	ms, err := call.ArgumentList[1].ToInteger()
	if err != nil {
		panic(err)
	}
	cb := call.ArgumentList[0]
	result, err = call.Otto.Run("new Object()")
	if err != nil {
		panic(err)
	}
	resultObj := result.Object()
	do := true
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		if do {
			if _, err := cb.Call(result); err != nil {
				panic(err)
			}
		}
	}()
	must(resultObj.Set("clear", func(call otto.FunctionCall) otto.Value {
		do = false
		return otto.Value{}
	}))
	return
}

func clearTimeout(call otto.FunctionCall) (result otto.Value) {
	if _, err := call.ArgumentList[0].Object().Call("clear"); err != nil {
		panic(err)
	}
	return
}

func setInterval(call otto.FunctionCall) (result otto.Value) {
	ms, err := call.ArgumentList[1].ToInteger()
	if err != nil {
		panic(err)
	}
	cb := call.ArgumentList[0]
	result, err = call.Otto.Run("new Object()")
	if err != nil {
		panic(err)
	}
	resultObj := result.Object()
	loop := true
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		for loop {
			if _, err := cb.Call(result); err != nil {
				panic(err)
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}()
	must(resultObj.Set("clear", func(call otto.FunctionCall) otto.Value {
		loop = false
		return otto.Value{}
	}))
	return
}

func clearInterval(call otto.FunctionCall) (result otto.Value) {
	if _, err := call.ArgumentList[0].Object().Call("clear"); err != nil {
		panic(err)
	}
	return
}

func loadFile(o *otto.Otto, s string) {
	f, err := os.Open(s)
	if err != nil {
		panic(err)
	}
	buf := &bytes.Buffer{}
	must2(io.Copy(buf, f))
	err = f.Close()
	if err != nil {
		panic(err)
	}
	must2(o.Run(string(buf.Bytes())))
}

func newOtto() *otto.Otto {
	f, err := os.Open("pusher.js")
	if err != nil {
		panic(err)
	}
	buf := &bytes.Buffer{}
	must2(io.Copy(buf, f))
	err = f.Close()
	if err != nil {
		panic(err)
	}
	o := otto.New()
	o.Set("WebSocket", newOttoSocket)
	o.Set("setInterval", setInterval)
	o.Set("clearInterval", clearInterval)
	o.Set("setTimeout", setTimeout)
	o.Set("clearTimeout", clearTimeout)
	loadFile(o, "pusher.js")
	loadFile(o, "json2.js")
	return o
}

func assertWithin(t *testing.T, f func() bool, d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		if f() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Expected %v to be true within %d", strings.Split(string(debug.Stack()), "\n")[2], d)
		}
		time.Sleep(time.Second / 5)
	}
}

func assertJS(t *testing.T, o *otto.Otto, s string) {
	assertWithin(t, func() bool {
		val, err := o.Run(s)
		if err != nil {
			t.Fatalf("%v", err)
		}
		bo, err := val.ToBoolean()
		if err != nil {
			t.Fatalf("%v", err)
		}
		return bo
	}, time.Second*2)
}

func connect(t *testing.T, o *otto.Otto) {
	must2(o.Run("var connected = false; var pusher = new Pusher({ url: 'ws://localhost:2222/', onconnect: function() { connected = true;	} });"))
	assertJS(t, o, "connected")

}

func init() {
	go func() {
		hub := hub.NewServer()
		hub.Authorizer(func(uri, token string, write bool) (bool, error) {
			return true, nil
		})
		http.ListenAndServe("0.0.0.0:2222", hub)
	}()
}

func TestConnect(t *testing.T) {
	o := newOtto()
	connect(t, o)
}

func TestReconnect(t *testing.T) {
	o := newOtto()
	connect(t, o)
	must2(o.Run("connected = false;"))
	must2(o.Run("pusher.close();"))
	assertJS(t, o, "connected")
}

func TestAutoAuthorizeForEmit(t *testing.T) {
	o := newOtto()
	connect(t, o)
	must2(o.Run("var authorized = false; pusher.authorizer = function(uri, write) { authorized = true; return ''; };"))
	must2(o.Run("pusher.emit('foo', 'brap, brop');"))
	assertJS(t, o, "authorized")
}

func TestAutoAuthorizeForSubscribe(t *testing.T) {
	o := newOtto()
	connect(t, o)
	must2(o.Run("var authorized = false; pusher.authorizer = function(uri, write) { authorized = true; return ''; };"))
	must2(o.Run("pusher.on('foo', function() { });"))
	assertJS(t, o, "authorized")
}

func TestSubscribeEmit(t *testing.T) {
	o := newOtto()
	connect(t, o)
	must2(o.Run("pusher.authorizer = function(uri, write) { return ''; };"))
	must2(o.Run("var received = false; pusher.on('foo', function() { received = true; });"))
	must2(o.Run("pusher.emit('foo', 'brap, brop');"))
	assertJS(t, o, "received")
}
