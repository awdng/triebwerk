package game

import (
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- a server without sockets and without a clock ----------------------------

// testConn is the server's end of a connection whose client is the test
type testConn struct {
	t    *testing.T
	name string
	in   chan testMessage
	out  chan []byte
	// set while the reader works on a message, closed when it asks for the next
	working chan struct{}

	closeOnce sync.Once
	closed    chan struct{}
	// messages read past while looking for another type
	skipped [][]byte
	// types of the messages written to each lane
	lanes     sync.Mutex
	reliable  map[byte]int
	datagrams map[byte]int
}

type testMessage struct {
	data []byte
	done chan struct{}
}

func (c *testConn) Identifier() string               { return c.name }
func (c *testConn) Ping(time.Duration)               {}
func (c *testConn) PrepareWrite(time.Duration)       {}
func (c *testConn) PrepareRead(int64, time.Duration) {}
func (c *testConn) Close(_ time.Duration, graceful bool) {
	if !graceful {
		c.closeOnce.Do(func() { close(c.closed) })
	}
}

func (c *testConn) Read() ([]byte, error) {
	// the reader comes back for more: the last message is in the game's hands
	if c.working != nil {
		close(c.working)
		c.working = nil
	}
	select {
	case message := <-c.in:
		c.working = message.done
		return message.data, nil
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *testConn) Write(data []byte) error {
	return c.write(data, true)
}

func (c *testConn) WriteUnreliable(data []byte) error {
	return c.write(data, false)
}

func (c *testConn) write(data []byte, reliable bool) error {
	select {
	case <-c.closed:
		return io.ErrClosedPipe
	default:
	}
	c.lanes.Lock()
	if reliable {
		c.reliable[data[0]]++
	} else {
		c.datagrams[data[0]]++
	}
	c.lanes.Unlock()
	c.out <- append([]byte{}, data...)
	return nil
}

// send returns when the server has taken the message in, it is part of the next tick
func (c *testConn) send(data []byte) {
	c.t.Helper()
	message := testMessage{data: data, done: make(chan struct{})}
	select {
	case c.in <- message:
	case <-time.After(2 * time.Second):
		c.t.Fatalf("%s: the server does not read", c.name)
	}
	select {
	case <-message.done:
	case <-c.closed:
	case <-time.After(2 * time.Second):
		c.t.Fatalf("%s: the server did not finish reading", c.name)
	}
}

// next returns the next message whatever it is
func (c *testConn) next() []byte {
	c.t.Helper()
	select {
	case data := <-c.out:
		return data
	case <-time.After(2 * time.Second):
		c.t.Fatalf("%s: no message", c.name)
		return nil
	}
}

// read returns the next message of a type, messages of other types are skipped
func (c *testConn) read(messageType byte) []byte {
	c.t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case data := <-c.out:
			if data[0] == messageType {
				return data
			}
			c.skipped = append(c.skipped, data)
		case <-timeout:
			c.t.Fatalf("%s: no message of type %d", c.name, messageType)
			return nil
		}
	}
}

func (c *testConn) snapshot() protocol.SnapshotMessage {
	c.t.Helper()
	snapshot, ok := protocol.DecodeSnapshot(c.read(protocol.Snapshot))
	require.True(c.t, ok, "a snapshot that can not be read")
	return snapshot
}

// nothing returns true if the server sent nothing for a moment
func (c *testConn) nothing() bool {
	select {
	case <-c.out:
		return false
	case <-time.After(50 * time.Millisecond):
		return true
	}
}

func (c *testConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	case <-time.After(time.Second):
		return false
	}
}

type testTransport struct {
	unregister func(conn model.Connection)
}

func (t *testTransport) Init()                                         {}
func (t *testTransport) GetAddress() string                            { return "test" }
func (t *testTransport) Run() error                                    { select {} }
func (t *testTransport) RegisterNewConnHandler(func(model.Connection)) {}
func (t *testTransport) UnregisterConnHandler(func(model.Connection))  {}
func (t *testTransport) Unregister(conn model.Connection)              { t.unregister(conn) }

type testMaster struct{}

