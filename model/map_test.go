package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func playersOnSpawns(m *Map, count int) []*Player {
	players := []*Player{}
	for i := 0; i < count; i++ {
		players = append(players, NewPlayer(i, m.Spawns[i].X, m.Spawns[i].Y, nil))
	}
	return players
}

func TestGetRandomSpawnFindsFreeSpawn(t *testing.T) {
	m := NewMap()
	players := playersOnSpawns(m, len(m.Spawns)-1)
	free := m.Spawns[len(m.Spawns)-1]

	start := time.Now()
	for i := 0; i < 100; i++ {
		assert.Equal(t, free, m.GetRandomSpawn(players))
	}
	assert.True(t, time.Since(start) < 100*time.Millisecond)
}

func TestGetRandomSpawnIgnoresDeadPlayers(t *testing.T) {
	m := NewMap()
	players := playersOnSpawns(m, len(m.Spawns))
	players[3].Health = 0

	assert.Equal(t, m.Spawns[3], m.GetRandomSpawn(players))
}

func TestGetRandomSpawnAllOccupied(t *testing.T) {
	m := NewMap()
	players := playersOnSpawns(m, len(m.Spawns))

	assert.NotNil(t, m.GetRandomSpawn(players))
}
