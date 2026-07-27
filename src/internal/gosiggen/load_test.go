package gosiggen

import (
	"go/token"
	gotypes "go/types"
	"testing"

	"goop.dev/compiler/internal/ast"
)

func TestParseSigContent(t *testing.T) {
	content := `
(* comment *)
type Builder
val Contains : string -> string -> bool
val Sprintf : string -> ...obj -> string
val (b : Builder ptr).Len : unit -> int
`
	types, vals, err := ParseSigContent("strings", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 || types[0].Name != "Builder" {
		t.Fatalf("types = %+v", types)
	}
	if len(vals) != 3 {
		t.Fatalf("vals len = %d", len(vals))
	}
	foundAny := false
	for _, v := range vals {
		if v.Name == "Sprintf" {
			if hasAnyIdent(v.Type) {
				foundAny = true
			}
		}
		if v.Name == "Len" && v.RecvType == nil {
			t.Error("expected method Len to have RecvType")
		}
	}
	if !foundAny {
		t.Error("expected obj rewritten to any in Sprintf")
	}
}

func hasAnyIdent(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.TIdent:
		return t.Name == "any"
	case *ast.TFun:
		return hasAnyIdent(t.From) || hasAnyIdent(t.To)
	case *ast.TVariadic:
		return hasAnyIdent(t.Elem)
	case *ast.TApp:
		return hasAnyIdent(t.Func) || hasAnyIdent(t.Arg)
	default:
		return false
	}
}

func TestMapMultiResultTuple(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	params := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "s", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "sep", gotypes.Typ[gotypes.String]),
	)
	results := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "before", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "after", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "found", gotypes.Typ[gotypes.Bool]),
	)
	sig := gotypes.NewSignatureType(nil, nil, nil, params, results, false)
	got := MapType(sig, pkg.Path())
	if !got.OK {
		t.Fatalf("expected OK, got %+v", got)
	}
	want := "string -> string -> string * string * bool"
	if got.Goop != want {
		t.Errorf("got %q, want %q", got.Goop, want)
	}
}
