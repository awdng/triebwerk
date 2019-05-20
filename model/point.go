package model

import "math"

// Point ...
type Point struct {
	X float32
	Y float32
}

func normalize(v *Point) {
	length := math.Sqrt(float64(v.X*v.X + v.Y*v.Y))
	v.X = v.X / float32(length)
	v.Y = v.Y / float32(length)
}
