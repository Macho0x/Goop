package typecheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/exhaustive"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typecheck"
	"goop.dev/compiler/internal/types"
)

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

func TestTypeCheckHello(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestTypeCheckShapes(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestTypeCheckResult(t *testing.T) {
	mod := mustParse(t, "result.goop")
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

// Regression: Some must be polymorphic per use site (not one shared type variable).
func TestNestedOptionSomeRegression(t *testing.T) {
	src := `module Test
type quote_params = { bid_offset_bps: int; max_slippage_bps: int option }
type decision = { quote: quote_params option; tag: string }
let q0 : quote_params = { bid_offset_bps = 25; max_slippage_bps = Some 40 }
let override_test : decision = { quote = Some q0; tag = "X" }
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestTypeCheckOrderbook(t *testing.T) {
	mod := mustParse(t, "orderbook.goop")
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Negative tests: type errors
// ---------------------------------------------------------------------------

func TestTypeMismatch(t *testing.T) {
	// Adding int and string should fail
	src := `module Test
let f (x: int) : int = x + "hello"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Error("expected type error for int + string")
	}
}

func TestPrivateSameModuleOk(t *testing.T) {
	src := `module main
private let helper x = x + 1
let main () = helper 1
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestPrivateCrossModuleRejected(t *testing.T) {
	lib := `module lib
private let helper x = x
let publicFn x = helper x
`
	consumer := `module main
let main () = helper 1
`
	libMod, err := parser.Parse("lib.goop", []byte(lib))
	if err != nil {
		t.Fatalf("parse lib: %v", err)
	}
	consMod, err := parser.Parse("main.goop", []byte(consumer))
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	_, _, errs := typecheck.CheckWithTypesAndDeps(consMod, map[string]*ast.Module{"lib": libMod})
	if len(errs) == 0 {
		t.Fatal("expected error referencing private helper")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "private") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected private access error, got %v", errs)
	}
}

func TestPrivateUppercaseNameRejected(t *testing.T) {
	src := `module main
private let Helper x = x
let main () = Helper 1
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected error for private uppercase name")
	}
}

func TestModuloFloatRejected(t *testing.T) {
	src := `module main
let main () = 1.5 % 1.0
`
	_, err := parser.Parse("test.goop", []byte(src))
	if err == nil {
		t.Fatal("expected parse error for % operator")
	}
	if !strings.Contains(err.Error(), "mod") {
		t.Errorf("expected error mentioning mod, got: %v", err)
	}
}

func TestModuloIntOk(t *testing.T) {
	src := `module main
let main () = 7 mod 3
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestRefWhileTypeCheck(t *testing.T) {
	src := `module Test
let main () =
  let r = ref 0 in
  while !r < 3 do
    r := !r + 1
  done
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestMutableLetRejected(t *testing.T) {
	src := `module Test
let main () =
  let mutable x = 0 in
  x
`
	_, err := parser.Parse("test.goop", []byte(src))
	if err == nil {
		// Parser may still accept mutable; typechecker must reject.
		mod, _ := parser.Parse("test.goop", []byte(src))
		if mod == nil {
			t.Fatal("expected parse or type error for let mutable")
		}
		errs := typecheck.Check(mod)
		if len(errs) == 0 {
			t.Fatal("expected type error for let mutable")
		}
		if !strings.Contains(errs[0].Error(), "ref") {
			t.Errorf("expected error mentioning ref, got: %v", errs[0])
		}
		return
	}
	if !strings.Contains(err.Error(), "mutable") && !strings.Contains(err.Error(), "ref") {
		t.Logf("parse rejected mutable let: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Location tests: verify type errors include source locations
// ---------------------------------------------------------------------------

func TestTypeErrorHasLocation(t *testing.T) {
	src := `module Test
let f (x: int) : int = x + "hello"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected a type error")
	}
	msg := errs[0].Error()
	// Should contain file:line:col format
	if !strings.Contains(msg, "test.goop:2:") {
		t.Errorf("error message should contain source location, got: %s", msg)
	}
}

func TestTypeErrorLocationBinaryOp(t *testing.T) {
	src := `module Test
let f () = true + 42
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected a type error for bool + int")
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "test.goop:2:") {
		t.Errorf("error should have location, got: %s", msg)
	}
}

func TestTypeErrorLocationIf(t *testing.T) {
	src := `module Test
let f () = if 42 then true else false
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected a type error for non-bool condition")
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "test.goop:2:") {
		t.Errorf("error should have location, got: %s", msg)
	}
}

func TestTypeErrorLocationApp(t *testing.T) {
	src := `module Test
let f () = 42 "hello"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected a type error for int applied as function")
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "test.goop:2:") {
		t.Errorf("error should have location, got: %s", msg)
	}
}

func TestUnknownIdentifier(t *testing.T) {
	// Use a known ADT where a constructor type mismatch occurs.
	// The bootstrap gives unknown identifiers fresh types, so this
	// won't fail by itself. We test a case that actually causes a
	// unification error.
	src := `module Test
type t = A | B

let f (x: t) : int =
  match x with
  | A -> 1
  | B -> true
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Error("expected type error for int vs bool in match arms")
	}
}

func TestWrongArgCount(t *testing.T) {
	// Function expecting two args is given one with wrong type
	src := `module Test
let add (x: int) (y: int) : int = x + y
let wrong = add true
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Error("expected type error for bool vs int argument")
	}
}

// ---------------------------------------------------------------------------
// Exhaustiveness tests
// ---------------------------------------------------------------------------

func TestExhaustiveMatchPasses(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	// Register ADTs as the CLI does
	registerADTs(mod)
	errs := exhaustive.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("unexpected exhaustiveness warning: %v", e)
		}
	}
}

