package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/effects"
	"goop.dev/compiler/internal/modresolve"
	"goop.dev/compiler/internal/parser"
)

// goopHome returns $GOOP_HOME or ~/.cache/goop.
func goopHome() string {
	if v := os.Getenv("GOOP_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "goop")
}

// exeName returns base with a .exe suffix on Windows so go build -o and
// exec.Command agree on the binary path.
func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// goopOutBinary is the cwd-relative name of the binary produced by goop build.
// On Windows, go build appends .exe when -o has no extension; name it explicitly.
func goopOutBinary(cwd string) string {
	return filepath.Join(cwd, exeName("goop-out"))
}

// buildCacheRoot returns $GOOP_HOME/build.
func buildCacheRoot() string {
	return filepath.Join(goopHome(), "build")
}

// newBuildDir creates a unique temporary directory under the build cache root.
func newBuildDir(prefix string) (string, error) {
	root := buildCacheRoot()
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("build cache mkdir: %w", err)
	}
	dir, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return "", err
	}
	return dir, nil
}

// writeGoopPackage writes generated Go source into dir.
func writeGoopPackage(dir, goSrc, goFile string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, goFile), []byte(goSrc), 0644)
}

// writeSourceMapFile writes a source map next to the generated Go file when sm is non-nil.
func writeSourceMapFile(outPath string, sm *codegen.SourceMap) error {
	if sm == nil {
		return nil
	}
	mapPath := outPath + ".map.json"
	f, err := os.Create(mapPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := sm.Write(f); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", mapPath)
	return nil
}

// depRelPath returns a filesystem-safe relative path for a Go import path under deps/.
func depRelPath(goImportPath string) string {
	return filepath.FromSlash(goImportPath)
}

func compileGoopModuleToGo(goopPath string, cfg *config.Config) (string, string, error) {
	src, err := os.ReadFile(goopPath)
	if err != nil {
		return "", "", err
	}
	mod, err := parser.Parse(goopPath, src)
	if err != nil {
		return "", "", err
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, typeErrs := typecheckModule(mod, goopPath, cfg)
	if len(typeErrs) > 0 {
		return "", "", fmt.Errorf("type errors in %s: %v", goopPath, typeErrs[0])
	}
	mod = effects.TransformCPS(mod)
	gen := codegen.NewGenerator(goopPath, cfg)
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		return "", "", err
	}
	return goSrc, gen.GoFileName(), nil
}

// writeImportDependencies compiles transitive import goop deps into tmpDir/deps/<import-path>/
// and returns a go.mod body with require/replace directives.
// moduleName is the go.mod module line for the root package (e.g. "test" or "goopbuild").
func writeImportDependencies(mod *ast.Module, cfg *config.Config, projectRoot, tmpDir, moduleName string) (string, error) {
	if moduleName == "" {
		moduleName = "goopbuild"
	}
	minimal := fmt.Sprintf("module %s\n\ngo 1.22\n", moduleName)
	if projectRoot == "" {
		return minimal, nil
	}
	lock, _ := config.LoadLockfile(filepath.Join(projectRoot, "goop.lock"))
	r := modresolve.New(cfg, lock, projectRoot)
	sources := make(map[string]string)

	var collect func(*ast.Module) error
	collect = func(m *ast.Module) error {
		for _, spec := range m.Imports {
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
			if _, ok := sources[resolved.SourceFile]; ok {
				continue
			}
			sources[resolved.SourceFile] = resolved.GoImportPath
			src, err := os.ReadFile(resolved.SourceFile)
			if err != nil {
				return err
			}
			depMod, err := parser.Parse(resolved.SourceFile, src)
			if err != nil {
				return err
			}
			depMod = desugar.DesugarModule(depMod)
			if err := collect(depMod); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(mod); err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return minimal, nil
	}

	var replaces []string
	var requires []string
	for goopPath, goPath := range sources {
		goSrc, goFile, err := compileGoopModuleToGo(goopPath, cfg)
		if err != nil {
			return "", err
		}
		rel := depRelPath(goPath)
		depDir := filepath.Join(tmpDir, "deps", rel)
		if err := os.MkdirAll(depDir, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(depDir, goFile), []byte(goSrc), 0644); err != nil {
			return "", err
		}
		depModContent := fmt.Sprintf("module %s\n\ngo 1.22\n", goPath)
		if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte(depModContent), 0644); err != nil {
			return "", err
		}
		replaces = append(replaces, fmt.Sprintf("replace %s => ./deps/%s", goPath, filepath.ToSlash(rel)))
		requires = append(requires, fmt.Sprintf("require %s v0.0.0", goPath))
	}
	goMod := minimal
	if len(requires) > 0 {
		goMod += "\n" + strings.Join(requires, "\n") + "\n"
	}
	if len(replaces) > 0 {
		goMod += "\n" + strings.Join(replaces, "\n") + "\n"
	}
	return goMod, nil
}

// tidyGoMod runs `go mod tidy` in dir so third-party `import go` packages
// (e.g. github.com/shopspring/decimal) resolve in the cache sandbox.
func tidyGoMod(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod tidy failed:\n%s", out)
	}
	return nil
}

// hasMainFunc reports whether generated Go source defines func main.
func hasMainFunc(goSrc string) bool {
	return strings.Contains(goSrc, "func main(") || strings.Contains(goSrc, "func main ()")
}
