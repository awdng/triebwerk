package spectate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a game with two tanks in a running round
func game(t *testing.T) (*model.GameState, *model.Player, *model.Player) {
	t.Helper()
	g := model.NewGameState("EU", time.Minute)
	anna := model.NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	anna.Nickname = "anna"
	g.AddPlayer(anna)
	ben := model.NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	ben.Nickname = "ben"
	g.AddPlayer(ben)
	g.Start()
	anna.Collider.SetPose(10, 20, 0, 0)
	ben.Collider.SetPose(-30, 40, 1.5, 0.5)
	return g, anna, ben
}

type watcher struct {
	t    *testing.T
	conn *websocket.Conn
}

func serve(t *testing.T, hub *Hub) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.StripPrefix("/spectate", hub.Handler()))
	t.Cleanup(server.Close)
	return server
}

func watch(t *testing.T, server *httptest.Server, query string) (*watcher, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/spectate/ws" + query
	conn, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, response, err
	}
	t.Cleanup(func() { conn.Close() })
	return &watcher{t: t, conn: conn}, response, nil
}

// next returns the next message of a type
func (w *watcher) next(messageType string) map[string]any {
	w.t.Helper()
	for {
		w.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := w.conn.ReadMessage()
		require.NoError(w.t, err, "no message of type %s", messageType)
		var message map[string]any
		require.NoError(w.t, json.Unmarshal(data, &message))
		if message["type"] == messageType {
			return message
		}
	}
}

func watching(t *testing.T, hub *Hub, count int) {
	t.Helper()
	require.Eventually(t, func() bool { return hub.Watchers() == count }, time.Second, time.Millisecond)
}

func TestWatcherSeesTheTanksOfEveryTick(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	g, anna, _ := game(t)
	w, _, err := watch(t, server, "")
	require.NoError(t, err)
	watching(t, hub, 1)

	anna.Health = 60
	g.Tick()
	hub.Publish(g)
	frame := w.next("frame")
	assert.InDelta(t, model.TickMillis, frame["t"], 0.5, "the time of the round")
	players := frame["players"].([]any)
	require.Len(t, players, 2)
	first := players[0].(map[string]any)
	assert.Equal(t, float64(1), first["id"])
	assert.Equal(t, float64(10), first["x"])
	assert.Equal(t, float64(20), first["y"])
	assert.Equal(t, float64(60), first["health"])
	assert.Equal(t, false, first["shoot"])
	// the end of the barrel, three units along the turret
	assert.InDelta(t, 10, first["tx"], 0.001)
	assert.InDelta(t, 23, first["ty"], 0.001)
	second := players[1].(map[string]any)
	assert.InDelta(t, 1.5, second["rot"], 0.001)
}

func TestShotIsShownInTheTickItWasFired(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	g, anna, _ := game(t)
	w, _, _ := watch(t, server, "")
	watching(t, hub, 1)

	hub.Publish(g)
	assert.Equal(t, false, w.next("frame")["players"].([]any)[0].(map[string]any)["shoot"])
	anna.Shots++
	hub.Publish(g)
	assert.Equal(t, true, w.next("frame")["players"].([]any)[0].(map[string]any)["shoot"])
	hub.Publish(g)
	assert.Equal(t, false, w.next("frame")["players"].([]any)[0].(map[string]any)["shoot"])
}

func TestWatcherIsToldWhoPlaysWithWhichKeysAndScore(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	g, anna, ben := game(t)
	w, _, _ := watch(t, server, "")
	watching(t, hub, 1)

	for i := 0; i < statsEveryTicks-1; i++ {
		g.Tick()
		hub.Publish(g)
	}
	// the keys of the tick that was just played
	anna.Control = model.Controls{Forward: true, TurretRight: true, Shoot: true}
	anna.Score, ben.Deaths = 3, 3
	hub.Publish(g)
	stats := w.next("stats")
	assert.Equal(t, true, stats["running"])
	assert.Equal(t, float64(2), stats["connected"])
	assert.Equal(t, float64(12), stats["clients"])
	assert.Equal(t, float64(60000), stats["roundLength"])
	assert.Equal(t, "Berlin 1", stats["name"])
	bots := stats["bots"].([]any)
	require.Len(t, bots, 2)
	first := bots[0].(map[string]any)
	assert.Equal(t, "anna", first["name"])
	assert.Equal(t, float64(1), first["id"])
	assert.Equal(t, float64(3), first["kills"])
	// forward, backward, left, right, turret right, turret left, shoot
	assert.Equal(t, []any{true, false, false, false, true, false, true}, first["keys"])
	assert.Equal(t, float64(3), bots[1].(map[string]any)["deaths"])
}

