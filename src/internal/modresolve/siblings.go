package modresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
)

// MergeSiblingModules appends declarations from other .goop files in the same
// directory that share mod.Name into mod. Deterministic file order.
// *_test.goop files are excluded when merging into a non-test entry; a test
// entry merges package (non-test) siblings only.
// Duplicate top-level names across files return an error.
func MergeSiblingModules(mod *ast.Module, srcFile string) error {
	if mod == nil || srcFile == "" {
		return nil
	}
	// Flat dirs (docs/examples, scaffolds) use many independent `module main`
	// files; never merge those. Real packages use a distinctive module name.
	if strings.EqualFold(mod.Name, "main") {
		return nil
	}
	dir := filepath.Dir(srcFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no dir / unreadable — leave mod as-is
	}
	entryBase := filepath.Base(srcFile)
	entryIsTest := strings.HasSuffix(entryBase, "_test.goop")

	var siblings []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, SourceExt) {
			continue
		}
		if name == entryBase {
			continue
		}
		isTest := strings.HasSuffix(name, "_test.goop")
		if entryIsTest {
			if isTest {
				continue // do not merge other test files
			}
		} else if isTest {
			continue // package build ignores tests
		}
		siblings = append(siblings, filepath.Join(dir, name))
	}
	sort.Strings(siblings)

	seen := declNames(mod)
	for _, path := range siblings {
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading sibling %s: %w", path, err)
		}
		sib, err := parser.Parse(path, src)
		if err != nil {
			return fmt.Errorf("parse sibling %s: %w", path, err)
		}
		sib = desugar.DesugarModule(sib)
		if sib.Name != mod.Name {
			continue
		}
		for _, d := range sib.Decls {
			for _, n := range topDeclNames(d) {
				if seen[n] {
					return fmt.Errorf("MODULE001: duplicate top-level name %q when merging %s into %s (same module %q)", n, path, srcFile, mod.Name)
				}
				seen[n] = true
			}
			mod.Decls = append(mod.Decls, d)
		}
		// Union imports (dedupe by kind+path+alias)
		for _, spec := range sib.Imports {
			if !hasImport(mod.Imports, spec) {
				mod.Imports = append(mod.Imports, spec)
			}
		}
	}
	return nil
}

func hasImport(specs []ast.ImportSpec, want ast.ImportSpec) bool {
	for _, s := range specs {
		if s.Kind == want.Kind && s.Path == want.Path && s.Alias == want.Alias {
			return true
		}
	}
	return false
}

func declNames(mod *ast.Module) map[string]bool {
	m := make(map[string]bool)
	for _, d := range mod.Decls {
		for _, n := range topDeclNames(d) {
			m[n] = true
		}
	}
	return m
}

func topDeclNames(d ast.TopDecl) []string {
	switch d := d.(type) {
	case *ast.LetDecl:
		var names []string
		for _, b := range d.Bindings {
			if b.Name != "" {
				names = append(names, b.Name)
			}
		}
		return names
	case *ast.TypeDecl:
		if d.Name != "" {
			return []string{d.Name}
		}
	case *ast.ExceptionDecl:
		if d.Name != "" {
			return []string{d.Name}
		}
	case *ast.EffectDecl:
		if d.Name != "" {
			return []string{d.Name}
		}
	case *ast.LangEmbedDecl:
		var names []string
		for _, v := range d.Vals {
			if v.Name != "" {
				names = append(names, v.Name)
			}
		}
		return names
	}
	return nil
}
