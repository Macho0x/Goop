package unused

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

func checkSrc(t *testing.T, src string) (errs, warns []error) {
	t.Helper()
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, terrs := typecheck.CheckWithTypes(mod)
	if len(terrs) > 0 {
		t.Fatalf("type errors: %v", terrs)
	}
	return CheckWithConfig(mod, tm, config.DefaultConfig())
}

func TestUnusedLetWarns(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  let x = 1 in
  0
`)
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "UNUSED001") {
		t.Fatalf("want UNUSED001, got %v", warns)
	}
}

func TestUsedLetNoWarn(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  let x = 1 in
  x
`)
	for _, w := range warns {
		if strings.Contains(w.Error(), "UNUSED001") {
			t.Fatalf("unexpected: %v", w)
		}
	}
}

func TestUnderscoreIgnored(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  let _ = 1 in
  let _y = 2 in
  0
`)
	for _, w := range warns {
		if strings.Contains(w.Error(), "UNUSED001") {
			t.Fatalf("unexpected: %v", w)
		}
	}
}

func TestUnitSequencingIgnored(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  let u = assert true in
  ()
`)
	for _, w := range warns {
		if strings.Contains(w.Error(), "UNUSED001") {
			t.Fatalf("unexpected unit sequencing warn: %v", w)
		}
	}
}

func checkUnusedImportsOnly(t *testing.T, src string) []error {
	t.Helper()
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	// Skip typecheck: import goop needs a resolver (IMPORT003). Unused walks AST only.
	_, warns := CheckWithConfig(mod, nil, config.DefaultConfig())
	var out []error
	for _, w := range warns {
		if strings.Contains(w.Error(), "UNUSED002") {
			out = append(out, w)
		}
	}
	return out
}

func TestImportUsedViaFieldAccess(t *testing.T) {
	warns := checkUnusedImportsOnly(t, `module main
import goop "example.com/sanitize"
let main () = sanitize.default_config
`)
	if len(warns) != 0 {
		t.Fatalf("expected no UNUSED002 for sanitize.default_config, got %v", warns)
	}
}

func TestImportUsedViaCapitalizedFieldAccess(t *testing.T) {
	warns := checkUnusedImportsOnly(t, `module main
import goop "example.com/Correlation"
let main () = Correlation.NewID
`)
	if len(warns) != 0 {
		t.Fatalf("expected no UNUSED002 for Correlation.NewID, got %v", warns)
	}
}

func TestImportUsedViaAliasFieldAccess(t *testing.T) {
	warns := checkUnusedImportsOnly(t, `module main
import s goop "example.com/sanitize"
let main () = s.default_config
`)
	if len(warns) != 0 {
		t.Fatalf("expected no UNUSED002 for aliased s.default_config, got %v", warns)
	}
}

func TestImportUnusedStillWarns(t *testing.T) {
	warns := checkUnusedImportsOnly(t, `module main
import goop "example.com/sanitize"
let main () = 0
`)
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "UNUSED002") {
		t.Fatalf("want UNUSED002 for unused import, got %v", warns)
	}
	if !strings.Contains(warns[0].Error(), "t.goop:") {
		t.Fatalf("UNUSED002 should include source location, got %v", warns[0])
	}
}

func TestImportUsedViaTypeAnnotation(t *testing.T) {
	warns := checkUnusedImportsOnly(t, `module main
import goop "example.com/sanitize"
let main (c: sanitize.Config) = c
`)
	if len(warns) != 0 {
		t.Fatalf("expected no UNUSED002 for type sanitize.Config, got %v", warns)
	}
}

func TestImportUsedViaLocalOpen(t *testing.T) {
	// Local open requires a capitalized module path (parser modulePathOf).
	warns := checkUnusedImportsOnly(t, `module main
import goop "example.com/Sanitize"
let main () = Sanitize.( default_config )
`)
	if len(warns) != 0 {
		t.Fatalf("expected no UNUSED002 for local open Sanitize.(…), got %v", warns)
	}
}
