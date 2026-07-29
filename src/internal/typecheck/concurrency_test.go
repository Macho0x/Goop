package typecheck_test

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
	"goop.dev/compiler/internal/types"
)

// TestChanElementTypeInfersInt verifies that Chan.make () used with a
// type annotation `: int chan` correctly infers the element type as int.
func TestChanElementTypeInfersInt(t *testing.T) {
	src := `module Main

let main () =
  let ch : int chan = Chan.make () in
  let v = Chan.recv ch in
  println (int_to_string v)
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

	// Find the Chan.make () AppExpr and verify its type is int chan.
	found := false
	for expr, typ := range tm {
		app, ok := expr.(*ast.AppExpr)
		if !ok {
			continue
		}
		field, ok := app.Func.(*ast.FieldAccessExpr)
		if !ok {
			continue
		}
		if !strings.Contains(fieldToString(field), "Chan.make") {
			continue
		}

		found = true
		tc, ok := typ.(*types.TChan)
		if !ok {
			t.Errorf("Chan.make () type should be TChan, got %T (%v)", typ, typ)
			continue
		}
		p, ok := tc.Elem.(*types.Prim)
		if !ok {
			t.Errorf("Chan element type should be concrete (Prim), got %T (%v)", tc.Elem, tc.Elem)
			continue
		}
		if p.Name != "int" {
			t.Errorf("expected chan element type 'int', got %q", p.Name)
		}
	}

	if !found {
		t.Error("did not find Chan.make () AppExpr in TypeMap")
	}
}

func fieldToString(e *ast.FieldAccessExpr) string {
	return exprName(e.Left) + "." + e.Field
}

func exprName(e ast.Expr) string {
	if ctor, ok := e.(*ast.ConstructorExpr); ok {
		return ctor.Name
	}
	if ident, ok := e.(*ast.IdentExpr); ok {
		return ident.Name
	}
	return "?"
}
