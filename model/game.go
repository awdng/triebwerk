package model

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// The server simulates in fixed steps. Everything that moves, moves once per
// tick: tanks by the input their client sent for it, shells and timers by
// themselves. Clients predict with the same step.
const (
	// TickMillis is the length of a tick in milliseconds
	TickMillis = 33
	// StepSeconds is the same in seconds, the dt of every simulation step
	StepSeconds float32 = 0.033
)

// GameState ...
type GameState struct {
	Region string
	// ticks since the server started, and the tick the round started at
	tick       atomic.Uint32
	roundStart atomic.Uint32
	// the tick the last round ended at
	roundEnd atomic.Uint32
	// rounds since the server started
	round uint32
	// the participants of the round are numbered; those who left during it
	// are kept for its report
	participants int
	departed     []PlayerSnapshot
	length       time.Duration
	inProgress   bool
	playerID     int64
	playerCount  int
	players      map[int]*Player
	Map          *Map
	mutex        *sync.RWMutex
}

// NewGameState creates the state of a server, a round lasts for length
func NewGameState(region string, length time.Duration) *GameState {
	return &GameState{
		Region:     region,
		inProgress: false,
		length:     length,
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
		p.Deaths = 0
		p.KillLog = nil
		p.respawnCountdown = 0
		p.Inputs.Reset()
		p.Control = Controls{}
		spawn := g.Map.GetRandomSpawn(players)
		p.Collider.ChangePosition(spawn.X, spawn.Y)
	}
	g.roundStart.Store(g.tick.Load())
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.inProgress = true
	// a new round, a new report: everybody who is here takes part
	g.round++
	g.departed = nil
	g.participants = 0
	for _, p := range players {
		g.participants++
		p.participant = g.participants
		p.joinedTick = g.tick.Load()
	}
}

// End ...
func (g *GameState) End() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.inProgress = false
	g.roundEnd.Store(g.tick.Load())
}

// InProgress ...
func (g *GameState) InProgress() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.inProgress
}

// HasEnded ...
func (g *GameState) HasEnded() bool {
	return time.Duration(g.GameTime())*time.Millisecond >= g.length
}

// GameTime is the time since the round started in milliseconds. It moves
// with the ticks, not with the clock.
func (g *GameState) GameTime() uint32 {
	return (g.tick.Load() - g.roundStart.Load()) * TickMillis
}

// TickNumber counts the ticks since the server started
func (g *GameState) TickNumber() uint32 {
	return g.tick.Load()
}

// RoundLength is how long a round lasts
func (g *GameState) RoundLength() time.Duration {
	return g.length
}

// RoundStartTick is the tick the running round started at
func (g *GameState) RoundStartTick() uint32 {
	return g.roundStart.Load()
}

// ApplyInputs gives every player the keys for the coming tick: the next
// input of its client, or the last one again if none has arrived. Between
// rounds inputs are thrown away.
func (g *GameState) ApplyInputs() {
	inProgress := g.InProgress()
	for _, p := range g.GetPlayers() {
		if !inProgress {
			p.Inputs.Reset()
			p.Control = Controls{}
			p.Starved = false
			continue
		}
		p.Control, p.Starved = p.Inputs.Next()
	}
}

// Tick advances the match by one step: every tank moves by its keys, every
// shell flies, every destroyed tank comes closer to its return. It does not
// matter what clients sent or whether they sent anything.
func (g *GameState) Tick() {
	if g.InProgress() {
		players := g.GetPlayers()
		for _, p := range players {
			p.Fired = false
			if p.IsAlive() {
				p.HandleMovement(players, g.Map, StepSeconds)
			} else {
				p.respawnCountdown += StepSeconds
			}
		}
		for _, p := range players {
			for _, w := range p.Weapons {
				w.Update(players, g.Map, StepSeconds)
			}
			if p.IsAlive() && p.Control.Shoot {
				p.Weapons[0].ShootAt(p.Collider.Turret.X, p.Collider.Turret.Y)
			}
		}
		for _, p := range players {
			p.HandleRespawn(g)
		}
	}
	g.tick.Add(1)
}

// GetPlayers returns the players by id: what a tick does and what clients
// are sent must not depend on the order of a map
func (g *GameState) GetPlayers() []*Player {
	g.mutex.RLock()
	players := make([]*Player, 0, len(g.players))
	for _, p := range g.players {
		players = append(players, p)
	}
	g.mutex.RUnlock()
	sort.Slice(players, func(i, j int) bool { return players[i].ID < players[j].ID })
	return players
}

// GetPlayerCount ...
func (g *GameState) GetPlayerCount() int {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.playerCount
}

// maxPlayerID is the highest player id, ids are sent to clients as a single byte
const maxPlayerID = 255

