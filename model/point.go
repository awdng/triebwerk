package model

import (
	"math"
)

// Point ...
type Point struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// DirectionTo ...
func (p *Point) DirectionTo(v *Point) *Point {
	dir := &Point{
		X: 0,
		Y: 0,
	}
	dir.X = p.X - v.X
	dir.Y = p.Y - v.Y
	normalize(dir)
	return dir
}

// WithinDistanceOf radius to another Point
func (p *Point) WithinDistanceOf(radius float32, v *Point) bool {
	isInRadius := math.Pow(float64(v.X-p.X), 2)+math.Pow(float64(v.Y-p.Y), 2) < math.Pow(float64(radius), 2)
	return isInRadius
}

// IsInPolygon adapted from https://wrf.ecse.rpi.edu/Research/Short_Notes/pnpoly.html
func (p *Point) IsInPolygon(polygon []*Point) bool {
	inside := false
	nvert := len(polygon)
	for i, j := 0, nvert-1; i < nvert; i++ {
		if (polygon[i].Y > p.Y) != (polygon[j].Y > p.Y) && p.X < (polygon[j].X-polygon[i].X)*(p.Y-polygon[i].Y)/(polygon[j].Y-polygon[i].Y)+polygon[i].X {
			inside = !inside
		}
		j = i
	}
	return inside
}

func normalize(v *Point) {
	length := math.Sqrt(float64(v.X*v.X + v.Y*v.Y))
	v.X = v.X / float32(length)
	v.Y = v.Y / float32(length)
}
