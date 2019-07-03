package model

import (
	"fmt"
	"math"
)

// Rect ...
type Rect struct {
	A *Point
	B *Point
	C *Point
	D *Point
}

type Polygon struct {
	Points []*Point
}

// RectCollider ...
type RectCollider struct {
	ID                 uint8
	Rect               *Rect
	Pivot              *Point
	Look               *Point
	Turret             *Point
	Dir                *Point
	Velocity           float32
	Rotation           float32
	LastRotation       float32
	TurretRotation     float32
	TurretLastRotation float32
	ForwardSpeed       float32
	RotationSpeed      float32
	CollisionFront     bool
	CollisionBack      bool
}

// NewRectCollider creates a new rectangle collider
func NewRectCollider(x float32, y float32, width float32, depth float32) *RectCollider {
	return &RectCollider{
		Pivot: &Point{
			X: x,
			Y: y,
		},
		Look: &Point{
			X: x,
			Y: 2,
		},
		Turret: &Point{
			X: x,
			Y: 3,
		},
		Dir: &Point{
			X: 0,
			Y: 0,
		},
		Rotation:           0,
		LastRotation:       0,
		TurretRotation:     0,
		TurretLastRotation: 0,
		ForwardSpeed:       15,
		RotationSpeed:      1.5,
		CollisionBack:      false,
		CollisionFront:     false,
		Rect: &Rect{
			A: &Point{
				X: x - (width / 2),
				Y: y + (depth / 2),
			},
			B: &Point{
				X: x + (width / 2),
				Y: y + (depth / 2),
			},
			C: &Point{
				X: x + (width / 2),
				Y: y - (depth / 2),
			},
			D: &Point{
				X: x - (width / 2),
				Y: y - (depth / 2),
			},
		},
	}
}

// CalcDirection calculates the direction vector of the collider
func (r *RectCollider) CalcDirection() {
	r.Dir.X = r.Look.X - r.Pivot.X
	r.Dir.Y = r.Look.Y - r.Pivot.Y
	normalize(r.Dir)
}

// Rotate applies a rotation to all points of the collider
func (r *RectCollider) Rotate(angle float32) {
	r.rotateRectPoint(angle, r.Rect.A)
	r.rotateRectPoint(angle, r.Rect.B)
	r.rotateRectPoint(angle, r.Rect.C)
	r.rotateRectPoint(angle, r.Rect.D)
	r.rotateRectPoint(angle, r.Look)
	r.rotateRectPoint(angle, r.Turret)
}

func (r *RectCollider) rotateRectPoint(angle float32, p *Point) {
	s := float32(math.Sin(float64(angle)))
	c := float32(math.Cos(float64(angle)))

	// translate point back to origin:
	p.X -= r.Pivot.X
	p.Y -= r.Pivot.Y

	// rotate point
	xnew := p.X*c - p.Y*s
	ynew := p.X*s + p.Y*c

	// translate point back:
	p.X = xnew + r.Pivot.X
	p.Y = ynew + r.Pivot.Y
}

func (r *RectCollider) collisionFront(other RectCollider) {
	frontPolygon := Polygon{
		Points: []*Point{r.Rect.A, r.Rect.B, r.Pivot},
	}
	otherPolygon := Polygon{
		Points: []*Point{other.Rect.A, other.Rect.B, other.Rect.C, other.Rect.D},
	}
	if r.doPolygonsIntersect(frontPolygon, otherPolygon) {
		r.CollisionFront = true
		fmt.Println("collision front!")
		return
	}

	r.CollisionFront = false
}

func (r *RectCollider) collisionBack(other RectCollider) {
	backPolygon := Polygon{
		Points: []*Point{r.Rect.C, r.Rect.D, r.Pivot},
	}
	otherPolygon := Polygon{
		Points: []*Point{other.Rect.A, other.Rect.B, other.Rect.C, other.Rect.D},
	}
	if r.doPolygonsIntersect(backPolygon, otherPolygon) {
		r.CollisionBack = true
		fmt.Println("collision back!")
		return
	}

	r.CollisionBack = false
}

func (r *RectCollider) doPolygonsIntersect(a Polygon, b Polygon) bool {
	return doPolygonsIntersect(a, b)
}

func doPolygonsIntersect(a Polygon, b Polygon) bool {
	for _, polygon := range [2]Polygon{a, b} {
		for i1 := 0; i1 < len(polygon.Points); i1++ {
			i2 := (i1 + 1) % len(polygon.Points)
			p1 := polygon.Points[i1]
			p2 := polygon.Points[i2]

			normal := Point{
				X: p2.Y - p1.Y,
				Y: p1.X - p2.X,
			}

			var minA, maxA *float32
			for _, p := range a.Points {
				projected := normal.X*p.X + normal.Y*p.Y
				if minA == nil || projected < *minA {
					minA = &projected
				}
				if maxA == nil || projected > *maxA {
					maxA = &projected
				}
			}

			var minB, maxB *float32
			for _, p := range b.Points {
				projected := normal.X*p.X + normal.Y*p.Y
				if minB == nil || projected < *minB {
					minB = &projected
				}
				if maxB == nil || projected > *maxB {
					maxB = &projected
				}
			}

			if *maxA < *minB || *maxB < *minA {
				return false
			}
		}
	}
	return true
}
