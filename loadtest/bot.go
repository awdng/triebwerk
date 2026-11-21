// Package loadtest simulates players that connect to a triebwerk server over
// a websocket or WebTransport, drive around with random human like inputs,
// shoot and ping. It records what every player receives so a run can be checked
// against expectations (see Report.Check). Used by cmd/loadtest and by the
// load test in the game package.
package loadtest

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
)

// the bits of the keys in an input, in the order of Bot.keys
var keyBits = []byte{
	protocol.BitForward, protocol.BitBackward, protocol.BitLeft, protocol.BitRight,
	protocol.BitTurretRight, protocol.BitTurretLeft, protocol.BitShoot,
}

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
	// shot counter of the own tank in the last snapshot, -1 before the first
	lastShots int
	// tick the last round ended at, as the server said
	roundEnd   uint32
	roundEnded bool
}

func newBot(id int, cfg *Config, stats *Stats) *Bot {
	name := fmt.Sprintf("load-%02d", cfg.FirstName+id)
	return &Bot{
		Name:      name,
		url:       cfg.URL,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano() + int64(id))),
		stats:     stats,
		cfg:       cfg,
		keys:      make([]byte, 7),
		report:    BotReport{Name: name},
		lastShots: -1,
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
	c, err := dial(b.url)
	if err != nil {
		b.cfg.logf("%s: dial failed: %v", b.Name, err)
		b.lost()
		return
	}
	defer c.Close()
	atomic.AddInt64(&b.stats.Connected, 1)
	defer atomic.AddInt64(&b.stats.Connected, -1)

	// the name is the auth token, the fake master server uses it as nickname
	c.Send(protocol.EncodeHello(b.Name))

	done := make(chan struct{})
	defer close(done)
	go b.sendInputs(c.Send, until, done)

	for time.Now().Before(until) {
		data, err := c.Read(until)
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
	if len(data) == 0 {
		b.bad()
		return
	}
	switch data[0] {
	case protocol.Welcome:
		if len(data) != 12 || data[1] != protocol.Version || data[3] != model.TickMillis {
			b.bad()
			return
		}
		b.mutex.Lock()
		b.playerID = int(data[2])
		b.report.PlayerIDs = append(b.report.PlayerIDs, b.playerID)
		b.lastShots = -1
		b.mutex.Unlock()
	case protocol.Snapshot:
		snapshot, ok := protocol.DecodeSnapshot(data)
		if !ok {
			b.bad()
			return
		}
		atomic.AddInt64(&b.stats.StateMessages, 1)
		b.recordState(snapshot)
		if b.cfg.OnState != nil {
			b.cfg.OnState(snapshot)
		}
	case protocol.RoundStart, protocol.RoundEnd:
		if len(data) != 5 {
			b.bad()
			return
		}
		tick := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16 | uint32(data[4])<<24
		b.mutex.Lock()
		event := "start"
		if data[0] == protocol.RoundStart {
			b.report.RoundsStarted++
		} else {
			event = "end"
			b.report.RoundsEnded++
			b.endSegment()
			b.roundEnd, b.roundEnded = tick, true
		}
		b.mutex.Unlock()
		if b.cfg.OnEvent != nil {
			b.cfg.OnEvent(event, tick)
		}
	case protocol.Scoreboard:
		if _, ok := protocol.DecodeScoreboard(data); !ok {
			b.bad()
			return
		}
		b.mutex.Lock()
		b.report.Scoreboards++
		b.mutex.Unlock()
	case protocol.Kill:
		if len(data) != 3 {
			b.bad()
			return
		}
		b.mutex.Lock()
		b.report.Kills++
		b.mutex.Unlock()
	case protocol.Pong:
		if len(data) != 5 {
			b.bad()
			return
		}
		b.mutex.Lock()
		b.report.Pongs++
		b.mutex.Unlock()
	case protocol.Reject:
		b.cfg.logf("%s: the server speaks another version of the protocol", b.Name)
		b.bad()
	}
	// types this bot does not know are ignored: new messages must not break old clients
}

// bad counts a message that did not have the agreed layout
func (b *Bot) bad() {
	b.mutex.Lock()
	b.report.BadMessages++
	b.mutex.Unlock()
}

// recordState checks one snapshot: timing, players, own health and shots,
// and how the server takes this bot's inputs
func (b *Bot) recordState(snapshot protocol.SnapshotMessage) {
	now := time.Now()
	b.mutex.Lock()
	defer b.mutex.Unlock()

	r := &b.report
	r.StateMessages++
	// a snapshot of a round that is over, overtaken by the news of its end:
	// it says nothing about the pause that follows
	late := b.roundEnded && snapshot.Tick <= b.roundEnd
	if !late {
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
	}

	// the server applies one input per tick: once it has the first, a tick
	// without one means inputs arrive later than they are needed
	if snapshot.Ack > 0 {
		r.Acknowledged++
		if snapshot.Waiting == protocol.WaitingStarved {
			r.Starved++
		}
	}
	if snapshot.Ack > r.LastAck {
		r.LastAck = snapshot.Ack
	}

	if len(snapshot.Players) > r.MaxPlayersSeen {
		r.MaxPlayersSeen = len(snapshot.Players)
	}
	seen := make(map[int]bool, len(snapshot.Players))
	for _, e := range snapshot.Players {
		if seen[e.ID] {
			r.DuplicateIDs++
		}
		seen[e.ID] = true
		if e.ID != b.playerID || b.playerID == 0 {
			continue
		}
		if b.report.lastHealth > 0 && e.Health == 0 {
			r.Deaths++
		}
		b.report.lastHealth = e.Health
		// shots are a counter that wraps
		if b.lastShots >= 0 {
			r.Shots += int(e.Shots - uint8(b.lastShots))
		}
		b.lastShots = int(e.Shots)
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

// sendInputs sends the pressed keys once per server tick. Every message
// carries the two inputs before it as well, like the browser client does.
func (b *Bot) sendInputs(write func([]byte) error, until time.Time, done chan struct{}) {
	var sequence uint32
	var recent []byte // keys of the last inputs, newest first
	nextChange := time.Now()
	lastPing := time.Now()
	ticker := time.NewTicker(model.TickMillis * time.Millisecond)
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
		var bits byte
		for i, pressed := range keys {
			if pressed == 1 {
				bits |= keyBits[i]
			}
		}
		sequence++
		recent = append([]byte{bits}, recent...)
		if len(recent) > protocol.MaxInputsPerMessage {
			recent = recent[:protocol.MaxInputsPerMessage]
		}
		if write(protocol.EncodeInput(sequence, recent)) != nil {
			return
		}
		b.mutex.Lock()
		b.report.InputsSent++
		b.mutex.Unlock()
		if time.Since(lastPing) > time.Second {
			lastPing = time.Now()
			write(protocol.EncodePing(uint32(time.Now().UnixNano() / 1e6)))
		}
	}
}
