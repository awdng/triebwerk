package model

import (
	"encoding/json"
	"math/rand"
	"time"
)

// Map represents a game map
type Map struct {
	Collider Polygon
	Spawns   []*Point
}

// NewMap creates a new map object
func NewMap() *Map {
	config := string(`[{"x":57.050838,"y":8.945308283649005},{"x":57.95399183489652,"y":-25.821737658258314},{"x":94.33105559930367,"y":-26.12821881046459},{"x":94.5047756059041,"y":9.225389014877914}]`)
	colliders := make([]*Point, 0)
	json.Unmarshal([]byte(config), &colliders)

	return &Map{
		Spawns: []*Point{
			&Point{
				X: 33.92122716470902,
				Y: 19.696850953769385,
			},
			&Point{
				X: 28.825795356963976,
				Y: 37.811654746868406,
			},
			&Point{
				X: 35.76508317625655,
				Y: 96.37030297792447,
			},
			&Point{
				X: -60.31281833122437,
				Y: 25.988099670613494,
			},
			&Point{
				X: -128.18862044178346,
				Y: 27.185409609029083,
			},
			&Point{
				X: -153.69567902314972,
				Y: 112.68349326947794,
			},
		},
		Collider: Polygon{
			Points: colliders,
		},
	}
}

// GetRandomSpawn Point
func (m *Map) GetRandomSpawn() *Point {
	rand.Seed(time.Now().Unix())
	return m.Spawns[rand.Intn(len(m.Spawns))]
}
