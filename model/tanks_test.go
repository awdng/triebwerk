package model

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clients that know nothing of kinds of tanks, and everybody on a server
// that is told nothing, drive the first kind. It is the tank the game
// always had: nothing about it may change by accident.
func TestTheFirstKindOfTankIsTheTankTheGameAlwaysHad(t *testing.T) {
	first := Tanks[0]
	assert.Equal(t, "Stormblade", first.Name)
	assert.Equal(t, float32(15), first.Speed)
	assert.Equal(t, float32(1.5), first.TurnRate)
	assert.Equal(t, float32(1.5), first.TurretRate)
	assert.Equal(t, float32(1.2), first.Reload)
	assert.Equal(t, 25, first.Damage)
	assert.Equal(t, float32(1), first.Armour)
	assert.Equal(t, float32(5), first.Width)
	assert.Equal(t, float32(7), first.Depth)

	p := NewPlayer(1, 10, 10, nil)
	assert.Equal(t, Loadout{}, p.Loadout)
	assert.Equal(t, first, p.Spec())
}

func TestThereAreSevenKindsWithNamesAndNumbersThatMakeSense(t *testing.T) {
	require.Len(t, Tanks, 7)
	names := map[string]bool{}
	for kind, spec := range Tanks {
		assert.NotEmpty(t, spec.Name, "kind %d", kind)
		assert.False(t, names[spec.Name], "%s twice", spec.Name)
		names[spec.Name] = true
		assert.InDelta(t, 16, float64(spec.Speed), 8, spec.Name)
		assert.InDelta(t, 1.8, float64(spec.TurnRate), 1, spec.Name)
		assert.InDelta(t, 1.8, float64(spec.TurretRate), 1, spec.Name)
		assert.InDelta(t, 1.3, float64(spec.Reload), 1, spec.Name)
		assert.Greater(t, spec.Damage, 0, spec.Name)
		assert.InDelta(t, 1.2, float64(spec.Armour), 0.5, spec.Name)
		// the map was built for a tank of 5 by 7: none is much bigger
		assert.LessOrEqual(t, spec.Width, float32(5.5), spec.Name)
		assert.LessOrEqual(t, spec.Depth, float32(7), spec.Name)
		assert.GreaterOrEqual(t, spec.Width, float32(3.5), spec.Name)
		assert.GreaterOrEqual(t, spec.Depth, float32(4.5), spec.Name)
	}
}

// what a kind of tank can do is what its numbers say
func TestEveryKindDrivesTurnsAndFiresByItsOwnNumbers(t *testing.T) {
	for kind, spec := range Tanks {
		p := NewPlayer(1, -128, 28, nil)
		p.SetTank(uint8(kind))
		game := runningGame(p)

		p.Control = Controls{Forward: true}
		ticks(game, ticksPerSecond)
		assert.InDelta(t, float64(spec.Speed)*ticksPerSecond*0.033, float64(p.Collider.Pivot.Y-28), 0.01, "%s: speed", spec.Name)

		p.Control = Controls{Backward: true}
		ticks(game, ticksPerSecond)
		assert.InDelta(t, 28, float64(p.Collider.Pivot.Y), 0.01, "%s: backwards as fast as forwards", spec.Name)

		p.Control = Controls{Right: true}
		game.Tick()
		assert.InDelta(t, float64(spec.TurnRate)*0.033, float64(p.Collider.Rotation), 1e-6, "%s: hull", spec.Name)
		assert.Equal(t, p.Collider.Rotation, p.Collider.TurretRotation, "%s: the turret sits on the hull", spec.Name)

		p.Control = Controls{TurretRight: true}
		game.Tick()
		assert.InDelta(t, float64(spec.TurnRate+spec.TurretRate)*0.033, float64(p.Collider.TurretRotation), 1e-6, "%s: turret", spec.Name)
	}
}

// The client shows its own shots before the server confirms them: it has to
// know the tick at which the gun of its tank is loaded again.
func TestEveryKindFiresAgainAfterItsReloadTicks(t *testing.T) {
	assert.Equal(t, 38, Tanks[0].ReloadTicks(), "the Stormblade, as ever")
	for kind, spec := range Tanks {
		p := NewPlayer(1, -128, 28, nil)
		p.SetTank(uint8(kind))
		game := runningGame(p)
		var fired []int
		for tick := 1; tick <= 4*spec.ReloadTicks(); tick++ {
			p.Control = Controls{Shoot: true}
			game.Tick()
			if p.Fired {
				fired = append(fired, tick)
			}
		}
		n := spec.ReloadTicks()
		assert.Equal(t, []int{1, 1 + n, 1 + 2*n, 1 + 3*n}, fired, spec.Name)
		assert.InDelta(t, float64(spec.Reload), float64(n)*0.033, 0.07, "%s: about its reload time", spec.Name)
	}
}

