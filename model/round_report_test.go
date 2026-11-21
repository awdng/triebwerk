package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// what the master server is told about a round: who took part, with which
// score, for how long

func joined(g *GameState, name string) *Player {
	p := NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	p.Nickname = name
	p.GlobalID = "id-" + name
	g.AddPlayer(p)
	return p
}

// seconds lets a little more than count seconds pass
func seconds(g *GameState, count int) {
	for i := 0; i < count*1000/TickMillis+1; i++ {
		g.Tick()
	}
}

func TestServerWithoutARoundReportsWhoIsThereAndNoRound(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	s := g.Snapshot()
	assert.Equal(t, 1, s.Online)
	assert.Equal(t, uint32(0), s.Round)
	assert.False(t, s.InProgress)
	assert.Empty(t, s.Players)
	assert.Equal(t, time.Minute, s.Length)
	assert.Equal(t, "EU", s.Region)
}

func TestRoundReportListsEverybodyWithScoreAndTimePlayed(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	anna, ben := joined(g, "anna"), joined(g, "ben")
	g.Start()
	seconds(g, 10)
	anna.Score, anna.Deaths = 3, 1
	ben.Score, ben.Deaths = 1, 3

	s := g.Snapshot()
	assert.Equal(t, uint32(1), s.Round)
	assert.True(t, s.InProgress)
	assert.Equal(t, 2, s.Online)
	assert.InDelta(t, 10000, int(s.GameTime), 1000)
	require.Len(t, s.Players, 2)
	assert.Equal(t, PlayerSnapshot{Slot: 1, GlobalID: "id-anna", Nickname: "anna", Score: 3, Deaths: 1, SecondsPlayed: 10, Connected: true}, s.Players[0])
	assert.Equal(t, PlayerSnapshot{Slot: 2, GlobalID: "id-ben", Nickname: "ben", Score: 1, Deaths: 3, SecondsPlayed: 10, Connected: true}, s.Players[1])
}

// who leaves keeps what they did: their kills count for the scoreboard of
// the world, and their time ends when they went
func TestPlayerWhoLeavesStaysInTheRoundReport(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	anna, ben := joined(g, "anna"), joined(g, "ben")
	g.Start()
	seconds(g, 5)
	ben.Score = 4
	g.RemovePlayer(ben)
	seconds(g, 5)
	anna.Score = 1

	s := g.Snapshot()
	assert.Equal(t, 1, s.Online)
	require.Len(t, s.Players, 2)
	assert.Equal(t, PlayerSnapshot{Slot: 1, GlobalID: "id-anna", Nickname: "anna", Score: 1, SecondsPlayed: 10, Connected: true}, s.Players[0])
	assert.Equal(t, PlayerSnapshot{Slot: 2, GlobalID: "id-ben", Nickname: "ben", Score: 4, SecondsPlayed: 5, Connected: false}, s.Players[1])
}

func TestPlayerWhoJoinsARunningRoundHasPlayedLess(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	g.Start()
	seconds(g, 20)
	joined(g, "ben")
	seconds(g, 5)

	s := g.Snapshot()
	require.Len(t, s.Players, 2)
	assert.Equal(t, 25, s.Players[0].SecondsPlayed)
	assert.Equal(t, "ben", s.Players[1].Nickname)
	assert.Equal(t, 2, s.Players[1].Slot)
	assert.Equal(t, 5, s.Players[1].SecondsPlayed)
}

// somebody who comes back gets a new place in the report: the two visits
// are two entries, none overwrites the other
func TestPlayerWhoComesBackIsListedTwice(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	ben := joined(g, "ben")
	g.Start()
	ben.Score = 2
	g.RemovePlayer(ben)
	again := joined(g, "ben")
	again.Score = 1

	s := g.Snapshot()
	require.Len(t, s.Players, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{s.Players[0].Slot, s.Players[1].Slot, s.Players[2].Slot})
	assert.Equal(t, 2, s.Players[1].Score)
	assert.False(t, s.Players[1].Connected)
	assert.Equal(t, 1, s.Players[2].Score)
	assert.True(t, s.Players[2].Connected)
}

