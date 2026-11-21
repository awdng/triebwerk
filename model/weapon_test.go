package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// puts a shell of the shooter right on the target and lets the weapon notice
func hit(shooter *Player, target *Player, players []*Player, m *Map) {
	w := shooter.Weapons[0]
	w.Projectiles = append(w.Projectiles, &Projectile{
		Position:  &Point{X: target.Collider.Pivot.X, Y: target.Collider.Pivot.Y},
		Direction: &Point{X: 0, Y: 0},
	})
	w.Update(players, m, 0.033)
}

// The scoreboard and the kill feed of the clients are fed from this.
func TestLethalHitIsRecorded(t *testing.T) {
	m := NewMap()
	shooter := NewPlayer(1, 10, 10, nil)
	target := NewPlayer(2, 40, 40, nil)
	players := []*Player{shooter, target}

	// three hits wound
	for i := 0; i < 3; i++ {
		hit(shooter, target, players, m)
	}
	assert.Equal(t, 25, target.Health)
	assert.Equal(t, 0, shooter.Score)
	assert.Equal(t, 0, target.Deaths)
	assert.Empty(t, shooter.KillLog)

	// the fourth one kills
	hit(shooter, target, players, m)
	assert.Equal(t, 0, target.Health)
	assert.Equal(t, 1, shooter.Score)
	assert.Equal(t, 1, target.Deaths)
	assert.Equal(t, []int{2}, shooter.KillLog)

	// a wreck can not be killed again
	hit(shooter, target, players, m)
	assert.Equal(t, 1, shooter.Score)
	assert.Equal(t, 1, target.Deaths)
	assert.Equal(t, []int{2}, shooter.KillLog)
}

func TestTakeKillsEmptiesTheLog(t *testing.T) {
	p := NewPlayer(1, 10, 10, nil)
	assert.Empty(t, p.TakeKills())
	p.KillLog = []int{4, 7}
	assert.Equal(t, []int{4, 7}, p.TakeKills())
	assert.Empty(t, p.TakeKills())
}

func TestStartOfARoundResetsScores(t *testing.T) {
	game := NewGameState("test", 5*time.Minute)
	p := NewPlayer(1, 10, 10, nil)
	p.Score = 5
	p.Deaths = 3
	p.KillLog = []int{2}
	game.AddPlayer(p)
	game.Start()
	assert.Equal(t, 0, p.Score)
	assert.Equal(t, 0, p.Deaths)
	assert.Empty(t, p.KillLog)
}

func TestScoreboardListsEveryPlayer(t *testing.T) {
	game := NewGameState("test", 90*time.Second)
	a := NewPlayer(1, 10, 10, nil)
	a.Nickname = "alpha"
	a.Score = 3
	a.Deaths = 1
	b := NewPlayer(2, 40, 40, nil)
	game.AddPlayer(a)
	game.AddPlayer(b)

	board := game.Scoreboard()
	assert.Equal(t, 90*time.Second, board.RoundLength)
	assert.ElementsMatch(t, []ScoreboardEntry{
		{ID: 1, Kills: 3, Deaths: 1, Name: "alpha"},
		{ID: 2, Kills: 0, Deaths: 0, Name: ""},
	}, board.Players)
}
