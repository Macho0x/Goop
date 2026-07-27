package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
)

func resetCLIFlags() {
	cliInTree = false
	cliEmitMap = false
}

func withGOOPHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GOOP_HOME", home)
	return home
}

func writeHelloGoop(t *testing.T, dir string) string {
	t.Helper()
	src := `module main

let main () =
  print_line "ok"
`
	path := filepath.Join(dir, "hello.goop")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func parserParse(path string, src []byte) (*ast.Module, error) {
	mod, err := parser.Parse(path, src)
	if err != nil {
		return nil, err
	}
	return desugar.DesugarModule(mod), nil
}

func runCompileReal(t *testing.T, path string) error {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg := loadProjectConfig(path)
	return runCompile(path, src, cfg, func() (*ast.Module, error) {
		return parserParse(path, src)
	})
}

func runBuildReal(t *testing.T, path string) error {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg := loadProjectConfig(path)
	return runBuild(path, src, cfg, func() (*ast.Module, error) {
		return parserParse(path, src)
	})
}

func TestCompileCacheDefault(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	out := captureStdout(t, func() {
		if err := runCompileReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "wrote ") {
		t.Fatalf("expected wrote path in stdout, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); !os.IsNotExist(err) {
		t.Fatalf("expected no in-tree main.go, err=%v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.map.json"))
	if len(matches) > 0 {
		t.Fatalf("expected no map files in tree, got %v", matches)
	}
	if !strings.Contains(out, "build") {
		t.Fatalf("expected cache build path, got %q", out)
	}
}

func TestCompileInTree(t *testing.T) {
	resetCLIFlags()
	cliInTree = true
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	out := captureStdout(t, func() {
		if err := runCompileReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, filepath.Join(dir, "main.go")) {
		t.Fatalf("expected in-tree write, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go.map.json")); !os.IsNotExist(err) {
		t.Fatal("expected no map without --emit-map")
	}
}

func TestCompileEmitMap(t *testing.T) {
	resetCLIFlags()
	cliInTree = true
	cliEmitMap = true
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	_ = captureStdout(t, func() {
		if err := runCompileReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "main.go.map.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCompileNoSourceMapCompat(t *testing.T) {
	resetCLIFlags()
	cliInTree = true
	cliEmitMap = false
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	_ = captureStdout(t, func() {
		if err := runCompileReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "main.go.map.json")); !os.IsNotExist(err) {
		t.Fatal("expected no map when emit disabled")
	}
}

func TestBuildCacheNoTreePollution(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runBuildReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "main.go")); !os.IsNotExist(err) {
		t.Fatal("expected no .go in source dir")
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); !os.IsNotExist(err) {
		t.Fatal("expected no go.mod leak in source dir")
	}
	if _, err := os.Stat(goopOutBinary(work)); err != nil {
		t.Fatalf("expected goop-out in cwd: %v\n%s", err, out)
	}
}

func TestBuildMainProducesBinary(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t, func() {
		if err := runBuildReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	cmd := exec.Command(goopOutBinary(work))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestBuildDoesNotLeakGoMod(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	path := writeHelloGoop(t, dir)
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t, func() {
		if err := runBuildReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); !os.IsNotExist(err) {
		t.Fatal("go.mod leaked into source dir")
	}
}

func TestBuildWithGoopDeps(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	root := t.TempDir()
	toml := `module_root = "example.com/demo"
`
	if err := os.WriteFile(filepath.Join(root, "goop.toml"), []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	libSrc := `module lib

let greet (name: string) : string = "hi " ^ name
`
	if err := os.WriteFile(filepath.Join(libDir, "lib.goop"), []byte(libSrc), 0644); err != nil {
		t.Fatal(err)
	}
	mainSrc := `module main

import goop . "example.com/demo/lib"

let main () =
  print_line (greet "world")
`
	mainPath := filepath.Join(root, "main.goop")
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runBuildReal(t, mainPath); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(root, "main.go")); !os.IsNotExist(err) {
		t.Fatal("expected no in-tree pollution")
	}
	bin := goopOutBinary(work)
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("binary missing: %v\n%s", err, out)
	}
	runOut, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, runOut)
	}
	if !strings.Contains(string(runOut), "hi world") {
		t.Fatalf("got %q", runOut)
	}
}

func TestBuildLibraryNoMain(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	src := `module utils

let double (x: int) : int = x + x
`
	path := filepath.Join(dir, "utils.goop")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t, func() {
		if err := runBuildReal(t, path); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(goopOutBinary(work)); !os.IsNotExist(err) {
		t.Fatal("library build should not produce goop-out")
	}
	if _, err := os.Stat(filepath.Join(dir, "utils.go")); !os.IsNotExist(err) {
		t.Fatal("expected no in-tree .go")
	}
}

func TestTestStillPasses(t *testing.T) {
	resetCLIFlags()
	_ = withGOOPHome(t)
	dir := t.TempDir()
	src := `module main

let main () =
  begin
    if 1 + 1 = 2 then () else failwith "math broken";
    ()
  end
`
	if err := os.WriteFile(filepath.Join(dir, "smoke_test.goop"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	code := runTests(dir)
	if code != 0 {
		t.Fatalf("runTests exit %d", code)
	}
}

func TestInTreeMixedBuild(t *testing.T) {
	resetCLIFlags()
	cliInTree = true
	_ = withGOOPHome(t)
	dir := t.TempDir()
	goopSrc := `module main

import go "fmt" {
  val Println : string -> unit
}

let main () = Println "from-goop"
`
	if err := os.WriteFile(filepath.Join(dir, "main.goop"), []byte(goopSrc), 0644); err != nil {
		t.Fatal(err)
	}
	helper := `package main

func init() {}
`
	if err := os.WriteFile(filepath.Join(dir, "helper.go"), []byte(helper), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module mixed\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	_ = captureStdout(t, func() {
		if err := runBuildReal(t, filepath.Join(dir, "main.goop")); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatal("expected generated main.go in-tree for mixed build")
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
}

func TestDepRelPathFullImport(t *testing.T) {
	p := depRelPath("github.com/Macho0x/treelog/sanitize")
	if !strings.Contains(p, "treelog") || !strings.Contains(p, "sanitize") {
		t.Fatalf("unexpected rel path %q", p)
	}
}

func TestNewBuildDirUnderGOOPHome(t *testing.T) {
	home := withGOOPHome(t)
	dir, err := newBuildDir("compile-*")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dir, filepath.Join(home, "build")) {
		t.Fatalf("expected under %s/build, got %s", home, dir)
	}
}
