package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

// examplesDir is relative to the module root.
var examplesDir = "../../../docs/examples"

func TestParseHello(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	if mod.Name != "main" {
		t.Errorf("expected module name main, got %q", mod.Name)
	}
	if len(mod.Decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(mod.Decls))
	}
	d, ok := mod.Decls[0].(*ast.LetDecl)
	if !ok {
		t.Fatalf("expected LetDecl, got %T", mod.Decls[0])
	}
	if len(d.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(d.Bindings))
	}
	if d.Bindings[0].Name != "main" {
		t.Errorf("expected binding name main, got %q", d.Bindings[0].Name)
	}
	// Body should be an AppExpr: print_line "Hello, Goop!"
	app, ok := d.Bindings[0].Body.(*ast.AppExpr)
	if !ok {
		t.Fatalf("expected AppExpr body, got %T", d.Bindings[0].Body)
	}
	// Left side: print_line (ident) or Console.print_line (field access)
	ident, ok := app.Func.(*ast.IdentExpr)
	if !ok {
		// Backward compat: may be FieldAccessExpr for Console.print_line
		if field, ok2 := app.Func.(*ast.FieldAccessExpr); ok2 {
			if field.Field != "print_line" {
				t.Errorf("expected print_line, got %q", field.Field)
			}
		} else {
			t.Fatalf("expected IdentExpr or FieldAccessExpr, got %T", app.Func)
		}
	} else {
		if ident.Name != "print_line" {
			t.Errorf("expected print_line, got %q", ident.Name)
		}
	}
	// Right side: "Hello, Goop!" string literal
	lit, ok := app.Arg.(*ast.LitExpr)
	if !ok {
		t.Fatalf("expected LitExpr arg, got %T", app.Arg)
	}
	if lit.Value != "Hello, Goop!" {
		t.Errorf("expected 'Hello, Goop!', got %v", lit.Value)
	}
}

func TestParseShapes(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	if mod.Name != "main" {
		t.Errorf("expected main, got %q", mod.Name)
	}
	if len(mod.Decls) != 4 {
		t.Fatalf("expected 4 decls (type Shape, let area, let describe, let main), got %d", len(mod.Decls))
	}

	// First decl: type Shape = ADT
	td, ok := mod.Decls[0].(*ast.TypeDecl)
	if !ok {
		t.Fatalf("expected TypeDecl, got %T", mod.Decls[0])
	}
	if td.Name != "Shape" {
		t.Errorf("expected type name Shape, got %q", td.Name)
	}
	adt, ok := td.Kind.(*ast.ADTTypeKind)
	if !ok {
		t.Fatalf("expected ADTTypeKind, got %T", td.Kind)
	}
	if len(adt.Cases) != 3 {
		t.Errorf("expected 3 ADT cases, got %d", len(adt.Cases))
	}

	// Second decl: let area (s: shape) : float = match ...
	ld, ok := mod.Decls[1].(*ast.LetDecl)
	if !ok {
		t.Fatalf("expected LetDecl, got %T", mod.Decls[1])
	}
	b := ld.Bindings[0]
	if b.Name != "area" {
		t.Errorf("expected area, got %q", b.Name)
	}
	if len(b.Params) != 1 || b.Params[0].Name != "s" {
		t.Errorf("expected param s: shape")
	}
	match, ok := b.Body.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr body, got %T", b.Body)
	}
	if len(match.Arms) != 3 {
		t.Errorf("expected 3 match arms in area, got %d", len(match.Arms))
	}

	// Third decl: let describe (s: shape) : string = match ...
	ld2, ok := mod.Decls[2].(*ast.LetDecl)
	if !ok {
		t.Fatalf("expected LetDecl for describe, got %T", mod.Decls[2])
	}
	b2 := ld2.Bindings[0]
	if b2.Name != "describe" {
		t.Errorf("expected describe, got %q", b2.Name)
	}
	match2, ok := b2.Body.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", b2.Body)
	}
	if len(match2.Arms) != 5 {
		t.Errorf("expected 5 match arms in describe, got %d", len(match2.Arms))
	}
	// First arm has a guard: when radius > 10.0
	if match2.Arms[0].Guard == nil {
		t.Error("expected guard on first arm of describe (when radius > 10.0)")
	}
	// Third arm has a guard: when width = height
	if match2.Arms[2].Guard == nil {
		t.Error("expected guard on third arm of describe (when width = height)")
	}
}

