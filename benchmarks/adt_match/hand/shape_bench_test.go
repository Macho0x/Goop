package hand

import "testing"

func BenchmarkHandArea(b *testing.B) {
	shapes := []Shape{
		Circle(1.5),
		Rect(2, 3),
		Point(),
		Circle(10),
		Rect(4, 4),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum float64
		for _, s := range shapes {
			sum += Area(s)
		}
		_ = sum
	}
}
