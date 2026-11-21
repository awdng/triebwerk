package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const ticksPerSecond = 1000 / TickMillis

func runningGame(players ...*Player) *GameState {
	game := NewGameState("test", time.Minute)
	placed := make([]Point, len(players))
	for i, p := range players {
		placed[i] = *p.Collider.Pivot
		game.AddPlayer(p)
	}
	game.Start()
	// the start of a round moves everybody to a random spawn point
	for i, p := range players {
		p.Collider.ChangePosition(placed[i].X, placed[i].Y)
	}
	return game
}

func ticks(game *GameState, count int) {
	for i := 0; i < count; i++ {
		game.Tick()
	}
}

// The browser stops sending inputs while its tank is destroyed.
func TestDestroyedPlayerReturnsWithoutInputs(t *testing.T) {
	p := NewPlayer(1, 10, 10, nil)
	game := runningGame(p)
	p.Health = 0

	ticks(game, 2*ticksPerSecond)
	assert.False(t, p.IsAlive(), "not yet")
	ticks(game, 2*ticksPerSecond)
	assert.True(t, p.IsAlive(), "back after three seconds")
	assert.Equal(t, 100, p.Health)
}

func TestShellsFlyWithoutInputs(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Control = Controls{Shoot: true}
	game.Tick()
	assert.Len(t, p.Weapons[0].Projectiles, 1)
	start := *p.Weapons[0].Projectiles[0].Position

	// the owner sends nothing any more
	p.Control = Controls{}
	ticks(game, 10)
	if assert.Len(t, p.Weapons[0].Projectiles, 1) {
		pos := p.Weapons[0].Projectiles[0].Position
		// shells fly at 100 units per second
		assert.InDelta(t, 33.0, float64(pos.Y-start.Y), 0.5)
	}
}

// and a destroyed tank's shells do not stop in the air
func TestShellsOfADestroyedTankKeepFlying(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Control = Controls{Shoot: true}
	game.Tick()
	start := *p.Weapons[0].Projectiles[0].Position
	p.Health = 0
	ticks(game, 5)
	if assert.Len(t, p.Weapons[0].Projectiles, 1) {
		assert.Greater(t, p.Weapons[0].Projectiles[0].Position.Y, start.Y+10)
	}
}

// One step per tick, however many inputs a client sends.
func TestSpeedDoesNotDependOnMessageRate(t *testing.T) {
	distance := func(inputsPerTick int) float32 {
		p := NewPlayer(1, -128, 28, nil)
		game := runningGame(p)
		sequence := uint32(0)
		for tick := 0; tick < ticksPerSecond; tick++ {
			for i := 0; i < inputsPerTick; i++ {
				sequence++
				p.Inputs.Push(Controls{Sequence: sequence, Forward: true})
			}
			game.ApplyInputs()
			game.Tick()
		}
		return p.Collider.Pivot.Y - 28
	}
	normal := distance(1)
	assert.InDelta(t, 14.85, float64(normal), 0.1, "15 units per second")
	assert.Equal(t, normal, distance(2), "twice the messages")
	assert.Equal(t, normal, distance(40), "a flood of messages")
}

func TestPlayerKnowsHowItsInputsArrive(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	step := func() uint8 {
		game.ApplyInputs()
		game.Tick()
		return p.InputsWaiting()
	}
	assert.Equal(t, uint8(WaitingStarved), step(), "nothing arrived")
	p.Inputs.Push(Controls{Sequence: 1})
	assert.Equal(t, uint8(0), step(), "just in time")
	p.Inputs.Push(Controls{Sequence: 2})
	p.Inputs.Push(Controls{Sequence: 3})
	p.Inputs.Push(Controls{Sequence: 4})
	assert.Equal(t, uint8(2), step(), "two wait for the next ticks")
	assert.Equal(t, uint8(1), step())
	assert.Equal(t, uint8(0), step())
	assert.Equal(t, uint8(WaitingStarved), step())
}

