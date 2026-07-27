// Package hand is the hand-written Go baseline for ADT-style match.
package hand

type ShapeKind int

const (
	ShapeCircle ShapeKind = iota
	ShapeRect
	ShapePoint
)

type Shape struct {
	Kind   ShapeKind
	Radius float64
	Width  float64
	Height float64
}

func Area(s Shape) float64 {
	switch s.Kind {
	case ShapeCircle:
		return 3.141592653589793 * s.Radius * s.Radius
	case ShapeRect:
		return s.Width * s.Height
	case ShapePoint:
		return 0
	default:
		panic("unreachable")
	}
}

func Circle(radius float64) Shape {
	return Shape{Kind: ShapeCircle, Radius: radius}
}

func Rect(width, height float64) Shape {
	return Shape{Kind: ShapeRect, Width: width, Height: height}
}

func Point() Shape {
	return Shape{Kind: ShapePoint}
}
