package infra

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a master server that writes down what it is asked
type request struct {
	method, path, key string
	body              map[string]any
}

type recordingMaster struct {
	t      *testing.T
	server *httptest.Server

	mutex    sync.Mutex
	requests []request
	// forget makes the master answer the next heartbeats like one that restarted
	forget bool
	down   bool
}

func newRecordingMaster(t *testing.T) *recordingMaster {
	m := &recordingMaster{t: t}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *recordingMaster) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(raw) > 0 {
		require.NoError(m.t, json.Unmarshal(raw, &body), "body: %s", raw)
		assert.Equal(m.t, "application/json", r.Header.Get("Content-Type"))
	}
	m.mutex.Lock()
	m.requests = append(m.requests, request{r.Method, r.URL.Path, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), body})
	forget, down := m.forget, m.down
	if forget && r.Method == http.MethodPost {
		m.forget = false
	}
	m.mutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case down:
		w.WriteHeader(http.StatusBadGateway)
	case r.Method == http.MethodGet && r.URL.Path == "/api/config":
		w.Write([]byte(`{"name":"test","guests":true}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/game/servers":
		w.Write([]byte(`{"id":"s_one","heartbeatSeconds":5}`))
	case r.Method == http.MethodPut && r.URL.Path == "/api/game/servers/s_one":
		if forget {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"register again"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/game/servers/s_one":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/game/authorize":
		switch body["token"] {
		case "pz_session":
			w.Write([]byte(`{"playerId":"p_anna","name":"anna","guest":false}`))
		case "visitor":
			w.Write([]byte(`{"playerId":"","name":"visitor","guest":true}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"sign in again"}`))
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *recordingMaster) seen() []request {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return append([]request(nil), m.requests...)
}

func (m *recordingMaster) calls() []string {
	var calls []string
	for _, r := range m.seen() {
		calls = append(calls, r.method+" "+r.path)
	}
	return calls
}

func client(m *recordingMaster) *MasterServerClient {
	return NewMasterServerClient(MasterConfig{
		URL: m.server.URL, Key: "the-key", Name: "Berlin 1", Region: "EU", Map: "dark-city",
		MaxPlayers: 12, Secure: true, Protocol: 2, Version: "abc123",
	})
}

func round(number uint32, inProgress bool, players ...model.PlayerSnapshot) model.StateSnapshot {
	online := 0
	for _, p := range players {
		if p.Connected {
			online++
		}
	}
	gameTime := uint32(31900)
	if number == 0 {
		// no round has been played, no time has passed in one
		gameTime = 0
	}
	return model.StateSnapshot{Region: "EU", Online: online, Round: number, InProgress: inProgress, GameTime: gameTime, Length: 5 * time.Minute, Players: players}
}

func TestServerRegistersWithWhatClientsNeedToKnow(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")

	seen := m.seen()
	require.Len(t, seen, 1)
	assert.Equal(t, "POST", seen[0].method)
	assert.Equal(t, "/api/game/servers", seen[0].path)
	assert.Equal(t, "the-key", seen[0].key)
	assert.Equal(t, map[string]any{
		"name": "Berlin 1", "address": "game.example:9090", "secure": true, "region": "EU",
		"map": "dark-city", "maxPlayers": float64(12), "protocol": float64(2), "version": "abc123",
	}, seen[0].body)
}

func TestServerWithoutANameIsCalledByRegionAndAddress(t *testing.T) {
	m := newRecordingMaster(t)
	c := NewMasterServerClient(MasterConfig{URL: m.server.URL, Region: "EU", Map: "dark-city", MaxPlayers: 12})
	c.Init("game.example:9090")
	assert.Equal(t, "EU game.example:9090", m.seen()[0].body["name"])
}

func TestHeartbeatCarriesTheRoundAndItsScores(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")
	c.SendHeartbeat(round(3, true,
		model.PlayerSnapshot{Slot: 1, GlobalID: "p_anna", Nickname: "anna", Score: 4, Deaths: 1, SecondsPlayed: 31, Connected: true},
		model.PlayerSnapshot{Slot: 2, Nickname: "visitor", Score: 0, Deaths: 2, SecondsPlayed: 12, Connected: false},
	))

	seen := m.seen()
	require.Len(t, seen, 2)
	assert.Equal(t, "PUT /api/game/servers/s_one", seen[1].method+" "+seen[1].path)
	assert.Equal(t, "the-key", seen[1].key)
	body := seen[1].body
	assert.Equal(t, float64(1), body["online"])
	roundSent := body["round"].(map[string]any)
	assert.NotEmpty(t, roundSent["key"])
	delete(roundSent, "key")
	assert.Equal(t, map[string]any{"active": true, "elapsedSeconds": float64(31), "lengthSeconds": float64(300), "ended": false}, roundSent)
	assert.Equal(t, []any{
		map[string]any{"slot": float64(1), "playerId": "p_anna", "name": "anna", "kills": float64(4), "deaths": float64(1), "secondsPlayed": float64(31), "connected": true},
		map[string]any{"slot": float64(2), "playerId": "", "name": "visitor", "kills": float64(0), "deaths": float64(2), "secondsPlayed": float64(12), "connected": false},
	}, body["players"])
}

func key(r request) string {
	return r.body["round"].(map[string]any)["key"].(string)
}

// the master tells rounds apart by their key: the same for every word about
// one round, another for the next, and never one a server used before it
// was restarted
func TestEveryRoundHasItsOwnKey(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")
	c.SendHeartbeat(round(1, true))
	c.SendHeartbeat(round(1, true))
	c.EndGame(round(1, false))
	c.SendHeartbeat(round(2, true))

	seen := m.seen()
	require.Len(t, seen, 5)
	assert.Equal(t, key(seen[1]), key(seen[2]))
	assert.Equal(t, key(seen[1]), key(seen[3]))
	assert.NotEqual(t, key(seen[1]), key(seen[4]))

	time.Sleep(2 * time.Millisecond)
	restarted := client(m)
	restarted.Init("game.example:9090")
	restarted.SendHeartbeat(round(1, true))
	assert.NotEqual(t, key(seen[1]), key(m.seen()[6]))
}

func TestEndOfARoundIsReportedAsEnded(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")
	c.EndGame(round(1, false, model.PlayerSnapshot{Slot: 1, Nickname: "anna", Score: 9, Connected: true}))

	sent := m.seen()[1].body
	roundSent := sent["round"].(map[string]any)
	assert.Equal(t, false, roundSent["active"])
	assert.Equal(t, true, roundSent["ended"])
	assert.Len(t, sent["players"], 1)
}

func TestServerThatHasPlayedNoRoundReportsNone(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")
	c.SendHeartbeat(round(0, false))
	sent := m.seen()[1].body
	assert.Equal(t, map[string]any{"key": "", "active": false, "elapsedSeconds": float64(0), "lengthSeconds": float64(300), "ended": false}, sent["round"])
	assert.Equal(t, []any{}, sent["players"])
}

// the master was restarted and has forgotten its servers
func TestServerRegistersAgainWhenTheMasterDoesNotKnowIt(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Init("game.example:9090")
	m.mutex.Lock()
	m.forget = true
	m.mutex.Unlock()
	c.SendHeartbeat(round(1, true))

	assert.Equal(t, []string{
		"POST /api/game/servers",
		"PUT /api/game/servers/s_one",
		"POST /api/game/servers",
		"PUT /api/game/servers/s_one",
	}, m.calls(), "the heartbeat is not lost: it is sent again once the server is known")
}

// the master was not there when the server started
func TestServerKeepsTryingToRegister(t *testing.T) {
	m := newRecordingMaster(t)
	m.down = true
	c := client(m)
	c.Init("game.example:9090")
	c.SendHeartbeat(round(1, true))
	assert.Equal(t, []string{"POST /api/game/servers", "POST /api/game/servers"}, m.calls(), "no heartbeat without an id")

	m.mutex.Lock()
	m.down = false
	m.mutex.Unlock()
	c.SendHeartbeat(round(1, true))
	assert.Equal(t, []string{"POST /api/game/servers", "PUT /api/game/servers/s_one"}, m.calls()[2:])
}

func TestMasterThatCanNotBeReachedDoesNotBlock(t *testing.T) {
	c := NewMasterServerClient(MasterConfig{URL: "http://127.0.0.1:1", Region: "EU", Map: "m", MaxPlayers: 12})
	start := time.Now()
	c.Init("game.example:9090")
	c.SendHeartbeat(round(1, true))
	_, _, err := c.AuthorizePlayer("visitor")
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestPlayerIsWhoTheMasterSaysTheTokenBelongsTo(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)

	id, name, err := c.AuthorizePlayer("pz_session")
	require.NoError(t, err)
	assert.Equal(t, "p_anna", id)
	assert.Equal(t, "anna", name)

	id, name, err = c.AuthorizePlayer("visitor")
	require.NoError(t, err)
	assert.Equal(t, "", id, "a guest has no account")
	assert.Equal(t, "visitor", name)

	seen := m.seen()
	assert.Equal(t, "POST /api/game/authorize", seen[0].method+" "+seen[0].path)
	assert.Equal(t, "the-key", seen[0].key)
	assert.Equal(t, map[string]any{"token": "pz_session"}, seen[0].body)
}

func TestPlayerTheMasterTurnsAwayIsNotAuthorized(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	_, _, err := c.AuthorizePlayer("pz_expired")
	assert.ErrorIs(t, err, ErrNotAuthorized)

	m.down = true
	_, _, err = c.AuthorizePlayer("visitor")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotAuthorized, "a master that is broken has not said no")
}

func TestServerSaysGoodbye(t *testing.T) {
	m := newRecordingMaster(t)
	c := client(m)
	c.Close() // never registered: nothing to say
	assert.Empty(t, m.calls())
	c.Init("game.example:9090")
	c.Close()
	assert.Equal(t, []string{"POST /api/game/servers", "DELETE /api/game/servers/s_one"}, m.calls())
}

func TestWaitingForTheMasterReturnsOnceItAnswers(t *testing.T) {
	m := newRecordingMaster(t)
	m.down = true
	c := client(m)
	c.retryEvery = 10 * time.Millisecond
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.mutex.Lock()
		m.down = false
		m.mutex.Unlock()
	}()
	done := make(chan struct{})
	go func() {
		c.WaitUntilReachable()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("still waiting for a master that answers")
	}
	assert.Greater(t, len(m.calls()), 1)
}

// a server the master refuses has to learn why from its own log
func TestRegistrationTheMasterRefusesSaysWhyInTheLog(t *testing.T) {
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"a server needs a name of 1 to 80 characters (SERVER_NAME)"}`))
	}))
	defer master.Close()
	var logged strings.Builder
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)

	c := NewMasterServerClient(MasterConfig{URL: master.URL, Region: "EU", Map: "m", MaxPlayers: 12})
	c.Init("game.example:9090")
	assert.Contains(t, logged.String(), "a server needs a name of 1 to 80 characters (SERVER_NAME)")
	assert.Contains(t, logged.String(), "400")
}
