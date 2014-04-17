var Pusher = function(options) {
  options = options || {};
  // next message id
  this.nextId = 0;
  // socket url
  this.url = options.url;
  // heartbeat interval
  this.heartbeat = null;
  // session id
  this.id = null;
  // min backoff
  this.minBackoff = 500;
  // max backoff
  this.maxBackoff = 30000;
  // exponential backoff
  this.backoff = this.minBackoff;
  // interval that sends heartbeats
  this.heartbeater = null;
  // socket
  this.socket = null;
  // reconnect timeout
  this.reconnector = null;
  // buffer while closed
  this.buffer = [];
  // subscriptions
  this.subscriptions = {};
  // callbacks
  this.callbacks = {};
  // errbacks
  this.errbacks = {};

  if (options.onerror) this.onerror = options.onerror;
  if (options.onconnect) this.onconnect = options.onconnect;
  if (options.authorizer) this.authorizer = options.authorizer;
};

Pusher.prototype.connect = function() {
  var self = this;
  var url = this.url;
  if (this.id !== null) {
    url = this.url + '?session_id=' + this.id;
  }
  this.socket = new WebSocket(url);

  this.socket.onopen = function() {
    this.lastHeartbeatReceived = new Date();
    this.backoff = this.minBackoff;
  }.bind(this);

  this.socket.onerror = function(err) {
    this.onerror(err);
  }.bind(this);

  this.socket.onmessage = function(message) {
    var msg = JSON.parse(message.data);
    this.handleMessage(msg);
  }.bind(this);

  this.socket.onclose = function() {
    this.close();
  }.bind(this);

};

Pusher.prototype.heartbeater = function() {
  if (new Date().getTime() - this.lastHeartbeatReceived.getTime() > this.heartbeat) {
    this.close();
  } else {
    if (this.socket.readyState === 1) {
      this.send({
        Type: 'Heartbeat'
      });
    }
  }
};

/*
* handle incoming messages
*/
Pusher.prototype.handleMessage = function(msg) {
  switch (msg.Type) {
    case 'Welcome': {
      this.heartbeat = msg.Welcome.Heartbeat;
      if (this.id !== null && msg.Welcome.Id !== this.id) {
        var oldSubscriptions = this.subscriptions;
        this.subscriptions = {};
        for (var uri in oldSubscriptions) {
          if (typeof oldSubscriptions[uri] !== 'undefined') {
            for (var sub in oldSubscriptions[uri]) {
              if (typeof oldSubscriptions[uri][sub] !== 'undefined') {
                var cb = oldSubscriptions[uri][sub];
                this.on(uri, cb);
              }
            }
          }
        }
      }
      this.id = msg.Welcome.Id;
      if (this.heartbeater !== null) {
        clearInterval(this.heartbeater);
      }
      setInterval(this.heartbeater, this.heartbeat / 2);
      while (this.buffer.length > 0) {
        this.send(this.buffer.shift());
      }
      this.onconnect(msg);
      break;
    }

    case 'Heartbeat': {
      this.lastHeartbeatReceived = new Date();
      break;
    }

    case 'Message': {
      var subscriptions = this.subscriptions[msg.URI];
      if (typeof subscriptions !== 'undefined') {
        for (var subscription in subscriptions) {
          if (typeof subscriptions[subscription] !== 'undefined') {
            if (msg.Data instanceof Array)
              subscriptions[subscription].apply(msg, msg.Data);
            else
              subscriptions[subscription].apply(msg, [msg.Data]);
          }
        }
      } else {
        this.onerror(msg);
      }
      break;
    }

    case 'Error': {
      if (msg.Data.Type === 'Subscribe') {
        delete(this.subscriptions[msg.Data.URI]);
      }
      var obj = this.errbacks[msg.Id];
      if (typeof obj !== 'undefined' && obj.errback) {
        obj.errback.call(obj, msg);
      } else {
        this.onerror(msg);
      }
      delete(this.errbacks[msg.Id]);
      delete(this.callbacks[msg.Id]);
      break;
    }

    case 'Ack': {
      var object = this.callbacks[msg.Id];
      if (typeof object !== 'undefined') {
        object.callback.call(object, msg);
      }
      delete(this.callbacks[msg.Id]);
      delete(this.errbacks[msg.Id]);
      break;
    }

    case 'AuthorizationError': {
      console.log('AUTH ERR');
    }

    default:
      this.onerror({
        Type: 'Error',
        Error: 'Unknown message type ' + msg.Type,
        Data: msg
      });
      break;
  }
};