// GetNewPlayerID returns the next free player id in the range 1 to 255.
// Ids are handed out round robin so that a recently disconnected player's id
// is not reused immediately.
func (g *GameState) GetNewPlayerID() int {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	for i := 0; i < maxPlayerID; i++ {
		g.playerID = g.playerID%maxPlayerID + 1
		if _, inUse := g.players[int(g.playerID)]; !inUse {
			return int(g.playerID)
		}
	}
	// every id is in use, this can not happen with a sane MAX_PLAYERS
	return int(g.playerID)
}

// AddPlayer to the game
func (g *GameState) AddPlayer(player *Player) int {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.players[player.ID] = player
	g.playerCount++
	// who joins a running round takes part in it; between two rounds the
	// next one counts
	player.participant = 0
	if g.inProgress {
		g.participants++
		player.participant = g.participants
		player.joinedTick = g.tick.Load()
	}
	return g.playerCount
}

// RemovePlayer from the game
func (g *GameState) RemovePlayer(player *Player) int {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	delete(g.players, player.ID)
	g.playerCount--
	if player.participant > 0 && player.Nickname != "" {
		g.remember(g.report(player, false))
	}
	return g.playerCount
}

// MaxRoundReport is the highest number of participants a round reports.
// A round that more connections came to and left forgets the earliest of
// those that never scored.
const MaxRoundReport = 150

// PlayerSnapshot is somebody who takes or took part in a round, as the
// master server is told
type PlayerSnapshot struct {
	// Slot numbers the participants of a round. Somebody who leaves and
	// comes back gets a new one.
	Slot int
	// GlobalID is the id of the account, empty for a guest
	GlobalID string
	Nickname string
	// Score counts the players this one destroyed
	Score  int
	Deaths int
	// SecondsPlayed of the round
	SecondsPlayed int
	// Connected is false for somebody who left during the round
	Connected bool
}

// StateSnapshot is a copy of the game state that can be handed to other goroutines
type StateSnapshot struct {
	Region string
	// Online is the number of players on the server right now
	Online int
	// Round counts the rounds since the server started, 0 before the first
	Round      uint32
	InProgress bool
	// GameTime of the round in milliseconds, of a round that is over the
	// time it lasted
	GameTime uint32
	// Length of a round
	Length time.Duration
	// Players are the participants of the round by slot, also those who left
	Players []PlayerSnapshot
}

// roundTicks is how long the round has lasted, or lasted if it is over
func (g *GameState) roundTicks(inProgress bool) uint32 {
	if inProgress {
		return g.tick.Load() - g.roundStart.Load()
	}
	return g.roundEnd.Load() - g.roundStart.Load()
}

// report is what the round's report says about a player. The mutex is held
// by the caller.
func (g *GameState) report(p *Player, connected bool) PlayerSnapshot {
	until := g.tick.Load()
	if !g.inProgress {
		until = g.roundEnd.Load()
	}
	played := uint32(0)
	if until > p.joinedTick {
		played = until - p.joinedTick
	}
	return PlayerSnapshot{
		Slot:          p.participant,
		GlobalID:      p.GlobalID,
		Nickname:      p.Nickname,
		Score:         p.Score,
		Deaths:        p.Deaths,
		SecondsPlayed: int(played * TickMillis / 1000),
		Connected:     connected,
	}
}

// remember keeps a player who left in the round's report. The mutex is
// held by the caller.
func (g *GameState) remember(left PlayerSnapshot) {
	// the report has room for MaxRoundReport: who is here, and who left
	if room := max(MaxRoundReport-len(g.players), 1); len(g.departed) >= room {
		drop := 0
		for i, p := range g.departed {
			if p.Score == 0 && p.Deaths == 0 {
				drop = i
				break
			}
		}
		g.departed = append(g.departed[:drop], g.departed[drop+1:]...)
	}
	g.departed = append(g.departed, left)
}

// Snapshot copies the current game state
func (g *GameState) Snapshot() StateSnapshot {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	snapshot := StateSnapshot{
		Region:     g.Region,
		Online:     g.playerCount,
		Round:      g.round,
		InProgress: g.inProgress,
		GameTime:   g.roundTicks(g.inProgress) * TickMillis,
		Length:     g.length,
		Players:    make([]PlayerSnapshot, 0, len(g.departed)+len(g.players)),
	}
	snapshot.Players = append(snapshot.Players, g.departed...)
	for _, p := range g.players {
		// a player is in the report once it takes part in the round and
		// the master server has said who it is
		if p.participant > 0 && p.Nickname != "" {
			snapshot.Players = append(snapshot.Players, g.report(p, true))
		}
	}
	sort.Slice(snapshot.Players, func(i, j int) bool { return snapshot.Players[i].Slot < snapshot.Players[j].Slot })
	if len(snapshot.Players) > MaxRoundReport {
		snapshot.Players = snapshot.Players[len(snapshot.Players)-MaxRoundReport:]
	}
	return snapshot
}
