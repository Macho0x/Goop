package listfold

import "testing"

func makeList(n int) []int {
	xs := make([]int, n)
	for i := range xs {
		xs[i] = i
	}
	return xs
}

func BenchmarkGenFoldAdd(b *testing.B) {
	xs := makeList(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Fold_add(xs, 0)
	}
}

func BenchmarkGenMapInc(b *testing.B) {
	xs := makeList(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Map_inc(xs)
	}
}
