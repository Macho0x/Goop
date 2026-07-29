package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestDefaultResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	goPath, goPkg := cfg.ResolveImport("std.io")
	if goPath != "github.com/Macho0x/Goop/std/io" {
		t.Errorf("expected github.com/Macho0x/Goop/std/io, got %s", goPath)
	}
	if goPkg != "io" {
		t.Errorf("expected package io, got %s", goPkg)
	}
}

func TestCustomMapping(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Mappings["MyPkg"] = "github.com/me/mypkg"

	goPath, goPkg := cfg.ResolveImport("MyPkg")
	if goPath != "github.com/me/mypkg" {
		t.Errorf("expected github.com/me/mypkg, got %s", goPath)
	}
	if goPkg != "mypkg" {
		t.Errorf("expected package mypkg, got %s", goPkg)
	}
}

func TestProjectModuleResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModuleRoot = "github.com/example/project"

	goPath, goPkg := cfg.ResolveImport("Trading.OrderBook")
	if goPath != "github.com/example/project/trading/orderbook" {
		t.Errorf("expected project path, got %s", goPath)
	}
	if goPkg != "orderbook" {
		t.Errorf("expected package orderbook, got %s", goPkg)
	}
}

func TestLoadConfigFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "goop.toml")
	content := `
module_root = "github.com/test/demo"

[mappings]
"Std.IO" = "github.com/override/io"
"MyLib"  = "github.com/me/lib"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ModuleRoot != "github.com/test/demo" {
		t.Errorf("expected module_root, got %s", cfg.ModuleRoot)
	}

	// Custom mapping should override default
	goPath, goPkg := cfg.ResolveImport("Std.IO")
	if goPath != "github.com/override/io" {
		t.Errorf("expected override path, got %s", goPath)
	}
	if goPkg != "io" {
		t.Errorf("expected package io, got %s", goPkg)
	}

	// Custom mapping
	goPath, goPkg = cfg.ResolveImport("MyLib")
	if goPath != "github.com/me/lib" {
		t.Errorf("expected mylib path, got %s", goPath)
	}

	// Project module resolution
	goPath, goPkg = cfg.ResolveImport("App.Core")
	if goPath != "github.com/test/demo/app/core" {
		t.Errorf("expected project module path, got %s", goPath)
	}
	if goPkg != "core" {
		t.Errorf("expected package core, got %s", goPkg)
	}
}

func TestMissingConfigFile(t *testing.T) {
	cfg, err := config.LoadConfig("/nonexistent/goop.toml")
	if err != nil {
		t.Fatalf("should not error on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config")
	}
	// Default resolution should work
	goPath, _ := cfg.ResolveImport("std.io")
	if goPath != "github.com/Macho0x/Goop/std/io" {
		t.Errorf("expected default path, got %s", goPath)
	}
}

func TestImportFromGeneratedCode(t *testing.T) {
	// Create a Goop file with open statements
	c0Content := `module TestMod

import goop . "std.io"

let greet () =
  println "hi"
`
	srcFile := filepath.Join(t.TempDir(), "test.goop")
	os.WriteFile(srcFile, []byte(c0Content), 0644)

	// This test just verifies that the config resolution works for
	// the kind of module names used in open statements.
	cfg := config.DefaultConfig()
	goPath, goPkg := cfg.ResolveImport("Std.IO")
	if goPath == "" || goPkg == "" {
		t.Error("expected non-empty resolution")
	}
	_ = srcFile
}

func TestC0TomlAtProjectRoot(t *testing.T) {
	// Verify the project root goop.toml exists and can be loaded
	paths := []string{
		"../../../../goop.toml", // from src/internal/config/
	}
	found := false
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			cfg, err := config.LoadConfig(p)
			if err != nil {
				t.Fatalf("load project goop.toml: %v", err)
			}
			if cfg.ModuleRoot != "github.com/Macho0x/Goop" {
				t.Errorf("unexpected module_root: %s", cfg.ModuleRoot)
			}
			found = true
			break
		}
	}
	if !found {
		t.Skip("goop.toml not found from test directory")
	}
	_ = strings.Join
}

func TestCheckConfigSeverities(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "goop.toml")
	content := `
[check]
concurrent = "warn"
refinement_unproven = "off"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Check.Concurrent != config.SeverityWarn {
		t.Errorf("concurrent: got %q", cfg.Check.Concurrent)
	}
	if cfg.Check.RefinementUnproven != config.SeverityOff {
		t.Errorf("refinement_unproven: got %q", cfg.Check.RefinementUnproven)
	}
}
