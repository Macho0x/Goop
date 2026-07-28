package gosiggen

import (
	"go/token"
	gotypes "go/types"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapBasicTypes(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	cases := []struct {
		goType string
		want   string
	}{
		{"bool", "bool"},
		{"string", "string"},
		{"int", "int"},
		{"int64", "int"},
		{"uint32", "int"},
		{"float64", "float"},
		{"float32", "float"},
		{"rune", "rune"},
		{"byte", "int"},
	}
	for _, tc := range cases {
		tv, err := parseTypeViaScope(pkg, tc.goType)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.goType, err)
		}
		got := MapType(tv, pkg.Path())
		if !got.OK || got.Goop != tc.want {
			t.Errorf("MapType(%q) = %+v, want OK goop=%q", tc.goType, got, tc.want)
		}
	}
}

func TestMapAnyToObj(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	for _, src := range []string{"any", "interface{}"} {
		tv, err := parseTypeViaScope(pkg, src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		got := MapType(tv, pkg.Path())
		if !got.OK || got.Goop != "obj" {
			t.Errorf("MapType(%q) = %+v, want obj", src, got)
		}
	}
}

func TestMapPointerSliceChan(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	obj := gotypes.NewTypeName(token.NoPos, pkg, "Buffer", nil)
	named := gotypes.NewNamed(obj, gotypes.NewStruct(nil, nil), nil)
	pkg.Scope().Insert(obj)
	_ = named

	cases := []struct {
		src  string
		want string
	}{
		{"*Buffer", "Buffer ptr"},
		{"[]string", "string go_slice"},
		{"[]byte", "bytes"},
		{"chan int", "int chan"},
		{"<-chan int", "int chan"},
		{"chan<- string", "string chan"},
		{"error", "error"},
	}
	for _, tc := range cases {
		tv, err := parseTypeViaScope(pkg, tc.src)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		got := MapType(tv, pkg.Path())
		if !got.OK || got.Goop != tc.want {
			t.Errorf("MapType(%q) = %+v, want %q", tc.src, got, tc.want)
		}
	}
}

func TestMapEmitsMapType(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	tv, err := parseTypeViaScope(pkg, "map[string]int")
	if err != nil {
		t.Fatal(err)
	}
	got := MapType(tv, pkg.Path())
	if !got.OK || got.Goop != "map[string] int" {
		t.Errorf("expected map[string] int, got %+v", got)
	}
}

func TestMapTupleErrorTODOH6(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	params := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "s", gotypes.Typ[gotypes.String]))
	results := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "", gotypes.Typ[gotypes.Int]),
		gotypes.NewVar(token.NoPos, pkg, "", gotypes.Universe.Lookup("error").Type()),
	)
	sig := gotypes.NewSignatureType(nil, nil, nil, params, results, false)
	got := MapType(sig, pkg.Path())
	if !got.OK {
		t.Fatalf("expected OK mapping, got %+v", got)
	}
	if !got.TODOH6 {
		t.Error("expected TODOH6 for (T, error)")
	}
	if got.Goop != "string -> int * error" {
		t.Errorf("got %q", got.Goop)
	}
}

func TestMapFuncCurried(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	params := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "a", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "b", gotypes.Typ[gotypes.String]),
	)
	results := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "", gotypes.Typ[gotypes.Bool]))
	sig := gotypes.NewSignatureType(nil, nil, nil, params, results, false)
	got := MapType(sig, pkg.Path())
	if !got.OK || got.Goop != "string -> string -> bool" {
		t.Errorf("got %+v", got)
	}
}

func TestMapFuncParamParenthesized(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	// func(string, func(rune) bool) bool
	innerParams := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "r", gotypes.Universe.Lookup("rune").Type()))
	innerResults := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "", gotypes.Typ[gotypes.Bool]))
	inner := gotypes.NewSignatureType(nil, nil, nil, innerParams, innerResults, false)
	params := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "s", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "f", inner),
	)
	results := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "", gotypes.Typ[gotypes.Bool]))
	sig := gotypes.NewSignatureType(nil, nil, nil, params, results, false)
	got := MapType(sig, pkg.Path())
	if !got.OK {
		t.Fatalf("%+v", got)
	}
	want := "string -> (rune -> bool) -> bool"
	if got.Goop != want {
		t.Errorf("got %q, want %q", got.Goop, want)
	}
}