Pusher.prototype.emit = function(uri) {
  // Copy arguments, becouse slice changes it.
  var args = null;
  var callback = function() {};
  if (typeof (arguments[arguments.length-1]) === 'function') {
    args = Array.prototype.slice.call(arguments, 0, arguments.length - 1);
    callback = arguments[arguments.length - 1];
  } else {
    args = Array.prototype.slice.call(arguments, 0);
  }
  var msg = Pusher.createMessage(uri, {data: args.slice(1), type: 'Message'}, callback);
  this.send(msg);
};

/*
* send a JSON encoded obj
*/
Pusher.prototype.send = function(obj) {
  if (this.socket.readyState === 1) {
    if (obj.Id) {
      obj.Id = this.id + ':' + this.nextId++;
      if (typeof obj.callback !== 'undefined') {
        this.callbacks[obj.Id] = obj;
      }
      if (typeof obj.errback !== null) {
        this.errbacks[obj.Id] = obj;
      }
    }
    this.socket.send(JSON.stringify(obj));
  } else {
    this.buffer.push(obj);
  }
};

Pusher.createMessage = function(uri, data, cb) {
  var msg = {
    Type: data.type,
    URI: uri,
    Data: data.data,
    Id: true,
    callback: function() { cb(this); },
    errback: function(err) {
      if (err.Error.Type === 'AuthorizationError') {
        this.authorizer(uri, true, function(token) {
          var authMessage = this.createAuthMessage(uri, token, true, authorizedCallback);
          this.send(authMessage);
        }).bind(this);
      } else {
        this.onerror(err);
      }
    }.bind(this)
  };
  var authorizedCallback = function() { this.send(msg); }.bind(this);
  return msg;
};

Pusher.createAuthMessage = function(uri, token, write, callback) {
  return {
    Type: 'Authorize',
    Token: token,
    URI: uri,
    Id: true,
    Write: write,
    callback: callback
  };
};

Pusher.prototype.on = function(uri, subscription, callback) {
  if (typeof this.subscriptions[uri] === 'undefined') {

    this.send({
      Type: 'Subscribe',
      URI: uri,
      Id: true,
      callback: function() {
        if (typeof this.subscriptions[uri] === 'undefined') {
          this.subscriptions[uri] = {};
        }
        this.subscriptions[uri][subscription] = subscription;
        if (typeof callback !== 'undefined') {
          callback(uri);
        }
      }.bind(this),
      errback: function(err) {
        if (err.Error.Type === 'AuthorizationError') {
          this.authorizer(uri, false, function(token) {
            var authMessage = this.createAuthMessage(uri, token, false, function() {
              this.on(uri, subscription, callback)
            }.bind(this));
            this.send(authMessage);
          }).bind(this);
        } else {
          this.onerror(err);
        }
      }.bind(this)
    });
  }
};

Pusher.prototype.off = function(uri, subscription) {
  delete(this.subscriptions[uri][subscription]);
  var left = 0;
  for (var sub in this.subscriptions[uri]) {
    if (this.subscriptions[uri]) {
      left++;
    }
  }
  if (left === 0) {
    this.send({
      Type: 'Unsubscribe',
      URI: uri,
      Id: true
    });
    delete(this.subscriptions[uri]);
  }
};

/*
* close
*/
Pusher.prototype.close = function() {
  if (this.heartbeater !== null) {
    clearInterval(this.heartbeater);
  }
  this.heartbeater = null;
  this.socket.close();
  if (this.backoff < this.maxBackoff) {
    this.backoff *= 2;
  }
  if (this.reconnector !== null) {
    clearTimeout(this.reconnector);
  }

  this.reconnector = setTimeout(this.connect.bind(this), this.backoff);
};

// error handler
Pusher.prototype.onerror = function(err) {
  console.log('pusher error:', err);
};
// on connect handler
Pusher.prototype.onconnect = function(msg) {
  console.log('pusher connected:', msg);
};
// authorizer is a function that generate authorization tokens for your rights.
Pusher.prototype.authorizer = function(uri, write, callback) {
  console.log('authorizer returned empty token for ', uri, write);
  callback('');
};

if (typeof module !== 'undefined') module.exports = Pusher;
