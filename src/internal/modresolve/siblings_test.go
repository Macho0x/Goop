package modresolve_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/modresolve"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

func TestMergeSiblingModules(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.goop")
	b := filepath.Join(dir, "b.goop")
	if err := os.WriteFile(a, []byte("module m\n\nlet public_a () = helper 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("module m\n\nprivate let helper (x: int) = x + 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(a)
	mod, err := parser.Parse(a, src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	if err := modresolve.MergeSiblingModules(mod, a); err != nil {
		t.Fatal(err)
	}
	if len(mod.Decls) < 2 {
		t.Fatalf("expected merged decls, got %d", len(mod.Decls))
	}
	_, _, errs := typecheck.CheckWithTypesForFile(mod, a, nil, nil)
	// Note: CheckWithTypesForFile merges again — write unique content carefully.
	// Re-parse fresh for typecheck path:
	src, _ = os.ReadFile(a)
	mod, err = parser.Parse(a, src)
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	_, _, errs = typecheck.CheckWithTypesForFile(mod, a, nil, nil)
	if len(errs) > 0 {
		t.Fatalf("typecheck: %v", errs)
	}
}

func TestMergeSiblingDuplicate(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.goop")
	b := filepath.Join(dir, "b.goop")
	_ = os.WriteFile(a, []byte("module m\nlet foo () = 1\n"), 0644)
	_ = os.WriteFile(b, []byte("module m\nlet foo () = 2\n"), 0644)
	src, _ := os.ReadFile(a)
	mod, _ := parser.Parse(a, src)
	mod = desugar.DesugarModule(mod)
	err := modresolve.MergeSiblingModules(mod, a)
	if err == nil || !strings.Contains(err.Error(), "MODULE001") {
		t.Fatalf("expected MODULE001, got %v", err)
	}
}

func TestMergeSkipsDifferentModule(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.goop")
	b := filepath.Join(dir, "b.goop")
	_ = os.WriteFile(a, []byte("module m\nlet x () = 1\n"), 0644)
	_ = os.WriteFile(b, []byte("module other\nlet y () = 2\n"), 0644)
	src, _ := os.ReadFile(a)
	mod, _ := parser.Parse(a, src)
	mod = desugar.DesugarModule(mod)
	if err := modresolve.MergeSiblingModules(mod, a); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range mod.Decls {
		if _, ok := d.(*ast.LetDecl); ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected only own decls, got %d let decls", n)
	}
}