func (testMaster) Init(string)                       {}
func (testMaster) GetServerState()                   {}
func (testMaster) SendHeartbeat(model.StateSnapshot) {}
func (testMaster) EndGame(model.StateSnapshot)       {}
func (testMaster) AuthorizePlayer(token string) (string, string, error) {
	if token == "" {
		return "", "", assert.AnError
	}
	return "id-" + token, token, nil
}

type testServer struct {
	t          *testing.T
	controller *Controller
	now        time.Time
}

func newTestServer(t *testing.T, maxPlayers int) *testServer {
	transport := &testTransport{}
	network := NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := NewController("test", maxPlayers, time.Minute, network, testMaster{})
	transport.unregister = controller.UnregisterPlayer
	go network.run()
	return &testServer{t: t, controller: controller, now: time.Unix(1000, 0)}
}

// tick runs one tick of the game loop, 33 ms later than the last
func (s *testServer) tick() {
	s.now = s.now.Add(model.TickMillis * time.Millisecond)
	s.controller.tick(s.now)
	// Ticks follow each other without the 33 ms of the real server. The
	// goroutines that write to the clients get the time they need here, or
	// the server would take a full buffer for a client that does not read.
	for _, p := range s.controller.state.GetPlayers() {
		for deadline := time.Now().Add(time.Second); len(p.Client.NetworkOut) > 20 && time.Now().Before(deadline); {
			time.Sleep(20 * time.Microsecond)
		}
	}
}

func (s *testServer) ticks(count int) {
	for i := 0; i < count; i++ {
		s.tick()
	}
}

func (s *testServer) connect(name string) *testConn {
	conn := &testConn{t: s.t, name: name, in: make(chan testMessage), out: make(chan []byte, 10000), closed: make(chan struct{}),
		reliable: map[byte]int{}, datagrams: map[byte]int{}}
	s.controller.RegisterPlayer(conn)
	s.tick()
	return conn
}

// join connects a client, says hello and returns once its first snapshot is
// read: from here on every tick has one snapshot for it
func (s *testServer) join(name string) (*testConn, int) {
	s.t.Helper()
	conn := s.connect(name)
	conn.send(protocol.EncodeHello(name))
	s.tick()
	welcome := conn.read(protocol.Welcome)
	conn.snapshot()
	return conn, int(welcome[2])
}

func (s *testServer) player(id int) *model.Player {
	for _, p := range s.controller.state.GetPlayers() {
		if p.ID == id {
			return p
		}
	}
	s.t.Fatalf("no player %d", id)
	return nil
}

func keys(sequence uint32, bits ...byte) []byte {
	return protocol.EncodeInput(sequence, bits)
}

// ---- joining -------------------------------------------------------------------

func TestConnectionBecomesAPlayerWhenItSaysHello(t *testing.T) {
	s := newTestServer(t, 12)
	conn := s.connect("anna")
	s.ticks(3)
	assert.Equal(t, 0, s.controller.state.GetPlayerCount(), "a connection that said nothing is not a tank")
	assert.True(t, conn.nothing())

	conn.send(protocol.EncodeHello("anna"))
	tick := s.controller.state.TickNumber()
	s.tick()
	// player 1, on a server that ticks every 33 ms and plays rounds of a minute
	assert.Equal(t, protocol.NewBinaryProtocol().Welcome(1, tick, time.Minute), conn.next(), "the welcome comes before everything else")
	assert.Equal(t, 1, s.controller.state.GetPlayerCount())

	// the first player starts the round
	start := conn.read(protocol.RoundStart)
	snapshot := conn.snapshot()
	assert.Equal(t, s.controller.state.TickNumber(), snapshot.Tick)
	assert.Equal(t, protocol.RoundStart, int(start[0]))
	if assert.Len(t, snapshot.Players, 1) {
		assert.Equal(t, 1, snapshot.Players[0].ID)
		assert.Equal(t, 100, snapshot.Players[0].Health)
	}
}

func TestTokenOfTheHelloBecomesTheName(t *testing.T) {
	s := newTestServer(t, 12)
	conn, id := s.join("anna")
	// the master server is asked in the background
	require.Eventually(t, func() bool {
		s.tick()
		return s.player(id).Nickname == "anna"
	}, 2*time.Second, time.Millisecond)
	s.tick()
	for {
		board, ok := protocol.DecodeScoreboard(conn.read(protocol.Scoreboard))
		require.True(t, ok)
		if len(board.Players) == 1 && board.Players[0].Name == "anna" {
			return
		}
	}
}

