// Package gosig provides an optional fallback for querying real Go function
// signatures using golang.org/x/tools/go/packages and go/types. This helps
// the Goop typechecker resolve lambda parameter types when Goop's own
// Hindley-Milner inference cannot determine them from local context alone.
package gosig

import (
	"context"
	"fmt"
	"sync"
	"time"

	gotypes "go/types"

	"golang.org/x/tools/go/packages"
)

// Param represents a Go function parameter with its name and Go type string.
type Param struct {
	Name string // parameter name (may be empty for unnamed params)
	Type string // Go type as a string, e.g. "int", "string", "func(int, int) bool"
}

// FuncSig holds the full Go function signature: parameter types and result types.
type FuncSig struct {
	Params  []Param
	Results []Param // result types (usually 0 or 1 for Goop FFI)
}

// cache avoids reloading the same package multiple times.
var cache sync.Map // key "importPath.funcName" → cachedResult

// loadDir is the working directory for packages.Load (project root / go.mod).
var (
	loadDirMu sync.RWMutex
	loadDir   string
)

type cachedResult struct {
	sig  *FuncSig
	typ  string    // for LookupVar
	info *TypeInfo // for LookupType
	err  error
}

// TypeKind classifies a looked-up Go named type.
type TypeKind int

const (
	TypeKindStruct TypeKind = iota
	TypeKindInterface
	TypeKindOther
)

// TypeField is an exported (or promoted) field of a Go struct.
type TypeField struct {
	Name string // exported Go field name
	Type string // Go type string via TypeString
}

// TypeInfo describes a Go named type for FFI struct/interface support.
type TypeInfo struct {
	Kind   TypeKind
	Fields []TypeField // exported (+ promoted) fields for structs; nil for interfaces
}

// SetLoadDir sets the directory used for packages.Load (typically the Go
// module root containing go.mod). Empty resets to the process working dir.
func SetLoadDir(dir string) {
	loadDirMu.Lock()
	loadDir = dir
	loadDirMu.Unlock()
	// Invalidate cache when the load directory changes.
	cache = sync.Map{}
}

func currentLoadDir() string {
	loadDirMu.RLock()
	defer loadDirMu.RUnlock()
	return loadDir
}

func loadPackage(importPath string) (*packages.Package, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedModule,
		Context: ctx,
		Dir:     currentLoadDir(),
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("loading package %q: %w", importPath, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for import path %q", importPath)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package %q has errors: %v", importPath, pkg.Errors[0])
	}
	if pkg.Types == nil || pkg.Types.Scope() == nil {
		return nil, fmt.Errorf("package %q has no type information (try adding it to go.mod?)", importPath)
	}
	return pkg, nil
}

// LookupFunc loads a Go package via packages.Load and looks up a function's
// parameter and result types. Results are cached in memory keyed by
// (importPath, funcName). A 5-second timeout prevents hung loads from
// blocking the compiler.
func LookupFunc(importPath, funcName string) (*FuncSig, error) {
	key := "fn:" + importPath + "." + funcName
	if cached, ok := cache.Load(key); ok {
		cr := cached.(cachedResult)
		return cr.sig, cr.err
	}

	pkg, err := loadPackage(importPath)
	if err != nil {
		cr := cachedResult{err: err}
		cache.Store(key, cr)
		return nil, err
	}

	obj := pkg.Types.Scope().Lookup(funcName)
	if obj == nil {
		cr := cachedResult{err: fmt.Errorf("function %q not found in package %q", funcName, importPath)}
		cache.Store(key, cr)
		return nil, cr.err
	}

	fn, ok := obj.(*gotypes.Func)
	if !ok {
		cr := cachedResult{err: fmt.Errorf("%q is not a function (got %T)", funcName, obj)}
		cache.Store(key, cr)
		return nil, cr.err
	}

	sig, ok := fn.Type().(*gotypes.Signature)
	if !ok {
		cr := cachedResult{err: fmt.Errorf("%q has non-signature type %T", funcName, fn.Type())}
		cache.Store(key, cr)
		return nil, cr.err
	}

	qual := relativeQualifier(importPath)

	fsig := &FuncSig{}

	params := sig.Params()
	fsig.Params = make([]Param, params.Len())
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		fsig.Params[i] = Param{
			Name: p.Name(),
			Type: gotypes.TypeString(p.Type(), qual),
		}
	}

	results := sig.Results()
	fsig.Results = make([]Param, results.Len())
	for i := 0; i < results.Len(); i++ {
		r := results.At(i)
		fsig.Results[i] = Param{
			Name: r.Name(),
			Type: gotypes.TypeString(r.Type(), qual),
		}
	}

	cr := cachedResult{sig: fsig}
	cache.Store(key, cr)
	return fsig, nil
}

// FuncHasTypeParams reports whether the named package-level function is generic
// (has type parameters on the signature). Used for GOSIG004 honesty warnings.
func FuncHasTypeParams(importPath, funcName string) (bool, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return false, err
	}
	obj := pkg.Types.Scope().Lookup(funcName)
	if obj == nil {
		return false, fmt.Errorf("function %q not found in package %q", funcName, importPath)
	}
	fn, ok := obj.(*gotypes.Func)
	if !ok {
		return false, fmt.Errorf("%q is not a function (got %T)", funcName, obj)
	}
	sig, ok := fn.Type().(*gotypes.Signature)
	if !ok {
		return false, fmt.Errorf("%q has non-signature type %T", funcName, fn.Type())
	}
	if tparams := sig.TypeParams(); tparams != nil && tparams.Len() > 0 {
		return true, nil
	}
	if rtparams := sig.RecvTypeParams(); rtparams != nil && rtparams.Len() > 0 {
		return true, nil
	}
	return false, nil
}

