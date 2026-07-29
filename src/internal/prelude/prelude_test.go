package prelude_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/prelude"
	"goop.dev/compiler/internal/typecheck"
)

func TestPreludeBindings(t *testing.T) {
	p := prelude.Default()
	if p == nil {
		t.Fatal("expected non-nil prelude")
	}

	// Check that key bindings exist
	names := []string{"println", "print", "int_to_string", "float_to_string",
		"string_concat", "String.length", "String.sub", "list_length", "list_append", "failwith", "ref"}
	for _, name := range names {
		b := p.Lookup(name)
		if b == nil {
			t.Errorf("missing prelude binding: %s", name)
			continue
		}
		if b.Lowering.Func == "" && b.Lowering.Operator == "" && b.Lowering.Custom == "" {
			t.Errorf("prelude binding %s has no lowering", name)
		}
		if b.Scheme == nil {
			t.Errorf("prelude binding %s has no type scheme", name)
		}
	}
}

func TestPrintlnTypeCheck(t *testing.T) {
	src := `module Test
let main () =
  println "hello"
`
	mod := parseString(t, src)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestPrintlnCompiles(t *testing.T) {
	src := `module Test
let main () =
  println "hello"
`
	mod := parseString(t, src)
	gen := codegen.NewGenerator("test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.Contains(goSrc, "fmt.Println") {
		t.Error("expected fmt.Println in generated code")
	}
	if !strings.Contains(goSrc, `"fmt"`) {
		t.Error("expected fmt import in generated code")
	}

	// Verify it builds
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644)

	cmd := exec.Command("go", "build", outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

func TestIntToStringCompiles(t *testing.T) {
	src := `module Test
let main () =
  println (int_to_string 42)
`
	mod := parseString(t, src)
	gen := codegen.NewGenerator("test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.Contains(goSrc, "strconv.Itoa") {
		t.Error("expected strconv.Itoa in generated code")
	}
}

func TestStringConcatLowering(t *testing.T) {
	src := `module Test
let main () =
  println (string_concat "Hello, " "World!")
`
	mod := parseString(t, src)
	gen := codegen.NewGenerator("test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// string_concat a b should lower to a + b
	if !strings.Contains(goSrc, `"Hello, " +`) {
		t.Error("expected string concat lowering to + operator")
	}
}

func TestConsoleDotPrintlnBackCompat(t *testing.T) {
	src := `module Test
let main () =
  Console.println "hello"
`
	mod := parseString(t, src)
	gen := codegen.NewGenerator("test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.Contains(goSrc, "fmt.Println") {
		t.Error("expected fmt.Println (Console.println backward compat)")
	}
}

func TestPreludeShadowable(t *testing.T) {
	// User can define their own println
	src := `module Test
let println (s: string) = s
let main () =
  println "hello"
`
	mod := parseString(t, src)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func parseString(t *testing.T, src string) *ast.Module {
	t.Helper()
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return mod
}