func TestPlayerThatJoinsARunningRoundIsToldWhenItStarted(t *testing.T) {
	s := newTestServer(t, 12)
	s.join("first")
	started := s.controller.state.RoundStartTick()
	s.ticks(40)

	conn := s.connect("second")
	conn.send(protocol.EncodeHello("second"))
	s.tick()
	welcome := conn.read(protocol.Welcome)
	assert.Equal(t, byte(2), welcome[2])
	assert.Equal(t, protocol.NewBinaryProtocol().RoundStart(started), conn.read(protocol.RoundStart))
	assert.Len(t, conn.snapshot().Players, 2)
}

func TestClientOfAnotherVersionIsTurnedAway(t *testing.T) {
	s := newTestServer(t, 12)
	conn := s.connect("old")
	conn.send([]byte{protocol.Hello, protocol.Version + 1, 'o', 'l', 'd'})
	s.tick()
	assert.Equal(t, []byte{protocol.Reject, protocol.Version}, conn.read(protocol.Reject))
	assert.True(t, conn.isClosed())
	s.ticks(2)
	assert.Equal(t, 0, s.controller.state.GetPlayerCount())
}

func TestConnectionThatNeverSaysHelloIsClosed(t *testing.T) {
	s := newTestServer(t, 12)
	silent := s.connect("silent")
	// version 1 clients start with this
	noise := s.connect("noise")
	noise.send([]byte{0, 0, 'o', 'l', 'd'})
	polite, _ := s.join("polite")

	s.ticks(helloTimeoutTicks + 5)
	assert.True(t, silent.isClosed())
	assert.True(t, noise.isClosed())
	select {
	case <-polite.closed:
		t.Fatal("a player was closed")
	default:
	}
	assert.Equal(t, 1, s.controller.state.GetPlayerCount())
}

func TestFullServerTurnsConnectionsAway(t *testing.T) {
	s := newTestServer(t, 2)
	s.join("one")
	// a connection that has not said hello yet holds a place too
	s.connect("two")
	third := s.connect("three")
	assert.True(t, third.isClosed())

	fourth := s.connect("four")
	assert.True(t, fourth.isClosed())
}

func TestPlaceOfAPlayerThatLeftIsFree(t *testing.T) {
	s := newTestServer(t, 1)
	conn, _ := s.join("one")
	conn.Close(0, false)
	require.Eventually(t, func() bool {
		s.tick()
		return s.controller.state.GetPlayerCount() == 0
	}, 2*time.Second, time.Millisecond)

	// and of a connection that left before it said hello
	waiting := s.connect("waiting")
	waiting.Close(0, false)
	require.Eventually(t, func() bool {
		s.tick()
		return len(s.controller.waiting) == 0
	}, 2*time.Second, time.Millisecond)
	_, id := s.join("next")
	assert.NotZero(t, id)
}

// ---- inputs and snapshots ----------------------------------------------------------

func TestSnapshotSaysWhichInputItContains(t *testing.T) {
	s := newTestServer(t, 12)
	conn, id := s.join("anna")
	tank := s.player(id).Collider
	// on open ground, spawn points can face a wall
	tank.SetPose(-128, 28, 0, 0)
	x, y := tank.Pivot.X, tank.Pivot.Y

	conn.send(keys(1, protocol.BitForward))
	s.tick()
	snapshot := conn.snapshot()
	assert.Equal(t, uint32(1), snapshot.Ack)
	assert.Equal(t, uint8(0), snapshot.Waiting)
	moved := snapshot.Players[0]
	assert.NotEqual(t, [2]float32{x, y}, [2]float32{moved.X, moved.Y}, "the tank drives")

	// No input for this tick: the keys stay pressed and the tick counts as
	// input 2. The client is told that it was a guess.
	s.tick()
	snapshot = conn.snapshot()
	assert.Equal(t, uint32(2), snapshot.Ack)
	assert.Equal(t, uint8(protocol.WaitingStarved), snapshot.Waiting)
	assert.NotEqual(t, [2]float32{moved.X, moved.Y}, [2]float32{snapshot.Players[0].X, snapshot.Players[0].Y}, "and drives on")

	// inputs 2, 3 and 4 at once: 2 comes too late for its tick, the others get one each
	conn.send(keys(4, 0, 0, 0))
	s.tick()
	snapshot = conn.snapshot()
	assert.Equal(t, uint32(3), snapshot.Ack)
	assert.Equal(t, uint8(1), snapshot.Waiting)
	s.tick()
	assert.Equal(t, uint32(4), conn.snapshot().Ack)
}