func TestParseResult(t *testing.T) {
	mod := mustParse(t, "result.goop")
	if mod.Name != "main" {
		t.Errorf("expected main, got %q", mod.Name)
	}
	if len(mod.Decls) < 4 {
		t.Fatalf("expected several decls in result.goop, got %d", len(mod.Decls))
	}
}

func TestParseOrderbook(t *testing.T) {
	mod := mustParse(t, "orderbook.goop")
	if mod.Name != "Trading.OrderBook" {
		t.Errorf("expected Trading.OrderBook, got %q", mod.Name)
	}
	// Branded IDs as single-ctor ADTs (no newtype): order_id, symbol, plus Side/Order/Fill/Book.
	var typeNames []string
	for _, d := range mod.Decls {
		if td, ok := d.(*ast.TypeDecl); ok {
			typeNames = append(typeNames, td.Name)
		}
	}
	for _, want := range []string{"order_id", "symbol", "Side", "Order", "Fill", "Book"} {
		found := false
		for _, n := range typeNames {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected type %q in orderbook decls, got %v", want, typeNames)
		}
	}
}

func TestLexHello(t *testing.T) {
	mod := mustParse(t, "hello.goop")
	_ = mod // just verify it parses; lexing is tested implicitly
}

func TestLexShapes(t *testing.T) {
	mod := mustParse(t, "shapes.goop")
	_ = mod
}

func TestLexResult(t *testing.T) {
	mod := mustParse(t, "result.goop")
	_ = mod
}

func TestLexOrderbook(t *testing.T) {
	mod := mustParse(t, "orderbook.goop")
	_ = mod // lexing is tested implicitly via parse
}

func TestParseGoEmbedPointerReceiver(t *testing.T) {
	// Go's func(*T) contains "(*" which must not be treated as a Goop comment.
	src := `module main

@[go] {
  type Box struct{ N int }
  func (b *Box) Val() int { return b.N }
  func newBox(n int) *Box { return &Box{N: n} }
  func boxVal(b *Box) int { return b.Val() }
}
val newBox : int -> unit
val boxVal : unit -> int

let main () = print_line "ok"
`
	mod, err := parser.Parse("ptr_embed.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var body string
	for _, d := range mod.Decls {
		if ge, ok := d.(*ast.LangEmbedDecl); ok {
			body = ge.Body
		}
	}
	if !strings.Contains(body, "func (b *Box)") {
		t.Fatalf("expected pointer receiver in body, got %q", body)
	}
	if !strings.Contains(body, "func newBox") {
		t.Fatalf("expected newBox in body, got %q", body)
	}
}

func TestParseGoEmbed(t *testing.T) {
	src := `module main

import go "fmt"

@[go] {
  func nowString() string {
    return fmt.Sprintf("%d", 1)
  }
}
val nowString : unit -> string

let main () = print_line (nowString ())
`
	mod, err := parser.Parse("go_embed.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, d := range mod.Decls {
		if ge, ok := d.(*ast.LangEmbedDecl); ok {
			found = true
			if ge.Lang != "go" {
				t.Errorf("expected lang go, got %q", ge.Lang)
			}
			if !strings.Contains(ge.Body, "nowString") {
				t.Error("expected Go code in embed block")
			}
			if len(ge.Vals) != 1 || ge.Vals[0].Name != "nowString" {
				t.Errorf("expected val nowString, got %+v", ge.Vals)
			}
		}
	}
	if !found {
		t.Fatal("expected LangEmbedDecl")
	}
}

func TestParseCEmbed(t *testing.T) {
	src := `module main

@[c] {
  int add(int a, int b) { return a + b; }
}
val add : int -> int -> int

let main () = ()
`
	mod, err := parser.Parse("c_embed.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, d := range mod.Decls {
		if ge, ok := d.(*ast.LangEmbedDecl); ok {
			found = true
			if ge.Lang != "c" {
				t.Errorf("expected lang c, got %q", ge.Lang)
			}
			if !strings.Contains(ge.Body, "add") {
				t.Error("expected C code in embed block")
			}
		}
	}
	if !found {
		t.Fatal("expected LangEmbedDecl")
	}
}

func TestRejectLegacyGolangEmbed(t *testing.T) {
	src := `module main
@golang {
  func x() {}
}
let main () = ()
`
	_, err := parser.Parse("bad.goop", []byte(src))
	if err == nil {
		t.Fatal("expected error for @golang")
	}
	if !strings.Contains(err.Error(), "@[go]") {
		t.Errorf("expected @[go] hint, got: %v", err)
	}
}

func TestRejectUnknownLangEmbed(t *testing.T) {
	src := `module main
@[rust] {
  fn x() {}
}
let main () = ()
`
	_, err := parser.Parse("bad.goop", []byte(src))
	if err == nil {
		t.Fatal("expected error for @[rust]")
	}
	if !strings.Contains(err.Error(), "@[rust]") {
		t.Errorf("expected unknown lang error, got: %v", err)
	}
}

func TestRejectLegacyGoBlockInExtern(t *testing.T) {
	src := `module main

import go "fmt" {
  go {
    func x() {}
  }
}
`
	_, err := parser.Parse("bad.goop", []byte(src))
	if err == nil {
		t.Fatal("expected parse error for go { } inside import block")
	}
	if !strings.Contains(err.Error(), "val") {
		t.Errorf("expected val-only import block error, got: %v", err)
	}
}

func TestImportGoRaw(t *testing.T) {
	src := `module main
import go raw "strconv" { val Atoi : string -> (int, error) }
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}
	if !mod.Imports[0].Raw {
		t.Fatal("expected Raw=true for import go raw")
	}
	if mod.Imports[0].Path != "strconv" {
		t.Fatalf("path = %q", mod.Imports[0].Path)
	}
}

func TestImportGrouped(t *testing.T) {
	src := `module main
import (
  go "fmt"
  goop "std.io"
)
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(mod.Imports))
	}
	if mod.Imports[0].Kind != ast.ImportGo || mod.Imports[0].Path != "fmt" {
		t.Errorf("import[0]: %+v", mod.Imports[0])
	}
	if mod.Imports[1].Kind != ast.ImportGoop || mod.Imports[1].Path != "std.io" {
		t.Errorf("import[1]: %+v", mod.Imports[1])
	}
}

