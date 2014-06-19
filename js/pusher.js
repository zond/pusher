var parseUrl = function(url){
  var parser = document.createElement('a');
  parser.href = url;

  return {
    protocol: parser.protocol, // => "http:"
    hostname: parser.hostname, // => "example.com"
    port: parser.port,         // => "3000"
    pathname: parser.pathname, // => "/pathname/"
    serach: parser.search,     // => "?search=test"
    hash: parser.hash,         // => "#hash"
    host: parser.host          // => "example.com:3000"
  };
};

var Pusher = Pusher || {};

Pusher.Transport = {};

Pusher.Transport.Socket = function(options){
  options = options || {};

  this.loglevel = options.loglevel || 'warn';
  this.seqNo = 0;
  this.minBackoff = options.minBackoff || 500;
  this.maxBackoff = options.maxBackoff || 10000;
  this.backoff = null;
  this.lastMessageReceivedAt = null;

  this.callbacks = {};
  this.buffer = [];
  this.subscriptions = {};
  this.emitter = new EventEmitter();
  this.socket = options.socket || null; // This is used for DI during testing.
};
Pusher.Transport.Socket.prototype = {
  open: function(host){
    if(host) {
      this.host = parseUrl(host);
    }

    if(!this.socket){
      this.socket = this.openSocket();
    }
  },

  reconnect: function(){
    if(this.socket) {
      this.socket.close();
    }
    this.open();

    this.backoff *= 2;
    this.backoff = this.backoff > this.maxBackoff ? this.maxBackoff : this.backoff;
  },

  openSocket: function(){
    var socket = new WebSocket(this._buildURL());

    socket.onopen = function() {
      this.isClosed = false;
      this.lastHeartbeatReceivedAt = new Date();
      this.backoff = this.minBackoff;
      this.emitter.emit('connect');
    }.bind(this);

    socket.onerror = function(message){
      console.warn('Pusher error: ', message.data);
    }.bind(this);

    socket.onmessage = function(message){
      this.read(message.data);
    }.bind(this);

    socket.onclose = function(){
      this.socket = null;
      if(!this.isClosed){ // We have lost the connection, *not* an active close
        this.stopHeartbeat();
        this.startReconnections();
      }
      this.emitter.emit('close');
    }.bind(this);

    return socket;
  },

  close: function(){
    console.trace();

    this.isClosed = true;
    this.stopHeartbeat();
    this.stopReconnections();
    this.socket.close();
  },

  write: function(msg){
    if(this.socket && this.socket.readyState === 1){
      msg.Id = this.id + ':' + this.seqNo++;

      if(msg.callback){
        this.callbacks[msg.Id] = msg.callback;
        delete msg.callback;
      }


      this.socket.send(JSON.stringify(msg));
    } else {
      this.buffer.push(msg);
    }
  },

  read: function(data){
    var packet;

    try {
      packet = JSON.parse(data);
    } catch(e){
      this.error(e);
      return;
    }

    var MESSAGE_TYPES = ['Welcome', 'Heartbeat', 'Ack', 'Message'];

    if(MESSAGE_TYPES.indexOf(packet.Type) > -1) {
      this.lastMessageReceivedAt = new Date();

      var method = packet.Type.toLowerCase();

      if(this[method]) this[method].call(this, packet);

      this.emitter.emit('read', packet);
    } else {
      this.error(packet);
    }
  },

  welcome: function(packet){
    this.id = packet.Welcome.Id;
    this.heartbeatInterval = packet.Welcome.Heartbeat;

    this.startHeartbeat();
    this.sendBuffer();
  },

  heartbeat: function(packet){
    if(this.loglevel === 'info') console.info('Heartbeat received: ', new Date().toString());
  },

  ack: function(packet){
    var cb = this.callbacks[packet.Id];

    delete this.callbacks[packet.Id];
    if(cb) cb(packet);
  },

  message: function(packet){
    this.emitter.emit('message', packet);
  },

  error: function(packet){
    if(packet.Error.Type !== 'AuthorizationError') {
      console.warn('Error from Pusher.Transport.Socket', packet.Error.Type, packet);
      this.emitter.emit('error', packet);
    } else {
      this.emitter.emit('authorization error', packet);
    }
  },


  destroy: function(){
    this.close();
    this.emitter.removeAllListeners();
    this.socket = null;
  },

  on: function(event, cb){
    this.emitter.on.apply(this.emitter, arguments);
  },

  off: function(event, cb){
    this.emitter.off.apply(this.emitter, arguments);
  },


  sendBuffer: function(){
    var msg;
    while(msg = this.buffer.shift()){
      this.write(msg);
    }
  },

  startHeartbeat: function(){
    this.heartbeater = setInterval(function(){
      if(this.loglevel === 'info') console.info('Sending heartbeat: ', new Date().toString());
      this.write({
        Type: 'Heartbeat'
      });
    }.bind(this), this.heartbeatInterval / 2);
  },

  stopHeartbeat: function(){
    clearInterval(this.heartbeater);
  },


  startReconnections: function(){
    this.reconnector = setTimeout(this.reconnect.bind(this), this.backoff);
  },

  stopReconnections: function(){
    this.clearInterval(this.reconnector);
  },

  _buildURL: function(){
    var protocol = parseUrl(window.location.toString()).protocol === 'https:' ? 'wss:' : 'ws:';
    var url = protocol + '//' + this.host.host;

    if(this.host.pathname) url += this.host.pathname;

    // TODO: Make session_id configurable?
    if(this.host.search) {
      url += this.host.search;
      if(this.id) {
        url += '&session_id=' + this.id;
      }
    } else if(this.id) {
      url += '?session_id=' + this.id;
    }

    return url;
  }
};

