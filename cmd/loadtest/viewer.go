package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awdng/triebwerk/loadtest"
	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
	"github.com/gorilla/websocket"
)

// viewer relays what the simulated players see to browsers
type viewer struct {
	stats       *loadtest.Stats
	bots        []*loadtest.Bot
	gamesPlayed int64
	upgrader    websocket.Upgrader

	mutex     sync.Mutex
	browsers  map[chan []byte]bool
	running   bool
	haveFrame bool
	// tick the running round started at and tick of the last frame
	roundStart uint32
	// shot counters of the tanks in the last frame
	shots       map[int]uint8
	lastFrame   uint32
	lastFrameAt time.Time
	frames      int
	maxGap      time.Duration
}

type playerView struct {
	ID       int     `json:"id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	TurretX  float32 `json:"tx"`
	TurretY  float32 `json:"ty"`
	Rotation float32 `json:"rot"`
	Shooting bool    `json:"shoot"`
	Health   int     `json:"health"`
}

type frameView struct {
	Type     string       `json:"type"`
	GameTime uint32       `json:"t"`
	Players  []playerView `json:"players"`
}

type eventView struct {
	Type     string `json:"type"`
	Event    string `json:"event"`
	GameTime uint32 `json:"t"`
}

type botView struct {
	Name     string `json:"name"`
	PlayerID int    `json:"id"`
	Keys     []bool `json:"keys"`
}

type statsView struct {
	Type          string    `json:"type"`
	Running       bool      `json:"running"`
	Connected     int64     `json:"connected"`
	Clients       int       `json:"clients"`
	LostConns     int64     `json:"lostConns"`
	UpdatesPerSec int       `json:"updatesPerSec"`
	MaxGap        int64     `json:"maxGap"`
	Bots          []botView `json:"bots"`
}

type mapView struct {
	Colliders []*model.Collider `json:"colliders"`
	Spawns    []*model.Point    `json:"spawns"`
	Min       *model.Point      `json:"min"`
	Max       *model.Point      `json:"max"`
}

func newViewer(s *loadtest.Stats, bots []*loadtest.Bot) *viewer {
	return &viewer{
		stats:    s,
		bots:     bots,
		browsers: make(map[chan []byte]bool),
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

// publishState forwards a snapshot, every player receives the same tanks
// so only the first copy of each tick is forwarded
func (v *viewer) publishState(snapshot protocol.SnapshotMessage) {
	v.mutex.Lock()
	if v.haveFrame && snapshot.Tick <= v.lastFrame {
		v.mutex.Unlock()
		return
	}
	gameTime := (snapshot.Tick - v.roundStart) * model.TickMillis
	now := time.Now()
	if v.haveFrame && now.Sub(v.lastFrameAt) > v.maxGap {
		v.maxGap = now.Sub(v.lastFrameAt)
	}
	v.haveFrame = true
	v.lastFrame = snapshot.Tick
	v.lastFrameAt = now
	v.frames++

	frame := frameView{Type: "frame", GameTime: gameTime, Players: []playerView{}}
	shots := make(map[int]uint8, len(snapshot.Players))
	for _, e := range snapshot.Players {
		// a shot is a change of the tank's shot counter
		last, known := v.shots[e.ID]
		shots[e.ID] = e.Shots
		// the end of the barrel, three units from the middle of the tank
		sin, cos := math.Sincos(float64(e.TurretRotation))
		frame.Players = append(frame.Players, playerView{
			ID:       e.ID,
			X:        e.X,
			Y:        e.Y,
			TurretX:  e.X - 3*float32(sin),
			TurretY:  e.Y + 3*float32(cos),
			Rotation: e.Rotation,
			Shooting: known && last != e.Shots,
			Health:   e.Health,
		})
	}
	v.shots = shots
	v.mutex.Unlock()
	v.broadcast(frame)
}

// publishEvent forwards game start and end, once per game
func (v *viewer) publishEvent(event string, tick uint32) {
	v.mutex.Lock()
	if (event == "start") == v.running {
		v.mutex.Unlock()
		return
	}
	v.running = event == "start"
	gameTime := uint32(0)
	if v.running {
		v.roundStart = tick
	} else {
		gameTime = (tick - v.roundStart) * model.TickMillis
	}
	v.mutex.Unlock()
	if event == "end" {
		atomic.AddInt64(&v.gamesPlayed, 1)
	}
	v.broadcast(eventView{Type: "event", Event: event, GameTime: gameTime})
}

func (v *viewer) broadcast(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("viewer: %v", err)
		return
	}
	v.mutex.Lock()
	defer v.mutex.Unlock()
	for browser := range v.browsers {
		select {
		case browser <- data:
		default: // slow browser, skip this message
		}
	}
}

func (v *viewer) publishStats() {
	for range time.Tick(500 * time.Millisecond) {
		v.mutex.Lock()
		stats := statsView{
			Type:          "stats",
			Running:       v.running,
			Connected:     atomic.LoadInt64(&v.stats.Connected),
			Clients:       len(v.bots),
			LostConns:     atomic.LoadInt64(&v.stats.LostConnections),
			UpdatesPerSec: v.frames * 2,
			MaxGap:        int64(v.maxGap / time.Millisecond),
		}
		v.frames = 0
		v.maxGap = 0
		v.mutex.Unlock()
		for _, b := range v.bots {
			id, keys := b.Keys()
			pressed := make([]bool, len(keys))
			for i, k := range keys {
				pressed[i] = k == 1
			}
			stats.Bots = append(stats.Bots, botView{Name: b.Name, PlayerID: id, Keys: pressed})
		}
		v.broadcast(stats)
	}
}

func (v *viewer) handleBrowser(w http.ResponseWriter, r *http.Request) {
	conn, err := v.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	messages := make(chan []byte, 64)
	v.mutex.Lock()
	v.browsers[messages] = true
	v.mutex.Unlock()
	defer func() {
		v.mutex.Lock()
		delete(v.browsers, messages)
		v.mutex.Unlock()
		conn.Close()
	}()

	// detect closed browser tabs
	closed := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(closed)
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

func (v *viewer) serve(addr string) {
	m := model.NewMap()
	mapJSON, _ := json.Marshal(mapView{Colliders: m.Collider, Spawns: m.Spawns, Min: m.Min, Max: m.Max})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(viewerPage))
	})
	mux.HandleFunc("/map.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mapJSON)
	})
	mux.HandleFunc("/ws", v.handleBrowser)

	go v.publishStats()
	log.Printf("live view on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