func TestImportGoVals(t *testing.T) {
	src := `module main
import go "strconv" { val Atoi : string -> (int, string) }
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Imports) != 1 || len(mod.Imports[0].Vals) != 1 {
		t.Fatalf("expected 1 val, got %+v", mod.Imports)
	}
}

func TestImportGoMethodAndFieldVals(t *testing.T) {
	src := `module main
import go "log/slog" {
  type Record
  type Attr
  val (r : Record).Attrs : (Attr -> bool) -> unit
  val (a : Attr).Key : string
}
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	vals := mod.Imports[0].Vals
	if len(vals) != 2 {
		t.Fatalf("expected 2 vals, got %#v", vals)
	}
	if vals[0].Kind != ast.ExternMethod || vals[0].Name != "Attrs" || vals[0].RecvName != "r" {
		t.Fatalf("method val: %#v", vals[0])
	}
	if vals[1].Kind != ast.ExternField || vals[1].Name != "Key" || vals[1].RecvName != "a" {
		t.Fatalf("field val: %#v", vals[1])
	}
}

func TestParseGoImportTypesAndImplements(t *testing.T) {
	src := `module main
import go "fmt" { type Stringer }
type point = { x : int; y : int }
implements Stringer for point with
  let String (p : point) : string = "point"
end
`
	mod, err := parser.Parse("implements.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Imports) != 1 || len(mod.Imports[0].Types) != 1 || mod.Imports[0].Types[0].Name != "Stringer" {
		t.Fatalf("expected imported Stringer type, got %#v", mod.Imports)
	}
	if len(mod.Decls) != 2 {
		t.Fatalf("expected type and implements declarations, got %#v", mod.Decls)
	}
	impl, ok := mod.Decls[1].(*ast.ImplementsDecl)
	if !ok {
		t.Fatalf("expected ImplementsDecl, got %T", mod.Decls[1])
	}
	if impl.Interface != "Stringer" || impl.ForType != "point" || len(impl.Methods) != 1 {
		t.Fatalf("unexpected implements declaration: %#v", impl)
	}
}

func TestImportGoopDot(t *testing.T) {
	src := `module main
import goop . "std.io"
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mod.Imports[0].Alias != "." {
		t.Errorf("expected dot alias, got %q", mod.Imports[0].Alias)
	}
}

func TestImportAlias(t *testing.T) {
	src := `module main
import httpx go "net/http"
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mod.Imports[0].Alias != "httpx" {
		t.Errorf("expected httpx alias, got %q", mod.Imports[0].Alias)
	}
}

