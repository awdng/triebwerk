package model

import "math"

// Rect ...
type Rect struct {
	A *Point
	B *Point
	C *Point
	D *Point
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

// newRectCollider creates a new rectangle collider
func newRectCollider(x float32, y float32, width float32, depth float32) RectCollider {
	return RectCollider{
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
