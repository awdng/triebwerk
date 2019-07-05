package model

import (
	"encoding/json"
)

// Map represents a game map
type Map struct {
	Collider Polygon
}

// NewMap creates a new map object
func NewMap() *Map {
	config := string(`[{"x":57.050838,"y":8.945308283649005},{"x":57.95399183489652,"y":-25.821737658258314},{"x":94.33105559930367,"y":-26.12821881046459},{"x":94.5047756059041,"y":9.225389014877914}]`)
	colliders := make([]*Point, 0)
	json.Unmarshal([]byte(config), &colliders)

	return &Map{
		Collider: Polygon{
			Points: colliders,
		},
	}
}
