package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
	"goop.dev/compiler/internal/types"
)

func TestMapParseAndTypecheck(t *testing.T) {
	src := `module Main

let main () =
  let m : map[string] int = Map.make () in
  let _ = Map.add m "a" 1 in
  let _ = assert (Map.size m = 1) in
  let _ = assert (Map.mem m "a") in
  match Map.get m "a" with
  | Some n -> assert (n = 1)
  | None -> assert false
`
	mod, err := parser.Parse("map_test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatalf("typecheck: %v", errs)
	}
	foundMap := false
	for _, typ := range tm {
		if _, ok := typ.(*types.TMap); ok {
			foundMap = true
			break
		}
	}
	if !foundMap {
		for _, typ := range vtm {
			if _, ok := typ.(*types.TMap); ok {
				foundMap = true
				break
			}
		}
	}
	if !foundMap {
		t.Fatal("expected a TMap in type maps")
	}

	gen := codegen.NewGenerator("map_test.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "map[string]int") {
		t.Fatalf("expected map[string]int in Go output:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "make(map[string]int)") {
		t.Fatalf("expected make(map[string]int):\n%s", goSrc)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "map.go")
	if err := os.WriteFile(outPath, []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(tmpDir, exeName("testbin"))
	cmd := exec.Command("go", "build", "-o", binPath, outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s\nGo source:\n%s", err, out, goSrc)
	}
	out, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
}
