package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlayerMovement(t *testing.T) {
	m := NewMap()
	player1 := NewPlayer(1, 10, 10, nil)

	assert.Equal(t, float32(10), player1.Collider.Pivot.X)
	assert.Equal(t, float32(10), player1.Collider.Pivot.Y)

	player1.Control.Forward = true
	player1.HandleMovement([]*Player{}, m, 1)

	assert.Equal(t, float32(10), player1.Collider.Pivot.X)
	assert.Equal(t, float32(25), player1.Collider.Pivot.Y)

	player1.Control.Left = true
	player1.HandleMovement([]*Player{}, m, 1)

	assert.Equal(t, float32(24.962425), player1.Collider.Pivot.X)
	assert.Equal(t, float32(26.061052), player1.Collider.Pivot.Y)
}
