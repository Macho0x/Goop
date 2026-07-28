package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNewCreatesScaffold(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "hello")
	if code := runNew([]string{target}); code != 0 {
		t.Fatalf("runNew exit %d", code)
	}
	for _, name := range []string{"goop.toml", "main.goop"} {
		p := filepath.Join(target, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Fatalf("empty %s", name)
		}
	}
	mainSrc, _ := os.ReadFile(filepath.Join(target, "main.goop"))
	if !strings.Contains(string(mainSrc), "module main") {
		t.Fatalf("missing module main: %s", mainSrc)
	}
}

func TestRunNewRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runNew([]string{dir}); code == 0 {
		t.Fatal("expected failure on non-empty dir")
	}
	if code := runNew([]string{dir, "--force"}); code != 0 {
		t.Fatalf("force should succeed, got %d", code)
	}
}
