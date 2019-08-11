package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolygonCollision(t *testing.T) {
	collider1 := NewRectCollider(0, 0, 10, 10)
	collider2 := NewRectCollider(20, 20, 10, 10)

	assert.Equal(t, false, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))

	collider1 = NewRectCollider(0, 0, 10, 10)
	collider2 = NewRectCollider(5, 5, 10, 10)

	assert.Equal(t, true, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))

	collider1 = NewRectCollider(0, 0, 10, 10)
	collider2 = NewRectCollider(5, 5, 10, 10)
	collider2.ChangePosition(-11, -11)

	assert.Equal(t, false, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))

	collider1 = NewRectCollider(0, 0, 2, 5)
	collider2 = NewRectCollider(3, 0, 2, 5)
	assert.Equal(t, false, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))

	collider2.Rotate(-90)
	assert.Equal(t, true, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))
	collider1.ChangePosition(0, 5)
	assert.Equal(t, false, doPolygonsIntersect(collider1.getPolygon(), collider2.getPolygon()))
}
