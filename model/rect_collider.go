package model

import (
	"math"
)

// Rect ...
type Rect struct {
	A *Point
	B *Point
	C *Point
	D *Point
}

// Polygon ...
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
			Y: y + 2,
		},
		Turret: &Point{
			X: x,
			Y: y + 3,
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

func (r *RectCollider) rotateRectPoint(theta float32, p *Point) {
	sinTheta := float32(math.Sin(float64(theta)))
	cosTheta := float32(math.Cos(float64(theta)))

	// point to origin
	p.X -= r.Pivot.X
	p.Y -= r.Pivot.Y

	// rotation
	x := p.X*cosTheta - p.Y*sinTheta
	y := p.X*sinTheta + p.Y*cosTheta

	// point back to original position
	p.X = x + r.Pivot.X
	p.Y = y + r.Pivot.Y
}

// ChangePosition of Collider
func (r *RectCollider) ChangePosition(posX, posY float32) {
	dX := posX - r.Pivot.X
	dY := posY - r.Pivot.Y

	r.Pivot.X = posX
	r.Pivot.Y = posY

	r.Look.X += dX
	r.Look.Y += dY

	r.Turret.X += dX
	r.Turret.Y += dY

	r.Rect.A.X += dX
	r.Rect.A.Y += dY

	r.Rect.B.X += dX
	r.Rect.B.Y += dY

	r.Rect.C.X += dX
	r.Rect.C.Y += dY

	r.Rect.D.X += dX
	r.Rect.D.Y += dY
}

func (r *RectCollider) getPolygon() Polygon {
	return Polygon{
		Points: []*Point{r.Rect.A, r.Rect.B, r.Rect.C, r.Rect.D},
	}
}

func (r *RectCollider) collisionPolygon(otherPolygon Polygon) bool {
	if r.doPolygonsIntersect(r.getPolygon(), otherPolygon) {
		return true
	}
	return false
}

func (r *RectCollider) collisionFrontRect(other RectCollider) {
	otherPolygon := Polygon{
		Points: []*Point{other.Rect.A, other.Rect.B, other.Rect.C, other.Rect.D},
	}
	r.collisionFront(otherPolygon)
}

func (r *RectCollider) collisionBackRect(other RectCollider) {
	otherPolygon := Polygon{
		Points: []*Point{other.Rect.A, other.Rect.B, other.Rect.C, other.Rect.D},
	}
	r.collisionBack(otherPolygon)
}

func (r *RectCollider) collisionFront(otherPolygon Polygon) {
	frontPolygon := Polygon{
		Points: []*Point{r.Rect.A, r.Rect.B, r.Pivot},
	}

	if r.doPolygonsIntersect(frontPolygon, otherPolygon) {
		r.CollisionFront = true
		return
	}

	r.CollisionFront = false
}

func (r *RectCollider) collisionBack(otherPolygon Polygon) {
	backPolygon := Polygon{
		Points: []*Point{r.Rect.C, r.Rect.D, r.Pivot},
	}

	if r.doPolygonsIntersect(backPolygon, otherPolygon) {
		r.CollisionBack = true
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