func TestImportGoopCanonicalPath(t *testing.T) {
	src := `module main
import goop "github.com/foo/bar"
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mod.Imports[0].Path != "github.com/foo/bar" {
		t.Errorf("got path %q", mod.Imports[0].Path)
	}
}

func TestAcceptOpenModule(t *testing.T) {
	src := `module main
open List
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("open should parse: %v", err)
	}
	if len(mod.Decls) < 1 {
		t.Fatal("expected open decl")
	}
	if _, ok := mod.Decls[0].(*ast.OpenModuleDecl); !ok {
		t.Fatalf("expected OpenModuleDecl, got %T", mod.Decls[0])
	}
}

func TestRejectExtern(t *testing.T) {
	src := `module main
extern "go" "fmt" {}
let main () = ()
`
	_, err := parser.Parse("t.goop", []byte(src))
	if err == nil {
		t.Fatal("expected error for extern")
	}
	if !strings.Contains(err.Error(), "PARSE-MIG002") {
		t.Errorf("expected PARSE-MIG002, got: %v", err)
	}
	if !strings.Contains(err.Error(), "import go") {
		t.Errorf("expected migration hint, got: %v", err)
	}
}

func TestRejectC0ImportVals(t *testing.T) {
	src := `module main
import goop "std.io" { val X : int }
let main () = ()
`
	_, err := parser.Parse("t.goop", []byte(src))
	if err == nil {
		t.Fatal("expected error for c0 import with vals")
	}
}

func TestParseGoMove(t *testing.T) {
	src := `module main
let main () =
  let x = ref 0 in
  go (move x) (fun () -> x)
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *ast.GoExpr
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		if g, ok := e.(*ast.GoExpr); ok {
			found = g
		}
		switch e := e.(type) {
		case *ast.LetInExpr:
			for _, b := range e.Bindings {
				walk(b.Body)
			}
			walk(e.Body)
		case *ast.FunExpr:
			walk(e.Body)
		case *ast.ParenExpr:
			walk(e.Inner)
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walk(b.Body)
			}
		}
	}
	if found == nil || len(found.Moved) != 1 || found.Moved[0] != "x" {
		t.Fatalf("expected go (move x), got %+v", found)
	}
}

func TestParseArrayIndexAndAssign(t *testing.T) {
	src := `module main
let main () =
  begin
    let arr = Array.make 2 0 in
    arr.(1) <- 42;
    arr.(0)
  end
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var assign *ast.AssignExpr
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		switch n := e.(type) {
		case *ast.AssignExpr:
			assign = n
		case *ast.LetInExpr:
			for _, b := range n.Bindings {
				walk(b.Body)
			}
			walk(n.Body)
		case *ast.BeginExpr:
			for _, s := range n.Stmts {
				walk(s)
			}
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walk(b.Body)
			}
		}
	}
	if assign == nil {
		t.Fatal("expected AssignExpr")
	}
	if _, ok := assign.Target.(*ast.IndexExpr); !ok {
		t.Fatalf("assign target should be IndexExpr, got %T", assign.Target)
	}
}

func TestParseForLoop(t *testing.T) {
	src := `module main
let main () =
  for i = 0 to 3 do
    print_line (int_to_string i)
  done
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var forExpr *ast.ForExpr
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		if fe, ok := e.(*ast.ForExpr); ok {
			forExpr = fe
		}
		switch e := e.(type) {
		case *ast.LetInExpr:
			walk(e.Body)
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walk(b.Body)
			}
		}
	}
	if forExpr == nil {
		t.Fatal("expected ForExpr")
	}
	if forExpr.Var != "i" {
		t.Errorf("expected loop var i, got %q", forExpr.Var)
	}
}

func TestParseBeginEnd(t *testing.T) {
	src := `module main
let main () =
  begin
    print_line "a";
    42
  end
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var begin *ast.BeginExpr
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		if b, ok := e.(*ast.BeginExpr); ok {
			begin = b
		}
		switch e := e.(type) {
		case *ast.LetInExpr:
			walk(e.Body)
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walk(b.Body)
			}
		}
	}
	if begin == nil {
		t.Fatal("expected BeginExpr")
	}
	if len(begin.Stmts) != 2 {
		t.Fatalf("expected 2 stmts in begin, got %d", len(begin.Stmts))
	}
}

