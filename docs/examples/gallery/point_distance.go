package main

import (
	"fmt"
	"math"
)

type point struct {
	X float64
	Y float64
}

func (p *point) Distance(q point) float64 {
	dx := p.X - q.X
	dy := p.Y - q.Y
	return math.Sqrt(dx*dx + dy*dy)
}

func main() {
	a := point{X: 0, Y: 0}
	b := point{X: 3, Y: 4}
	fmt.Println(a.Distance(b))
}