// Every message carries the inputs before its own: a lost message loses
// nothing. Its tick is bridged, and the input it carried becomes the reserve.
func TestLostInputMessageLosesNothing(t *testing.T) {
	s := newTestServer(t, 12)
	conn, _ := s.join("anna")
	conn.send(keys(1, protocol.BitLeft))
	s.tick()
	assert.Equal(t, uint32(1), conn.snapshot().Ack)
	// the message of input 2 never arrives
	s.tick()
	assert.Equal(t, uint8(protocol.WaitingStarved), conn.snapshot().Waiting)
	// input 3 brings it along
	conn.send(keys(3, protocol.BitLeft, protocol.BitLeft, protocol.BitLeft))
	s.tick()
	snapshot := conn.snapshot()
	assert.Equal(t, uint32(2), snapshot.Ack)
	assert.Equal(t, uint8(1), snapshot.Waiting)
	s.tick()
	assert.Equal(t, uint32(3), conn.snapshot().Ack)
}

func TestEveryClientGetsItsOwnAcknowledgement(t *testing.T) {
	s := newTestServer(t, 12)
	anna, _ := s.join("anna")
	ben, _ := s.join("ben")
	// of the two ticks ben joined in
	anna.snapshot()
	anna.snapshot()

	anna.send(keys(7, protocol.BitForward))
	s.tick()
	forAnna, forBen := anna.snapshot(), ben.snapshot()
	assert.Equal(t, uint32(7), forAnna.Ack)
	assert.Equal(t, uint32(0), forBen.Ack)
	assert.Equal(t, uint8(protocol.WaitingStarved), forBen.Waiting)
	assert.Equal(t, forAnna.Tick, forBen.Tick)
	assert.Equal(t, forAnna.Players, forBen.Players, "both see the same world")
	assert.Len(t, forAnna.Players, 2)
	assert.Less(t, forAnna.Players[0].ID, forAnna.Players[1].ID, "tanks come in the order of their ids")
}

// A client can send as much and as fast as it likes: the server keeps the
// newest few inputs, applies one per tick and stays responsive for the rest.
func TestFloodOfInputsMovesATankNoFasterAndBlocksNobody(t *testing.T) {
	s := newTestServer(t, 12)
	flood, floodID := s.join("flood")
	calm, calmID := s.join("calm")
	flood.snapshot()
	flood.snapshot()
	// on open ground, away from each other (spawn points can face a wall)
	s.player(floodID).Collider.SetPose(-128, 28, 0, 0)
	s.player(calmID).Collider.SetPose(-128, 90, 0, 0)
	before := *s.player(floodID).Collider.Pivot

	// a thousand inputs and some garbage in one tick
	for sequence := uint32(1); sequence <= 1000; sequence++ {
		flood.send(keys(sequence, protocol.BitForward, protocol.BitForward, protocol.BitForward))
		if sequence%100 == 0 {
			flood.send([]byte{protocol.Input, 0xff, 0xff, 0xff, 0xff, 3, 1, 1, 1})
			flood.send([]byte{250, 1, 2, 3})
		}
	}
	calm.send(keys(1, protocol.BitForward))
	s.tick()

	snapshot := flood.snapshot()
	assert.LessOrEqual(t, snapshot.Waiting, uint8(model.InputQueueCapacity))
	moved := s.player(floodID).Collider.Pivot
	// one step of 0.495 units, the flood bought nothing
	assert.InDelta(t, 0.495, math.Hypot(float64(moved.X-before.X), float64(moved.Y-before.Y)), 1e-3)
	assert.Equal(t, uint32(1), calm.snapshot().Ack, "the other player is served as always")

	// the flood is not kept for later: a second afterwards the queue is empty
	s.ticks(40)
	assert.Equal(t, 0, s.player(floodID).Inputs.Waiting())
}

