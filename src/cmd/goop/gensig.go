package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goop.dev/compiler/internal/gosiggen"
)

// runGenSig implements `goop gen-sig`.
//
// Usage:
//
//	goop gen-sig <import-path>          generate one package
//	goop gen-sig --curated              generate H5 curated set
//	goop gen-sig --smoke                generate smoke subset
//	goop gen-sig --out DIR ...          write under DIR instead of cache
//	goop gen-sig --project DIR ...      project root (for goop-sigs/ note)
//
// Default output: $GOOP_HOME/build/go-sigs/<pkg>.gosig
// Hand-curated overrides live in <project>/goop-sigs/ (checked at resolve time).
func runGenSig(args []string) int {
	var (
		outDir      string
		projectRoot string
		curated     bool
		smoke       bool
		pkgs        []string
	)

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "gen-sig: --out requires a directory\n")
				return 1
			}
			i++
			outDir = args[i]
		case "--project":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "gen-sig: --project requires a directory\n")
				return 1
			}
			i++
			projectRoot = args[i]
		case "--curated":
			curated = true
		case "--smoke":
			smoke = true
		case "-h", "--help":
			printGenSigUsage()
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "gen-sig: unknown flag %s\n", a)
				printGenSigUsage()
				return 1
			}
			pkgs = append(pkgs, a)
		}
	}

	switch {
	case curated:
		pkgs = append(pkgs, gosiggen.CuratedPackages...)
	case smoke:
		pkgs = append(pkgs, gosiggen.SmokePackages...)
	}

	if len(pkgs) == 0 {
		printGenSigUsage()
		return 1
	}

	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}

	home := gosiggen.GoopHome()
	opts := gosiggen.Options{}
	var failed int

	for _, importPath := range pkgs {
		path, res, err := gosiggen.GenerateAndWrite(importPath, opts, home, outDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", importPath, err)
			failed++
			continue
		}
		fmt.Printf("wrote %s (%d skipped", path, len(res.Skipped))
		if len(res.TODOH6) > 0 {
			fmt.Printf(", %d TODO(H6)", len(res.TODOH6))
		}
		fmt.Printf(")\n")

		// Note if a project override exists (does not overwrite it).
		if ov := gosiggen.OverridePath(projectRoot, importPath); ov != "" {
			if st, err := os.Stat(ov); err == nil && !st.IsDir() {
				fmt.Printf("  note: override present at %s (wins over cache)\n", ov)
			}
		}
	}

	if outDir == "" {
		fmt.Printf("cache root: %s\n", gosiggen.CacheDir(home))
	}
	if projectRoot != "" {
		fmt.Printf("override dir: %s\n", filepath.Join(projectRoot, gosiggen.OverrideDir))
	}

	if failed > 0 {
		return 1
	}
	return 0
}

func printGenSigUsage() {
	fmt.Fprintf(os.Stderr, `Usage: goop gen-sig [flags] <import-path>...
       goop gen-sig --smoke
       goop gen-sig --curated

Generate .gosig stubs from Go packages (H5 foundation).

Flags:
  --out DIR       write .gosig files under DIR (default: $GOOP_HOME/build/go-sigs)
  --project DIR   project root for goop-sigs/ override notes (default: cwd)
  --smoke         generate strings, fmt, errors, strconv
  --curated       generate the full H5 curated package list

Environment:
  GOOP_HOME       cache root (default: ~/.cache/goop)

Overrides:
  Place hand-curated files in <project>/goop-sigs/<pkg>.gosig to win over cache.
`)
}
