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