func TestNonExhaustiveMatch(t *testing.T) {
	src := `module Test
type color = Red | Green | Blue

let describe (c: color) : string =
  match c with
  | Red -> "red"
  | Green -> "green"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registerADTs(mod)
	errs := exhaustive.Check(mod)
	if len(errs) == 0 {
		t.Error("expected exhaustiveness warning for missing Blue")
	}
}

func TestExhaustiveWithWildcard(t *testing.T) {
	src := `module Test
type color = Red | Green | Blue

let describe (c: color) : string =
  match c with
  | Red -> "red"
  | _ -> "other"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registerADTs(mod)
	errs := exhaustive.Check(mod)
	if len(errs) > 0 {
		t.Errorf("unexpected exhaustiveness warning: %v", errs[0])
	}
}

func ExhaustiveResultMatch(t *testing.T) {
	src := `module Test
let f (r: result) : string =
  match r with
  | Ok x -> "ok"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registerADTs(mod)
	errs := exhaustive.Check(mod)
	if len(errs) == 0 {
		t.Error("expected exhaustiveness warning for missing Error")
	}
}

func registerADTs(mod *ast.Module) {
	for _, d := range mod.Decls {
		if td, ok := d.(*ast.TypeDecl); ok {
			if adt, ok := td.Kind.(*ast.ADTTypeKind); ok {
				var ctors []string
				for _, c := range adt.Cases {
					ctors = append(ctors, c.Name)
				}
				exhaustive.RegisterADT(td.Name, ctors)
			}
		}
	}
	// Register built-in ADTs
	exhaustive.RegisterADT("result", []string{"Ok", "Error"})
	exhaustive.RegisterADT("option", []string{"None", "Some"})
}

// ---------------------------------------------------------------------------
// Bidirectional lambda inference tests
// ---------------------------------------------------------------------------

// TestBidirectionalLambdaKnownFunc verifies that when a lambda is passed to
// a function with a known signature, the lambda's unannotated parameter is
// inferred from the expected function type.
func TestBidirectionalLambdaKnownFunc(t *testing.T) {
	// applyTo42 : (int -> int) -> int
	// applyTo42 (fun x -> x + 1)  — should infer x as int from the expected type.
	src := `module Test
let apply_to_42 (f: int -> int) : int = f 42
let result = apply_to_42 (fun x -> x + 1)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	tm, _, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	// Find the FunExpr and verify its type is int -> int.
	found := false
	for expr, typ := range tm {
		fn, ok := expr.(*ast.FunExpr)
		if !ok {
			continue
		}
		found = true
		tfun, ok := typ.(*types.TFun)
		if !ok {
			t.Errorf("lambda type should be TFun, got %T (%v)", typ, typ)
			continue
		}
		if _, ok := tfun.From.(*types.Prim); !ok {
			t.Errorf("lambda param type should be concrete Prim, got %T (%v)", tfun.From, tfun.From)
			continue
		}
		p := tfun.From.(*types.Prim)
		if p.Name != "int" {
			t.Errorf("expected lambda param type 'int', got %q", p.Name)
		}
		if _, ok := tfun.To.(*types.Prim); !ok || tfun.To.(*types.Prim).Name != "int" {
			t.Errorf("expected lambda return type 'int', got %v", tfun.To)
		}
		_ = fn // used
	}
	if !found {
		t.Error("did not find FunExpr in TypeMap")
	}
}