func TestWatcherIsToldWhenARoundStartsAndEnds(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	g, _, _ := game(t)
	w, _, _ := watch(t, server, "")
	watching(t, hub, 1)

	// who starts to watch in the middle of a round learns that one is running
	hub.Publish(g)
	assert.Equal(t, "start", w.next("event")["event"])
	for i := 0; i < 10; i++ {
		g.Tick()
	}
	g.End()
	hub.Publish(g)
	end := w.next("event")
	assert.Equal(t, "end", end["event"])
	assert.InDelta(t, 10*model.TickMillis, end["t"], 0.5)
	g.Start()
	hub.Publish(g)
	assert.Equal(t, "start", w.next("event")["event"])
}

// the game loop calls Publish in every tick: with nobody watching it must
// cost nothing, and a watcher that does not read must not hold it up
func TestGameLoopIsNeverHeldUp(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	g, _, _ := game(t)
	assert.False(t, hub.Watching())
	assert.Zero(t, testing.AllocsPerRun(100, func() { hub.Publish(g) }), "nobody watches: nothing is built")

	_, _, err := watch(t, server, "") // connects and never reads
	require.NoError(t, err)
	watching(t, hub, 1)
	assert.True(t, hub.Watching())
	start := time.Now()
	for i := 0; i < 20000; i++ {
		g.Tick()
		hub.Publish(g)
	}
	assert.Less(t, time.Since(start), 5*time.Second, "ten minutes of ticks for a watcher that reads nothing")
}

func TestWatcherThatLeavesIsForgotten(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	w, _, _ := watch(t, server, "")
	watching(t, hub, 1)
	w.conn.Close()
	watching(t, hub, 0)
	assert.False(t, hub.Watching())
}

func TestOnlySoManyMayWatchAtOnce(t *testing.T) {
	hub := NewHub("Berlin 1", "", 12)
	server := serve(t, hub)
	for i := 0; i < MaxWatchers; i++ {
		_, _, err := watch(t, server, "")
		require.NoError(t, err)
	}
	watching(t, hub, MaxWatchers)
	_, response, err := watch(t, server, "")
	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

func TestWatchingNeedsTheKeyWhenThereIsOne(t *testing.T) {
	hub := NewHub("Berlin 1", "the-key", 12)
	server := serve(t, hub)
	for _, query := range []string{"", "?key=", "?key=wrong", "?key=the-key2"} {
		_, response, err := watch(t, server, query)
		require.Error(t, err, query)
		assert.Equal(t, http.StatusForbidden, response.StatusCode, query)
		for _, path := range []string{"/spectate/", "/spectate/map.json"} {
			r, err := http.Get(server.URL + path + query)
			require.NoError(t, err)
			r.Body.Close()
			assert.Equal(t, http.StatusForbidden, r.StatusCode, path+query)
		}
	}
	_, _, err := watch(t, server, "?key=the-key")
	assert.NoError(t, err)
}

func TestPageAndMapAreServed(t *testing.T) {
	hub := NewHub("Berlin <1>", "", 12)
	server := serve(t, hub)

	r, err := http.Get(server.URL + "/spectate/")
	require.NoError(t, err)
	defer r.Body.Close()
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Contains(t, r.Header.Get("Content-Type"), "text/html")
	page := make([]byte, 1<<20)
	n, _ := r.Body.Read(page)
	assert.Contains(t, string(page[:n]), "Berlin &lt;1&gt;", "a name is text, not markup")
	assert.NotContains(t, string(page[:n]), "Berlin <1>")

	m, err := http.Get(server.URL + "/spectate/map.json")
	require.NoError(t, err)
	defer m.Body.Close()
	var world map[string]any
	require.NoError(t, json.NewDecoder(m.Body).Decode(&world))
	assert.NotEmpty(t, world["colliders"])
	assert.NotEmpty(t, world["spawns"])
	assert.Contains(t, world, "min")
	assert.Contains(t, world, "max")
	// a page of another origin shows the view in a frame and may read the map
	assert.Equal(t, "*", m.Header.Get("Access-Control-Allow-Origin"))
}

func TestKeyOfTheSpectatorsComesFromTheKeyOfTheServers(t *testing.T) {
	assert.Equal(t, "", Key(""), "no server key: everybody may watch")
	key := Key("server-key")
	assert.Len(t, key, 32)
	assert.Equal(t, key, Key("server-key"))
	assert.NotEqual(t, key, Key("another-key"))
	assert.NotContains(t, key, "server-key")
	// the master server computes the same, see its api/admin_test.go
	assert.Equal(t, "06c0de71ffe2bdd53019292e401ea947", Key("server-key-for-tests"))
}