func TestMessagesThatMakeNoSenseAreIgnored(t *testing.T) {
	s := newTestServer(t, 12)
	conn, _ := s.join("anna")
	for _, data := range [][]byte{{}, {99}, {protocol.Input}, {protocol.Input, 1, 0, 0, 0, 0}, {protocol.Snapshot, 1, 2, 3}, {protocol.Hello, protocol.Version, 'x'}, {protocol.Hello, 9}} {
		conn.send(data)
	}
	s.tick()
	snapshot := conn.snapshot()
	assert.Equal(t, uint32(0), snapshot.Ack)
	assert.Equal(t, 1, s.controller.state.GetPlayerCount(), "a second hello changes nothing")
}

func TestPingIsAnsweredWithoutWaitingForATick(t *testing.T) {
	s := newTestServer(t, 12)
	conn, _ := s.join("anna")
	conn.send(protocol.EncodePing(123456))
	assert.Equal(t, protocol.NewBinaryProtocol().Pong(123456), conn.read(protocol.Pong))

	// also before the hello: a client measures before it joins
	early := s.connect("early")
	early.send(protocol.EncodePing(7))
	assert.Equal(t, protocol.NewBinaryProtocol().Pong(7), early.read(protocol.Pong))
}

// What must arrive goes out on the reliable lane, what the next message
// replaces anyway on the unreliable one. A connection decides what a lane is.
func TestEveryMessageGoesOutOnItsLane(t *testing.T) {
	transport := &testTransport{}
	network := NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := NewController("test", 12, time.Second, network, testMaster{})
	transport.unregister = controller.UnregisterPlayer
	go network.run()
	s := &testServer{t: t, controller: controller, now: time.Unix(1000, 0)}

	conn, _ := s.join("anna")
	conn.send(protocol.EncodePing(1))
	conn.read(protocol.Pong)
	// a whole round: start, snapshots, scoreboards, end
	s.ticks(1000/model.TickMillis + 2)
	conn.read(protocol.RoundEnd)
	old := s.connect("old")
	old.send([]byte{protocol.Hello, protocol.Version + 1})
	s.tick()
	old.read(protocol.Reject)

	conn.lanes.Lock()
	defer conn.lanes.Unlock()
	for _, messageType := range []byte{protocol.Welcome, protocol.RoundStart, protocol.RoundEnd, protocol.Scoreboard} {
		assert.NotZero(t, conn.reliable[messageType], "type %d on the reliable lane", messageType)
		assert.Zero(t, conn.datagrams[messageType], "type %d on the unreliable lane", messageType)
	}
	for _, messageType := range []byte{protocol.Snapshot, protocol.Pong} {
		assert.NotZero(t, conn.datagrams[messageType], "type %d on the unreliable lane", messageType)
		assert.Zero(t, conn.reliable[messageType], "type %d on the reliable lane", messageType)
	}
	old.lanes.Lock()
	defer old.lanes.Unlock()
	assert.Equal(t, 1, old.reliable[protocol.Reject])
}

func TestRoundEndsAndTheNextOneStarts(t *testing.T) {
	transport := &testTransport{}
	network := NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := NewController("test", 12, time.Second, network, testMaster{})
	transport.unregister = controller.UnregisterPlayer
	go network.run()
	s := &testServer{t: t, controller: controller, now: time.Unix(1000, 0)}

	conn, _ := s.join("anna")
	started := s.controller.state.RoundStartTick()
	s.ticks(1000/model.TickMillis + 2)
	end := conn.read(protocol.RoundEnd)
	assert.Equal(t, protocol.NewBinaryProtocol().RoundEnd(started+31), end, "one second is 31 ticks of 33 ms")

	// nothing but the intermission until the next round
	s.ticks(int(intermission/(model.TickMillis*time.Millisecond)) + 2)
	next := conn.read(protocol.RoundStart)
	assert.Equal(t, protocol.NewBinaryProtocol().RoundStart(s.controller.state.RoundStartTick()), next)
	assert.Greater(t, s.controller.state.RoundStartTick(), started+31)
}
