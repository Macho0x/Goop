package discardedresult

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

func TestDiscardedResultWarns(t *testing.T) {
	src := []byte(`module main

let boom () : (int, string) result = Error "no"

let main () =
  begin
    boom ();
    ()
  end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	errs, warns := CheckWithConfig(mod, tm, config.DefaultConfig())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0].Error(), "RESULT001") {
		t.Fatalf("want RESULT001: %v", warns[0])
	}
}

func TestDiscardedResultOff(t *testing.T) {
	src := []byte(`module main
let boom () : (int, string) result = Error "no"
let main () = begin boom (); () end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	cfg := config.DefaultConfig()
	cfg.Check.DiscardedResult = config.SeverityOff
	errs, warns := CheckWithConfig(mod, tm, cfg)
	if len(errs)+len(warns) != 0 {
		t.Fatalf("expected silence, got %v %v", errs, warns)
	}
}

func TestBoundResultNoWarn(t *testing.T) {
	src := []byte(`module main
let boom () : (int, string) result = Error "no"
let main () =
  begin
    let _ = boom () in
    ()
  end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	_, warns := CheckWithConfig(mod, tm, config.DefaultConfig())
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
}

func TestDiscardedOptionWarns(t *testing.T) {
	src := []byte(`module main
let peek () : int option = Some 1
let main () =
  begin
    peek ();
    ()
  end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	errs, warns := CheckWithConfig(mod, tm, config.DefaultConfig())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "OPTION001") {
		t.Fatalf("want OPTION001, got %v", warns)
	}
}

func TestDiscardedOptionOff(t *testing.T) {
	src := []byte(`module main
let peek () : int option = Some 1
let main () = begin peek (); () end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	cfg := config.DefaultConfig()
	cfg.Check.DiscardedOption = config.SeverityOff
	errs, warns := CheckWithConfig(mod, tm, cfg)
	if len(errs)+len(warns) != 0 {
		t.Fatalf("expected silence, got %v %v", errs, warns)
	}
}

func TestBoundOptionNoWarn(t *testing.T) {
	src := []byte(`module main
let peek () : int option = Some 1
let main () =
  begin
    let _ = peek () in
    ()
  end
`)
	mod, err := parser.Parse("t.goop", src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	_, warns := CheckWithConfig(mod, tm, config.DefaultConfig())
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
}
