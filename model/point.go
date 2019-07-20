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
	if math.Pow(float64(v.X-p.X), 2)+math.Pow(float64(v.Y-p.Y), 2) < math.Pow(float64(radius), 2) {
		return true
	}
	return false
}

// IsInPolygon adapted from http://www.ecse.rpi.edu/Homepages/wrf/Research/Short_Notes/pnpoly.html
func (p *Point) IsInPolygon(polygon []*Point) bool {
	// add first element as last point again
	polygon = append(polygon, polygon[0])
	inside := false
	j := 0
	for i := 1; i < len(polygon); i++ {
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
