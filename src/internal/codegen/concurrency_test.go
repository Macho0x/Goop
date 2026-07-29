// Package codegen_test — concurrency codegen tests.
package codegen_test

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

// TestChanMakeInt verifies that Chan.make () used with a let‑binding
// that constrains the element type (via type annotation) produces
// C0ChanMake() (with the C0Chan wrapper struct) in the generated Go code.
func TestChanMakeInt(t *testing.T) {
	src := `module Main

let main () =
  let ch : int chan = Chan.make () in
  let v = Chan.recv ch in
  println (int_to_string v)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	// Run type checking to populate the TypeMap.
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	gen := codegen.NewGenerator("test.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The generated Go must use C0ChanMake(), not make(chan int).
	if !strings.Contains(goSrc, "C0ChanMake()") {
		t.Errorf("expected C0ChanMake() in generated code, got:\n%s", goSrc)
	}
	// Should NOT emit bare make(chan ...) at the call site (only inside C0ChanMake helper))
	if strings.Contains(goSrc, ":= make(chan") || strings.Contains(goSrc, "= make(chan") {
		t.Errorf("generated code should NOT contain bare make(chan ...) at call site:\n%s", goSrc)
	}
	// Should emit the C0Chan wrapper struct
	if !strings.Contains(goSrc, "type C0Chan struct") {
		t.Errorf("expected C0Chan struct in generated code, got:\n%s", goSrc)
	}
	// Should emit C0ChanRecv
	if !strings.Contains(goSrc, "C0ChanRecv(ch)") {
		t.Errorf("expected C0ChanRecv(ch) in generated code, got:\n%s", goSrc)
	}
}
