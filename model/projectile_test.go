package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectilePlayerCollision(t *testing.T) {
	player := NewPlayer(1, 0, 0, nil)
	projectile := Projectile{
		Position: &Point{
			X: 1,
			Y: 1,
		},
	}

	assert.Equal(t, true, projectile.IsCollidingWithPlayer(player))

	projectile.Position.X = 6
	assert.Equal(t, false, projectile.IsCollidingWithPlayer(player))
}

func TestProjectileEnvironmentCollision(t *testing.T) {
	m := NewMap()
	projectile := Projectile{
		Position: &Point{
			X: -135,
			Y: 37,
		},
	}

	assert.Equal(t, true, projectile.IsCollidingWithEnvironment(m))
}

func TestProjectileOutOfBoundsIsRemoved(t *testing.T) {
	m := NewMap()
	player := NewPlayer(1, 0, 0, nil)
	player.Weapons[0].Projectiles = append(player.Weapons[0].Projectiles, &Projectile{
		Position:  &Point{X: m.Max.X - 1, Y: 0},
		Direction: &Point{X: 1, Y: 0},
	})

	// no other players are alive, the projectile still has to be removed
	player.Weapons[0].Update([]*Player{player}, m, 1)
	assert.Equal(t, 0, len(player.Weapons[0].Projectiles))
}

func TestProjectileEnvironmentCollisionWithoutEnemies(t *testing.T) {
	m := NewMap()
	player := NewPlayer(1, 0, 0, nil)
	player.Weapons[0].Projectiles = append(player.Weapons[0].Projectiles, &Projectile{
		Position:  &Point{X: -135, Y: 37},
		Direction: &Point{X: 0, Y: 0},
	})

	player.Weapons[0].Update([]*Player{player}, m, 1)
	assert.Equal(t, 0, len(player.Weapons[0].Projectiles))
}