// what a hit takes: the damage of the gun, as much of it as the armour lets through
func TestAHitTakesWhatTheGunGivesAndTheArmourLetsThrough(t *testing.T) {
	m := NewMap()
	for _, shooterKind := range []uint8{Stormblade, PainMaker, Buggy} {
		for targetKind, armoured := range Tanks {
			shooter := NewPlayer(1, 10, 10, nil)
			shooter.SetTank(shooterKind)
			target := NewPlayer(2, 40, 40, nil)
			target.SetTank(uint8(targetKind))
			hit(shooter, target, []*Player{shooter, target}, m)
			want := int(math.Round(float64(float32(shooter.Spec().Damage) * armoured.Armour)))
			assert.Equal(t, 100-want, target.Health, "%s hits %s", shooter.Spec().Name, armoured.Name)
			assert.Greater(t, want, 0)
		}
	}
	// the numbers of the plan, spelled out once
	assert.Equal(t, 25, Tanks[Stormblade].DamageTo(Tanks[Stormblade]))
	assert.Equal(t, 56, Tanks[PainMaker].DamageTo(Tanks[Buggy]))
	assert.Equal(t, 8, Tanks[Buggy].DamageTo(Tanks[Grunt]))
}

// a heavy hit on a light tank takes all it has, and no more
func TestHealthNeverGoesBelowNothing(t *testing.T) {
	m := NewMap()
	shooter := NewPlayer(1, 10, 10, nil)
	shooter.SetTank(PainMaker)
	target := NewPlayer(2, 40, 40, nil)
	target.SetTank(Buggy)
	players := []*Player{shooter, target}
	hit(shooter, target, players, m)
	hit(shooter, target, players, m)
	assert.Equal(t, 0, target.Health)
	assert.Equal(t, 1, shooter.Score)
	assert.Equal(t, 1, target.Deaths)
}

func TestAKindThatDoesNotExistIsTheFirstKind(t *testing.T) {
	p := NewPlayer(1, 10, 10, nil)
	p.SetTank(200)
	assert.Equal(t, uint8(0), p.Loadout.Tank)
	assert.Equal(t, Tanks[0], p.Spec())
	assert.Equal(t, Tanks[0], SpecOf(200))
	assert.Equal(t, Tanks[Buggy], SpecOf(Buggy))
}

// the tank stays where it is and has the size of its kind from then on
func TestAnotherKindOfTankStandsWhereTheTankStood(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	for tick := 0; tick < 40; tick++ {
		p.Control = Controls{Forward: tick%3 != 0, Right: true, TurretLeft: tick > 10}
		game.Tick()
	}
	x, y := p.Collider.Pivot.X, p.Collider.Pivot.Y
	rotation, turret := p.Collider.Rotation, p.Collider.TurretRotation

	p.SetTank(Buggy)
	c := p.Collider
	assert.Equal(t, uint8(Buggy), p.Loadout.Tank)
	assert.Equal(t, x, c.Pivot.X)
	assert.Equal(t, y, c.Pivot.Y)
	assert.Equal(t, rotation, c.Rotation)
	assert.Equal(t, turret, c.TurretRotation)
	side := func(a, b *Point) float64 {
		return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
	}
	assert.InDelta(t, float64(Tanks[Buggy].Width), side(c.Rect.A, c.Rect.B), 1e-3)
	assert.InDelta(t, float64(Tanks[Buggy].Depth), side(c.Rect.B, c.Rect.C), 1e-3)

	// and drives on from there like a tank that came the same way
	p.Control = Controls{Forward: true}
	game.Tick()
	assert.InDelta(t, float64(Tanks[Buggy].Speed)*0.033, math.Hypot(float64(c.Pivot.X-x), float64(c.Pivot.Y-y)), 1e-3)
}

// ---- what a player chose ------------------------------------------------------------

// The tank a player joins with is theirs at once. Changing it in the middle
// of a life would be a tank that turns into another one in front of
// everybody: the new one comes with the next spawn.
func TestTheFirstChoiceCountsAtOnceALaterOneAtTheNextSpawn(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	other := NewPlayer(2, 40, 40, nil)
	game := runningGame(p, other)

	assert.True(t, p.Choose(Loadout{Tank: Charger, Skin: 4}), "the first choice")
	assert.Equal(t, Loadout{Tank: Charger, Skin: 4}, p.Loadout)
	assert.Equal(t, Tanks[Charger], p.Spec())

	assert.False(t, p.Choose(Loadout{Tank: Buggy, Skin: 1}), "while the tank drives")
	assert.Equal(t, Loadout{Tank: Charger, Skin: 4}, p.Loadout)
	ticks(game, ticksPerSecond)
	assert.Equal(t, Loadout{Tank: Charger, Skin: 4}, p.Loadout)

	// destroyed, and back three seconds later in what was chosen
	p.Health = 0
	ticks(game, 4*ticksPerSecond)
	require.True(t, p.IsAlive())
	assert.Equal(t, Loadout{Tank: Buggy, Skin: 1}, p.Loadout)
	assert.Equal(t, Tanks[Buggy], p.Spec())
	assert.Equal(t, 100, p.Health)
	assert.Equal(t, Loadout{}, other.Loadout, "nobody else changes")
}

