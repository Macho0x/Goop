// Package modresolve resolves Goop import paths and loads transitive module graphs.
package modresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
)

const (
	goopProjectImportPrefix = "github.com/Macho0x/Goop/"
	SourceExt               = ".goop"
	TestGlob                = "*_test.goop"
)

// Resolver resolves import paths and loads Goop modules from disk or cache.
type Resolver struct {
	Cfg         *config.Config
	Lock        *config.Lockfile
	ProjectRoot string
	CacheDir    string
}

// New creates a module resolver.
func New(cfg *config.Config, lock *config.Lockfile, projectRoot string) *Resolver {
	cache := os.Getenv("GOOP_HOME")
	if cache == "" {
		home, _ := os.UserHomeDir()
		cache = filepath.Join(home, ".cache", "goop")
	}
	return &Resolver{
		Cfg:         cfg,
		Lock:        lock,
		ProjectRoot: projectRoot,
		CacheDir:    filepath.Join(cache, "pkg", "mod"),
	}
}

// FindProjectRoot walks upward from srcFile looking for goop.toml or std/.
func FindProjectRoot(srcFile string) string {
	dir, err := filepath.Abs(filepath.Dir(srcFile))
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "goop.toml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "std")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ResolvedPath holds the result of resolving a Goop import path string.
type ResolvedPath struct {
	LogicalPath  string
	GoImportPath string
	PkgName      string
	SourceFile   string // path to .goop entry file
}

// ResolveGoopPath maps a logical or canonical Goop path to Go import path and source file.
func (r *Resolver) ResolveGoopPath(logicalPath string) (ResolvedPath, error) {
	var res ResolvedPath
	res.LogicalPath = logicalPath

	goPath, pkg := r.resolveGoImport(logicalPath)
	res.GoImportPath = goPath
	res.PkgName = pkg

	if src := r.locateSourceFile(goPath); src != "" {
		res.SourceFile = src
	}
	return res, nil
}

func (r *Resolver) resolveGoImport(path string) (goImportPath, pkgName string) {
	if r.Cfg == nil {
		r.Cfg = config.DefaultConfig()
	}
	// Canonical github path
	if strings.Contains(path, "/") && !strings.HasPrefix(path, "std.") {
		pkg := packageNameFromPath(path)
		if depVer, ok := r.Cfg.Dependencies[path]; ok && r.Lock != nil {
			if m, ok := r.Lock.Lookup(path); ok && m.Source != "" {
				_ = depVer
				return m.Source, packageNameFromPath(m.Source)
			}
		}
		return path, pkg
	}
	// Logical path via mappings / defaults
	if mapped, ok := r.Cfg.Mappings[path]; ok {
		return mapped, packageNameFromPath(mapped)
	}
	return r.Cfg.ResolveImport(path)
}

func packageNameFromPath(path string) string {
	segments := strings.Split(path, "/")
	name := segments[len(segments)-1]
	return strings.ReplaceAll(name, "-", "_")
}

func (r *Resolver) locateSourceFile(goImport string) string {
	if r.ProjectRoot != "" {
		if local := localGoopPathForImport(r.ProjectRoot, goImport); local != "" {
			if _, err := os.Stat(local); err == nil {
				return local
			}
		}
		if r.Cfg != nil && r.Cfg.ModuleRoot != "" && strings.HasPrefix(goImport, r.Cfg.ModuleRoot) {
			rel := strings.TrimPrefix(goImport, r.Cfg.ModuleRoot)
			rel = strings.TrimPrefix(rel, "/")
			var p string
			if rel == "" {
				// Root package: github.com/acme/foo → acme/foo/treelog.goop at project root
				p = filepath.Join(r.ProjectRoot, packageNameFromPath(goImport)+SourceExt)
			} else {
				pkg := filepath.Base(rel)
				p = filepath.Join(r.ProjectRoot, filepath.FromSlash(rel), pkg+SourceExt)
			}
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if r.Lock != nil {
		if m, ok := r.Lock.Lookup(goImport); ok {
			cached := filepath.Join(r.CacheDir, filepath.FromSlash(m.Source), packageNameFromPath(m.Source)+".goop")
			if _, err := os.Stat(cached); err == nil {
				return cached
			}
		}
	}
	cached := filepath.Join(r.CacheDir, filepath.FromSlash(goImport), packageNameFromPath(goImport)+".goop")
	if _, err := os.Stat(cached); err == nil {
		return cached
	}
	return ""
}

func localGoopPathForImport(projectRoot, goImport string) string {
	if !strings.HasPrefix(goImport, goopProjectImportPrefix) {
		return ""
	}
	rel := strings.TrimPrefix(goImport, goopProjectImportPrefix)
	pkg := filepath.Base(rel)
	return filepath.Join(projectRoot, filepath.FromSlash(rel), pkg+SourceExt)
}

// LoadModuleGraph loads the entry module and all transitive import goop dependencies.
// Returns map keyed by canonical Go import path.
func (r *Resolver) LoadModuleGraph(entryFile string, entry *ast.Module) (map[string]*ast.Module, error) {
	graph := make(map[string]*ast.Module)
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var load func(file string, mod *ast.Module, goKey string) error
	load = func(file string, mod *ast.Module, goKey string) error {
		key := goKey
		if key == "" {
			key = file
		}
		if visiting[key] {
			return fmt.Errorf("circular import detected at %s", key)
		}
		if visited[key] {
			return nil
		}
		visiting[key] = true
		if goKey != "" {
			graph[goKey] = mod
		}
		for _, spec := range mod.Imports {
			if spec.Kind != ast.ImportGoop {
				continue
			}
			resolved, err := r.ResolveGoopPath(spec.Path)
			if err != nil {
				return err
			}
			if resolved.SourceFile == "" {
				return fmt.Errorf("module %q not found", spec.Path)
			}
			if visited[resolved.GoImportPath] {
				continue
			}
			src, err := os.ReadFile(resolved.SourceFile)
			if err != nil {
				return fmt.Errorf("reading %s: %w", resolved.SourceFile, err)
			}
			depMod, err := parser.Parse(resolved.SourceFile, src)
			if err != nil {
				return fmt.Errorf("parse %s: %w", resolved.SourceFile, err)
			}
			depMod = desugar.DesugarModule(depMod)
			if err := load(resolved.SourceFile, depMod, resolved.GoImportPath); err != nil {
				return err
			}
		}
		delete(visiting, key)
		visited[key] = true
		return nil
	}

	if err := load(entryFile, entry, ""); err != nil {
		return nil, err
	}
	return graph, nil
}

// ExportNames returns non-private top-level binding names from a module.
func ExportNames(mod *ast.Module) []string {
	if mod == nil {
		return nil
	}
	var names []string
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			if d.Private {
				continue
			}
			for _, b := range d.Bindings {
				names = append(names, b.Name)
			}
		case *ast.TypeDecl:
			if d.Private {
				continue
			}
			names = append(names, d.Name)
		}
	}
	return names
}

// ImportAlias returns the local name for an import spec (default package name or explicit alias).
func ImportAlias(spec ast.ImportSpec, resolved ResolvedPath) string {
	if spec.Alias == "." {
		return "."
	}
	if spec.Alias != "" {
		return spec.Alias
	}
	return resolved.PkgName
}
