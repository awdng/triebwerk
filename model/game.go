package model

import (
	"sync"
	"sync/atomic"
	"time"
)

const gameLength = 5

// GameState ...
type GameState struct {
	Region      string
	startTime   time.Time
	length      time.Duration
	inProgress  bool
	playerID    int64
	playerCount int
	players     map[int]*Player
	Map         *Map
	mutex       *sync.RWMutex
}

// NewGameState ...
func NewGameState(region string) *GameState {
	return &GameState{
		Region:     region,
		inProgress: false,
		length:     time.Minute * gameLength,
		players:    make(map[int]*Player),
		Map:        NewMap(),
		mutex:      &sync.RWMutex{},
	}
}

// ReadyToStart ...
func (g *GameState) ReadyToStart() bool {
	return g.GetPlayerCount() >= 1 && !g.InProgress()
}

// Start ...
func (g *GameState) Start() {
	players := g.GetPlayers()
	// randomize player spawns
	for _, p := range players {
		p.Health = 100
		p.Score = 0
		spawn := g.Map.GetRandomSpawn(players)
		p.Collider.ChangePosition(spawn.X, spawn.Y)
	}
	g.startTime = time.Now()
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.inProgress = true
}

// End ...
func (g *GameState) End() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.inProgress = false
}

// InProgress ...
func (g *GameState) InProgress() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.inProgress
}

// HasEnded ...
func (g *GameState) HasEnded() bool {
	return time.Now().Sub(g.startTime) >= g.length
}

// GameTime returns the current game time since start in milliseconds
func (g *GameState) GameTime() uint32 {
	return uint32(time.Now().Sub(g.startTime) / time.Millisecond)
}

// GetPlayers returns the PlayerList
func (g *GameState) GetPlayers() []*Player {
	players := make([]*Player, 0)
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	for _, p := range g.players {
		players = append(players, p)
	}
	return players
}

// GetPlayerCount ...
func (g *GameState) GetPlayerCount() int {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.playerCount
}

// GetNewPlayerID ...
func (g *GameState) GetNewPlayerID() int {
	return int(atomic.AddInt64(&g.playerID, 1))
}

// AddPlayer to the game
func (g *GameState) AddPlayer(player *Player) int {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.players[player.ID] = player
	g.playerCount++
	return g.playerCount
}

// RemovePlayer from the game
func (g *GameState) RemovePlayer(player *Player) int {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	delete(g.players, player.ID)
	g.playerCount--
	return g.playerCount
}
