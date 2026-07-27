package adtmatch

import "testing"

func BenchmarkGenArea(b *testing.B) {
	shapes := []shape{
		NewshapeCircle(1.5),
		NewshapeRect(2, 3),
		NewshapePoint(),
		NewshapeCircle(10),
		NewshapeRect(4, 4),
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