// TestBidirectionalLambdaCurried verifies bidirectional inference with a
// curried function that takes multiple lambda arguments.
func TestBidirectionalLambdaCurried(t *testing.T) {
	// compose : (int -> int) -> (int -> int) -> int -> int
	// compose (fun x -> x * 2) (fun y -> y + 1) 5
	src := `module Test
let compose (f: int -> int) (g: int -> int) (a: int) : int = g (f a)
let result = compose (fun x -> x * 2) (fun y -> y + 1) 5
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	tm, _, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	// Count FunExprs and verify each has int -> int type.
	count := 0
	for expr, typ := range tm {
		if _, ok := expr.(*ast.FunExpr); !ok {
			continue
		}
		count++
		tfun, ok := typ.(*types.TFun)
		if !ok {
			t.Errorf("lambda type should be TFun, got %T (%v)", typ, typ)
			continue
		}
		if p, ok := tfun.From.(*types.Prim); !ok || p.Name != "int" {
			t.Errorf("lambda param type should be int, got %v", tfun.From)
		}
		if p, ok := tfun.To.(*types.Prim); !ok || p.Name != "int" {
			t.Errorf("lambda return type should be int, got %v", tfun.To)
		}
	}
	if count < 2 {
		t.Errorf("expected 2 FunExpr in TypeMap, found %d", count)
	}
}

// TestBidirectionalLambdaNoAnnotation verifies that a completely unannotated
// lambda passed to a known function gets correct type inference.
func TestBidirectionalLambdaNoAnnotation(t *testing.T) {
	src := `module Test
let call_with_hello (f: string -> string) : string = f "hello"
let result = call_with_hello (fun s -> string_concat s " world")
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	tm, _, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	for expr, typ := range tm {
		fn, ok := expr.(*ast.FunExpr)
		if !ok {
			continue
		}
		_ = fn
		tfun, ok := typ.(*types.TFun)
		if !ok {
			t.Errorf("lambda type should be TFun, got %T", typ)
			continue
		}
		if p, ok := tfun.From.(*types.Prim); !ok || p.Name != "string" {
			t.Errorf("lambda param type should be string, got %v", tfun.From)
		}
		if p, ok := tfun.To.(*types.Prim); !ok || p.Name != "string" {
			t.Errorf("lambda return type should be string, got %v", tfun.To)
		}
	}
}

// TestBidirectionalWithKnownListMap verifies that HM inference still works
// for the classic list_map example (polymorphic function + concrete list).
func TestBidirectionalWithKnownListMap(t *testing.T) {
	src := `module Test
let map (f: 'a -> 'b) (xs: 'a list) : 'b list =
  match xs with
  | [] -> []
  | x :: rest -> f x :: map f rest

let result = map (fun x -> x + 1) (1 :: 2 :: 3 :: [])
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	_, _, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}
}

// TestBidirectionalFallbackToFresh verifies that when the function type is
// not resolved to a concrete TFun (still a TVar), we fall back to fresh
// vars — i.e. the bidirectional path degrades gracefully.
func TestBidirectionalFallbackToFresh(t *testing.T) {
	// identity : 'a -> 'a — when applied to (fun x -> x), the function
	// type is polymorphic; the lambda should still typecheck correctly.
	src := `module Test
let identity (x: 'a) : 'a = x
let result = identity (fun x -> x)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	_, _, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}
}

// TestExternDeclaredTupleReturn typechecks an @[go] binding that returns a
// multi-value Goop tuple (explicit declaration; no gosig / H5 required).
func TestExternDeclaredTupleReturn(t *testing.T) {
	src := `module Test
import go "strconv" {}
@[go] {
  func atoiPair(s string) (int, string) {
    n, err := strconv.Atoi(s)
    if err != nil { return 0, err.Error() }
    return n, ""
  }
}
val atoiPair : string -> (int, string)
let pair = atoiPair "42"
let main () = assert (pair.F0 = 42)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	_, vm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatal("typecheck failed")
	}
	got := vm["pair"]
	if got == nil {
		t.Fatal("expected pair in var map")
	}
	tup, ok := got.(*types.TTuple)
	if !ok || len(tup.Elems) != 2 {
		t.Fatalf("expected (int, string) tuple, got %T (%v)", got, got)
	}
}

// TestExternImportTupleReturn typechecks import go vals declared with a
// multi-value (T, error) return. H6 coerces the call-site type to result.
func TestExternImportTupleReturn(t *testing.T) {
	src := `module Test
import go "strconv" { val Atoi : string -> (int, error) }
let pair = Atoi "42"
let main () = match pair with | Ok n -> assert (n = 42) | Error _ -> assert false
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	_, vm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatal("typecheck failed")
	}
	got := vm["pair"]
	if got == nil {
		t.Fatal("expected pair in var map")
	}
	tc, ok := got.(*types.TCon)
	if !ok || tc.Name != "result" || len(tc.Args) != 2 {
		t.Fatalf("expected result<int, error>, got %T (%v)", got, got)
	}
	if _, ok := tc.Args[1].(*types.TError); !ok {
		t.Fatalf("expected err type to be error, got %T (%v)", tc.Args[1], tc.Args[1])
	}
}

