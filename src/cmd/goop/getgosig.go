package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goop.dev/compiler/internal/gosiggen"
)

// runGetGoSig implements `goop get-go-sig <import-path>`.
// It generates a .gosig stub for any Go import path (stdlib or module),
// writes it under $GOOP_HOME/build/go-sigs/, and prints unrepresentable
// exports as warnings. Project overrides live in goop-sigs/ — see
// docs/design/23-go-sig-resolution.md.
func runGetGoSig(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(os.Stderr, "usage: goop get-go-sig <go-import-path>\n")
		fmt.Fprintf(os.Stderr, "  Generate a .gosig stub for a Go package and cache it under\n")
		fmt.Fprintf(os.Stderr, "  $GOOP_HOME/build/go-sigs/. Hand-curated overrides win via goop-sigs/.\n")
		return 1
	}

	importPath := strings.TrimSpace(args[0])
	if importPath == "" {
		fmt.Fprintf(os.Stderr, "usage: goop get-go-sig <go-import-path>\n")
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	loadDir := findGoModDir(cwd)

	home := goopHome()
	path, res, err := gosiggen.GenerateAndWrite(importPath, gosiggen.Options{
		LoadDir: loadDir,
	}, home, "")
	if err != nil {
		// Graceful failure when the generator / package load is unavailable.
		fmt.Fprintf(os.Stderr, "goop get-go-sig: %v\n", err)
		if isGeneratorUnavailable(err) {
			fmt.Fprintf(os.Stderr, "goop get-go-sig: generator not yet available\n")
		}
		return 1
	}

	fmt.Printf("wrote %s\n", path)
	if !gosiggen.IsCurated(importPath) {
		fmt.Fprintf(os.Stderr, "goop: warning: %q is outside the curated H5 set; mapping quality may vary\n", importPath)
	}

	printSigWarnings(res)

	if ov := gosiggen.OverridePath(cwd, importPath); ov != "" {
		if st, err := os.Stat(ov); err == nil && !st.IsDir() {
			fmt.Fprintf(os.Stderr, "goop: note: project override exists at %s (wins over cache at compile time)\n", ov)
		}
	}
	return 0
}

func printSigWarnings(res *gosiggen.Result) {
	if res == nil {
		return
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "goop: warning: %s\n", w)
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(os.Stderr, "goop: warning: unrepresentable %s %s — %s\n", s.Kind, s.Name, s.Reason)
		fmt.Fprintf(os.Stderr, "  hint: add a hand-curated stub under %s/ to override\n", gosiggen.OverrideDir)
	}
	if len(res.TODOH6) > 0 {
		fmt.Fprintf(os.Stderr, "goop: note: %d export(s) return (T, error); H6 will coerce to result T\n", len(res.TODOH6))
	}
}

func findGoModDir(start string) string {
	dir := start
	for dir != "" {
		if st, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start
}

func isGeneratorUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "generator not yet available") ||
		strings.Contains(msg, "not implemented")
}
