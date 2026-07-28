package typecheck

import (
	"testing"

	"goop.dev/compiler/internal/types"
)

func TestGoTypeToC0TypeDirectionalChan(t *testing.T) {
	cases := []struct {
		src string
	}{
		{"chan int"},
		{"<-chan int"},
		{"chan<- int"},
		{"<-chan Time"},
	}
	for _, tc := range cases {
		got := goTypeToC0TypeInPkg(tc.src, "time")
		ch, ok := got.(*types.TChan)
		if !ok || ch == nil {
			t.Fatalf("%q → %#v, want TChan", tc.src, got)
		}
	}
}
