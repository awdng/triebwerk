package main

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/gorilla/websocket"
)

// viewer relays what the simulated players see to browsers
type viewer struct {
	stats    *stats
	bots     []*bot
	upgrader websocket.Upgrader

	mutex       sync.Mutex
	browsers    map[chan []byte]bool
	running     bool
	haveFrame   bool
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

func newViewer(s *stats, bots []*bot) *viewer {
	return &viewer{
		stats:    s,
		bots:     bots,
		browsers: make(map[chan []byte]bool),
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func float(data []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
}

// publishState forwards a state broadcast, every player receives the same
// broadcast so only the first copy of each tick is forwarded
func (v *viewer) publishState(data []byte) {
	gameTime := binary.LittleEndian.Uint32(data[2:])
	v.mutex.Lock()
	if v.haveFrame && gameTime <= v.lastFrame {
		v.mutex.Unlock()
		return
	}
	now := time.Now()
	if v.haveFrame && now.Sub(v.lastFrameAt) > v.maxGap {
		v.maxGap = now.Sub(v.lastFrameAt)
	}
	v.haveFrame = true
	v.lastFrame = gameTime
	v.lastFrameAt = now
	v.frames++
	v.mutex.Unlock()

	frame := frameView{Type: "frame", GameTime: gameTime, Players: []playerView{}}
	for i := 0; i+entrySize <= len(data); i += entrySize {
		e := data[i : i+entrySize]
		frame.Players = append(frame.Players, playerView{
			ID:       int(e[0]),
			X:        float(e, 10),
			Y:        float(e, 14),
			TurretX:  float(e, 18),
			TurretY:  float(e, 22),
			Rotation: float(e, 26),
			Shooting: e[34] == 1,
			Health:   int(e[35]),
		})
	}
	v.broadcast(frame)
}

// publishEvent forwards game start and end, once per game
func (v *viewer) publishEvent(event string, gameTime uint32) {
	v.mutex.Lock()
	if (event == "start") == v.running {
		v.mutex.Unlock()
		return
	}
	v.running = event == "start"
	v.haveFrame = false // game time starts at zero again
	v.mutex.Unlock()
	if event == "end" {
		atomic.AddInt64(&v.stats.gamesPlayed, 1)
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
			Connected:     atomic.LoadInt64(&v.stats.connected),
			Clients:       len(v.bots),
			LostConns:     atomic.LoadInt64(&v.stats.lostConns),
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
			stats.Bots = append(stats.Bots, botView{Name: b.name, PlayerID: id, Keys: pressed})
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
