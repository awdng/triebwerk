package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// player ids are sent as a single byte, they must stay below 256 and must
// not collide with connected players
func TestNewPlayerIDStaysInByteRangeAndUnique(t *testing.T) {
	g := NewGameState("test")
	p1 := NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	g.AddPlayer(p1)
	p2 := NewPlayer(g.GetNewPlayerID(), 0, 0, nil)
	g.AddPlayer(p2)
	assert.Equal(t, 1, p1.ID)
	assert.Equal(t, 2, p2.ID)

	seen := map[int]bool{}
	for i := 0; i < 1000; i++ {
		id := g.GetNewPlayerID()
		assert.True(t, id >= 1 && id <= 255, "id %d out of byte range", id)
		assert.NotEqual(t, p1.ID, id)
		assert.NotEqual(t, p2.ID, id)
		seen[id] = true
	}
	assert.Equal(t, 253, len(seen), "all free ids should be used before repeating")
}
