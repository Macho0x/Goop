package visibilitywarns

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
)

func TestVIS002PublicExposesPrivate(t *testing.T) {
	src := `module main
private type secret = Secret of int
let leak (s: secret) : int =
  match s with | Secret n -> n
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "VIS002") {
		t.Fatalf("want VIS002, got %v", warns)
	}
}

func TestVIS002PrivateFnOK(t *testing.T) {
	src := `module main
private type secret = Secret of int
private let leak (s: secret) : int =
  match s with | Secret n -> n
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs)+len(warns) != 0 {
		t.Fatalf("expected silence, got %v %v", errs, warns)
	}
}