// TestExternImportRawTupleReturn keeps (T, error) as a product under import go raw.
func TestExternImportRawTupleReturn(t *testing.T) {
	src := `module Test
import go raw "strconv" { val Atoi : string -> (int, error) }
let pair = Atoi "42"
let main () = assert (pair.F0 = 42)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	_, vm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatal("typecheck failed")
	}
	got := vm["pair"]
	if got == nil {
		t.Fatal("expected pair in var map")
	}
	tup, ok := got.(*types.TTuple)
	if !ok || len(tup.Elems) != 2 {
		t.Fatalf("expected (int, error) tuple under raw, got %T (%v)", got, got)
	}
	if _, ok := tup.Elems[1].(*types.TError); !ok {
		t.Fatalf("expected F1 to be error, got %T (%v)", tup.Elems[1], tup.Elems[1])
	}
}

// TestGoSigFallbackExtern verifies that the gosig fallback correctly
// refines an extern binding's type using the real Go signature.
// We use "strings.Contains" which has func(string, string) bool.
func TestGoSigFallbackExtern(t *testing.T) {
	src := `module Test
import go "strings" {
  val Contains : string -> string -> bool
}

let main () =
  let got = Contains "hello" "he" in
  print_line (if got then "ok" else "no")
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		// If gosig fallback fails (e.g. packages.Load can't load), the
		// declared type should still work; errors here indicate a regression
		// in the declared type path.
		for _, e := range errs {
			// Only fail if the error is a type mismatch, not a gosig warning.
			if strings.Contains(e.Error(), "type mismatch") {
				t.Errorf("type error: %v", e)
			} else {
				t.Logf("gosig warning (non-fatal): %v", e)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Effect row tests
// ---------------------------------------------------------------------------

// TestEffectRowIo verifies that a function with `with { io }` has the effect
// in its inferred type.
func TestEffectRowIo(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestEffectRowPure verifies that a pure function (no `with`) has nil Effects
// (unknown, not pure).
func TestEffectRowPure(t *testing.T) {
	src := `module Test
let double (x: int) : int = x * 2
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	_, vm, errs := typecheck.CheckWithTypes(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
		t.Fatalf("typecheck failed")
	}

	tfun, ok := vm["double"].(*types.TFun)
	if !ok {
		t.Fatalf("expected TFun for double, got %T", vm["double"])
	}
	// No `with` clause → nil Effects (unknown, permissive)
	// Actually, since the function has typed params, we expect nil Effects
	// because the parser didn't see `with`.
	if tfun.Effects != nil {
		t.Errorf("expected nil Effects for pure function, got %v", tfun.Effects)
	}
}

// TestEffectRowPolymorphic verifies that a row-polymorphic function
// `f : unit -> 'a with { e }` accepts any effectful callback.
func TestEffectRowPolymorphic(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestEffectRowBackwardCompat verifies that existing code without `with` clauses
// still works perfectly (nil Effects = permissive).
func TestEffectRowBackwardCompat(t *testing.T) {
	src := `module Test
let add (x: int) (y: int) : int = x + y
let result = add 3 4
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

// TestEffectRowExternOpen verifies that extern functions with effect annotation
// work correctly.
func TestEffectRowExternAnnotated(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestEffectRowMultipleEffects verifies multiple effects in a row.
func TestEffectRowMultipleEffects(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestEffectRowWithExplicitPure verifies that `with {}` means explicitly pure.
func TestEffectRowExplicitPure(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestEffectRowOpen verifies `with { e | .. }` creates an open effect row.
func TestEffectRowOpen(t *testing.T) {
	t.Skip("effect rows removed from surface syntax (PARSE-MIG016); Phase 6 handlers replace them")
}

// TestTypeCheckRegion verifies that region { let! x = ...; return ... } typechecks.
func TestTypeCheckRegion(t *testing.T) {
	t.Skip("region { … } computation expressions removed (PARSE-MIG013)")
}

// TestTypeCheckRegionReturnType verifies that region infers the return type.
func TestTypeCheckRegionReturnType(t *testing.T) {
	t.Skip("region { … } computation expressions removed (PARSE-MIG013)")
}

func TestTypeCheckTryRaise(t *testing.T) {
	src := `module Test
exception Oops of string
let main () =
  try
    raise (Oops "boom")
  with
  | Oops msg -> msg
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestTypeCheckArraySyntax(t *testing.T) {
	src := `module Test
let fill (n: int) (v: int) : int array =
  begin
    let arr = Array.make n v in
    for i = 0 to n - 1 do
      arr.(i) <- v + i
    done;
    arr
  end
let main () =
  let xs = fill 3 10 in
  assert (Array.length xs = 3 && xs.(2) = 12)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestAssignToImmutableBinding(t *testing.T) {
	src := `module Test
let main () =
  let x = 0 in
  x <- 1
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected type error for assign to immutable")
	}
	if !strings.Contains(errs[0].Error(), "TYPE011") {
		t.Errorf("expected TYPE011 prefix, got: %v", errs[0])
	}
	if !strings.Contains(errs[0].Error(), "cannot assign to immutable binding") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestInvalidAssignmentTarget(t *testing.T) {
	src := `module Test
let main () =
  (fun x -> x) <- 1
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected type error for invalid assignment target")
	}
	if !strings.Contains(errs[0].Error(), "TYPE012") {
		t.Errorf("expected TYPE012 prefix, got: %v", errs[0])
	}
	if !strings.Contains(errs[0].Error(), "invalid assignment target") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestQualifiedConstructorUndefined(t *testing.T) {
	src := `module Test
type Color = Red | Green
let main () = Color.Blue
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected type error for undefined qualified constructor")
	}
	if !strings.Contains(errs[0].Error(), "constructor Color.Blue is not defined") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestPerformInsideGoRejected(t *testing.T) {
	src := `module Test
effect Flip : unit -> bool
let main () : unit =
  let _ = go (fun () ->
    let _ = perform (Flip ()) in
    ()) in
  ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected error for perform inside go body")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "perform is not allowed inside go body") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected perform-in-go error, got %v", errs)
	}
}

func TestEffectContinueTypechecks(t *testing.T) {
	src := `module Test
effect Flip : unit -> bool
let coin () : bool =
  begin
    perform (Flip ());
    false
  end
let run () : bool =
  match coin () with
  | effect (Flip _) k -> continue k true
  | v -> v
let main () = assert (run () = true)
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// M5 — inference holes closed for 1.0
// ---------------------------------------------------------------------------

func checkOK(t *testing.T, src string) {
	t.Helper()
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mod = desugar.DesugarModule(mod)
	errs := typecheck.Check(mod)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("type error: %v", e)
		}
	}
}

func TestInferEqEqOperator(t *testing.T) {
	checkOK(t, `module Test
let eq_int = 1 == 1
let eq_str = "a" == "a"
let main () = assert (eq_int = true)
`)
}

func TestInferPipeOperatorBinary(t *testing.T) {
	// Parser emits BinaryExpr for |>; must infer like application.
	checkOK(t, `module Test
let add1 x = x + 1
let n = 41 |> add1
let main () = assert (n = 42)
`)
}

func TestInferPipeChained(t *testing.T) {
	checkOK(t, `module Test
let add1 x = x + 1
let double x = x * 2
let n = 20 |> add1 |> double
let main () = assert (n = 42)
`)
}

func TestInferSupportedBinaryOpsSmoke(t *testing.T) {
	checkOK(t, `module Test
let arith = 1 + 2 * 3 - 4 / 2
let bit = (5 land 3) lor (1 lxor 0)
let md = 7 mod 3
let fl = 1.0 +. 2.0 *. 3.0 -. 4.0 /. 2.0
let cmp = (1 < 2) && (3 >= 3) || (4 <> 5) && (6 != 7)
let cat = "a" ^ "b"
let lst = 1 :: [2; 3]
let main () = ()
`)
}

func TestUnsupportedBinaryOperatorRestriction(t *testing.T) {
	// Construct a BinaryExpr with an operator the parser never emits as binary.
	loc := token.SourceLoc{File: "test.goop", Line: 2, Column: 10}
	mod := &ast.Module{
		Name: "Test",
		Decls: []ast.TopDecl{
			&ast.LetDecl{Bindings: []ast.LetBinding{{
				Name: "bad",
				Body: &ast.BinaryExpr{
					Left:  &ast.LitExpr{Value: int64(1), Kind: token.INT, Loc: loc},
					Op:    token.AT,
					Right: &ast.LitExpr{Value: int64(2), Kind: token.INT, Loc: loc},
					Loc:   loc,
				},
			}}},
		},
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected unsupported binary operator error")
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "unsupported binary operator") {
		t.Errorf("expected 'unsupported binary operator', got: %s", msg)
	}
	if !strings.Contains(msg, "@") {
		t.Errorf("expected operator name in message, got: %s", msg)
	}
	if !strings.Contains(msg, "|>") {
		t.Errorf("expected supported-ops list including |>, got: %s", msg)
	}
	if !strings.Contains(msg, "annotate") {
		t.Errorf("expected annotation guidance, got: %s", msg)
	}
	if strings.Contains(msg, "not implemented") {
		t.Errorf("must not use incomplete-inference wording, got: %s", msg)
	}
}

func TestPercentBinaryRestrictionMessage(t *testing.T) {
	loc := token.SourceLoc{File: "test.goop", Line: 2, Column: 10}
	mod := &ast.Module{
		Name: "Test",
		Decls: []ast.TopDecl{
			&ast.LetDecl{Bindings: []ast.LetBinding{{
				Name: "bad",
				Body: &ast.BinaryExpr{
					Left:  &ast.LitExpr{Value: int64(5), Kind: token.INT, Loc: loc},
					Op:    token.PERCENT,
					Right: &ast.LitExpr{Value: int64(2), Kind: token.INT, Loc: loc},
					Loc:   loc,
				},
			}}},
		},
	}
	errs := typecheck.Check(mod)
	if len(errs) == 0 {
		t.Fatal("expected %% restriction error")
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "mod") {
		t.Errorf("expected guidance to use mod, got: %s", msg)
	}
	if strings.Contains(msg, "not implemented") {
		t.Errorf("must not use incomplete-inference wording, got: %s", msg)
	}
}

func TestRemovedConstructRestrictionMessages(t *testing.T) {
	loc := token.SourceLoc{File: "test.goop", Line: 2, Column: 1}
	cases := []struct {
		name string
		body ast.Expr
		want []string
	}{
		{
			name: "question",
			body: &ast.QuestionExpr{Left: &ast.IdentExpr{Name: "x", Loc: loc}, Loc: loc},
			want: []string{"?", "match", "result"},
		},
		{
			name: "guard",
			body: &ast.GuardExpr{Loc: loc},
			want: []string{"guard", "match"},
		},
		{
			name: "is",
			body: &ast.IsExpr{Left: &ast.IdentExpr{Name: "x", Loc: loc}, Pattern: &ast.WildcardPattern{}, Loc: loc},
			want: []string{"is", "match"},
		},
		{
			name: "as_match",
			body: &ast.AsMatchExpr{
				Left:     &ast.IdentExpr{Name: "x", Loc: loc},
				Pattern:  &ast.WildcardPattern{},
				Body:     &ast.LitExpr{Value: int64(1), Kind: token.INT, Loc: loc},
				ElseBody: &ast.LitExpr{Value: int64(0), Kind: token.INT, Loc: loc},
				Loc:      loc,
			},
			want: []string{"as", "match"},
		},
		{
			name: "comp",
			body: &ast.CompExpr{Builder: "async", Loc: loc},
			want: []string{"computation expressions", "async", "match"},
		},
		{
			name: "region",
			body: &ast.RegionExpr{Loc: loc},
			want: []string{"region", "try"},
		},
		{
			name: "module_app",
			body: &ast.ModuleAppExpr{Func: "F", Arg: "M", Loc: loc},
			want: []string{"functor application", "F(M)", "module"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := &ast.Module{
				Name: "Test",
				Decls: []ast.TopDecl{
					&ast.LetDecl{Bindings: []ast.LetBinding{{Name: "x", Body: tc.body}}},
				},
			}
			errs := typecheck.Check(mod)
			if len(errs) == 0 {
				t.Fatal("expected restriction error")
			}
			msg := errs[0].Error()
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("expected %q in error, got: %s", w, msg)
				}
			}
			if strings.Contains(msg, "not implemented") {
				t.Errorf("must not use incomplete-inference wording, got: %s", msg)
			}
		})
	}
}
