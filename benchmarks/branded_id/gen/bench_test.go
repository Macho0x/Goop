package brandedid

import "testing"

func BenchmarkGenRoundtrip(b *testing.B) {
	s := "ord-123456789"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Roundtrip(s)
	}
}
