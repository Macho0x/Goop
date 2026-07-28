package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const newMainGoop = `(* Scaffolded by goop new. Next: goop check main.goop && goop build main.goop *)
module main

let main () =
  print_line "Hello, Goop!"
`

const newGoopToml = `# Goop project config — see docs/design/20-cli-artifacts.md
module_root = ""

[check]
exhaust_redundant = "warn"
exhaust_missing = "error"
effect_inference = true
concurrent = "error"
refinement_unproven = "warn"
discarded_result = "warn"
discarded_option = "warn"
unused = "warn"
private_in_public = "warn"
money_float = "warn"
verify_ffi = false
deadlock = "warn"
smt = false
`

func runNew(args []string) int {
	force := false
	dir := "."
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "Usage: goop new [dir] [--force]\n")
			fmt.Fprintf(os.Stderr, "  Create goop.toml + main.goop in dir (default: .)\n")
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "goop new: unknown flag %s\n", a)
				return 1
			}
			dir = a
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "goop new: %v\n", err)
		return 1
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goop new: %v\n", err)
		return 1
	}
	if len(entries) > 0 && !force {
		// Allow re-running only if target files are missing and dir has other junk? Plan: refuse non-empty unless --force.
		fmt.Fprintf(os.Stderr, "goop new: directory %q is not empty (use --force to overwrite scaffold files)\n", dir)
		return 1
	}

	tomlPath := filepath.Join(dir, "goop.toml")
	mainPath := filepath.Join(dir, "main.goop")
	if !force {
		if _, err := os.Stat(tomlPath); err == nil {
			fmt.Fprintf(os.Stderr, "goop new: %s already exists (use --force)\n", tomlPath)
			return 1
		}
		if _, err := os.Stat(mainPath); err == nil {
			fmt.Fprintf(os.Stderr, "goop new: %s already exists (use --force)\n", mainPath)
			return 1
		}
	}

	if err := os.WriteFile(tomlPath, []byte(newGoopToml), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "goop new: write %s: %v\n", tomlPath, err)
		return 1
	}
	if err := os.WriteFile(mainPath, []byte(newMainGoop), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "goop new: write %s: %v\n", mainPath, err)
		return 1
	}

	abs, _ := filepath.Abs(dir)
	fmt.Printf("created project in %s\n", abs)
	fmt.Printf("  %s\n", filepath.Join(abs, "goop.toml"))
	fmt.Printf("  %s\n", filepath.Join(abs, "main.goop"))
	fmt.Printf("next: cd %s && goop check main.goop && goop build main.goop\n", dir)
	return 0
}