// LookupType loads a package-level named type and returns its kind and, for
// structs, exported (+ promoted) fields. Results are cached keyed by
// "ty:"+importPath+"."+typeName.
func LookupType(importPath, typeName string) (*TypeInfo, error) {
	key := "ty:" + importPath + "." + typeName
	if cached, ok := cache.Load(key); ok {
		cr := cached.(cachedResult)
		return cr.info, cr.err
	}

	pkg, err := loadPackage(importPath)
	if err != nil {
		cr := cachedResult{err: err}
		cache.Store(key, cr)
		return nil, err
	}

	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		cr := cachedResult{err: fmt.Errorf("type %q not found in package %q", typeName, importPath)}
		cache.Store(key, cr)
		return nil, cr.err
	}

	tn, ok := obj.(*gotypes.TypeName)
	if !ok {
		cr := cachedResult{err: fmt.Errorf("%q is not a type (got %T)", typeName, obj)}
		cache.Store(key, cr)
		return nil, cr.err
	}

	qual := relativeQualifier(importPath)
	info := &TypeInfo{}

	switch u := tn.Type().Underlying().(type) {
	case *gotypes.Struct:
		info.Kind = TypeKindStruct
		info.Fields = collectStructFields(u, qual)
	case *gotypes.Interface:
		info.Kind = TypeKindInterface
	default:
		info.Kind = TypeKindOther
	}

	cr := cachedResult{info: info}
	cache.Store(key, cr)
	return info, nil
}

// collectStructFields walks a Go struct, collecting exported fields and
// promoted fields from anonymous struct embeds (Go field promotion).
func collectStructFields(s *gotypes.Struct, qual gotypes.Qualifier) []TypeField {
	var fields []TypeField
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		if f.Anonymous() {
			ut := f.Type().Underlying()
			if ptr, ok := ut.(*gotypes.Pointer); ok {
				ut = ptr.Elem().Underlying()
			}
			if emb, ok := ut.(*gotypes.Struct); ok {
				fields = append(fields, collectStructFields(emb, qual)...)
				continue
			}
			// Non-struct embed (e.g. interface): include if exported.
			if f.Exported() {
				fields = append(fields, TypeField{
					Name: f.Name(),
					Type: gotypes.TypeString(f.Type(), qual),
				})
			}
			continue
		}
		if f.Exported() {
			fields = append(fields, TypeField{
				Name: f.Name(),
				Type: gotypes.TypeString(f.Type(), qual),
			})
		}
	}
	return fields
}

// Assignable reports whether the named type fromTypeName is assignable to
// toTypeName within the same package (e.g. slog.Level → slog.Leveler).
func Assignable(importPath, fromTypeName, toTypeName string) (bool, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return false, err
	}

	fromObj := pkg.Types.Scope().Lookup(fromTypeName)
	if fromObj == nil {
		return false, fmt.Errorf("type %q not found in package %q", fromTypeName, importPath)
	}
	toObj := pkg.Types.Scope().Lookup(toTypeName)
	if toObj == nil {
		return false, fmt.Errorf("type %q not found in package %q", toTypeName, importPath)
	}

	fromTN, ok := fromObj.(*gotypes.TypeName)
	if !ok {
		return false, fmt.Errorf("%q is not a type (got %T)", fromTypeName, fromObj)
	}
	toTN, ok := toObj.(*gotypes.TypeName)
	if !ok {
		return false, fmt.Errorf("%q is not a type (got %T)", toTypeName, toObj)
	}

	return gotypes.AssignableTo(fromTN.Type(), toTN.Type()), nil
}

// LookupVar loads a package-level var or const and returns its Go type string.
func LookupVar(importPath, varName string) (string, error) {
	key := "var:" + importPath + "." + varName
	if cached, ok := cache.Load(key); ok {
		cr := cached.(cachedResult)
		return cr.typ, cr.err
	}

	pkg, err := loadPackage(importPath)
	if err != nil {
		cr := cachedResult{err: err}
		cache.Store(key, cr)
		return "", err
	}

	obj := pkg.Types.Scope().Lookup(varName)
	if obj == nil {
		cr := cachedResult{err: fmt.Errorf("var/const %q not found in package %q", varName, importPath)}
		cache.Store(key, cr)
		return "", cr.err
	}

	switch obj.(type) {
	case *gotypes.Var, *gotypes.Const:
		// ok
	default:
		cr := cachedResult{err: fmt.Errorf("%q is not a var/const (got %T)", varName, obj)}
		cache.Store(key, cr)
		return "", cr.err
	}

	qual := relativeQualifier(importPath)
	typ := normalizeGoTypeString(gotypes.TypeString(obj.Type(), qual))
	cr := cachedResult{typ: typ}
	cache.Store(key, cr)
	return typ, nil
}

// normalizeGoTypeString maps untyped constants to their default Go types.
func normalizeGoTypeString(typ string) string {
	switch typ {
	case "untyped int":
		return "int"
	case "untyped float":
		return "float64"
	case "untyped string":
		return "string"
	case "untyped bool":
		return "bool"
	case "untyped rune":
		return "rune"
	default:
		return typ
	}
}

// relativeQualifier returns a types.Qualifier that strips the current package
// path from type names, making them more suitable for Goop type mapping.
func relativeQualifier(importPath string) gotypes.Qualifier {
	return func(pkg *gotypes.Package) string {
		if pkg.Path() == importPath {
			return ""
		}
		return pkg.Name()
	}
}
