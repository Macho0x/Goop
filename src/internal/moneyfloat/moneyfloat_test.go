package moneyfloat

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
)

func TestMoneyFloatWarns(t *testing.T) {
	src := `module main
type Order = { price: float; qty: int }
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "DECIMAL001") {
		t.Fatalf("want DECIMAL001, got %v", warns)
	}
}

func TestMultiplierExcluded(t *testing.T) {
	src := `module main
let scale (bid_size_mult: float) : float = bid_size_mult
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	_, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(warns) != 0 {
		t.Fatalf("unexpected: %v", warns)
	}
}
