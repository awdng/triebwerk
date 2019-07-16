package model

import "math"

// Point ...
type Point struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

func (p *Point) directionTo(v *Point) *Point {
	dir := &Point{
		X: 0,
		Y: 0,
	}
	dir.X = v.X - p.X
	dir.Y = v.Y - p.Y
	normalize(dir)
	return dir
}

func normalize(v *Point) {
	length := math.Sqrt(float64(v.X*v.X + v.Y*v.Y))
	v.X = v.X / float32(length)
	v.Y = v.Y / float32(length)
}
