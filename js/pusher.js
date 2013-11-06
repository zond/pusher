var Pusher = function(options) {
  var that = this;
  // next message id
  that.nextId = 0;
  // socket url
  that.url = options.url;
  // heartbeat interval
  that.heartbeat = null;
  // session id
  that.id = null;
  // min backoff
  that.minBackoff = 500;
  // max backoff
  that.maxBackoff = 30000;
  // exponential backoff
  that.backoff = that.minBackoff;
  // interval that sends heartbeats
  that.heartbeater = null;
  // socket
  that.socket = null;
  // reconnect timeout
  that.reconnector = null;
  // buffer while closed
  that.buffer = [];
  // subscriptions
  that.subscriptions = {};
  // callbacks
  that.callbacks = {};
  // errbacks
  that.errbacks = {};
  // error handler
  that.onerror = options.onerror || function(err) {
    console.log('pusher error:', err);
  };
  // on connect handler
  that.onconnect = options.onconnect || function(msg) {
    console.log('pusher connected:', msg);
  };
  // authorizer is a function that generate authorization tokens for your rights.
  that.authorizer = options.authorizer || function(uri, write, callback) {
    console.log('authorizer returned empty token for ', uri, write);
    callback('');
  };
  /*
   * set up the socket
   */
  that.connect = function() {
    var url = that.url;
    if (that.id !== null) {
      url += '?session_id=' + that.id;
    }
    that.socket = new WebSocket(url);
    that.socket.onopen = function() {
      that.lastHeartbeatReceived = new Date();
      that.backoff = that.minBackoff;
    };
    that.socket.onerror = function(err) {
      that.onerror(err);
    };
    that.socket.onmessage = function(message) {
      var msg = JSON.parse(message.data);
      that.handleMessage(msg);
    };
    that.socket.onclose = function() {
      that.close();
    };
  };
  /*
   * handle incoming messages
   */
  that.handleMessage = function(msg) {
    if (msg.Type === 'Welcome') {
      that.heartbeat = msg.Welcome.Heartbeat;
      if (that.id !== null && msg.Welcome.Id !== that.id) {
        var oldSubscriptions = that.subscriptions;
        that.subscriptions = {};
        for (var uri in oldSubscriptions) {
          if (typeof oldSubscriptions[uri] !== 'undefined') {
            for (var i = 0; i < oldSubscriptions[uri].length; i++) {
              that.on(uri, oldSubscriptions[uri][i]);
            }
          }
        }
      }
      that.id = msg.Welcome.Id;
      if (that.heartbeater !== null) {
        clearInterval(that.heartbeater);
      }
      that.heartbeater = setInterval(function() {
        if (new Date().getTime() - that.lastHeartbeatReceived.getTime() > that.heartbeat) {
          that.close();
        } else {
          if (that.socket.readyState === 1) {
            that.send({
              Type: 'Heartbeat'
            });
          }
        }
      }, that.heartbeat / 2);
      while (that.buffer.length > 0) {
        that.send(that.buffer.shift());
      }
      that.onconnect(msg);
    } else if (msg.Type === 'Heartbeat') {
      that.lastHeartbeatReceived = new Date();
    } else if (msg.Type === 'Message') {
      var subscriptions = that.subscriptions[msg.URI];
      if (subscriptions !== null) {
        for (var subscription in subscriptions) {
          if (typeof subscriptions[subscription] !== 'undefined') {
            subscriptions[subscription].apply(msg, msg.Data);
          }
        }
      }
    } else if (msg.Type === 'Error') {
      if (msg.Data.Type === 'Subscribe') {
        delete(that.subscriptions[msg.Data.URI]);
      }
      var obj = that.errbacks[msg.Id];
      if (obj !== null && obj.errback) {
        obj.errback.call(obj, msg);
      } else {
        that.onerror(msg);
      }
      delete(that.errbacks[msg.Id]);
      delete(that.callbacks[msg.Id]);
    } else if (msg.Type === 'Ack') {
      var object = that.callbacks[msg.Id];
      if (object !== null) {
        object.callback.call(object, msg);
      }
      delete(that.callbacks[msg.Id]);
      delete(that.errbacks[msg.Id]);
    } else {
      that.onerror({
        Type: 'Error',
        Error: 'Unknown message type ' + msg.Type,
        Data: msg
      });
    }
  };
  that.emit = function(uri) {
    // Copy arguments, becouse slice changes it.
    var args = null;
    var callback = function() {};
    if (typeof (arguments[arguments.length-1]) === 'function') {
      args = Array.prototype.slice.call(arguments, 0, arguments.length - 1);
      callback = arguments[arguments.length - 1];
    } else {
      args = Array.prototype.slice.call(arguments, 0);
    }
    var sendFunc = null;
    var msg = {
        Type: 'Message',
        URI: uri,
        Data: args.slice(1),
        Id: true,
        errback: function(err) {
          if (err.Error.Type === 'AuthorizationError') {
            that.authorizer(uri, true, function(token) {
              that.send({
                Type: 'Authorize',
                URI: uri,
                Write: true,
                Token: token,
                Id: true,
                callback: sendFunc
              });
            });
          } else {
            that.onerror(err);
          }
        }
    };
    msg.callback = function() {
      callback(msg);
    };
    sendFunc = function() {
      that.send(msg);
    };
    sendFunc();
  };
  that.on = function(uri, subscription, callback) {
    if (typeof that.subscriptions[uri] === 'undefined') {
      that.send({
        Type: 'Subscribe',
        URI: uri,
        Id: true,
        callback: function() {
          if (typeof that.subscriptions[uri] === 'undefined') {
            that.subscriptions[uri] = {};
          }
          that.subscriptions[uri][subscription] = subscription;
          if (callback !== null) {
            callback(uri);
          }
        },
        errback: function(err) {
          if (err.Error.Type === 'AuthorizationError') {
            that.authorizer(uri, false, function(token) {
              that.send({
                Type: 'Authorize',
                URI: uri,
                Id: true,
                Token: token,
                callback: function() {
                  that.on(uri, subscription, callback);
                }
              });
            });
          } else {
            that.onerror(err);
          }
        }
      });
    }
  };
  that.off = function(uri, subscription) {
    delete(that.subscriptions[uri][subscription]);
    var left = 0;
    for (var sub in that.subscriptions[uri]) {
      if (that.subscriptions[uri]) {
        left++;
      }
    }
    if (left === 0) {
      that.send({
        Type: 'Unsubscribe',
        URI: uri,
        Id: true
      });
      delete(that.subscriptions[uri]);
    }
  };
  /*
   * close
   */
  that.close = function() {
    clearInterval(that.heartbeater);
    that.heartbeater = null;
    that.socket.close();
    if (that.backoff < that.maxBackoff) {
      that.backoff *= 2;
    }
    if (that.reconnector !== null) {
      clearTimeout(that.reconnector);
    }
    that.reconnector = setTimeout(that.connect, that.backoff);
  };
  /*
   * send a JSON encoded obj
   */
  that.send = function(obj) {
    if (that.socket.readyState === 1) {
      if (obj.Id) {
        obj.Id = that.id + ':' + that.nextId++;
        if (obj.callback !== null) {
          that.callbacks[obj.Id] = obj;
        }
        if (obj.errback !== null) {
          that.errbacks[obj.Id] = obj;
        }
      }
      that.socket.send(JSON.stringify(obj));
    } else {
      that.buffer.push(obj);
    }
  };
  that.connect();
  return that;
};

module.exports = Pusher;
