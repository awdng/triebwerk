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
