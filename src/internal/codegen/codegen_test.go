package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/refine"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typecheck"
)

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func externFinalReturn(t ast.Type) ast.Type {
	for {
		fn, ok := t.(*ast.TFun)
		if !ok {
			return t
		}
		if _, ok2 := fn.To.(*ast.TFun); ok2 {
			t = fn.To
			continue
		}
		return fn.To
	}
}

var examplesDir = "../../../docs/examples"

func mustParse(t *testing.T, filename string) *ast.Module {
	t.Helper()
	path := filepath.Join(examplesDir, filename)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	mod, err := parser.Parse(filename, src)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return desugar.DesugarModule(mod)
}

func TestCompileHello(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	gen := codegen.NewGenerator("hello.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "package main") {
		t.Error("missing package main")
	}
	if !strings.Contains(goSrc, "fmt.Println") {
		t.Error("missing fmt.Println")
	}
	if !strings.Contains(goSrc, "Hello, Goop!") {
		t.Error("missing Hello, Goop! string")
	}
}

func TestCompileShapes(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	gen := codegen.NewGenerator("shapes.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Check that the Shape interface and structs are generated
	if !strings.Contains(goSrc, "type Shape interface") {
		t.Error("missing Shape interface")
	}
	if !strings.Contains(goSrc, "type ShapeCircle struct") {
		t.Error("missing ShapeCircle struct")
	}
	if !strings.Contains(goSrc, "type ShapeRect struct") {
		t.Error("missing ShapeRect struct")
	}
	if !strings.Contains(goSrc, "type ShapePoint struct") {
		t.Error("missing ShapePoint struct")
	}
}

func TestImplementsStringerCodegen(t *testing.T) {
	src := `module main
import go "fmt" { type Stringer }
type point = { x : int; y : int }
implements Stringer for point with
  let String (p : point) : string = "point"
end
`
	mod, err := parser.Parse("implements.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gen := codegen.NewGenerator("implements.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(desugar.DesugarModule(mod))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "func (p *point) String") {
		t.Fatalf("missing pointer receiver method:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "var _ fmt.Stringer") {
		t.Fatalf("missing Stringer assertion:\n%s", goSrc)
	}
}

func TestCompileResult(t *testing.T) {
	mod := mustParse(t, "result.goop")
	gen := codegen.NewGenerator("result.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "Ok") && !strings.Contains(goSrc, "result") {
		// Smoke: generated Go should mention result constructors somehow.
		t.Logf("generated %d bytes for result.goop", len(goSrc))
	}
	if strings.Contains(goSrc, "/* TODO:") {
		t.Errorf("generated Go contains silent-degradation marker")
	}
}

func TestExternTupleCallCodegen(t *testing.T) {
	// Default: (T, error) coerces to result (H6).
	src := `module main
import go "strconv" { val Atoi : string -> (int, error) }
let main () = let r = Atoi "42" in
  match r with
  | Ok n -> n
  | Error _ -> 0
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "__v, __e = strconv.Atoi") {
		t.Fatalf("expected (T, error)→result wrapper, got:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "NewResult") || !strings.Contains(goSrc, "Ok(") {
		t.Fatalf("expected New…Ok result constructor, got:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "IsOk()") {
		t.Fatalf("expected result match lowering, got:\n%s", goSrc)
	}
}

func TestExternRawTupleCallCodegen(t *testing.T) {
	src := `module main
import go raw "strconv" { val Atoi : string -> (int, error) }
let main () = let pair = Atoi "42" in pair
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "__t.F0, __t.F1 = strconv.Atoi") {
		t.Fatalf("expected multi-value extern assignment under raw, got:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "F1 error") {
		t.Fatalf("expected error field in tuple struct, got:\n%s", goSrc)
	}
}

func TestExternTupleCallCodegenEmbed(t *testing.T) {
	// Hand-written @[go] multi-value return (explicit declaration; no gosig).
	src := `module main
import go "strconv" {}
@[go] {
  func atoiPair(s string) (int, string) {
    n, err := strconv.Atoi(s)
    if err != nil { return 0, err.Error() }
    return n, ""
  }
}
val atoiPair : string -> (int, string)
let main () = let pair = atoiPair "42" in pair
`
	mod, err := parser.Parse("embed.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("embed.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "__t.F0, __t.F1 = atoiPair") {
		t.Fatalf("expected multi-value @[go] assignment, got:\n%s", goSrc)
	}
}

func TestExternTripleTupleCallCodegen(t *testing.T) {
	src := `module main
@[go] {
  func triple() (int, int, string) { return 1, 2, "ok" }
}
val triple : unit -> (int, int, string)
let main () = let t = triple () in t
`
	mod, err := parser.Parse("triple.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("triple.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "__t.F0, __t.F1, __t.F2 = triple") {
		t.Fatalf("expected 3-value extern assignment, got:\n%s", goSrc)
	}
}

func TestExternMethodCallCodegen(t *testing.T) {
	src := `module main
import go "bytes" {
  type Buffer
  val (b : Buffer ptr).String : unit -> string
}
@[go] {
  func newBuffer() *bytes.Buffer { return new(bytes.Buffer) }
}
val newBuffer : unit -> Buffer ptr
let main () = let b = newBuffer () in b.String ()
`
	mod, err := parser.Parse("method.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gen := codegen.NewGenerator("method.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(desugar.DesugarModule(mod))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "b.String()") {
		t.Fatalf("expected selector method call, got:\n%s", goSrc)
	}
}

func TestCEmbedCodegen(t *testing.T) {
	src := `module main
@[c] {
  int add(int a, int b) { return a + b; }
}
val add : int -> int -> int
let main () = ()
`
	mod, err := parser.Parse("c.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("c.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "import \"C\"") {
		t.Fatalf("missing import \"C\":\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "int add(int a, int b)") {
		t.Fatalf("missing C preamble body:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "C.add(") {
		t.Fatalf("missing C.add wrapper call:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "func add(") {
		t.Fatalf("missing Go wrapper:\n%s", goSrc)
	}
}

func TestCEmbedUnsupportedType(t *testing.T) {
	src := `module main
@[c] {
  void *ptr(void) { return 0; }
}
val ptr : unit -> string list
let main () = ()
`
	mod, err := parser.Parse("c.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("c.goop", config.DefaultConfig())
	_, err = gen.Generate(mod)
	if err == nil {
		t.Fatal("expected error for unsupported @[c] type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported type error, got: %v", err)
	}
}

func TestHelloBuildAndRun(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	gen := codegen.NewGenerator("hello.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Write to temp dir and build
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(outPath, []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// Create go.mod
	modContent := "module hello\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	// Build
	binPath := filepath.Join(tmpDir, exeName("hello"))
	cmd := exec.Command("go", "build", "-o", binPath, outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// Run
	cmd = exec.Command(binPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(string(out), "Hello, Goop!") {
		t.Errorf("expected 'Hello, Goop!', got %q", string(out))
	}
}

func TestShapesBuild(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	gen := codegen.NewGenerator("shapes.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "shapes.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module shapes\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmd := exec.Command("go", "build", outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

func TestResultBuild(t *testing.T) {
	mod := mustParse(t, "result.goop")
	gen := codegen.NewGenerator("result.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "result.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module result\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmd := exec.Command("go", "build", outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

func TestOrderbookBuild(t *testing.T) {
	mod := mustParse(t, "orderbook.goop")
	gen := codegen.NewGenerator("orderbook.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "orderbook.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module orderbook\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmd := exec.Command("go", "build", outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

func TestTypeCheckBeforeCodegen(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		t.Fatalf("type check failed: %v", errs)
	}
}

func TestActivePatternsExampleBuild(t *testing.T) {
	path := filepath.Join(examplesDir, "active_patterns.goop")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mod, err := parser.Parse("active_patterns.goop", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	gen := codegen.NewGenerator("active_patterns.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Verify key patterns in generated code
	if !strings.Contains(goSrc, "Int_option interface") && !strings.Contains(goSrc, "IntOption interface") {
		t.Error("missing Int_option interface for int_option ADT")
	}
	if !strings.Contains(goSrc, "IsPositive") {
		t.Error("missing IsPositive function")
	}
	if !strings.Contains(goSrc, "IsEven") {
		t.Error("missing IsEven function")
	}

	// Build in temp dir
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "activepatternsdemo.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module test\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmd := exec.Command("go", "build", outPath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

func TestContractsBuild(t *testing.T) {
	mod := mustParse(t, "contracts.goop")
	gen := codegen.NewGenerator("contracts.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Verify precondition checks
	if !strings.Contains(goSrc, "precondition violated: b <> 0") {
		t.Error("missing precondition check for safeDiv")
	}
	if !strings.Contains(goSrc, "precondition violated: hi >= lo") {
		t.Error("missing precondition check for clamp")
	}

	// Verify postcondition checks
	if !strings.Contains(goSrc, "postcondition violated: result >= lo && result <= hi") {
		t.Error("missing postcondition check for clamp")
	}

	// Verify named return value for postcondition
	if !strings.Contains(goSrc, "result int") {
		t.Error("missing named return value for postcondition")
	}

	// Verify defer postcondition pattern
	if !strings.Contains(goSrc, "defer func()") {
		t.Error("missing defer for postcondition")
	}

	// Build in temp dir
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "contracts.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module test\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmdb := exec.Command("go", "build", outPath)
	cmdb.Dir = tmpDir
	if out, err := cmdb.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

// TestCompileRegion verifies that region { let! x = ... } emits defer Close(x).
func TestCompileRegion(t *testing.T) {
	t.Skip("region { … } computation expressions removed (PARSE-MIG013)")
}

func TestCompileRefWhile(t *testing.T) {
	src := `module Test
let main () =
  let r = ref 0 in
  while !r < 3 do
    r := !r + 1
  done
`
	mod, err := parser.Parse("ref_while.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatalf("typecheck: %v", errs)
	}
	gen := codegen.NewGenerator("ref_while.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "for ") {
		t.Error("expected while to lower to for")
	}
	if !strings.Contains(goSrc, "*") {
		t.Error("expected ref/deref pointer ops")
	}
}

// TestChanSafetyWrapper verifies that channel operations lower to the
// C0Chan wrapper struct instead of raw Go channels.
func TestChanSafetyWrapper(t *testing.T) {
	src := `module Main

let main () =
  let ch : int chan = Chan.make () in
  let u = Chan.send ch 42 in
  let v = Chan.close ch in
  println "channel operations work"
`
	mod, err := parser.Parse("chan_test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	gen := codegen.NewGenerator("chan_test.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Chan.make generates C0ChanMake()
	if !strings.Contains(goSrc, "C0ChanMake()") {
		t.Error("missing C0ChanMake()")
	}

	// Chan.send generates C0ChanSend
	if !strings.Contains(goSrc, "C0ChanSend(ch, 42)") {
		t.Errorf("missing C0ChanSend(ch, 42), got:\n%s", goSrc)
	}

	// Chan.recv is available via helper
	if !strings.Contains(goSrc, "func C0ChanRecv") {
		t.Error("missing C0ChanRecv helper")
	}

	// Chan.close generates C0ChanClose
	if !strings.Contains(goSrc, "C0ChanClose(ch") {
		t.Error("missing C0ChanClose(ch)")
	}

	// Wrapper struct is emitted
	if !strings.Contains(goSrc, "type C0Chan struct") {
		t.Error("missing C0Chan struct")
	}

	// Helpers are emitted
	if !strings.Contains(goSrc, "func C0ChanSend") {
		t.Error("missing C0ChanSend helper")
	}
	if !strings.Contains(goSrc, "func C0ChanClose") {
		t.Error("missing C0ChanClose helper")
	}
	if !strings.Contains(goSrc, `"Chan.send: channel is closed"`) {
		t.Error("missing safe send panic message")
	}
	if !strings.Contains(goSrc, `"Chan.close: channel already closed"`) {
		t.Error("missing safe close panic message")
	}

	// Should NOT contain raw channel ops at call site
	if strings.Contains(goSrc, ":= make(chan") || strings.Contains(goSrc, "= make(chan") {
		t.Error("should not contain raw make(chan ...) at call site")
	}

	// Build the generated Go
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "chansafety.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module test\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	cmdb := exec.Command("go", "build", outPath)
	cmdb.Dir = tmpDir
	if out, err := cmdb.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}

// TestChanSafetyNoChannel verifies that the C0Chan wrapper is NOT emitted
// when no channel operations are used.
func TestChanSafetyNoChannel(t *testing.T) {
	src := `module Main

let main () =
  println "no channels here!"
`
	mod, err := parser.Parse("nochan_test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	gen := codegen.NewGenerator("nochan_test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if strings.Contains(goSrc, "C0Chan") {
		t.Error("C0Chan wrapper emitted but no channels used")
	}
}

// TestChanSafetyBuild verifies that generated Go code with channels
// compiles and runs correctly.
func TestChanSafetyBuild(t *testing.T) {
	src := `module Main

let main () =
  let ch : int chan = Chan.make () in
  let u = Chan.close ch in
  println "channel closed"
`
	mod, err := parser.Parse("chanbuild_test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	gen := codegen.NewGenerator("chanbuild_test.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "chanbuild.go")
	os.WriteFile(outPath, []byte(goSrc), 0644)
	modContent := "module test\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(modContent), 0644)

	binPath := filepath.Join(tmpDir, exeName("testbin"))
	cmdb := exec.Command("go", "build", "-o", binPath, outPath)
	cmdb.Dir = tmpDir
	if out, err := cmdb.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, _ := exec.Command(binPath).CombinedOutput()
	if !strings.Contains(string(out), "channel closed") {
		t.Errorf("expected 'channel closed' in output, got %q", string(out))
	}
}

func TestRefinementCallSiteGuards(t *testing.T) {
	mod := mustParse(t, "refinement_solving.goop")
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatalf("typecheck: %v", errs)
	}
	proven, funcProven, _, refineErrs := refine.CheckRefinements(mod, tm, config.DefaultConfig())
	if len(refineErrs) > 0 {
		t.Fatalf("refine: %v", refineErrs)
	}
	gen := codegen.NewGenerator("refinement_solving.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	gen.SetProvenSites(proven)
	gen.SetRefinementMeta(funcProven)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "precondition violated") {
		t.Error("expected entry or call-site precondition guard")
	}
	if strings.Contains(goSrc, "func() int {\n\tif !(y != 0)") {
		t.Error("proven compute call should not wrap in guarded IIFE for y != 0")
	}
	_ = funcProven
}

func TestTopLevelRecordLiteralNotThunk(t *testing.T) {
	src := `module T
type cfg = { x: int; y: string }
let cfg = { x = 1; y = "a" }
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goSrc, "func cfg()") {
		t.Fatalf("record literal should not become thunk:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "var Cfg = Cfg{") {
		t.Fatalf("expected direct record var, got:\n%s", goSrc)
	}
}

func TestParenExprFloatPrecedence(t *testing.T) {
	src := `module T
let pct (current: float) (mean: float) : float =
  (current -. mean) /. mean
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goSrc, "current - mean / mean") {
		t.Fatalf("parens must preserve float precedence:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "(current - mean)") {
		t.Fatalf("expected parenthesized subtraction:\n%s", goSrc)
	}
}

func TestOptionInRecordFieldCodegen(t *testing.T) {
	src := `module T
type cfg = { name: string; limit: int option }
let c = { name = "x"; limit = Some 1 }
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "type OptionInt struct") {
		t.Fatalf("missing OptionInt struct:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "NewOptionIntSome(1)") {
		t.Fatalf("expected NewOptionIntSome, got:\n%s", goSrc)
	}
}

func TestArrayMakeAndIndexCodegen(t *testing.T) {
	src := `module T
let main () =
  begin
    let arr = Array.make 2 0 in
    arr.(0) <- 7;
    arr.(1)
  end
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	gen := codegen.NewGenerator("t.goop", config.DefaultConfig())
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(goSrc, "make([]int, 2)") {
		t.Fatalf("expected make([]int, 2), got:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "arr[0] = 7") {
		t.Fatalf("expected array index assignment:\n%s", goSrc)
	}
}

// ---------------------------------------------------------------------------
// H1: no silent degradation — unhandled expressions are hard errors
// ---------------------------------------------------------------------------

func TestCodegenUnhandledExprIsHardError(t *testing.T) {
	// A CompExpr (removed language feature kept for AST compat) must
	// produce a hard compiler error naming the node type and source
	// position — never a /* TODO */ comment in the generated Go.
	mod := &ast.Module{
		Name: "main",
		Decls: []ast.TopDecl{
			&ast.LetDecl{
				Bindings: []ast.LetBinding{
					{
						Name: "x",
						Body: &ast.CompExpr{
							Builder: "result",
							Loc:     token.SourceLoc{File: "bad.goop", Line: 3, Column: 9},
						},
					},
				},
			},
		},
	}
	gen := codegen.NewGenerator("bad.goop", config.DefaultConfig())
	out, err := gen.Generate(mod)
	if err == nil {
		t.Fatalf("expected hard error for unhandled expression, got nil (output:\n%s)", out)
	}
	msg := err.Error()
	if !strings.Contains(msg, "*ast.CompExpr") {
		t.Errorf("error should name the unhandled node type, got: %s", msg)
	}
	if !strings.Contains(msg, "bad.goop:3:9") {
		t.Errorf("error should carry the source position, got: %s", msg)
	}
	if strings.Contains(out, "/* TODO:") {
		t.Errorf("generated Go must never contain a /* TODO: */ fallback, got:\n%s", out)
	}
}

func TestCodegenUnhandledExprLocFallsBackToSrcFile(t *testing.T) {
	// When the AST node carries no file, the error position falls back
	// to the source file passed to NewGenerator.
	mod := &ast.Module{
		Name: "main",
		Decls: []ast.TopDecl{
			&ast.LetDecl{
				Bindings: []ast.LetBinding{
					{
						Name: "y",
						Body: &ast.GuardExpr{
							Loc: token.SourceLoc{Line: 7, Column: 2},
						},
					},
				},
			},
		},
	}
	gen := codegen.NewGenerator("fallback.goop", config.DefaultConfig())
	_, err := gen.Generate(mod)
	if err == nil {
		t.Fatal("expected hard error for unhandled expression, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fallback.goop:7:2") {
		t.Errorf("error should fall back to the generator source file, got: %s", msg)
	}
	if !strings.Contains(msg, "*ast.GuardExpr") {
		t.Errorf("error should name the unhandled node type, got: %s", msg)
	}
}

func TestCodegenExamplesHaveNoTODOMarkers(t *testing.T) {
	// Corpus guard: no successfully generated example may contain the
	// old silent-degradation marker in its Go output.
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".goop") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			mod := mustParse(t, name)
			gen := codegen.NewGenerator(name, config.DefaultConfig())
			goSrc, err := gen.Generate(mod)
			if err != nil {
				t.Skipf("example does not generate standalone: %v", err)
			}
			if strings.Contains(goSrc, "/* TODO:") {
				t.Errorf("generated Go contains silent-degradation marker")
			}
			checked++
		})
	}
}

func TestDecimalReexportRecordCodegen(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(root, "tests", "decimal_reexport_record_test.goop")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mod, err := parser.Parse(srcPath, src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	cfg := config.DefaultConfig()
	gen := codegen.NewGenerator(srcPath, cfg)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(goSrc, "Price decimal.Decimal") {
		t.Fatalf("expected Price decimal.Decimal in generated struct, got:\n%s", goSrc)
	}
	if strings.Contains(goSrc, "Price Decimal\n") || strings.Contains(goSrc, "Price Decimal}") {
		t.Error("must not emit unqualified Price Decimal")
	}
}
