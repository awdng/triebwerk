// Package spectate lets a browser watch a match from above: every tank,
// every shot, who plays and with which keys. The page is the live view of
// the load test (cmd/loadtest), which shows what its bots are sent; a game
// server shows its own state through a Hub.
//
// A hub serves, below the path it is mounted at:
//
//	./          the page
//	./map.json  walls, spawn points and bounds of the map
//	./ws        a WebSocket with JSON messages:
//	            frame  {t, players: [{id, x, y, tx, ty, rot, shoot, health}]}  every tick
//	            event  {event: "start" | "end", t}                             rounds
//	            stats  {running, connected, clients, roundLength, name,
//	                    updatesPerSec, maxGap, bots: [{id, name, keys, kills, deaths}]}
//
// Watchers get what every player is sent anyway, and the keys.
package spectate

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/gorilla/websocket"
)

const (
	// MaxWatchers at once: every one costs the server a copy of each tick
	MaxWatchers = 8
	// names, keys and scores are sent every so many ticks (half a second)
	statsEveryTicks = 15
	// messages that wait for a watcher; one that is further behind loses them
	watcherBuffer = 64
	// the barrel is drawn this far from the middle of the tank
	barrelLength = 3
)

// Key derives what a watcher has to show from the key game servers share
// with the master server, which hands it to its administrator. Without a
// server key anybody may watch.
func Key(serverKey string) string {
	if serverKey == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(serverKey))
	mac.Write([]byte("spectate"))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