func TestMapVariadic(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/p", "p")
	anyType := gotypes.Universe.Lookup("any").Type()
	params := gotypes.NewTuple(
		gotypes.NewVar(token.NoPos, pkg, "format", gotypes.Typ[gotypes.String]),
		gotypes.NewVar(token.NoPos, pkg, "args", gotypes.NewSlice(anyType)),
	)
	results := gotypes.NewTuple(gotypes.NewVar(token.NoPos, pkg, "", gotypes.Typ[gotypes.String]))
	sig := gotypes.NewSignatureType(nil, nil, nil, params, results, true)
	got := MapType(sig, pkg.Path())
	if !got.OK {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Goop, "...obj") {
		t.Errorf("expected ...obj in %q", got.Goop)
	}
}

func TestCachePaths(t *testing.T) {
	cp := CachePath(filepath.Join(string(filepath.Separator), "tmp", "goop-home"), "encoding/json")
	wantSuffix := filepath.Join("build", "go-sigs", "encoding_json.gosig")
	if !strings.HasSuffix(cp, wantSuffix) {
		t.Errorf("cache path = %q, want suffix %q", cp, wantSuffix)
	}
	ov := OverridePath(filepath.Join(string(filepath.Separator), "proj"), "strings")
	wantOv := filepath.Join(string(filepath.Separator), "proj", "goop-sigs", "strings.gosig")
	if ov != wantOv {
		t.Errorf("override = %q, want %q", ov, wantOv)
	}
}

func TestResolveOverrideWins(t *testing.T) {
	dir := t.TempDir()
	ovDir := filepath.Join(dir, "goop-sigs")
	if err := WriteSig(filepath.Join(ovDir, "strings.gosig"), "type Builder\n"); err != nil {
		t.Fatal(err)
	}
	path, fromOv, exists := ResolveSigPath(dir, filepath.Join(dir, "cache-home"), "strings")
	if !exists || !fromOv {
		t.Fatalf("expected override hit, got path=%s fromOv=%v exists=%v", path, fromOv, exists)
	}
	want := filepath.Join(ovDir, "strings.gosig")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

func TestIsCurated(t *testing.T) {
	if !IsCurated("strings") {
		t.Error("strings should be curated")
	}
	if IsCurated("github.com/foo/bar") {
		t.Error("arbitrary module should not be curated")
	}
}

// parseTypeViaScope builds types manually for test cases we care about.
func parseTypeViaScope(pkg *gotypes.Package, src string) (gotypes.Type, error) {
	src = strings.TrimSpace(src)
	switch src {
	case "bool":
		return gotypes.Typ[gotypes.Bool], nil
	case "string":
		return gotypes.Typ[gotypes.String], nil
	case "int":
		return gotypes.Typ[gotypes.Int], nil
	case "int64":
		return gotypes.Typ[gotypes.Int64], nil
	case "uint32":
		return gotypes.Typ[gotypes.Uint32], nil
	case "float64":
		return gotypes.Typ[gotypes.Float64], nil
	case "float32":
		return gotypes.Typ[gotypes.Float32], nil
	case "rune":
		return gotypes.Universe.Lookup("rune").Type(), nil
	case "byte":
		return gotypes.Universe.Lookup("byte").Type(), nil
	case "any":
		return gotypes.Universe.Lookup("any").Type(), nil
	case "interface{}":
		return gotypes.NewInterfaceType(nil, nil).Complete(), nil
	case "error":
		return gotypes.Universe.Lookup("error").Type(), nil
	case "[]string":
		return gotypes.NewSlice(gotypes.Typ[gotypes.String]), nil
	case "[]byte":
		return gotypes.NewSlice(gotypes.Typ[gotypes.Byte]), nil
	case "chan int":
		return gotypes.NewChan(gotypes.SendRecv, gotypes.Typ[gotypes.Int]), nil
	case "<-chan int":
		return gotypes.NewChan(gotypes.RecvOnly, gotypes.Typ[gotypes.Int]), nil
	case "chan<- string":
		return gotypes.NewChan(gotypes.SendOnly, gotypes.Typ[gotypes.String]), nil
	case "map[string]int":
		return gotypes.NewMap(gotypes.Typ[gotypes.String], gotypes.Typ[gotypes.Int]), nil
	case "*Buffer":
		obj := pkg.Scope().Lookup("Buffer")
		if obj == nil {
			return nil, gotypes.Error{Msg: "Buffer not in scope"}
		}
		return gotypes.NewPointer(obj.Type()), nil
	default:
		return nil, gotypes.Error{Msg: "unsupported test type " + src}
	}
}
