function Pusher(options) {
  var that = this;
	that.url = options.url;
	that.socket = new WebSocket(url);
	that.state = 'opening';
	that.heartbeat = null;
	that.id = null;
	socket.onopen = function() {
	  that.state = 'open'
	  console.log('opened', that.url);
	};
	socket.onerror = function() {
		console.log('error', arguments);
	};
	socket.onmessage = function(message) {
	  console.log('message', message);
		var msg = JSON.parse(message.data);
		if (msg.Type == "Welcome") {
		  that.heartbeat = msg.Data.Heartbeat;
			that.id = msg.Data.Id;
			setInterval(function() {
			  console.log('heart beating');
			  that.send({
				  Type: "Heartbeat",
				});
			}, that.heartbeat / 2);
		}
	};
	socket.onclose = function() {
		console.log('closed', that.url);
	  that.state = 'closed';
	};
	that.send = function(obj) {
	  that.socket.send(JSON.stringify(obj));
	};
  return that;
}