type tankView struct {
	ID       int     `json:"id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	TurretX  float32 `json:"tx"`
	TurretY  float32 `json:"ty"`
	Rotation float32 `json:"rot"`
	Shooting bool    `json:"shoot"`
	Health   int     `json:"health"`
	// the kind of tank and the size of what collides
	Kind  string  `json:"kind"`
	Width float32 `json:"w"`
	Depth float32 `json:"d"`
}

type frameView struct {
	Type     string     `json:"type"`
	GameTime uint32     `json:"t"`
	Players  []tankView `json:"players"`
}

type eventView struct {
	Type     string `json:"type"`
	Event    string `json:"event"`
	GameTime uint32 `json:"t"`
}

type playerView struct {
	Name     string `json:"name"`
	PlayerID int    `json:"id"`
	// forward, backward, left, right, turret right, turret left, shoot
	Keys   []bool `json:"keys"`
	Kills  int    `json:"kills"`
	Deaths int    `json:"deaths"`
}

type statsView struct {
	Type          string       `json:"type"`
	Name          string       `json:"name"`
	Running       bool         `json:"running"`
	Connected     int          `json:"connected"`
	Clients       int          `json:"clients"`
	RoundLength   int64        `json:"roundLength"`
	UpdatesPerSec int          `json:"updatesPerSec"`
	MaxGap        int64        `json:"maxGap"`
	Bots          []playerView `json:"bots"`
}

type mapView struct {
	Colliders []*model.Collider `json:"colliders"`
	Spawns    []*model.Point    `json:"spawns"`
	Min       *model.Point      `json:"min"`
	Max       *model.Point      `json:"max"`
}

// Hub sends the state of a game to the browsers that watch it
type Hub struct {
	title      string
	key        string
	maxPlayers int
	upgrader   websocket.Upgrader
	mapJSON    []byte

	// read by the game loop in every tick
	count atomic.Int32

	mutex    sync.Mutex
	watchers map[chan []byte]bool

	// what the game loop remembers between two ticks, only it touches these
	running    bool
	announced  bool
	shots      map[int]uint8
	ticks      int
	lastTickAt time.Time
	maxGap     time.Duration
	statsAt    time.Time
}

// NewHub creates the live view of a server. title names it on the page,
// key is what watchers have to show ("" for none), maxPlayers what the
// server takes.
func NewHub(title, key string, maxPlayers int) *Hub {
	m := model.NewMap()
	mapJSON, _ := json.Marshal(mapView{Colliders: m.Collider, Spawns: m.Spawns, Min: m.Min, Max: m.Max})
	return &Hub{
		title:      title,
		key:        key,
		maxPlayers: maxPlayers,
		upgrader:   websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		mapJSON:    mapJSON,
		watchers:   make(map[chan []byte]bool),
		shots:      make(map[int]uint8),
	}
}

// Watching says whether anybody watches
func (h *Hub) Watching() bool {
	return h.count.Load() > 0
}

// Watchers is how many watch
func (h *Hub) Watchers() int {
	return int(h.count.Load())
}

// Publish shows the watchers the state after a tick. The game loop calls
// it in every tick; with nobody watching it does nothing, and it never
// waits for a watcher.
func (h *Hub) Publish(state *model.GameState) {
	if !h.Watching() {
		// whoever comes next starts from scratch
		h.announced = false
		return
	}
	now := time.Now()
	running := state.InProgress()
	gameTime := state.Snapshot().GameTime
	if !h.announced || running != h.running {
		h.announced = true
		h.running = running
		h.ticks, h.maxGap, h.statsAt = 0, 0, now
		// shots that were counted for an earlier watcher are long over
		h.shots = make(map[int]uint8)
		if running {
			h.broadcast(eventView{Type: "event", Event: "start"})
		} else {
			h.broadcast(eventView{Type: "event", Event: "end", GameTime: gameTime})
		}
	} else if gap := now.Sub(h.lastTickAt); gap > h.maxGap {
		h.maxGap = gap
	}
	h.lastTickAt = now

	players := state.GetPlayers()
	if running {
		frame := frameView{Type: "frame", GameTime: gameTime, Players: make([]tankView, 0, len(players))}
		shots := make(map[int]uint8, len(players))
		for _, p := range players {
			// a shot is a change of the tank's shot counter
			last, known := h.shots[p.ID]
			shots[p.ID] = p.Shots
			sin, cos := math.Sincos(float64(p.Collider.TurretRotation))
			frame.Players = append(frame.Players, tankView{
				ID:       p.ID,
				X:        p.Collider.Pivot.X,
				Y:        p.Collider.Pivot.Y,
				TurretX:  p.Collider.Pivot.X - barrelLength*float32(sin),
				TurretY:  p.Collider.Pivot.Y + barrelLength*float32(cos),
				Rotation: p.Collider.Rotation,
				Shooting: known && last != p.Shots,
				Health:   p.Health,
				Kind:     p.Spec().Name,
				Width:    p.Spec().Width,
				Depth:    p.Spec().Depth,
			})
		}
		h.shots = shots
		h.broadcast(frame)
	}

	h.ticks++
	if h.ticks%statsEveryTicks != 0 {
		return
	}
	stats := statsView{
		Type:        "stats",
		Name:        h.title,
		Running:     running,
		Connected:   len(players),
		Clients:     h.maxPlayers,
		RoundLength: state.RoundLength().Milliseconds(),
		MaxGap:      h.maxGap.Milliseconds(),
		Bots:        make([]playerView, 0, len(players)),
	}
	if elapsed := now.Sub(h.statsAt); elapsed > 0 {
		stats.UpdatesPerSec = int(math.Round(statsEveryTicks / elapsed.Seconds()))
	}
	h.statsAt, h.maxGap = now, 0
	for _, p := range players {
		c := p.Control
		stats.Bots = append(stats.Bots, playerView{
			Name:     p.Nickname,
			PlayerID: p.ID,
			Keys:     []bool{c.Forward, c.Backward, c.Left, c.Right, c.TurretRight, c.TurretLeft, c.Shoot},
			Kills:    p.Score,
			Deaths:   p.Deaths,
		})
	}
	h.broadcast(stats)
}

func (h *Hub) broadcast(message any) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	for watcher := range h.watchers {
		select {
		case watcher <- data:
		default: // a watcher that is behind loses this message
		}
	}
}

// Handler serves the page, the map and the WebSocket, see the package comment
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.guarded(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(Page(h.title))
	}))
	mux.HandleFunc("GET /map.json", h.guarded(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(h.mapJSON)
	}))
	mux.HandleFunc("GET /ws", h.guarded(h.serveWatcher))
	return mux
}

// guarded lets through who has the key, if there is one
func (h *Hub) guarded(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.key != "" && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(h.key)) != 1 {
			http.Error(w, "watching this server needs the link from its master server", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (h *Hub) serveWatcher(w http.ResponseWriter, r *http.Request) {
	messages := make(chan []byte, watcherBuffer)
	h.mutex.Lock()
	if len(h.watchers) >= MaxWatchers {
		h.mutex.Unlock()
		http.Error(w, "too many are watching this server already", http.StatusServiceUnavailable)
		return
	}
	h.watchers[messages] = true
	h.count.Store(int32(len(h.watchers)))
	h.mutex.Unlock()
	leave := func() {
		h.mutex.Lock()
		delete(h.watchers, messages)
		h.count.Store(int32(len(h.watchers)))
		h.mutex.Unlock()
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		leave()
		return
	}
	defer func() {
		leave()
		conn.Close()
	}()
	// a browser tab that is closed ends the reading
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-closed:
			return
		case data := <-messages:
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if conn.WriteMessage(websocket.TextMessage, data) != nil {
				return
			}
		}
	}
}
