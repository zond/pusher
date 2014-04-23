Pusher = function(options) {
  options = options || {};
  // next message id
  this.nextId = 0;
  // socket url
  this.url = options.url;
  // heartbeat interval
  this.heartbeat = null;
  // session id
  this.id = options.id || null;
  // min backoff
  this.minBackoff = 500;
  // max backoff
  this.maxBackoff = 30000;
  // exponential backoff
  this.backoff = this.minBackoff;
  // interval that sends heartbeats
  this._heartbeater = null;
  // socket
  this.socket = null;
  // reconnect timeout
  this.reconnector = null;
  // buffer while closed
  this.buffer = [];
  // subscriptions
  this.subscriptions = {};
  // sentMessages
  this.sentMessages = {};

  if (options.onerror) this.onerror = options.onerror;
  if (options.onconnect) this.onconnect = options.onconnect;
  if (options.authorizer) this.authorizer = options.authorizer;
  if (options.socket) this.socket = options.socket;
};

Pusher.prototype = {

  connect: function() {
    var url = this.url;
    if (this.id !== null) {
      url = this.url + '?session_id=' + this.id;
    }

    if (!this.socket) {
      this.socket = new WebSocket(url);
    }

    this.socket.onopen = function() {
      this.lastHeartbeatReceived = new Date();
      this.backoff = this.minBackoff;
    }.bind(this);

    this.socket.onerror = function(err) {
      this._handleMessage(err);
    }.bind(this);

    this.socket.onmessage = function(message) {
      var msg = JSON.parse(message.data);
      this._handleMessage(msg);
    }.bind(this);

    this.socket.onclose = function() {
      this.close();
    }.bind(this);
  },

  emit: function(uri) {
    // Copy arguments, becouse slice changes it.
    var args = null;
    var callback = function() {};
    if (typeof (arguments[arguments.length-1]) === 'function') {
      args = Array.prototype.slice.call(arguments, 0, arguments.length - 1);
      callback = arguments[arguments.length - 1];
    } else {
      args = Array.prototype.slice.call(arguments, 0);
    }
    var msg = {
      Type: 'Message',
      URI: uri,
      Data: args.slice(1),
      Id: true,
      callback: callback
    };
    this._send(msg);
  },

  subscribe: function(uri, subscription, callback) {
    if (typeof this.subscriptions[uri] === 'undefined') {
      this._send({
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
        }.bind(this)
      });
    }
  },

  unsubscribe: function(uri, subscription) {
    delete(this.subscriptions[uri][subscription]);
    var left = 0;
    for (var sub in this.subscriptions[uri]) {
      if (this.subscriptions[uri]) {
        left++;
      }
    }
    if (left === 0) {
      this._send({
        Type: 'Unsubscribe',
        URI: uri,
        Id: true
      });
      delete(this.subscriptions[uri]);
    }
  },

  /*
  * close
  */
  close: function() {
    if (this._heartbeater !== null) {
      clearInterval(this._heartbeater);
    }
    this._heartbeater = null;
    this.socket.close();
    if (this.backoff < this.maxBackoff) {
      this.backoff *= 2;
    }
    if (this.reconnector !== null) {
      clearTimeout(this.reconnector);
    }

    this.reconnector = setTimeout(this.connect.bind(this), this.backoff);
  },

  // error handler
  onerror: function(err) {
    console.log('pusher error:', err);
  },

  // on connect handler
  onconnect: function(msg) {
    console.log('pusher connected:', msg);
  },

  // authorizer is a function that generate authorization tokens for your rights.
  authorizer: function(uri, write, callback) {
    console.log('authorizer returned empty token for ', uri, write);
    callback('');
  },

  _heartbeater: function() {
    if (new Date().getTime() - this.lastHeartbeatReceived.getTime() > this.heartbeat) {
      this.close();
    } else {
      if (this.socket.readyState === 1) {
        this._send({
          Type: 'Heartbeat'
        });
      }
    }
  },

  /*
  * handle incoming messages
  */
  _handleMessage: function(msg) {
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
                  this.subscribe(uri, cb);
                }
              }
            }
          }
        }
        this.id = msg.Welcome.Id;
        if (this._heartbeater !== null) {
          clearInterval(this._heartbeater);
        }
        setInterval(this._heartbeater, this.heartbeat / 2);
        while (this.buffer.length > 0) {
          this._send(this.buffer.shift());
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
              if (msg.Data instanceof Array) {
                subscriptions[subscription].apply(msg, msg.Data);
              } else {
                subscriptions[subscription].apply(msg, [msg.Data]);
              }
            }
          }
        } else {
          this._handleError(msg);
        }
        break;
      }

      case 'Error': {
        if (msg.Data.Type === 'Subscribe') {
          delete(this.subscriptions[msg.Data.URI]);
        }

        this._handleError(msg);

        delete(this.sentMessages[msg.Id]);
        break;
      }

      case 'Ack': {
        var object = this.sentMessages[msg.Id];
        if (typeof object !== 'undefined') {
          object.callback.call(object, msg);
        }
        delete(this.sentMessages[msg.Id]);
        break;
      }

      default:
        this._handleError(msg, 'Unknown message type ' + msg.Type);
        break;
    }
  },

  _handleError: function(msg, errString) {
    if (!msg.Error) {
      this.onerror(msg);
      return false;
    }
    switch (msg.Error.Type) {
      case 'AuthorizationError':
        this.authorizer(msg.URI, msg.Write || false, function(token) {
          var authMsg = {
            Type: 'Authorize',
            Token: token,
            URI: msg.URI,
            Id: true,
            Write: msg.Write || false,
            callback: function() { this._send(msg); }.bind(this)
          };
          this._send(authMsg);
        }.bind(this));
        break;
      default:
        this.onerror({
          Type: 'Error',
          Error: errString || msg.Error.Message,
          Data: msg
        });
        break;
    }
  },

  /*
  * _send a JSON encoded obj
  */
  _send: function(obj) {
    if (this.socket.readyState === 1) {
      if (obj.Id) {
        obj.Id = this.id + ':' + this.nextId++;
        this.sentMessages[obj.Id] = obj;
      }
      this.socket.send(JSON.stringify(obj));
    } else {
      this.buffer.push(obj);
    }
  }
};
