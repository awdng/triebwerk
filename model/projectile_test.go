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
