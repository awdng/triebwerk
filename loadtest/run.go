package loadtest

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awdng/triebwerk/protocol"
)

// Config of a load test run
type Config struct {
	// websocket url of the server, e.g. ws://localhost:9090/echo
	URL     string
	Players int
	// how long every player stays, 0 runs until the context is cancelled
	Duration time.Duration
	// players that leave once after ReconnectAfter and rejoin 5 seconds later
	Reconnect      int
	ReconnectAfter time.Duration
	// players join at a random time within this window, default 3s
	Stagger time.Duration

	// optional hooks, called from the bot goroutines: every snapshot a bot
	// receives, and "start" and "end" of rounds with the tick they happened at
	OnState func(snapshot protocol.SnapshotMessage)
	OnEvent func(event string, tick uint32)
	// progress output, default log.Printf, set to a no-op to silence
	Logf func(format string, args ...interface{})
	// how often progress is logged, 0 disables it
	LogInterval time.Duration
}

func (c *Config) logf(format string, args ...interface{}) {
	if c.Logf != nil {
		c.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// Runner holds the bots of one run so they can be observed while it runs
type Runner struct {
	Config Config
	Stats  *Stats
	Bots   []*Bot
}

// NewRunner creates the bots, nothing connects before Run
func NewRunner(cfg Config) *Runner {
	if cfg.Stagger == 0 {
		cfg.Stagger = 3 * time.Second
	}
	r := &Runner{Config: cfg, Stats: &Stats{}}
	for i := 0; i < cfg.Players; i++ {
		r.Bots = append(r.Bots, newBot(i, &r.Config, r.Stats))
	}
	return r
}

// Run plays until the duration is over or the context is cancelled and
// returns what every player has seen
func (r *Runner) Run(ctx context.Context) Report {
	cfg := &r.Config
	start := time.Now()
	end := start.Add(cfg.Duration)
	if cfg.Duration == 0 {
		end = start.Add(100 * 365 * 24 * time.Hour)
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(end) {
		end = deadline
	}
	ctx, cancel := context.WithDeadline(ctx, end)
	defer cancel()

	var wg sync.WaitGroup
	for i, b := range r.Bots {
		wg.Add(1)
		go func(i int, b *Bot) {
			defer wg.Done()
			if !sleep(ctx, time.Duration(rand.Int63n(int64(cfg.Stagger)))) {
				return
			}
			if i < cfg.Reconnect {
				after := cfg.ReconnectAfter
				if after == 0 {
					after = time.Duration(60+rand.Intn(120)) * time.Second
				}
				leave := time.Now().Add(after)
				if leave.Before(end) {
					b.play(leave)
					cfg.logf("%s: left voluntarily, rejoining in 5s", b.Name)
					if !sleep(ctx, 5*time.Second) {
						return
					}
				}
			}
			b.play(end)
		}(i, b)
	}

	if cfg.LogInterval > 0 {
		go r.logProgress(ctx)
	}
	wg.Wait()

	report := Report{Duration: time.Since(start)}
	for _, b := range r.Bots {
		report.Bots = append(report.Bots, b.Report())
	}
	return report
}

func (r *Runner) logProgress(ctx context.Context) {
	ticker := time.NewTicker(r.Config.LogInterval)
	defer ticker.Stop()
	var previous int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		current := atomic.LoadInt64(&r.Stats.StateMessages)
		r.Config.logf("connected: %d | state msgs/s: %.0f | max gap between updates: %dms | lost connections: %d",
			atomic.LoadInt64(&r.Stats.Connected), float64(current-previous)/r.Config.LogInterval.Seconds(),
			r.Stats.TakeMaxGap()/time.Millisecond, atomic.LoadInt64(&r.Stats.LostConnections))
		previous = current
	}
}

// sleep waits for d, returns false if the context ended first
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
