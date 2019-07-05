package model

import "math"

// Point ...
type Point struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

func normalize(v *Point) {
	length := math.Sqrt(float64(v.X*v.X + v.Y*v.Y))
	v.X = v.X / float32(length)
	v.Y = v.Y / float32(length)
}
