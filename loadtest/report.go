package loadtest

import (
	"fmt"
	"strings"
	"time"
)

// BotReport is what one simulated player saw during a run
type BotReport struct {
	Name string
	// player ids the server assigned, one per connection
	PlayerIDs       []int
	LostConnections int
	RoundsStarted   int
	RoundsEnded     int
	StateMessages   int
	// time between the first and the last state broadcast of every round,
	// summed up, used to compute the update rate
	ActiveTime time.Duration
	// longest gap between two state broadcasts within a round
	MaxGap time.Duration
	// most player entries in one state broadcast
	MaxPlayersSeen int
	// player ids that appeared twice in one broadcast
	DuplicateIDs int
	// own deaths and shots as reported by the server
	Deaths int
	Shots  int
	// scoreboards and kill events the server sent, and messages of those
	// kinds that did not have the agreed layout
	Scoreboards int
	Kills       int
	BadMessages int

	lastHealth int
}

// UpdateRate is the number of state broadcasts per second during rounds
func (r BotReport) UpdateRate() float64 {
	if r.ActiveTime <= 0 {
		return 0
	}
	return float64(r.StateMessages) / r.ActiveTime.Seconds()
}

// Report of a whole run
type Report struct {
	Duration time.Duration
	Bots     []BotReport
}

// Expectations a healthy server has to meet, zero values are not checked
type Expectations struct {
	// Scoreboard: every player has to receive names and scores
	Scoreboard bool
	// all of them in one match at the same time
	Players int
	// rounds every player has seen end
	MinRounds int
	// longest allowed pause between two state broadcasts within a round
	MaxGap time.Duration
	// state broadcasts per second every player has to receive, the server ticks at 30
	MinUpdateRate float64
}

// DefaultExpectations for a run with the given number of players
func DefaultExpectations(players int) Expectations {
	return Expectations{
		Players:       players,
		Scoreboard:    true,
		MinRounds:     1,
		MaxGap:        250 * time.Millisecond,
		MinUpdateRate: 25,
	}
}

// Check returns every violated expectation, nil if the run was healthy
func (r Report) Check(e Expectations) []error {
	var errs []error
	fail := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Errorf(format, args...))
	}
	if e.Players > 0 && len(r.Bots) < e.Players {
		fail("only %d of %d players took part", len(r.Bots), e.Players)
	}
	maxPlayers := 0
	for _, b := range r.Bots {
		if len(b.PlayerIDs) == 0 {
			fail("%s was never registered by the server", b.Name)
		}
		if b.LostConnections > 0 {
			fail("%s lost its connection %d times", b.Name, b.LostConnections)
		}
		if b.DuplicateIDs > 0 {
			fail("%s saw %d duplicate player ids in state broadcasts", b.Name, b.DuplicateIDs)
		}
		if e.MinRounds > 0 && b.RoundsEnded < e.MinRounds {
			fail("%s saw %d round ends, expected at least %d", b.Name, b.RoundsEnded, e.MinRounds)
		}
		if e.MaxGap > 0 && b.MaxGap > e.MaxGap {
			fail("%s waited %v for a state broadcast, allowed %v", b.Name, b.MaxGap.Round(time.Millisecond), e.MaxGap)
		}
		if e.MinUpdateRate > 0 && b.UpdateRate() < e.MinUpdateRate {
			fail("%s received %.1f state broadcasts per second, expected at least %.0f", b.Name, b.UpdateRate(), e.MinUpdateRate)
		}
		if e.Scoreboard && b.Scoreboards == 0 {
			fail("%s never received a scoreboard", b.Name)
		}
		if b.BadMessages > 0 {
			fail("%s received %d messages it could not read", b.Name, b.BadMessages)
		}
		if b.MaxPlayersSeen > maxPlayers {
			maxPlayers = b.MaxPlayersSeen
		}
	}
	if e.Players > 0 && maxPlayers < e.Players {
		fail("never saw all %d players in one match, at most %d", e.Players, maxPlayers)
	}
	return errs
}

// String is a table of the run, one row per player
func (r Report) String() string {
	var s strings.Builder
	fmt.Fprintf(&s, "load test ran %v with %d players\n", r.Duration.Round(time.Second), len(r.Bots))
	fmt.Fprintf(&s, "%-8s %6s %5s %5s %7s %8s %8s %6s %5s %6s %4s\n",
		"player", "ids", "start", "end", "states", "rate/s", "max gap", "seen", "shots", "deaths", "lost")
	for _, b := range r.Bots {
		fmt.Fprintf(&s, "%-8s %6s %5d %5d %7d %8.1f %8s %6d %5d %6d %4d\n",
			b.Name, joinInts(b.PlayerIDs), b.RoundsStarted, b.RoundsEnded, b.StateMessages, b.UpdateRate(),
			b.MaxGap.Round(time.Millisecond), b.MaxPlayersSeen, b.Shots, b.Deaths, b.LostConnections)
	}
	return s.String()
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ",")
}
