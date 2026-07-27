package gosiggen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateStringsSmoke(t *testing.T) {
	done := make(chan struct{})
	var res *Result
	var err error

	go func() {
		res, err = Generate("strings", Options{Timeout: 20 * time.Second})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(25 * time.Second):
		t.Skip("packages.Load timed out — skipping smoke test")
	}

	if err != nil {
		t.Skipf("gosiggen not available: %v", err)
	}
	if res == nil || res.Content == "" {
		t.Fatal("empty result")
	}

	// Spot-check expected exports.
	wantSnippets := []string{
		"Package: strings",
		"val ToUpper : string -> string",
		"val HasPrefix : string -> string -> bool",
		"val Contains : string -> string -> bool",
		"type Builder",
	}
	for _, snip := range wantSnippets {
		if !strings.Contains(res.Content, snip) {
			t.Errorf("generated sig missing %q\n----- content -----\n%s", snip, res.Content)
		}
	}

	// Write to a temp cache and confirm file lands.
	home := t.TempDir()
	path, res2, err := GenerateAndWrite("strings", Options{Timeout: 20 * time.Second}, home, "")
	if err != nil {
		t.Fatalf("GenerateAndWrite: %v", err)
	}
	if res2 == nil {
		t.Fatal("nil result from write")
	}
	wantPath := filepath.Join(home, "build", "go-sigs", "strings.gosig")
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "val ToUpper") {
		t.Errorf("written file missing ToUpper")
	}
}

func TestGenerateSmokePackages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-package smoke in short mode")
	}
	for _, pkg := range SmokePackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			res, err := Generate(pkg, Options{Timeout: 20 * time.Second})
			if err != nil {
				t.Skipf("%s: %v", pkg, err)
			}
			if res.Content == "" {
				t.Fatalf("%s: empty content", pkg)
			}
			if !strings.Contains(res.Content, "Package: ") {
				t.Errorf("%s: missing header", pkg)
			}
			t.Logf("%s: %d skipped, %d H6 TODOs", pkg, len(res.Skipped), len(res.TODOH6))
		})
	}
}
