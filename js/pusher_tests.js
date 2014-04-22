var mockSocket, pusher = {};
var WELCOMEMSG = {data: JSON.stringify({"Type":"Welcome","Welcome":{"Heartbeat":5000,"SessionTimeout":30000,"Id":"42"}})};
var AUTHERRORMSG = {data: JSON.stringify({"Type":"Error","Id":"42","Error":{"Message":"MT9282wofb0iwpcjtZBgtw== not authorized for subscribing to /soundzone/U291bmRab25lLCwxazc3bThrNXV5by9Mb2NhdGlvbiwsMWpmaG9kdGFzamsvQWNjb3VudCwsMWpudm8yM3g0aHMv/emitEvent/SoundZone/soundzonechange","Type":"AuthorizationError"},"Data":{"Type":"Subscribe","Id":"227a0000","URI":"/soundzone/U291bmRab25lLCwxazc3bThrNXV5by9Mb2NhdGlvbiwsMWpmaG9kdGFzamsvQWNjb3VudCwsMWpudm8yM3g0aHMv/emitEvent/SoundZone/soundzonechange"}})};
var ACKMSG = {data: JSON.stringify({"Type":"Ack","Id":"42:0","Data":["Message","testdata"]})};
var MSG = {data: JSON.stringify({"Type":"Message","Id":"41:0","URI": "/foo", "Data":["Message","testdata"]})};

beforeEach(function() {
  mockSocket = {
    readyState: 1,
    init: function(){
      this.onopen();
      this.readyState = 1;
    },
    reply: function(msg) {
      this.onmessage(msg);
    },
    send: function(msg) {
    }
  };

  pusher = new Pusher({
    id: 42,
    socket: mockSocket,
    authorizer: function(uri, write, cb) {
      cb('');
    },

    onconnect: function(msg) {
    },

    onerror: function(err) {
    }
  });
});

describe('Pusher message handling', function(){
  it('should be connected after pushers connect method runs', function(done) {
    var connected = false;

    pusher.onconnect = function(msg) {
      connected = true;
      expect(connected).to.equal(true);
      done();
    };

    expect(connected).to.equal(false);
    pusher.connect();
    mockSocket.init();
    mockSocket.reply(WELCOMEMSG);
  });

  it('should call the authorizer when not authorized for emit', function(done) {
    var authorized = false;
    pusher.authorizer = function(uri, write, cb) {
      authorized = true;
      expect(authorized).to.equal(true);
      cb('');
      done();
    };

    expect(authorized).to.equal(false);
    pusher.connect();
    pusher.emit('/foo', 'Message', 'testdata', function(msg) {});
    mockSocket.init();
    mockSocket.reply(AUTHERRORMSG);
  });

  it('should call the authorizer when not authorized for subscribe', function(done) {
    var authorized = false;
    pusher.authorizer = function(uri, write, cb) {
      authorized = true;
      expect(authorized).to.equal(true);
      cb('');
      done();
    };

    expect(authorized).to.equal(false);
    pusher.connect();
    pusher.subscribe('/foo', function() {});
    mockSocket.init();
    mockSocket.reply(AUTHERRORMSG);
  });

  it('should get an ACK when subscribing', function(done) {
    pusher.connect();
    pusher.subscribe('/foo', function(type, text) {}, function() { done(); });
    mockSocket.init();
    mockSocket.reply(ACKMSG);
  });

  it('should resubscribe current subscriptions on reconnection', function(done) {
    pusher.connect();
    mockSocket.init();
    pusher.subscribe('/foo', function(type, text) {
      if (pusher.socket.readyState === 1) done();
      pusher.socket.readyState = 1;
    }, function() {});
    mockSocket.reply(ACKMSG);
    pusher.socket.readyState = 3;
    mockSocket.reply(MSG);
    mockSocket.reply(MSG);
  });
});
