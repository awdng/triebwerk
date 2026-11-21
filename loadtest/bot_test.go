package loadtest

import (
	"testing"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
	"github.com/stretchr/testify/assert"
)

func testBot() *Bot {
	cfg := &Config{Logf: func(string, ...interface{}) {}}
	return newBot(1, cfg, &Stats{})
}

// what the server sends a bot, built with the server's own encoder
func snapshotFor(tick, ack uint32, waiting uint8, tanks ...*model.Player) []byte {
	p := protocol.NewBinaryProtocol()
	return p.Snapshot(tick, ack, waiting, p.SnapshotBody(tanks))
}

func tank(id int, health int, shots uint8) *model.Player {
	p := model.NewPlayer(id, 0, 0, nil)
	p.Health = health
	p.Shots = shots
	return p
}

func TestBotLearnsItsPlayerIDFromTheWelcome(t *testing.T) {
	b := testBot()
	b.handle(protocol.NewBinaryProtocol().Welcome(7, 100, time.Minute))
	id, _ := b.Keys()
	assert.Equal(t, 7, id)
	assert.Equal(t, []int{7}, b.Report().PlayerIDs)
	assert.Zero(t, b.Report().BadMessages)
}

func TestBotCountsWhatHappensToItsTank(t *testing.T) {
	p := protocol.NewBinaryProtocol()
	b := testBot()
	b.handle(p.Welcome(2, 100, time.Minute))
	b.handle(p.RoundStart(100))

	// shots are a counter that wraps, deaths are health reaching zero
	b.handle(snapshotFor(101, 0, protocol.WaitingStarved, tank(1, 100, 9), tank(2, 100, 254)))
	b.handle(snapshotFor(102, 1, 0, tank(1, 100, 10), tank(2, 75, 254)))
	b.handle(snapshotFor(103, 2, 1, tank(1, 100, 10), tank(2, 75, 255)))
	b.handle(snapshotFor(104, 2, protocol.WaitingStarved, tank(1, 100, 10), tank(2, 0, 0)))
	b.handle(snapshotFor(105, 3, 0, tank(1, 100, 10), tank(2, 0, 0)))
	b.handle(p.Kill(model.Kill{Killer: 1, Victim: 2}))
	b.handle(p.Scoreboard(&model.Scoreboard{Players: []model.ScoreboardEntry{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}}))
	b.handle(p.Pong(5))
	b.handle(p.RoundEnd(106))

	r := b.Report()
	assert.Equal(t, 5, r.StateMessages)
	assert.Equal(t, 2, r.Shots, "254 to 0 are two shots")
	assert.Equal(t, 1, r.Deaths)
	assert.Equal(t, 2, r.MaxPlayersSeen)
	assert.Equal(t, uint32(3), r.LastAck)
	assert.Equal(t, 4, r.Acknowledged, "the snapshot before the first input does not count")
	assert.Equal(t, 1, r.Starved)
	assert.Equal(t, 1, r.Kills)
	assert.Equal(t, 1, r.Scoreboards)
	assert.Equal(t, 1, r.Pongs)
	assert.Equal(t, 1, r.RoundsStarted)
	assert.Equal(t, 1, r.RoundsEnded)
	assert.Zero(t, r.BadMessages)
	assert.Zero(t, r.DuplicateIDs)
}

func TestBotCountsMessagesItCanNotRead(t *testing.T) {
	p := protocol.NewBinaryProtocol()
	good := snapshotFor(1, 1, 0, tank(1, 100, 0))
	scoreboard := p.Scoreboard(&model.Scoreboard{Players: []model.ScoreboardEntry{{ID: 1, Name: "a"}}})
	broken := [][]byte{
		{},
		good[:len(good)-1],
		append(append([]byte{}, good...), 0),
		scoreboard[:len(scoreboard)-1],
		{protocol.Kill, 1},
		{protocol.RoundStart, 1},
		{protocol.RoundEnd, 1, 2, 3, 4, 5},
		{protocol.Pong},
		{protocol.Welcome, protocol.Version, 1},
		{protocol.Reject, 9},
	}
	b := testBot()
	for _, data := range broken {
		b.handle(data)
	}
	assert.Equal(t, len(broken), b.Report().BadMessages)

	// a snapshot with the same tank twice can be read but is wrong
	b.handle(snapshotFor(1, 1, 0, tank(1, 100, 0), tank(1, 100, 0)))
	assert.Equal(t, 1, b.Report().DuplicateIDs)
}

