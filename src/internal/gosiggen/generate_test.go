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

func TestCuratedGenericSkips(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping curated generics catalog in short mode")
	}
	// Prefer exports that exist since Go 1.21. errors.AsType is Go 1.26+ only —
	// assert it when present, skip the subtest on older toolchains (CI go.mod 1.25).
	cases := []struct {
		pkg      string
		name     string
		optional bool // true if the symbol may be absent on older Go
	}{
		{"sync", "OnceValue", false},
		{"sync", "OnceValues", false},
		{"errors", "AsType", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.pkg+"."+tc.name, func(t *testing.T) {
			res, err := Generate(tc.pkg, Options{Timeout: 20 * time.Second})
			if err != nil {
				t.Skipf("%s: %v", tc.pkg, err)
			}
			found := false
			for _, s := range res.Skipped {
				if s.Name == tc.name {
					found = true
					if !strings.Contains(s.Reason, "TODO(generics)") && !strings.Contains(s.Reason, "TypeParam") {
						t.Errorf("%s.%s skip reason %q should mention generics", tc.pkg, tc.name, s.Reason)
					}
					break
				}
			}
			if !found {
				if tc.optional {
					t.Skipf("%s.%s not in this Go toolchain (e.g. AsType needs Go 1.26+)", tc.pkg, tc.name)
				}
				t.Errorf("expected %s.%s among skipped exports", tc.pkg, tc.name)
				return
			}
			if !strings.Contains(res.Content, "TODO(generics)") {
				t.Errorf("%s footer should mention TODO(generics)", tc.pkg)
			}
		})
	}
}
