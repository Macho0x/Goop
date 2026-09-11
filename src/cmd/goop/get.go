package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"goop.dev/compiler/internal/config"
)

func runGet(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: goop get <module-path>[@version]  (Goop module or Go import path)\n")
		return 1
	}
	modPath := args[0]
	version := "main"
	if at := strings.LastIndex(modPath, "@"); at >= 0 {
		version = modPath[at+1:]
		modPath = modPath[:at]
	}

	cwd, _ := os.Getwd()
	if looksLikeGoImport(modPath) {
		sigArgs := []string{modPath}
		code := runGetGoSig(sigArgs)
		ensureConsumerGoMod(cwd, modPath)
		if !looksLikeGitHost(modPath) {
			if code != 0 {
				return code
			}
			fmt.Printf("added go sig %s\n", modPath)
			return 0
		}
	}
	cfgPath := filepath.Join(cwd, "goop.toml")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading goop.toml: %v\n", err)
		return 1
	}
	if cfg.Dependencies == nil {
		cfg.Dependencies = make(map[string]string)
	}
	cfg.Dependencies[modPath] = version

	dest := filepath.Join(goopHome(), "pkg", "mod", filepath.FromSlash(modPath))
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cache mkdir: %v\n", err)
		return 1
	}
	_ = os.RemoveAll(dest)
	clone := exec.Command("git", "clone", "--depth", "1", "--branch", version,
		fmt.Sprintf("https://%s", modPath), dest)
	clone.Stderr = os.Stderr
	if err := clone.Run(); err != nil {
		// fallback: clone default branch and checkout
		clone2 := exec.Command("git", "clone", "--depth", "1",
			fmt.Sprintf("https://%s", modPath), dest)
		clone2.Stderr = os.Stderr
		if err2 := clone2.Run(); err2 != nil {
			fmt.Fprintf(os.Stderr, "fetch %s: %v\n", modPath, err)
			return 1
		}
	}

	lockPath := filepath.Join(cwd, "goop.lock")
	lock, _ := config.LoadLockfile(lockPath)
	lock.Upsert(config.LockModule{
		Path:    modPath,
		Version: version,
		Source:  modPath,
	})
	if err := os.WriteFile(lockPath, []byte(lock.Format()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write goop.lock: %v\n", err)
		return 1
	}

	if err := writeTomlDependencies(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "update goop.toml: %v\n", err)
		return 1
	}
	fmt.Printf("added %s@%s\n", modPath, version)
	return 0
}

func writeTomlDependencies(path string, cfg *config.Config) error {
	data, err := os.ReadFile(path)
	var lines []string
	if err == nil {
		lines = strings.Split(string(data), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}
	out := make([]string, 0, len(lines)+4)
	inDeps := false
	depsWritten := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[dependencies]") {
			inDeps = true
			continue
		}
		if inDeps && strings.HasPrefix(trim, "[") {
			inDeps = false
		}
		if inDeps {
			continue
		}
		out = append(out, line)
	}
	if len(out) > 0 && out[len(out)-1] != "" {
		out = append(out, "")
	}
	out = append(out, "[dependencies]")
	for path, ver := range cfg.Dependencies {
		out = append(out, fmt.Sprintf("%q = %q", path, ver))
	}
	depsWritten = true
	_ = depsWritten
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0644)
}

func looksLikeGoImport(p string) bool {
	if p == "" || strings.HasPrefix(p, "std.") {
		return false
	}
	first := strings.Split(p, "/")[0]
	if strings.Contains(first, ".") {
		return true
	}
	return p == strings.ToLower(p)
}

func looksLikeGitHost(p string) bool {
	if strings.HasPrefix(p, "std.") {
		return false
	}
	first := strings.Split(p, "/")[0]
	return strings.Contains(first, ".")
}

func ensureConsumerGoMod(dir, importPath string) {
	if dir == "" {
		return
	}
	goMod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		cmd := exec.Command("go", "mod", "init", "goopmod")
		cmd.Dir = dir
		_ = cmd.Run()
	}
	if looksLikeGitHost(importPath) {
		cmd := exec.Command("go", "get", importPath)
		cmd.Dir = dir
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}