func TestAChoiceIsTakenAtTheStartOfARound(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Choose(Loadout{Tank: Grunt, Skin: 2})
	p.Choose(Loadout{Tank: Liquidator, Skin: 3})
	p.Choose(Loadout{Tank: PainMaker, Skin: 0})
	assert.Equal(t, Loadout{Tank: Grunt, Skin: 2}, p.Loadout)

	game.End()
	game.Start()
	assert.Equal(t, Loadout{Tank: PainMaker, Skin: 0}, p.Loadout, "the last thing that was chosen")
	game.End()
	game.Start()
	assert.Equal(t, Loadout{Tank: PainMaker, Skin: 0}, p.Loadout)
}

// what makes no sense is the first kind; the skin is the client's business
func TestAChoiceOfATankThatDoesNotExistIsTheFirstKind(t *testing.T) {
	p := NewPlayer(1, 10, 10, nil)
	p.Choose(Loadout{Tank: 99, Skin: 250})
	assert.Equal(t, Loadout{Tank: 0, Skin: 250}, p.Loadout)
}

// choosing the same again changes nothing and is not a change to wait for
func TestChoosingWhatThePlayerDrivesIsNoChange(t *testing.T) {
	p := NewPlayer(1, -128, 28, nil)
	game := runningGame(p)
	p.Choose(Loadout{Tank: Charger, Skin: 4})
	p.Choose(Loadout{Tank: Buggy, Skin: 1})
	p.Choose(Loadout{Tank: Charger, Skin: 4})
	p.Health = 0
	ticks(game, 4*ticksPerSecond)
	assert.Equal(t, Loadout{Tank: Charger, Skin: 4}, p.Loadout)
}

// ---- the map ------------------------------------------------------------------------

// A tank is put on a spawn point looking the way it always does. Whatever
// its size, it must not stand in a wall there.
func TestEverySpawnPointHasRoomForEveryKindOfTank(t *testing.T) {
	m := NewMap()
	require.NotEmpty(t, m.Spawns)
	for kind, spec := range Tanks {
		for _, spawn := range m.Spawns {
			p := NewPlayer(1, spawn.X, spawn.Y, nil)
			p.SetTank(uint8(kind))
			for i, wall := range m.Collider {
				assert.False(t, p.Collider.collisionPolygon(wall.getPolygon()), "%s at %.0f, %.0f stands in wall %d", spec.Name, spawn.X, spawn.Y, i)
			}
			assert.False(t, m.IsOutOfBounds(p.Collider.Pivot), spec.Name)
		}
	}
}

// ---- what clients are told ----------------------------------------------------------

// The browser predicts its own tank with the numbers of the server: it
// reads this table from the same code, compiled to WebAssembly.
func TestTheTableIsHandedToClientsAsJSON(t *testing.T) {
	var table []map[string]any
	require.NoError(t, json.Unmarshal([]byte(TankTableJSON()), &table))
	require.Len(t, table, len(Tanks))
	assert.Equal(t, map[string]any{
		"name": "Stormblade", "speed": float64(15), "turnRate": 1.5, "turretRate": 1.5, "reload": 1.2, "reloadTicks": float64(38),
		"damage": float64(25), "armour": float64(1), "width": float64(5), "depth": float64(7),
	}, table[0])
	for kind, spec := range Tanks {
		assert.Equal(t, spec.Name, table[kind]["name"])
		assert.Equal(t, float64(spec.ReloadTicks()), table[kind]["reloadTicks"])
	}
}

// ---- balance ------------------------------------------------------------------------

// secondsToDestroy is how long a gun that never misses takes for a tank at full health
func secondsToDestroy(shooter, target TankSpec) float64 {
	hits := int(math.Ceil(100 / float64(shooter.DamageTo(target))))
	return float64(hits-1) * float64(shooter.ReloadTicks()) * float64(StepSeconds)
}

// No kind wins by its numbers alone. In a duel against the tank the game
// always had, with every shot a hit, none is much quicker than the other;
// and no gun does much more damage in a second than another.
func TestNoKindOfTankIsTheOneToTake(t *testing.T) {
	first := Tanks[Stormblade]
	for _, spec := range Tanks {
		wins := secondsToDestroy(spec, first)
		loses := secondsToDestroy(first, spec)
		t.Logf("%-12s destroys a Stormblade in %.1f s, is destroyed by one in %.1f s, drives %.1f", spec.Name, wins, loses, spec.Speed)
		assert.InDelta(t, 4.4, wins, 1.2, "%s: seconds to destroy a Stormblade", spec.Name)
		perSecond := float64(spec.Damage) / (float64(spec.ReloadTicks()) * float64(StepSeconds))
		assert.InDelta(t, 19, perSecond, 2.5, "%s: damage per second", spec.Name)
		// what wins the duel is not also the faster tank
		if loses-wins > 0.5 {
			assert.Less(t, spec.Speed, first.Speed, "%s wins the duel and outruns the loser", spec.Name)
		}
		if spec.Speed > first.Speed {
			assert.LessOrEqual(t, loses, wins, "%s is faster and wins the duel", spec.Name)
		}
	}
}