func TestEndedRoundKeepsItsNumbersUntilTheNextOneStarts(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	anna := joined(g, "anna")
	g.Start()
	seconds(g, 30)
	anna.Score = 5
	g.End()
	seconds(g, 8) // the pause between two rounds
	late := joined(g, "late")

	s := g.Snapshot()
	assert.Equal(t, uint32(1), s.Round)
	assert.False(t, s.InProgress)
	assert.InDelta(t, 30000, int(s.GameTime), 1000, "the clock of a round stops with it")
	assert.Equal(t, 2, s.Online, "who joins between two rounds is on the server")
	require.Len(t, s.Players, 1, "but took no part in the round that is over")
	assert.Equal(t, 5, s.Players[0].Score)
	assert.Equal(t, 30, s.Players[0].SecondsPlayed)

	g.Start()
	seconds(g, 2)
	s = g.Snapshot()
	assert.Equal(t, uint32(2), s.Round)
	require.Len(t, s.Players, 2)
	assert.Equal(t, 0, s.Players[0].Score)
	assert.Equal(t, 2, s.Players[0].SecondsPlayed)
	assert.Equal(t, "late", s.Players[1].Nickname)
	_ = late
}

func TestNextRoundForgetsWhoLeftTheLastOne(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	ben := joined(g, "ben")
	g.Start()
	g.RemovePlayer(ben)
	g.End()
	g.Start()
	s := g.Snapshot()
	require.Len(t, s.Players, 1)
	assert.Equal(t, "anna", s.Players[0].Nickname)
	assert.Equal(t, 1, s.Players[0].Slot)
}

// a connection is a player before the master server has said who it is; if
// the master says no, nobody took part
func TestPlayerNobodyVouchedForIsNotInTheReport(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	g.Start()
	unknown := NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	g.AddPlayer(unknown)
	assert.Len(t, g.Snapshot().Players, 1)
	g.RemovePlayer(unknown)
	assert.Len(t, g.Snapshot().Players, 1)

	// once it has a name it is listed, from the moment it connected
	known := NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	g.AddPlayer(known)
	seconds(g, 3)
	known.Nickname = "cem"
	s := g.Snapshot()
	require.Len(t, s.Players, 2)
	assert.Equal(t, 3, s.Players[1].SecondsPlayed)
}

// connections that come and go all round long must not make the report
// grow without end; who scored is kept longest
func TestReportOfARoundWithEndlessVisitorsStaysSmall(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	g.Start()
	scorer := joined(g, "scorer")
	scorer.Score = 7
	g.RemovePlayer(scorer)
	for i := 0; i < 1000; i++ {
		g.RemovePlayer(joined(g, "visitor"))
	}
	s := g.Snapshot()
	assert.LessOrEqual(t, len(s.Players), MaxRoundReport)
	slots := map[int]bool{}
	scorerKept := false
	for _, p := range s.Players {
		assert.False(t, slots[p.Slot], "slot %d twice", p.Slot)
		slots[p.Slot] = true
		scorerKept = scorerKept || p.Nickname == "scorer"
	}
	assert.True(t, scorerKept)
	assert.Equal(t, "anna", s.Players[0].Nickname)
}

// with twelve players on a server there is room for many who left
func TestReportKeepsThoseWhoLeftUpToItsSize(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	g.Start()
	for i := 0; i < 100; i++ {
		leaver := joined(g, "leaver")
		leaver.Score = 1
		g.RemovePlayer(leaver)
	}
	assert.Len(t, g.Snapshot().Players, 101, "nobody who scored is forgotten while there is room")
}

// the master server keeps an account to one match: it is told who is here
func TestSnapshotListsTheAccountsThatAreOnTheServer(t *testing.T) {
	g := NewGameState("EU", time.Minute)
	joined(g, "anna")
	guest := joined(g, "visitor")
	guest.GlobalID = ""
	g.AddPlayer(NewPlayer(g.GetNewPlayerID(), 0, 0, nil)) // nobody has said who this is
	assert.Equal(t, []string{"id-anna"}, g.Snapshot().Accounts, "between rounds too")

	g.Start()
	ben := joined(g, "ben")
	assert.Equal(t, []string{"id-anna", "id-ben"}, g.Snapshot().Accounts)
	g.RemovePlayer(ben)
	assert.Equal(t, []string{"id-anna"}, g.Snapshot().Accounts, "who left is not here, though still in the round's report")
}
