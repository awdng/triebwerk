package loadtest

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func healthyBot(name string, id int) BotReport {
	return BotReport{
		Name:           name,
		PlayerIDs:      []int{id},
		RoundsStarted:  2,
		RoundsEnded:    1,
		StateMessages:  900,
		ActiveTime:     30 * time.Second,
		MaxGap:         60 * time.Millisecond,
		MaxPlayersSeen: 2,
		Scoreboards:    30,
	}
}

func TestCheckPassesAHealthyRun(t *testing.T) {
	report := Report{Bots: []BotReport{healthyBot("a", 1), healthyBot("b", 2)}}
	assert.Empty(t, report.Check(DefaultExpectations(2)))
}

func TestCheckReportsEveryProblem(t *testing.T) {
	broken := healthyBot("b", 2)
	broken.LostConnections = 1
	broken.MaxGap = time.Second
	broken.RoundsEnded = 0
	broken.StateMessages = 100
	broken.DuplicateIDs = 3
	unregistered := healthyBot("c", 0)
	unregistered.PlayerIDs = nil
	report := Report{Bots: []BotReport{healthyBot("a", 1), broken, unregistered}}

	errs := report.Check(DefaultExpectations(3))
	messages := make([]string, len(errs))
	for i, err := range errs {
		messages[i] = err.Error()
	}
	all := strings.Join(messages, "\n")
	assert.Contains(t, all, "b lost its connection")
	assert.Contains(t, all, "b waited 1s")
	assert.Contains(t, all, "b saw 0 round ends")
	assert.Contains(t, all, "b received 3.3 state broadcasts per second")
	assert.Contains(t, all, "b saw 3 duplicate player ids")
	assert.Contains(t, all, "c was never registered")
	assert.Contains(t, all, "never saw all 3 players in one match")
}

func TestCheckWantsScoreboards(t *testing.T) {
	silent := healthyBot("b", 2)
	silent.Scoreboards = 0
	garbled := healthyBot("c", 3)
	garbled.BadMessages = 2
	report := Report{Bots: []BotReport{healthyBot("a", 1), silent, garbled}}

	errs := report.Check(DefaultExpectations(0))
	messages := make([]string, len(errs))
	for i, err := range errs {
		messages[i] = err.Error()
	}
	all := strings.Join(messages, "\n")
	assert.Contains(t, all, "b never received a scoreboard")
	assert.Contains(t, all, "c received 2 messages it could not read")
	assert.Len(t, errs, 2)

	// a server from before the scoreboard is fine when nobody asks for one
	expectations := DefaultExpectations(0)
	expectations.Scoreboard = false
	assert.Len(t, report.Check(expectations), 1)
}

func TestParseScoreboard(t *testing.T) {
	body := []byte{0xe0, 0x93, 0x04, 0, 2, 3, 7, 0, 2, 0, 3, 'a', 'b', 'c', 12, 0, 0, 0, 0, 0}
	players, ok := parseScoreboard(body)
	assert.True(t, ok)
	assert.Equal(t, map[int]string{3: "abc", 12: ""}, players)

	_, ok = parseScoreboard(body[:len(body)-1])
	assert.False(t, ok, "cut off")
	_, ok = parseScoreboard(append(append([]byte{}, body...), 1))
	assert.False(t, ok, "bytes left over")
	_, ok = parseScoreboard([]byte{0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 1, 0xff})
	assert.False(t, ok, "name is not utf8")
	_, ok = parseScoreboard(nil)
	assert.False(t, ok, "empty")
}

func TestUpdateRate(t *testing.T) {
	assert.Equal(t, 0.0, BotReport{StateMessages: 10}.UpdateRate())
	assert.InDelta(t, 30.0, BotReport{StateMessages: 300, ActiveTime: 10 * time.Second}.UpdateRate(), 0.001)
}

func TestReportString(t *testing.T) {
	report := Report{Duration: 30 * time.Second, Bots: []BotReport{healthyBot("load-00", 4)}}
	out := report.String()
	assert.Contains(t, out, "load test ran 30s with 1 players")
	assert.Contains(t, out, "load-00")
}