// new kinds of messages must not break the bots of an older build
func TestBotIgnoresMessagesItDoesNotKnow(t *testing.T) {
	b := testBot()
	b.handle([]byte{200, 1, 2, 3})
	b.handle([]byte{15})
	assert.Zero(t, b.Report().BadMessages)
}

func TestBotTellsListenersAboutRounds(t *testing.T) {
	p := protocol.NewBinaryProtocol()
	var events []string
	var ticks []uint32
	var snapshots []protocol.SnapshotMessage
	cfg := &Config{
		Logf:    func(string, ...interface{}) {},
		OnEvent: func(event string, tick uint32) { events = append(events, event); ticks = append(ticks, tick) },
		OnState: func(snapshot protocol.SnapshotMessage) { snapshots = append(snapshots, snapshot) },
	}
	b := newBot(1, cfg, &Stats{})
	b.handle(p.RoundStart(70000))
	b.handle(snapshotFor(70001, 0, 0, tank(1, 100, 0)))
	b.handle(p.RoundEnd(70300))
	assert.Equal(t, []string{"start", "end"}, events)
	assert.Equal(t, []uint32{70000, 70300}, ticks)
	if assert.Len(t, snapshots, 1) {
		assert.Equal(t, uint32(70001), snapshots[0].Tick)
	}
}

// The end of a round travels on the reliable lane, snapshots on the other:
// the last snapshots of a round can arrive after its end.
func TestBotDoesNotTakeALateSnapshotForTheNextRound(t *testing.T) {
	p := protocol.NewBinaryProtocol()
	b := testBot()
	b.handle(p.Welcome(1, 100, time.Minute))
	b.handle(p.RoundStart(100))
	b.handle(snapshotFor(101, 1, 0, tank(1, 100, 0)))
	b.handle(snapshotFor(102, 2, 0, tank(1, 100, 0)))
	b.handle(p.RoundEnd(104))
	// overtaken by the end of the round
	b.handle(snapshotFor(103, 3, 0, tank(1, 100, 0)))
	b.handle(snapshotFor(104, 4, 0, tank(1, 100, 0)))

	// the pause between two rounds is not a stall
	time.Sleep(300 * time.Millisecond)
	b.handle(p.RoundStart(400))
	b.handle(snapshotFor(401, 5, 0, tank(1, 100, 0)))
	b.handle(snapshotFor(402, 6, 0, tank(1, 100, 0)))

	r := b.Report()
	assert.Less(t, r.MaxGap, 250*time.Millisecond)
	assert.Equal(t, 6, r.StateMessages, "they are still snapshots that arrived")
	assert.Equal(t, uint32(6), r.LastAck)
}

// bots are guests called load-NN unless they are given the tokens of
// accounts: then the master server knows them and their scores count
func TestBotSaysHelloWithItsTokenIfItWasGivenOne(t *testing.T) {
	cfg := &Config{FirstName: 12, Tokens: []string{"pz_first", "pz_second"}}
	assert.Equal(t, "pz_first", newBot(0, cfg, &Stats{}).hello())
	assert.Equal(t, "pz_second", newBot(1, cfg, &Stats{}).hello())
	third := newBot(2, cfg, &Stats{})
	assert.Equal(t, "load-14", third.hello(), "no token left: a guest by its name")
	assert.Equal(t, "load-14", third.Name)
	assert.Equal(t, "load-12", newBot(0, cfg, &Stats{}).Name, "the report never shows a token")
}
