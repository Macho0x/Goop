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
)

// genBrand parses, desugars and generates Go for a source snippet.
func genBrand(t *testing.T, src string) string {
	t.Helper()
	mod, err := parser.Parse("brand_test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := codegen.NewGenerator("brand_test.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(desugar.DesugarModule(mod))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return goSrc
}

// H4c: a single-constructor ADT over a primitive payload lowers to a Go
// defined type — no interface, no variant struct, no boxing.
// See docs/design/21-branded-ids.md.
func TestZeroCostBrandGeneratesDefinedType(t *testing.T) {
	goSrc := genBrand(t, `module main

type order_id = | Order_id of string
type qty = | Qty of int

let main () : unit =
  print_line "ok"
`)
	for _, want := range []string{
		"type order_id string",
		"func Neworder_idOrder_id(v string) order_id {",
		"return order_id(v)",
		"type qty int",
		"func NewqtyQty(v int) qty {",
		"return qty(v)",
	} {
		if !strings.Contains(goSrc, want) {
			t.Fatalf("missing %q in Go output:\n%s", want, goSrc)
		}
	}
	for _, unwanted := range []string{
		"type order_id interface",
		"type order_idOrder_id struct",
		"isorder_id()",
	} {
		if strings.Contains(goSrc, unwanted) {
			t.Fatalf("unexpected interface lowering %q in Go output:\n%s", unwanted, goSrc)
		}
	}
}

// Multi-constructor ADTs keep the interface + variant struct lowering.
func TestMultiCtorAdtKeepsInterface(t *testing.T) {
	goSrc := genBrand(t, `module main

type shape = | Circle of float | Square of float

let area (sh : shape) : float =
  match sh with
  | Circle r -> r
  | Square s -> s

let main () : unit =
  print_line "ok"
`)
	for _, want := range []string{
		"type shape interface",
		"type shapeCircle struct",
		"type shapeSquare struct",
		"func (shapeCircle) isshape() {}",
		"switch v := ",
	} {
		if !strings.Contains(goSrc, want) {
			t.Fatalf("missing %q in Go output:\n%s", want, goSrc)
		}
	}
	if strings.Contains(goSrc, "type shape float64") {
		t.Fatalf("multi-ctor ADT must not be lowered as a brand:\n%s", goSrc)
	}
}

// Payloads that are not primitive/string-like keep the interface lowering.
func TestSingleCtorNonPrimitivePayloadKeepsInterface(t *testing.T) {
	goSrc := genBrand(t, `module main

type wrapper = | Wrap of { name : string }
type pair = | Pair of (int * int)

let main () : unit =
  print_line "ok"
`)
	for _, want := range []string{
		"type wrapper interface",
		"type wrapperWrap struct",
		"type pair interface",
		"type pairPair struct",
	} {
		if !strings.Contains(goSrc, want) {
			t.Fatalf("missing %q in Go output:\n%s", want, goSrc)
		}
	}
}

// Unwrapping a brand is a conversion, not a type switch, and the round trip
// still produces the original payload at run time.
func TestZeroCostBrandMatchRoundTrip(t *testing.T) {
	goSrc := genBrand(t, `module main

type order_id = | Order_id of string
type qty = | Qty of int

let id_string (oid : order_id) : string =
  match oid with
  | Order_id s -> s

let qty_or_zero (q : qty) : int =
  match q with
  | Qty n when n > 0 -> n
  | _ -> 0

let main () : unit =
  let oid = Order_id "ord-1" in
  if qty_or_zero (Qty 7) = 7 then print_line (id_string oid) else print_line "bad qty"
`)
	if !strings.Contains(goSrc, "s := string(") {
		t.Fatalf("expected an unwrap conversion for the string brand:\n%s", goSrc)
	}
	if !strings.Contains(goSrc, "n := int(") {
		t.Fatalf("expected an unwrap conversion for the int brand:\n%s", goSrc)
	}
	if strings.Contains(goSrc, ".(type)") {
		t.Fatalf("brand match must not emit a type switch:\n%s", goSrc)
	}
	if out := runGo(t, goSrc); out != "ord-1\n" {
		t.Fatalf("unexpected program output %q", out)
	}
}

// Two brands over the same representation stay distinct Go types.
func TestBrandsOverSameRepStayDistinct(t *testing.T) {
	goSrc := genBrand(t, `module main

type order_id = | Order_id of string
type symbol = | Symbol of string

let place (sym : symbol) (oid : order_id) : string =
  match oid with
  | Order_id s -> s

let main () : unit =
  print_line (place (Symbol "ETH-USD") (Order_id "ord-1"))
`)
	if !strings.Contains(goSrc, "type order_id string") || !strings.Contains(goSrc, "type symbol string") {
		t.Fatalf("expected two distinct defined types:\n%s", goSrc)
	}
	if out := runGo(t, goSrc); out != "ord-1\n" {
		t.Fatalf("unexpected program output %q", out)
	}
}

func TestBrandedIdsExampleBuilds(t *testing.T) {
	mod := mustParse(t, "branded_ids.goop")
	gen := codegen.NewGenerator("branded_ids.goop", config.DefaultConfig())
	goSrc, err := gen.Generate(mod)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, want := range []string{"type order_id string", "type symbol string"} {
		if !strings.Contains(goSrc, want) {
			t.Fatalf("expected zero-cost brand %q:\n%s", want, goSrc)
		}
	}
	buildGo(t, goSrc, "brandedids.go")
}

// buildGo compiles generated Go source, failing the test on any error.
func buildGo(t *testing.T, goSrc, fileName string) string {
	t.Helper()
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, fileName)
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
	return binPath
}

// runGo builds and runs generated Go source, returning its combined output.
func runGo(t *testing.T, goSrc string) string {
	t.Helper()
	out, err := exec.Command(buildGo(t, goSrc, "main.go")).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return string(out)
}
