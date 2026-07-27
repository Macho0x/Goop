package hand

import "testing"

func makeList(n int) []int {
	xs := make([]int, n)
	for i := range xs {
		xs[i] = i
	}
	return xs
}

func BenchmarkHandFoldAdd(b *testing.B) {
	xs := makeList(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FoldAdd(xs, 0)
	}
}

func BenchmarkHandMapInc(b *testing.B) {
	xs := makeList(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MapInc(xs)
	}
}
