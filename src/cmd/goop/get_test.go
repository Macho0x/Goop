package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
)

func TestGetUsage(t *testing.T) {
	if runGet(nil) == 0 {
		t.Fatal("expected failure with no args")
	}
}

func TestLooksLikeGoImport(t *testing.T) {
	if !looksLikeGoImport("os") || !looksLikeGoImport("encoding/json") {
		t.Fatal("stdlib paths should be Go imports")
	}
	if !looksLikeGoImport("github.com/foo/bar") {
		t.Fatal("module path should be a Go import")
	}
	if looksLikeGoImport("std.list") {
		t.Fatal("std.list is a Goop module, not a Go import")
	}
	if looksLikeGitHost("os") || looksLikeGitHost("std.list") {
		t.Fatal("stdlib / std.* should not clone as git hosts")
	}
	if !looksLikeGitHost("github.com/foo/bar") {
		t.Fatal("github.com should look like a git host")
	}
}

func TestWriteTomlDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goop.toml")
	cfg := config.DefaultConfig()
	cfg.Dependencies = map[string]string{"github.com/acme/lib": "v1.0.0"}
	if err := writeTomlDependencies(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "github.com/acme/lib") {
		t.Fatalf("missing dependency: %s", data)
	}
}
