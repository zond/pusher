
(function(root, factory) {
  if (typeof define === 'function' && define.amd) {
    define(factory);
  } else if (typeof exports === 'object') {
    module.exports = factory(require, exports, module);
  } else {
    root.Pusher = factory();
  }
}(this, function(require, exports, module) {

/*!
 * EventEmitter v4.2.7 - git.io/ee
 * Oliver Caldwell
 * MIT license
 * @preserve
 */

(function () {
	'use strict';

	/**
	 * Class for managing events.
	 * Can be extended to provide event functionality in other classes.
	 *
	 * @class EventEmitter Manages event registering and emitting.
	 */
	function EventEmitter() {}

	// Shortcuts to improve speed and size
	var proto = EventEmitter.prototype;
	var exports = this;
	var originalGlobalValue = exports.EventEmitter;

	/**
	 * Finds the index of the listener for the event in it's storage array.
	 *
	 * @param {Function[]} listeners Array of listeners to search through.
	 * @param {Function} listener Method to look for.
	 * @return {Number} Index of the specified listener, -1 if not found
	 * @api private
	 */
	function indexOfListener(listeners, listener) {
		var i = listeners.length;
		while (i--) {
			if (listeners[i].listener === listener) {
				return i;
			}
		}

		return -1;
	}

	/**
	 * Alias a method while keeping the context correct, to allow for overwriting of target method.
	 *
	 * @param {String} name The name of the target method.
	 * @return {Function} The aliased method
	 * @api private
	 */
	function alias(name) {
		return function aliasClosure() {
			return this[name].apply(this, arguments);
		};
	}

	/**
	 * Returns the listener array for the specified event.
	 * Will initialise the event object and listener arrays if required.
	 * Will return an object if you use a regex search. The object contains keys for each matched event. So /ba[rz]/ might return an object containing bar and baz. But only if you have either defined them with defineEvent or added some listeners to them.
	 * Each property in the object response is an array of listener functions.
	 *
	 * @param {String|RegExp} evt Name of the event to return the listeners from.
	 * @return {Function[]|Object} All listener functions for the event.
	 */
	proto.getListeners = function getListeners(evt) {
		var events = this._getEvents();
		var response;
		var key;

		// Return a concatenated array of all matching events if
		// the selector is a regular expression.
		if (evt instanceof RegExp) {
			response = {};
			for (key in events) {
				if (events.hasOwnProperty(key) && evt.test(key)) {
					response[key] = events[key];
				}
			}
		}
		else {
			response = events[evt] || (events[evt] = []);
		}

		return response;
	};

	/**
	 * Takes a list of listener objects and flattens it into a list of listener functions.
	 *
	 * @param {Object[]} listeners Raw listener objects.
	 * @return {Function[]} Just the listener functions.
	 */
	proto.flattenListeners = function flattenListeners(listeners) {
		var flatListeners = [];
		var i;

		for (i = 0; i < listeners.length; i += 1) {
			flatListeners.push(listeners[i].listener);
		}

		return flatListeners;
	};

	/**
	 * Fetches the requested listeners via getListeners but will always return the results inside an object. This is mainly for internal use but others may find it useful.
	 *
	 * @param {String|RegExp} evt Name of the event to return the listeners from.
	 * @return {Object} All listener functions for an event in an object.
	 */
	proto.getListenersAsObject = function getListenersAsObject(evt) {
		var listeners = this.getListeners(evt);
		var response;

		if (listeners instanceof Array) {
			response = {};
			response[evt] = listeners;
		}

		return response || listeners;
	};

	/**
	 * Adds a listener function to the specified event.
	 * The listener will not be added if it is a duplicate.
	 * If the listener returns true then it will be removed after it is called.
	 * If you pass a regular expression as the event name then the listener will be added to all events that match it.
	 *
	 * @param {String|RegExp} evt Name of the event to attach the listener to.
	 * @param {Function} listener Method to be called when the event is emitted. If the function returns true then it will be removed after calling.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.addListener = function addListener(evt, listener) {
		var listeners = this.getListenersAsObject(evt);
		var listenerIsWrapped = typeof listener === 'object';
		var key;

		for (key in listeners) {
			if (listeners.hasOwnProperty(key) && indexOfListener(listeners[key], listener) === -1) {
				listeners[key].push(listenerIsWrapped ? listener : {
					listener: listener,
					once: false
				});
			}
		}

		return this;
	};

	/**
	 * Alias of addListener
	 */
	proto.on = alias('addListener');

	/**
	 * Semi-alias of addListener. It will add a listener that will be
	 * automatically removed after it's first execution.
	 *
	 * @param {String|RegExp} evt Name of the event to attach the listener to.
	 * @param {Function} listener Method to be called when the event is emitted. If the function returns true then it will be removed after calling.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.addOnceListener = function addOnceListener(evt, listener) {
		return this.addListener(evt, {
			listener: listener,
			once: true
		});
	};

	/**
	 * Alias of addOnceListener.
	 */
	proto.once = alias('addOnceListener');

	/**
	 * Defines an event name. This is required if you want to use a regex to add a listener to multiple events at once. If you don't do this then how do you expect it to know what event to add to? Should it just add to every possible match for a regex? No. That is scary and bad.
	 * You need to tell it what event names should be matched by a regex.
	 *
	 * @param {String} evt Name of the event to create.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.defineEvent = function defineEvent(evt) {
		this.getListeners(evt);
		return this;
	};

	/**
	 * Uses defineEvent to define multiple events.
	 *
	 * @param {String[]} evts An array of event names to define.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.defineEvents = function defineEvents(evts) {
		for (var i = 0; i < evts.length; i += 1) {
			this.defineEvent(evts[i]);
		}
		return this;
	};

	/**
	 * Removes a listener function from the specified event.
	 * When passed a regular expression as the event name, it will remove the listener from all events that match it.
	 *
	 * @param {String|RegExp} evt Name of the event to remove the listener from.
	 * @param {Function} listener Method to remove from the event.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.removeListener = function removeListener(evt, listener) {
		var listeners = this.getListenersAsObject(evt);
		var index;
		var key;

		for (key in listeners) {
			if (listeners.hasOwnProperty(key)) {
				index = indexOfListener(listeners[key], listener);

				if (index !== -1) {
					listeners[key].splice(index, 1);
				}
			}
		}

		return this;
	};

	/**
	 * Alias of removeListener
	 */
	proto.off = alias('removeListener');

	/**
	 * Adds listeners in bulk using the manipulateListeners method.
	 * If you pass an object as the second argument you can add to multiple events at once. The object should contain key value pairs of events and listeners or listener arrays. You can also pass it an event name and an array of listeners to be added.
	 * You can also pass it a regular expression to add the array of listeners to all events that match it.
	 * Yeah, this function does quite a bit. That's probably a bad thing.
	 *
	 * @param {String|Object|RegExp} evt An event name if you will pass an array of listeners next. An object if you wish to add to multiple events at once.
	 * @param {Function[]} [listeners] An optional array of listener functions to add.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.addListeners = function addListeners(evt, listeners) {
		// Pass through to manipulateListeners
		return this.manipulateListeners(false, evt, listeners);
	};

	/**
	 * Removes listeners in bulk using the manipulateListeners method.
	 * If you pass an object as the second argument you can remove from multiple events at once. The object should contain key value pairs of events and listeners or listener arrays.
	 * You can also pass it an event name and an array of listeners to be removed.
	 * You can also pass it a regular expression to remove the listeners from all events that match it.
	 *
	 * @param {String|Object|RegExp} evt An event name if you will pass an array of listeners next. An object if you wish to remove from multiple events at once.
	 * @param {Function[]} [listeners] An optional array of listener functions to remove.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.removeListeners = function removeListeners(evt, listeners) {
		// Pass through to manipulateListeners
		return this.manipulateListeners(true, evt, listeners);
	};

	/**
	 * Edits listeners in bulk. The addListeners and removeListeners methods both use this to do their job. You should really use those instead, this is a little lower level.
	 * The first argument will determine if the listeners are removed (true) or added (false).
	 * If you pass an object as the second argument you can add/remove from multiple events at once. The object should contain key value pairs of events and listeners or listener arrays.
	 * You can also pass it an event name and an array of listeners to be added/removed.
	 * You can also pass it a regular expression to manipulate the listeners of all events that match it.
	 *
	 * @param {Boolean} remove True if you want to remove listeners, false if you want to add.
	 * @param {String|Object|RegExp} evt An event name if you will pass an array of listeners next. An object if you wish to add/remove from multiple events at once.
	 * @param {Function[]} [listeners] An optional array of listener functions to add/remove.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.manipulateListeners = function manipulateListeners(remove, evt, listeners) {
		var i;
		var value;
		var single = remove ? this.removeListener : this.addListener;
		var multiple = remove ? this.removeListeners : this.addListeners;

		// If evt is an object then pass each of it's properties to this method
		if (typeof evt === 'object' && !(evt instanceof RegExp)) {
			for (i in evt) {
				if (evt.hasOwnProperty(i) && (value = evt[i])) {
					// Pass the single listener straight through to the singular method
					if (typeof value === 'function') {
						single.call(this, i, value);
					}
					else {
						// Otherwise pass back to the multiple function
						multiple.call(this, i, value);
					}
				}
			}
		}
		else {
			// So evt must be a string
			// And listeners must be an array of listeners
			// Loop over it and pass each one to the multiple method
			i = listeners.length;
			while (i--) {
				single.call(this, evt, listeners[i]);
			}
		}

		return this;
	};

	/**
	 * Removes all listeners from a specified event.
	 * If you do not specify an event then all listeners will be removed.
	 * That means every event will be emptied.
	 * You can also pass a regex to remove all events that match it.
	 *
	 * @param {String|RegExp} [evt] Optional name of the event to remove all listeners for. Will remove from every event if not passed.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.removeEvent = function removeEvent(evt) {
		var type = typeof evt;
		var events = this._getEvents();
		var key;

		// Remove different things depending on the state of evt
		if (type === 'string') {
			// Remove all listeners for the specified event
			delete events[evt];
		}
		else if (evt instanceof RegExp) {
			// Remove all events matching the regex.
			for (key in events) {
				if (events.hasOwnProperty(key) && evt.test(key)) {
					delete events[key];
				}
			}
		}
		else {
			// Remove all listeners in all events
			delete this._events;
		}

		return this;
	};

	/**
	 * Alias of removeEvent.
	 *
	 * Added to mirror the node API.
	 */
	proto.removeAllListeners = alias('removeEvent');

	/**
	 * Emits an event of your choice.
	 * When emitted, every listener attached to that event will be executed.
	 * If you pass the optional argument array then those arguments will be passed to every listener upon execution.
	 * Because it uses `apply`, your array of arguments will be passed as if you wrote them out separately.
	 * So they will not arrive within the array on the other side, they will be separate.
	 * You can also pass a regular expression to emit to all events that match it.
	 *
	 * @param {String|RegExp} evt Name of the event to emit and execute listeners for.
	 * @param {Array} [args] Optional array of arguments to be passed to each listener.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.emitEvent = function emitEvent(evt, args) {
		var listeners = this.getListenersAsObject(evt);
		var listener;
		var i;
		var key;
		var response;

		for (key in listeners) {
			if (listeners.hasOwnProperty(key)) {
				i = listeners[key].length;

				while (i--) {
					// If the listener returns true then it shall be removed from the event
					// The function is executed either with a basic call or an apply if there is an args array
					listener = listeners[key][i];

					if (listener.once === true) {
						this.removeListener(evt, listener.listener);
					}

					response = listener.listener.apply(this, args || []);

					if (response === this._getOnceReturnValue()) {
						this.removeListener(evt, listener.listener);
					}
				}
			}
		}

		return this;
	};

	/**
	 * Alias of emitEvent
	 */
	proto.trigger = alias('emitEvent');

	/**
	 * Subtly different from emitEvent in that it will pass its arguments on to the listeners, as opposed to taking a single array of arguments to pass on.
	 * As with emitEvent, you can pass a regex in place of the event name to emit to all events that match it.
	 *
	 * @param {String|RegExp} evt Name of the event to emit and execute listeners for.
	 * @param {...*} Optional additional arguments to be passed to each listener.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.emit = function emit(evt) {
		var args = Array.prototype.slice.call(arguments, 1);
		return this.emitEvent(evt, args);
	};

	/**
	 * Sets the current value to check against when executing listeners. If a
	 * listeners return value matches the one set here then it will be removed
	 * after execution. This value defaults to true.
	 *
	 * @param {*} value The new value to check for when executing listeners.
	 * @return {Object} Current instance of EventEmitter for chaining.
	 */
	proto.setOnceReturnValue = function setOnceReturnValue(value) {
		this._onceReturnValue = value;
		return this;
	};

	/**
	 * Fetches the current value to check against when executing listeners. If
	 * the listeners return value matches this one then it should be removed
	 * automatically. It will return true by default.
	 *
	 * @return {*|Boolean} The current value to check for or the default, true.
	 * @api private
	 */
	proto._getOnceReturnValue = function _getOnceReturnValue() {
		if (this.hasOwnProperty('_onceReturnValue')) {
			return this._onceReturnValue;
		}
		else {
			return true;
		}
	};

	/**
	 * Fetches the events object and creates one if required.
	 *
	 * @return {Object} The events storage object.
	 * @api private
	 */
	proto._getEvents = function _getEvents() {
		return this._events || (this._events = {});
	};

	/**
	 * Reverts the global {@link EventEmitter} to its previous value and returns a reference to this version.
	 *
	 * @return {Function} Non conflicting EventEmitter class.
	 */
	EventEmitter.noConflict = function noConflict() {
		exports.EventEmitter = originalGlobalValue;
		return EventEmitter;
	};

	// Expose the class either via AMD, CommonJS or the global object
	if (typeof define === 'function' && define.amd) {
		define(function () {
			return EventEmitter;
		});
	}
	else if (typeof module === 'object' && module.exports){
		module.exports = EventEmitter;
	}
	else {
		this.EventEmitter = EventEmitter;
	}
}.call(this));

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

  unsubscribe: function(channel){
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

return Pusher;

}));