// a client whose inputs arrive late does not stand still for others
func TestTankKeepsDrivingThroughATickWithoutInput(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	for tick := uint32(1); tick <= 30; tick++ {
		if tick%3 != 0 { // every third input comes too late for its tick
			p.Inputs.Push(Controls{Sequence: tick, Forward: true})
		}
		before := p.Collider.Pivot.Y
		game.ApplyInputs()
		game.Tick()
		if tick > 1 {
			assert.InDelta(t, 0.495, float64(p.Collider.Pivot.Y-before), 0.001, "tick %d", tick)
		}
	}
}

func TestFiringIsReportedForOneTickAndCounted(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Control = Controls{Shoot: true}
	game.Tick()
	assert.True(t, p.Fired)
	assert.Equal(t, uint8(1), p.Shots)

	// the trigger stays pressed, the gun is loading
	game.Tick()
	assert.False(t, p.Fired)
	assert.Equal(t, uint8(1), p.Shots)

	// 1.2 seconds later it fires again
	p.Control = Controls{Shoot: true}
	for i := 0; i < 2*ticksPerSecond && p.Shots == 1; i++ {
		p.Control.Shoot = true
		game.Tick()
	}
	assert.Equal(t, uint8(2), p.Shots)
	assert.InDelta(t, 1200, float64(game.GameTime()), 100)
}

func TestGameTimeFollowsTheTicks(t *testing.T) {
	game := NewGameState("test", 2*time.Second)
	game.AddPlayer(NewPlayer(1, 10, 10, nil))
	ticks(game, 7) // the server ran before the round
	game.Start()
	assert.Equal(t, uint32(0), game.GameTime())
	ticks(game, 10)
	assert.Equal(t, uint32(10*TickMillis), game.GameTime())
	assert.Equal(t, uint32(17), game.TickNumber(), "ticks count on from round to round")
	assert.False(t, game.HasEnded())
	ticks(game, 51)
	assert.True(t, game.HasEnded(), "2 seconds are 61 ticks of 33 ms")
}

func TestNothingMovesBetweenRounds(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := NewGameState("test", time.Minute)
	game.AddPlayer(p)
	p.Inputs.Push(Controls{Sequence: 1, Forward: true})
	game.ApplyInputs()
	game.Tick()
	assert.Equal(t, float32(28), p.Collider.Pivot.Y)
	assert.Equal(t, 0, p.Inputs.Waiting(), "inputs between rounds are not kept for the next one")
}

// the same inputs give the same match, whatever order the players are stored in
func TestTicksAreDeterministic(t *testing.T) {
	play := func() []float32 {
		a := NewPlayer(1, -128, 28, nil)
		b := NewPlayer(2, -128, 40, nil)
		c := NewPlayer(3, -122, 34, nil)
		game := runningGame(c, a, b)
		for tick := uint32(1); tick <= 90; tick++ {
			a.Inputs.Push(Controls{Sequence: tick, Forward: true, Right: tick%7 < 3})
			b.Inputs.Push(Controls{Sequence: tick, Backward: true, Left: tick%5 < 2, Shoot: true})
			c.Inputs.Push(Controls{Sequence: tick, Forward: tick%2 == 0, TurretLeft: true, Shoot: tick > 40})
			game.ApplyInputs()
			game.Tick()
		}
		return []float32{a.Collider.Pivot.X, a.Collider.Pivot.Y, a.Collider.Rotation, b.Collider.Pivot.X, b.Collider.Pivot.Y, c.Collider.Pivot.X, c.Collider.TurretRotation, float32(a.Health + b.Health + c.Health)}
	}
	first := play()
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, play())
	}
}

// the keys of the turret turn it the way the keys of the same name turn the hull
func TestTurretTurnsTheWayTheHullDoes(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Control = Controls{Right: true}
	game.Tick()
	hull := p.Collider.Rotation
	assert.InDelta(t, 1.5*0.033, float64(hull), 1e-6)
	assert.Equal(t, hull, p.Collider.TurretRotation, "the turret sits on the hull")

	p.Control = Controls{TurretRight: true}
	game.Tick()
	assert.InDelta(t, 2*1.5*0.033, float64(p.Collider.TurretRotation), 1e-6)
	assert.Equal(t, hull, p.Collider.Rotation)

	p.Control = Controls{TurretLeft: true}
	game.Tick()
	game.Tick()
	assert.InDelta(t, 0, float64(p.Collider.TurretRotation), 1e-6)
}
