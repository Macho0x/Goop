package gosiggen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// OverrideDir is the project-level directory where hand-curated .gosig
	// files win over generated cache entries.
	OverrideDir = "goop-sigs"

	// CacheSubdir is under $GOOP_HOME/build/.
	CacheSubdir = "go-sigs"
)

// GoopHome returns $GOOP_HOME or ~/.cache/goop.
func GoopHome() string {
	if v := os.Getenv("GOOP_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "goop")
}

// CacheDir returns $GOOP_HOME/build/go-sigs (or overrideRoot if non-empty).
func CacheDir(goopHome string) string {
	if goopHome == "" {
		goopHome = GoopHome()
	}
	return filepath.Join(goopHome, "build", CacheSubdir)
}

// SigFileName turns an import path into a filesystem-safe .gosig basename.
// e.g. "encoding/json" → "encoding_json.gosig", "strings" → "strings.gosig".
func SigFileName(importPath string) string {
	safe := strings.ReplaceAll(importPath, "/", "_")
	safe = strings.ReplaceAll(safe, ".", "_")
	return safe + ".gosig"
}

// CachePath is where a generated sig is written for importPath.
func CachePath(goopHome, importPath string) string {
	return filepath.Join(CacheDir(goopHome), SigFileName(importPath))
}

// OverridePath is the project-level hand-curated sig path, if projectRoot set.
func OverridePath(projectRoot, importPath string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, OverrideDir, SigFileName(importPath))
}

// ResolveSigPath returns the sig path that should be used at compile time:
// project goop-sigs/ override wins over the generated cache.
// exists is true when the winning file is present on disk.
func ResolveSigPath(projectRoot, goopHome, importPath string) (path string, fromOverride, exists bool) {
	if ov := OverridePath(projectRoot, importPath); ov != "" {
		if st, err := os.Stat(ov); err == nil && !st.IsDir() {
			return ov, true, true
		}
	}
	cp := CachePath(goopHome, importPath)
	if st, err := os.Stat(cp); err == nil && !st.IsDir() {
		return cp, false, true
	}
	return cp, false, false
}

// WriteSig writes content to path, creating parent directories as needed.
func WriteSig(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
