package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Two inputs can be processed in the same server tick. The second input
// must not hide that the weapon fired during this tick.
func TestFiredSurvivesSecondInputInSameTick(t *testing.T) {
	m := NewMap()
	game := NewGameState("test")
	p := NewPlayer(1, 10, 10, nil)
	players := []*Player{p}

	p.Fired = false
	p.Control.Shoot = true
	p.Update(players, game, 0.033)
	assert.True(t, p.Fired)
	assert.Len(t, p.Weapons[0].Projectiles, 1)

	p.Control.Shoot = true
	p.Update(players, game, 0.033)
	assert.True(t, p.Fired, "second input in the same tick reset the fired flag")
	assert.Len(t, p.Weapons[0].Projectiles, 1)

	// next tick
	p.Fired = false
	p.Control.Shoot = true
	p.Update(players, game, 0.033)
	assert.False(t, p.Fired, "weapon is not ready yet")
	_ = m
}
