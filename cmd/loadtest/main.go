// Command loadtest connects simulated players to a triebwerk server. Every
// player holds random key combinations for a human-like duration, shoots and
// syncs its clock. A live view of the match is served over HTTP.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// size of one player entry in a state broadcast:
// id, type, gametime(4), sequence(4), 6*float32, shooting, health
const entrySize = 36

const (
	msgState     = 1
	msgRegister  = 2
	msgTime      = 5
	msgGameStart = 6
	msgGameEnd   = 7
)

type stats struct {
	stateMsgs   int64
	maxGap      int64 // ms between state broadcasts received by a client
	lostConns   int64
	connected   int64
	gamesPlayed int64
}

func maxStore(p *int64, v int64) {
	for {
		o := atomic.LoadInt64(p)
		if v <= o || atomic.CompareAndSwapInt64(p, o, v) {
			return
		}
	}
}

// bot is one simulated player
type bot struct {
	name  string
	url   string
	rng   *rand.Rand
	stats *stats
	view  *viewer

	mutex    sync.Mutex
	playerID int
	keys     []byte // forward, backward, left, right, turret right, turret left, shoot
}

func newBot(id int, url string, s *stats, v *viewer) *bot {
	return &bot{
		name:  fmt.Sprintf("load-%02d", id),
		url:   url,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano() + int64(id))),
		stats: s,
		view:  v,
		keys:  make([]byte, 7),
	}
}

// Keys returns the player id on the server and the currently pressed keys
func (b *bot) Keys() (int, []byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.playerID, append([]byte{}, b.keys...)
}

// pressRandomKeys picks the next key combination, returns how long it is held
func (b *bot) pressRandomKeys() time.Duration {
	keys := make([]byte, 7)
	switch r := b.rng.Float64(); {
	case r < 0.55:
		keys[0] = 1
	case r < 0.70:
		keys[1] = 1
	}
	if b.rng.Float64() < 0.4 {
		keys[2+b.rng.Intn(2)] = 1 // left or right
	}
	if b.rng.Float64() < 0.5 {
		keys[4+b.rng.Intn(2)] = 1 // turret
	}
	if b.rng.Float64() < 0.35 {
		keys[6] = 1
	}
	if b.rng.Float64() < 0.05 { // mash everything
		for i := range keys {
			keys[i] = 1
		}
	}
	b.mutex.Lock()
	b.keys = keys
	b.mutex.Unlock()
	return time.Duration(150+b.rng.Intn(1800)) * time.Millisecond
}

// play connects to the server and plays until the deadline or connection loss
func (b *bot) play(until time.Time) {
	c, _, err := websocket.DefaultDialer.Dial(b.url, nil)
	if err != nil {
		log.Printf("%s: dial failed: %v", b.name, err)
		atomic.AddInt64(&b.stats.lostConns, 1)
		return
	}
	defer c.Close()
	atomic.AddInt64(&b.stats.connected, 1)
	defer atomic.AddInt64(&b.stats.connected, -1)

	var writeMutex sync.Mutex
	write := func(data []byte) error {
		writeMutex.Lock()
		defer writeMutex.Unlock()
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		return c.WriteMessage(websocket.BinaryMessage, data)
	}
	write(append([]byte{0, 0}, []byte(b.name)...))

	done := make(chan struct{})
	defer close(done)
	go b.sendInputs(write, until, done)

	var last time.Time
	for time.Now().Before(until) {
		c.SetReadDeadline(until)
		_, data, err := c.ReadMessage()
		if err != nil {
			if time.Now().Before(until) {
				log.Printf("%s: connection lost: %v", b.name, err)
				atomic.AddInt64(&b.stats.lostConns, 1)
			}
			break
		}
		if len(data) < 2 {
			continue
		}
		switch data[1] {
		case msgRegister:
			b.mutex.Lock()
			b.playerID = int(data[0])
			b.mutex.Unlock()
		case msgState:
			atomic.AddInt64(&b.stats.stateMsgs, 1)
			if !last.IsZero() {
				maxStore(&b.stats.maxGap, int64(time.Since(last)/time.Millisecond))
			}
			last = time.Now()
			b.view.publishState(data)
		case msgGameStart:
			b.view.publishEvent("start", binary.LittleEndian.Uint32(data[2:]))
		case msgGameEnd:
			last = time.Time{}
			b.view.publishEvent("end", binary.LittleEndian.Uint32(data[2:]))
		}
	}
	b.mutex.Lock()
	b.playerID = 0
	b.mutex.Unlock()
}

// sendInputs sends the pressed keys at 30Hz like a render loop would
func (b *bot) sendInputs(write func([]byte) error, until time.Time, done chan struct{}) {
	var sequence uint32
	nextChange := time.Now()
	lastTimeSync := time.Now()
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(until) {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		if time.Now().After(nextChange) {
			nextChange = time.Now().Add(b.pressRandomKeys())
		}
		_, keys := b.Keys()
		sequence++
		message := append([]byte{0, 1}, keys...)
		message = append(message, 0, 0, 0, 0)
		binary.BigEndian.PutUint32(message[9:], sequence)
		if write(message) != nil {
			return
		}
		if time.Since(lastTimeSync) > time.Second {
			lastTimeSync = time.Now()
			message := make([]byte, 6)
			message[1] = msgTime
			binary.BigEndian.PutUint32(message[2:], uint32(time.Now().UnixNano()/1e6))
			write(message)
		}
	}
}

func main() {
	clients := flag.Int("n", 12, "number of simulated players")
	duration := flag.Duration("d", 0, "duration of the test, 0 runs until interrupted")
	reconnect := flag.Int("reconnect", 0, "players that leave once mid-game and rejoin")
	url := flag.String("url", "ws://localhost:9090/echo", "websocket url of the server")
	viewAddr := flag.String("view", ":8090", "address of the live view, empty to disable")
	flag.Parse()

	if *duration == 0 {
		*duration = 100 * 365 * 24 * time.Hour
	}
	end := time.Now().Add(*duration)

	s := &stats{}
	bots := make([]*bot, *clients)
	view := newViewer(s, bots)
	for i := range bots {
		bots[i] = newBot(i, *url, s, view)
	}
	if *viewAddr != "" {
		go view.serve(*viewAddr)
	}

	var wg sync.WaitGroup
	for i, b := range bots {
		wg.Add(1)
		go func(i int, b *bot) {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(3000)) * time.Millisecond) // staggered joins
			if i < *reconnect {
				leave := time.Now().Add(time.Duration(60+rand.Intn(120)) * time.Second)
				b.play(leave)
				log.Printf("%s: left voluntarily, rejoining in 5s", b.name)
				time.Sleep(5 * time.Second)
			}
			b.play(end)
		}(i, b)
	}

	go func() {
		var previous int64
		for range time.Tick(10 * time.Second) {
			current := atomic.LoadInt64(&s.stateMsgs)
			log.Printf("connected: %d | state msgs/s: %.0f | max gap between updates: %dms | lost connections: %d",
				atomic.LoadInt64(&s.connected), float64(current-previous)/10,
				atomic.SwapInt64(&s.maxGap, 0), atomic.LoadInt64(&s.lostConns))
			previous = current
		}
	}()

	wg.Wait()
	log.Printf("done, lost connections: %d", atomic.LoadInt64(&s.lostConns))
}