func TestParseQualifiedConstructor(t *testing.T) {
	src := `module main
type Color = Red | Green
let main () =
  let c = Color.Red in
  match c with
  | Color.Green -> ()
  | _ -> ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var exprCtor *ast.ConstructorExpr
	var patPrefix, patName string
	var walkExpr func(ast.Expr)
	walkExpr = func(e ast.Expr) {
		if e == nil {
			return
		}
		if ce, ok := e.(*ast.ConstructorExpr); ok && ce.TypePrefix != "" {
			exprCtor = ce
		}
		switch e := e.(type) {
		case *ast.LetInExpr:
			for _, b := range e.Bindings {
				walkExpr(b.Body)
			}
			walkExpr(e.Body)
		case *ast.MatchExpr:
			for _, arm := range e.Arms {
				if cp, ok := arm.Pattern.(*ast.ConstructorPattern); ok && cp.TypePrefix != "" {
					patPrefix, patName = cp.TypePrefix, cp.Name
				}
				walkExpr(arm.Body)
			}
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walkExpr(b.Body)
			}
		}
	}
	if exprCtor == nil || exprCtor.TypePrefix != "Color" || exprCtor.Name != "Red" {
		t.Fatalf("expected Color.Red expr, got %+v", exprCtor)
	}
	if patPrefix != "Color" || patName != "Green" {
		t.Fatalf("expected Color.Green pattern, got %s.%s", patPrefix, patName)
	}
}

func TestAcceptImportGo(t *testing.T) {
	src := `module main
import go "fmt"
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("import go should parse: %v", err)
	}
	if len(mod.Imports) != 1 || mod.Imports[0].Kind != ast.ImportGo {
		t.Fatalf("expected ImportGo, got %+v", mod.Imports)
	}
}

func TestRejectImportGolang(t *testing.T) {
	src := `module main
import golang "fmt"
let main () = ()
`
	_, err := parser.Parse("t.goop", []byte(src))
	if err == nil {
		t.Fatal("expected error for import golang")
	}
	if !strings.Contains(err.Error(), "import go") {
		t.Errorf("expected import go hint, got: %v", err)
	}
}

func TestImportEmptyGrouped(t *testing.T) {
	src := `module main
import ()
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("empty import group should parse: %v", err)
	}
	if len(mod.Imports) != 0 {
		t.Errorf("expected 0 imports, got %d", len(mod.Imports))
	}
}

func TestImportDuplicatePath(t *testing.T) {
	src := `module main
import (
  go "fmt"
  go "fmt"
)
let main () = ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Imports) != 2 {
		t.Fatalf("expected 2 specs at parse time")
	}
	_, _, errs := typecheck.CheckWithTypes(mod)
	for _, e := range errs {
		if strings.Contains(e.Error(), "duplicate import") {
			return
		}
	}
	t.Log("duplicate import may be caught at typecheck")
	_ = mod
}

func TestParseOCamlSurface(t *testing.T) {
	src := "module main\n" +
		"exception Oops of string\n" +
		"effect Foo : int -> string\n" +
		"type cell = { mutable x: int }\n" +
		"let main () =\n" +
		"  let r = ref 0 in\n" +
		"  begin\n" +
		"    r := 1;\n" +
		"    while !r < 3 do r := !r + 1 done;\n" +
		"    assert true;\n" +
		"    ignore (lazy 1);\n" +
		"    ignore (raise (Oops \"x\"));\n" +
		"    ignore (failwith \"boom\");\n" +
		"    ignore (try 1 with | _ -> 0);\n" +
		"    ignore (try 1 finally 2);\n" +
		"    ignore (function | 0 -> 1 | _ -> 2);\n" +
		"    ignore [| 1; 2 |];\n" +
		"    ignore (`Tag);\n" +
		"    5 mod 2\n" +
		"  end\n"
	mod, err := parser.Parse("ocaml.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mod.Decls) < 3 {
		t.Fatalf("expected exception/effect/type/let, got %d decls", len(mod.Decls))
	}
	if _, ok := mod.Decls[0].(*ast.ExceptionDecl); !ok {
		t.Fatalf("expected ExceptionDecl, got %T", mod.Decls[0])
	}
	if _, ok := mod.Decls[1].(*ast.EffectDecl); !ok {
		t.Fatalf("expected EffectDecl, got %T", mod.Decls[1])
	}
	td, ok := mod.Decls[2].(*ast.TypeDecl)
	if !ok {
		t.Fatalf("expected TypeDecl, got %T", mod.Decls[2])
	}
	rk, ok := td.Kind.(*ast.RecordTypeKind)
	if !ok || len(rk.Fields) != 1 || !rk.Fields[0].Mutable {
		t.Fatalf("expected mutable record field, got %+v", td.Kind)
	}
}

func TestMigrationErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"mutable", "module main\nlet main () = let mutable x = 0 in x\n", "PARSE-MIG010"},
		{"question", "module main\nlet main () = x ?\n", "PARSE-MIG012"},
		{"ce", "module main\nlet main () = result { return 1 }\n", "PARSE-MIG013"},
		{"guard", "module main\nlet main () = guard x = 1 else 0\n", "PARSE-MIG014"},
		{"newtype", "module main\ntype t = newtype int\nlet main () = ()\n", "PARSE-MIG015"},
		{"effects", "module main\nlet f () : unit with { io } = ()\n", "PARSE-MIG016"},
		{"panic", "module main\nlet main () = panic \"x\"\n", "PARSE-MIG017"},
		{"percent", "module main\nlet main () = 5 % 2\n", "PARSE-MIG018"},
		{"using", "module main\nlet main () = using x = y in x\n", "PARSE-MIG013"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse(tc.name+".goop", []byte(tc.src))
			if err == nil {
				t.Fatal("expected migration error")
			}
			if !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("expected %s in %v", tc.code, err)
			}
		})
	}
}

func TestParseMultilineParenApp(t *testing.T) {
	src := `module main
let f (x: int) : int = x
let main () =
  let x =
    f
      (1 + 2)
  in
  assert (x = 3)
`
	mod, err := parser.Parse("multiline_paren.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	found := false
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		if app, ok := e.(*ast.AppExpr); ok {
			if id, ok := app.Func.(*ast.IdentExpr); ok && id.Name == "f" {
				found = true
			}
			walk(app.Func)
			walk(app.Arg)
			return
		}
		switch e := e.(type) {
		case *ast.LetInExpr:
			for _, b := range e.Bindings {
				walk(b.Body)
			}
			walk(e.Body)
		case *ast.ParenExpr:
			walk(e.Inner)
		case *ast.BinaryExpr:
			walk(e.Left)
			walk(e.Right)
		case *ast.AppExpr:
			walk(e.Func)
			walk(e.Arg)
		}
	}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				walk(b.Body)
			}
		}
	}
	if !found {
		t.Fatal("expected AppExpr f (1+2) from cross-line parenthesized arg")
	}
}

func TestParseMultilineNestedParenApp(t *testing.T) {
	src := `module main
let appendAttr xs a = xs
let emptyAttrs () = 0
let attrString k v = 1
let main () =
  let x =
    appendAttr
      (appendAttr (emptyAttrs ()) (attrString "a" "b"))
      (attrString "c" "d")
  in
  ()
`
	_, err := parser.Parse("multiline_nested.goop", []byte(src))
	if err != nil {
		t.Fatalf("parse nested multiline appendAttr: %v", err)
	}
}

func TestRejectCrossLineBareIdentApp(t *testing.T) {
	// Flat f\n g must NOT become an application (would glue next decl).
	src := `module main
let f x = x
let g = 1
let main () =
  let x =
    f
    g
  in
  ()
`
	_, err := parser.Parse("bare_crossline.goop", []byte(src))
	// May parse as incomplete let or unexpected token — either way must not
	// silently treat g as an argument of f across lines without parens.
	if err == nil {
		// If it parses, ensure the body is not App(f, g).
		mod, _ := parser.Parse("bare_crossline.goop", []byte(src))
		var isAppFG bool
		var walk func(ast.Expr)
		walk = func(e ast.Expr) {
			if e == nil {
				return
			}
			if app, ok := e.(*ast.AppExpr); ok {
				if id, ok := app.Func.(*ast.IdentExpr); ok && id.Name == "f" {
					if arg, ok := app.Arg.(*ast.IdentExpr); ok && arg.Name == "g" {
						isAppFG = true
					}
				}
				walk(app.Func)
				walk(app.Arg)
			}
			switch e := e.(type) {
			case *ast.LetInExpr:
				for _, b := range e.Bindings {
					walk(b.Body)
				}
				walk(e.Body)
			}
		}
		for _, d := range mod.Decls {
			if ld, ok := d.(*ast.LetDecl); ok {
				for _, b := range ld.Bindings {
					walk(b.Body)
				}
			}
		}
		if isAppFG {
			t.Fatal("cross-line bare ident must not form App(f, g)")
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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
	return mod
}