Pusher.Client = function(options){
  options = options || {};

  if(!options.authorizer){
    throw new Error('You must specify an authorizer for Pusher!');
  }

  this.authorizer = options.authorizer;
  this.loglevel = options.loglevel || 'warn';

  this.maxRetries = options.maxRetries || 3;
  this.socket = options.socket || new Pusher.Transport.Socket({ loglevel: this.loglevel }); // Used for DI during testing.

  this.message = this.message.bind(this);
  this.error = this.error.bind(this);
  this.authorize = this.authorize.bind(this);

  this.socket.on('message', this.message);
  this.socket.on('error', this.error);
  this.socket.on('authorization error', this.authorize);

  this.socket.on('connect', function(){
    this.retries = 0;
    this.emitter.emit('connect');
  }.bind(this));

  this.emitter = new EventEmitter();
};

Pusher.Client.prototype = {
  connect: function(uri){
    this.socket.open(uri);
  },

  subscribe: function(channel, cb){
    this.emitter.on(channel, cb);
    this.socket.write({
      Type: 'Subscribe',
      URI: channel
    });
  },

  unsubscribe: function(channel, cb){
    this.emitter.off(channel, cb);
    this.socket.write({
      Type: 'Unsubscribe',
      URI: channel
    });
  },

  emit: function(){ // channel, ...data
    var args = [].slice.call(arguments);
    var channel = args.shift();
    var data = args;

    this.socket.write({
      Type: 'Message',
      URI: channel,
      Data: data
    });
  },

  message: function(packet){
    this.emitter.emit(packet.URI, packet.Data);
  },

  authorize: function(packet){
    if(this.retries >= this.maxRetries) return;

    var isWrite = packet.Data.Type === 'Message';
    this.authorizer(packet.Data.URI, isWrite, function(token){
      this.socket.write({
        Type: 'Authorize',
        Token: token,
        URI: packet.Data.URI,
        Write: isWrite,
        callback: function(){
          this.retries = 0;
          this.socket.write(packet.Data); // Resends the failed packet that we need authorization for.
        }.bind(this)
      });
    }.bind(this));
    this.retries++;
  },

  error: function(packet){
    console.warn('Unknown pusher error:', packet);
  },

  on: function(){
    this.emitter.on.apply(this.emitter, arguments);
  },

  off: function(){
    this.emitter.off.apply(this.emitter, arguments);
  },

  destroy: function(){
    this.socket.destroy();
    this.emitter.removeAllListeners();
  }
};
