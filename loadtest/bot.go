// Package loadtest simulates players that connect to a triebwerk server over
// websockets, drive around with random human like inputs, shoot and sync
// their clock. It records what every player receives so a run can be checked
// against expectations (see Report.Check). Used by cmd/loadtest and by the
// load test in the game package.
package loadtest

import (
	"encoding/binary"
	"fmt"
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

// Stats are live counters shared by all bots of a run
type Stats struct {
	Connected       int64
	StateMessages   int64
	LostConnections int64
	// longest gap between two state broadcasts received by any bot since the
	// last call of TakeMaxGap, in milliseconds
	maxGap int64
}

// TakeMaxGap returns the longest gap since the last call and resets it
func (s *Stats) TakeMaxGap() time.Duration {
	return time.Duration(atomic.SwapInt64(&s.maxGap, 0)) * time.Millisecond
}

func (s *Stats) recordGap(gap time.Duration) {
	v := int64(gap / time.Millisecond)
	for {
		old := atomic.LoadInt64(&s.maxGap)
		if v <= old || atomic.CompareAndSwapInt64(&s.maxGap, old, v) {
			return
		}
	}
}

// Bot is one simulated player
type Bot struct {
	Name string

	url   string
	rng   *rand.Rand
	stats *Stats
	cfg   *Config

	mutex    sync.Mutex
	playerID int
	keys     []byte // forward, backward, left, right, turret right, turret left, shoot
	report   BotReport

	// current stretch of continuous state broadcasts inside a round
	segmentStart time.Time
	segmentLast  time.Time
}

func newBot(id int, cfg *Config, stats *Stats) *Bot {
	name := fmt.Sprintf("load-%02d", id)
	return &Bot{
		Name:   name,
		url:    cfg.URL,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano() + int64(id))),
		stats:  stats,
		cfg:    cfg,
		keys:   make([]byte, 7),
		report: BotReport{Name: name},
	}
}

// Keys returns the player id on the server and the currently pressed keys
func (b *Bot) Keys() (int, []byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.playerID, append([]byte{}, b.keys...)
}

// Report returns a copy of what the bot has seen so far
func (b *Bot) Report() BotReport {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	r := b.report
	r.PlayerIDs = append([]int{}, b.report.PlayerIDs...)
	return r
}

// pressRandomKeys picks the next key combination, returns how long it is held
func (b *Bot) pressRandomKeys() time.Duration {
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
func (b *Bot) play(until time.Time) {
	c, _, err := websocket.DefaultDialer.Dial(b.url, nil)
	if err != nil {
		b.cfg.logf("%s: dial failed: %v", b.Name, err)
		b.lost()
		return
	}
	defer c.Close()
	atomic.AddInt64(&b.stats.Connected, 1)
	defer atomic.AddInt64(&b.stats.Connected, -1)

	var writeMutex sync.Mutex
	write := func(data []byte) error {
		writeMutex.Lock()
		defer writeMutex.Unlock()
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		return c.WriteMessage(websocket.BinaryMessage, data)
	}
	// the name is the auth token, the fake master server uses it as nickname
	write(append([]byte{0, 0}, []byte(b.Name)...))

	done := make(chan struct{})
	defer close(done)
	go b.sendInputs(write, until, done)

	for time.Now().Before(until) {
		c.SetReadDeadline(until)
		_, data, err := c.ReadMessage()
		if err != nil {
			if time.Now().Before(until) {
				b.cfg.logf("%s: connection lost: %v", b.Name, err)
				b.lost()
			}
			break
		}
		b.handle(data)
	}

	b.mutex.Lock()
	b.endSegment()
	b.playerID = 0
	b.mutex.Unlock()
}

func (b *Bot) lost() {
	atomic.AddInt64(&b.stats.LostConnections, 1)
	b.mutex.Lock()
	b.report.LostConnections++
	b.mutex.Unlock()
}

func (b *Bot) handle(data []byte) {
	if len(data) < 6 {
		return
	}
	gameTime := binary.LittleEndian.Uint32(data[2:])
	switch data[1] {
	case msgRegister:
		b.mutex.Lock()
		b.playerID = int(data[0])
		b.report.PlayerIDs = append(b.report.PlayerIDs, b.playerID)
		b.mutex.Unlock()
	case msgState:
		atomic.AddInt64(&b.stats.StateMessages, 1)
		b.recordState(data)
		if b.cfg.OnState != nil {
			b.cfg.OnState(data)
		}
	case msgGameStart:
		b.mutex.Lock()
		b.report.RoundsStarted++
		b.mutex.Unlock()
		if b.cfg.OnEvent != nil {
			b.cfg.OnEvent("start", gameTime)
		}
	case msgGameEnd:
		b.mutex.Lock()
		b.report.RoundsEnded++
		b.endSegment()
		b.mutex.Unlock()
		if b.cfg.OnEvent != nil {
			b.cfg.OnEvent("end", gameTime)
		}
	}
}

// recordState checks one state broadcast: timing, players, own health and shots
func (b *Bot) recordState(data []byte) {
	now := time.Now()
	b.mutex.Lock()
	defer b.mutex.Unlock()

	r := &b.report
	r.StateMessages++
	if !b.segmentLast.IsZero() {
		gap := now.Sub(b.segmentLast)
		if gap > r.MaxGap {
			r.MaxGap = gap
		}
		b.stats.recordGap(gap)
	} else {
		b.segmentStart = now
	}
	b.segmentLast = now

	entries := len(data) / entrySize
	if entries > r.MaxPlayersSeen {
		r.MaxPlayersSeen = entries
	}
	seen := make(map[byte]bool, entries)
	for i := 0; i+entrySize <= len(data); i += entrySize {
		e := data[i : i+entrySize]
		if seen[e[0]] {
			r.DuplicateIDs++
		}
		seen[e[0]] = true
		if int(e[0]) != b.playerID || b.playerID == 0 {
			continue
		}
		health := int(e[35])
		if b.report.lastHealth > 0 && health == 0 {
			r.Deaths++
		}
		b.report.lastHealth = health
		if e[34] == 1 {
			r.Shots++
		}
	}
}

// endSegment closes the current stretch of state broadcasts, the gap to the
// next round or connection does not count as a stall. Needs the mutex.
func (b *Bot) endSegment() {
	if !b.segmentLast.IsZero() {
		b.report.ActiveTime += b.segmentLast.Sub(b.segmentStart)
	}
	b.segmentStart = time.Time{}
	b.segmentLast = time.Time{}
}

// sendInputs sends the pressed keys at 30Hz like a render loop would
func (b *Bot) sendInputs(write func([]byte) error, until time.Time, done chan struct{}) {
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
