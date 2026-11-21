package game_test

import (
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/loadtest"
	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	loadTestPlayers = 12
	loadTestRound   = 12 * time.Second
	// long enough to see a round end and the next one start (10s intermission)
	loadTestDuration = 26 * time.Second
)

// fakeMaster accepts every player and records what the server reports
type fakeMaster struct {
	mutex      sync.Mutex
	heartbeats []model.StateSnapshot
	endedGames []model.StateSnapshot
}

func (m *fakeMaster) Init(address string) {}
func (m *fakeMaster) GetServerState()     {}

func (m *fakeMaster) SendHeartbeat(s model.StateSnapshot) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.heartbeats = append(m.heartbeats, s)
}

func (m *fakeMaster) EndGame(s model.StateSnapshot) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.endedGames = append(m.endedGames, s)
}

func (m *fakeMaster) AuthorizePlayer(token string) (string, string, error) {
	return "id-" + token, token, nil
}

func (m *fakeMaster) lastHeartbeat() (model.StateSnapshot, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(m.heartbeats) == 0 {
		return model.StateSnapshot{}, false
	}
	return m.heartbeats[len(m.heartbeats)-1], true
}

func (m *fakeMaster) games() []model.StateSnapshot {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return append([]model.StateSnapshot{}, m.endedGames...)
}

// startServer runs a complete game server in this process on a free port
func startServer(t *testing.T, master *fakeMaster) string {
	transport := websocket.NewTransport("localhost", 0)
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := game.NewController("test", loadTestPlayers, loadTestRound, networkManager, master)
	transport.RegisterNewConnHandler(controller.RegisterPlayer)
	transport.UnregisterConnHandler(controller.UnregisterPlayer)
	go controller.Init()

	// the transport picks a free port when it starts listening
	var address string
	require.Eventually(t, func() bool {
		host, port, err := net.SplitHostPort(transport.GetAddress())
		if err != nil || port == "0" {
			return false
		}
		address = net.JoinHostPort(host, port)
		return true
	}, 5*time.Second, 20*time.Millisecond)
	return "ws://" + address + "/echo"
}

// TestLoad12Players plays with the maximum number of players through a
// complete round and the start of the next one and checks that nobody is
// dropped, updates arrive at the tick rate without stalls, a 13th player is
// turned away and the server forgets everybody after they left.
func TestLoad12Players(t *testing.T) {
	if testing.Short() {
		t.Skip("load test takes about 40 seconds, skipped with -short")
	}
	master := &fakeMaster{}
	url := startServer(t, master)

	runner := loadtest.NewRunner(loadtest.Config{
		URL:      url,
		Players:  loadTestPlayers,
		Duration: loadTestDuration,
		Stagger:  time.Second,
		Logf:     t.Logf,
	})

	// once everybody is in, one more player must be rejected
	rejected := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if len(runner.Bots) > 0 && connected(runner) == loadTestPlayers {
				rejected <- expectRejected(url)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		rejected <- assert.AnError
	}()

	report := runner.Run(context.Background())
	t.Log("\n" + report.String())
	for _, err := range report.Check(loadtest.DefaultExpectations(loadTestPlayers)) {
		t.Error(err)
	}
	assert.NoError(t, <-rejected, "a 13th player must be rejected while the server is full")

	// the round end was reported to the master server with every player
	games := master.games()
	if assert.NotEmpty(t, games, "no round end was reported to the master server") {
		assert.Len(t, games[0].Players, loadTestPlayers)
		names := map[string]bool{}
		for _, p := range games[0].Players {
			names[p.Nickname] = true
		}
		assert.True(t, names["load-00"], "nicknames come from the auth token, got %v", names)
	}

	// after everybody left the heartbeat reports an empty server
	assert.Eventually(t, func() bool {
		s, ok := master.lastHeartbeat()
		return ok && len(s.Players) == 0
	}, 12*time.Second, 200*time.Millisecond, "players are still on the server after they disconnected")
}

func connected(runner *loadtest.Runner) int {
	count := 0
	for _, b := range runner.Bots {
		if id, _ := b.Keys(); id != 0 {
			count++
		}
	}
	return count
}

// expectRejected connects one more player and expects the server to close
// the connection right away
func expectRejected(url string) error {
	c, _, err := gorilla.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil
	}
	defer c.Close()
	c.WriteMessage(gorilla.BinaryMessage, protocol.EncodeHello("one-too-many"))
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return errTimeout
			}
			return nil
		}
		// a register message means the server accepted the player
		if len(data) > 1 && data[1] == 2 {
			return errAccepted(int(data[0]))
		}
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTimeout = testError("the connection stayed open")

func errAccepted(id int) error {
	return testError("the server accepted the player as id " + strconv.Itoa(id))
}
