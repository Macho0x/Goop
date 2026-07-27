// Package codegen emits idiomatic Go source from a typed Goop AST.
//
// Lowering strategy (see docs/design/04-go-lowering.md):
//   - ADTs → Go interface + one concrete struct per variant + constructors
//   - Records → Go structs with exported fields
//   - Pattern matching → Go type switches with default panic
//   - Option<T> / Result<T,E> → tagged structs with IsOk/MustOk methods
//   - Lists → Go slices []T
//   - Tuples → generated structs
//   - Curried functions → multi-parameter Go functions
//   - `?` operator → Go if err != nil { return err } idiom
//   - Pipelines |> → nested function calls
//   - Source maps → //line directives
package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"goop.dev/compiler/internal/active"
	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/modresolve"
	"goop.dev/compiler/internal/prelude"
	"goop.dev/compiler/internal/refine"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

// Generator emits Go source code for a Goop module.
type Generator struct {
	buf     strings.Builder
	indent  int
	srcFile string // original Goop source file path

	// source-map tracking
	srcMap *SourceMap
	goLine int // current Go output line (1-based)
	goCol  int // current Go output column (1-based)

	// module state
	moduleName string // Goop module name
	goPkg      string // Go package name
	goFileName string // suggested output file name

	// type tracking
	adts            map[string]*ast.TypeDecl // ADT declarations
	records         map[string]*ast.TypeDecl // record declarations
	importedRecords map[string]*ast.TypeDecl // records from open-imported Goop modules
	importedOptions map[string]string        // OptionGoType → owning Go package (from imported records)
	opaqueTypes     map[string]*ast.TypeDecl // opaque (linear) type declarations
	newtypes        map[string]*ast.TypeDecl // nominal newtype declarations
	classes         map[string]*ast.ClassDecl
	extensibleCases map[string][]ast.ADTCase
	usedOption      map[string]string   // Go type name → element Go type
	usedResult      map[string][]string // Go type name → [okGoType, errGoType]
	usedTuple       map[string][]string // Go type name → field Go type names
	funcRetType     map[string]string   // Goop func name → Go return type (for ?)
	funcParamCount  map[string]int      // Goop func name → number of parameters
	funcParamTypes  map[string][]string // Goop func name → Go types of parameters

	// inferred types from typechecker (for polymorphic codegen)
	typeMap    typeinfo.TypeMap
	varTypeMap typeinfo.VarTypeMap

	// name mapping
	goopToGo map[string]string // Goop name → Go name
	goToGoop map[string]string // Go name → Goop name (reverse)

	// module / import resolution
	cfg             *config.Config    // project configuration
	resolvedImports map[string]string // Goop module name → Go import path
	importPkgs      map[string]string // Go import path → Go package name
	openExports     map[string]string // exported name from open → Go package name

	// prelude bindings
	prelude *prelude.Prelude

	// extern tracking
	externImports      map[string]string   // Go import path → package name
	externNames        map[string]string   // Goop name → Go qualified name (pkg.Name)
	externRetTypes     map[string]ast.Type // Goop extern val name → full function type
	externResultCoerce map[string]bool     // H6: (T, error) → result wrapper at call site
	goTypeQual         map[string]string   // Goop type name → Go qualified name
	externMethods      map[string]externMethod
	externFields       map[string]map[string]string

	// row-polymorphic function params: funcName → field names
	rowParams    map[string][]string
	rowParamName map[string]string // funcName → row param name

	currentFunc         string // function being generated (for ? operator)
	optionCtorHint      string // OptionGoType hint while emitting typed record fields
	varCounter          int    // for generating unique tmp variable names
	needFmt             bool   // whether to import "fmt"
	needCgo             bool   // whether @[c] embeds require import "C"
	cgoPreamble         string // concatenated @[c] bodies for cgo comment
	needUnsafe          bool   // C.CString free via unsafe.Pointer
	errs                []string
	usedChan            bool // whether any channel operations are used in this module
	usedHTTP            bool // whether HTTP/JSON helpers are needed
	needsEffectRuntime  bool
	needsLazyRuntime    bool
	needsPolyvarRuntime bool

	// refinement contract tracking
	provenSites      refine.ProvenSites        // call sites proven safe by refinement solver
	funcAllProven    map[string]bool           // functions with all call sites proven
	funcPrivate      map[string]bool           // private function names
	refinementParams map[string][]refinedParam // func name → refinement-annotated params
}

type externMethod struct {
	recvGoType string
	recvGoop   string
	methodName string
	pkg        string
}

// refinedParam stores information about a refinement-annotated parameter.
type refinedParam struct {
	index int                 // parameter index
	name  string              // parameter name
	rt    *ast.RefinementType // the refinement type
}

// NewGenerator creates a new code generator.
func NewGenerator(srcFile string, cfg *config.Config) *Generator {
	return &Generator{
		srcFile:            srcFile,
		goLine:             1,
		goCol:              1,
		cfg:                cfg,
		prelude:            prelude.Default(),
		externImports:      make(map[string]string),
		externNames:        make(map[string]string),
		externRetTypes:     make(map[string]ast.Type),
		externResultCoerce: make(map[string]bool),
		goTypeQual:         make(map[string]string),
		externMethods:      make(map[string]externMethod),
		externFields:       make(map[string]map[string]string),
		rowParams:          make(map[string][]string),
		rowParamName:       make(map[string]string),
		resolvedImports:    make(map[string]string),
		importPkgs:         make(map[string]string),
		openExports:        make(map[string]string),
		adts:               make(map[string]*ast.TypeDecl),
		records:            make(map[string]*ast.TypeDecl),
		importedRecords:    make(map[string]*ast.TypeDecl),
		importedOptions:    make(map[string]string),
		opaqueTypes:        make(map[string]*ast.TypeDecl),
		newtypes:           make(map[string]*ast.TypeDecl),
		classes:            make(map[string]*ast.ClassDecl),
		extensibleCases:    make(map[string][]ast.ADTCase),
		usedOption:         make(map[string]string),
		usedResult:         make(map[string][]string),
		usedTuple:          make(map[string][]string),
		funcRetType:        make(map[string]string),
		funcParamCount:     make(map[string]int),
		funcParamTypes:     make(map[string][]string),
		goopToGo:           make(map[string]string),
		goToGoop:           make(map[string]string),
		refinementParams:   make(map[string][]refinedParam),
		funcAllProven:      make(map[string]bool),
		funcPrivate:        make(map[string]bool),
	}
}

// GoFileName returns the suggested output .go file name.
func (g *Generator) GoFileName() string {
	return g.goFileName
}

// GoPkg returns the generated Go package name.
func (g *Generator) GoPkg() string {
	return g.goPkg
}

// SourceMap returns the accumulated source-map data.
func (g *Generator) SourceMap() *SourceMap {
	return g.srcMap
}

// SetTypeMap stores the type-map produced by the type checker so that
// codegen can use inferred (resolved) types when lowering polymorphic
// operations such as channel creation.
func (g *Generator) SetTypeMap(tm typeinfo.TypeMap, vtm typeinfo.VarTypeMap) {
	g.typeMap = tm
	g.varTypeMap = vtm
}

// SetProvenSites stores the proven call sites from the refinement checker.
// Call sites in this set have had their refinement contracts proven at
// compile time, so codegen can skip emitting runtime panic guards for them.
func (g *Generator) SetProvenSites(proven refine.ProvenSites) {
	g.provenSites = proven
}

// SetRefinementMeta stores per-function refinement metadata from the checker.
func (g *Generator) SetRefinementMeta(funcAllProven map[string]bool) {
	g.funcAllProven = funcAllProven
}

// errorfAt records a hard codegen error with a Goop source position.
// No silent degradation: the compiler either succeeds or reports a
// file:line diagnostic at Goop compile time — it never emits
// known-broken Go that fails later at Go compile time.
func (g *Generator) errorfAt(loc token.SourceLoc, format string, args ...interface{}) {
	file := loc.File
	if file == "" {
		file = g.srcFile
	}
	pos := fmt.Sprintf("%s:%d:%d", file, loc.Line, loc.Column)
	g.errs = append(g.errs, fmt.Sprintf(pos+": "+format, args...))
}

// typeOf returns the inferred type for an expression node, or nil if
// no type was recorded.
func (g *Generator) typeOf(node ast.Expr) types.Type {
	if g.typeMap == nil {
		return nil
	}
	return g.typeMap[node]
}

// internalTypeToGo converts an internal types.Type to a Go type string.
// It handles the basic set of types that can appear as channel element types.
func (g *Generator) internalTypeToGo(t types.Type) string {
	if t == nil {
		return "interface{}"
	}
	switch t := t.(type) {
	case *types.Prim:
		switch t.Name {
		case "int":
			return "int"
		case "float":
			return "float64"
		case "bool":
			return "bool"
		case "string":
			return "string"
		case "unit":
			return "struct{}"
		case "any":
			return "interface{}"
		default:
			return t.Name
		}
	case *types.TChan:
		return "*C0Chan"
	case *types.TMap:
		return "map[" + g.internalTypeToGo(t.Key) + "]" + g.internalTypeToGo(t.Val)
	case *types.TVar:
		return "interface{}"
	case *types.TCon:
		if t.Name == "list" && len(t.Args) > 0 {
			return "[]" + g.internalTypeToGo(t.Args[0])
		}
		if t.Name == "array" && len(t.Args) > 0 {
			return "[]" + g.internalTypeToGo(t.Args[0])
		}
		if t.Name == "ref" && len(t.Args) > 0 {
			return "*" + g.internalTypeToGo(t.Args[0])
		}
		if t.Name == "lazy" && len(t.Args) > 0 {
			return "func() " + g.internalTypeToGo(t.Args[0])
		}
		if t.Name == "option" && len(t.Args) > 0 {
			return "Option" + optionTypeSuffix(g.internalTypeToGo(t.Args[0]))
		}
		if t.Name == "result" && len(t.Args) > 0 {
			arg := g.internalTypeToGo(t.Args[0])
			return "Result" + optionTypeSuffix(arg)
		}
		return "interface{}"
	case *types.TRecord:
		return g.internalRecordToGo(t)
	case *types.TNewtype:
		return newtypeGoName(t.Name)
	case *types.TAdt:
		if t.Name == "owned_chan" {
			return "*C0Chan"
		}
		return g.goName(t.Name)
	case *types.TGoNamed:
		return t.Pkg + "." + t.Name
	case *types.TPtr:
		return "*" + g.internalTypeToGo(t.Elem)
	case *types.TGoSlice:
		return "[]" + g.internalTypeToGo(t.Elem)
	default:
		return "interface{}"
	}
}

func (g *Generator) internalRecordToGo(t *types.TRecord) string {
	for name, td := range g.records {
		if rk, ok := td.Kind.(*ast.RecordTypeKind); ok {
			if len(rk.Fields) == len(t.Fields) {
				match := true
				for i, f := range rk.Fields {
					if f.Name != t.Fields[i].Name {
						match = false
						break
					}
				}
				if match {
					return g.goName(name)
				}
			}
		}
	}
	for name, td := range g.importedRecords {
		if rk, ok := td.Kind.(*ast.RecordTypeKind); ok {
			if len(rk.Fields) == len(t.Fields) {
				match := true
				for i, f := range rk.Fields {
					if f.Name != t.Fields[i].Name {
						match = false
						break
					}
				}
				if match {
					if pkg, ok := g.openExports[name]; ok {
						return pkg + "." + exported(name)
					}
					return exported(name)
				}
			}
		}
	}
	return "interface{}"
}

// recordMapping records a Goop→Go position mapping at the current Go output position.
func (g *Generator) recordMapping(c0Line, c0Col int) {
	if g.srcMap != nil {
		g.srcMap.Add(c0Line, c0Col, g.goLine, g.goCol)
	}
}

// Generate produces Go source code from a Goop module.
func (g *Generator) Generate(mod *ast.Module) (string, error) {
	g.moduleName = mod.Name
	g.goPkg = goPkgName(mod.Name)
	g.goFileName = goFileName(mod.Name)

	// Flatten nested modules for emission
	flat := *mod
	flat.Decls = flattenDecls(mod.Decls)
	mod = &flat

	// Initialise source map (generated path is the Go file, source is the Goop file)
	g.srcMap = NewSourceMap(g.srcFile, g.goFileName)

	// Register Goop → Go name mappings
	g.goopToGo["int"] = "int"
	g.goopToGo["float"] = "float64"
	g.goopToGo["bool"] = "bool"
	g.goopToGo["string"] = "string"
	g.goopToGo["unit"] = "struct{}"

	// Pre-scan: collect type and function declarations
	g.prescan(mod)

	// Resolve imports (go + goop)
	g.collectImports(mod)

	// Build the body first so we know what imports are needed
	var body strings.Builder
	origBuf := g.buf
	g.buf = body

	// Tuple type definitions
	g.emitTupleTypes()
	// Option/Result type definitions
	g.emitOptionTypes()
	g.emitResultTypes()
	// Record type definitions
	g.emitRecordTypes()
	// ADT type definitions
	g.emitADTTypes()
	// Nominal newtype definitions
	g.emitNewtypeTypes()
	// Opaque type definitions (linear types erased in Go)
	g.emitOpaqueTypes()

	// Top-level value declarations
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			g.emitLetDecl(d)
		case *ast.ImplementsDecl:
			g.emitImplementsDecl(d)
		case *ast.LangEmbedDecl:
			g.emitLangEmbedDecl(d)
		case *ast.ClassDecl:
			g.emitClassDecl(d)
		case *ast.ExceptionDecl:
			g.emitf("var %s = %#v\n\n", g.goName(d.Name), d.Name)
		case *ast.EffectDecl:
			// Effect ops are values used with perform; emit as tagged strings.
			g.emitf("var %s = %#v\n\n", g.goName(d.Name), d.Name)
		}
	}

	// Emit C0Chan wrapper if channels are used
	g.emitChanHelpers()

	// Emit HTTP/JSON helpers if needed
	g.emitHTTPHelpers()

	// Effect CPS runtime
	g.emitEffectHelpers()
	g.emitLazyHelpers()
	g.emitPolyvarHelpers()
	g.emitGoSliceHelpers()

	bodyStr := g.buf.String()
	g.buf = origBuf

	// Emit header with source map
	g.emitLine(1, 1)
	g.emitf("// generated by c0; do not edit\n")
	g.emitf("package %s\n\n", g.goPkg)

	// cgo preamble must sit immediately before import "C"
	if g.needCgo {
		g.buf.WriteString("/*\n")
		headers := ""
		if !strings.Contains(g.cgoPreamble, "stdlib.h") {
			headers += "#include <stdlib.h>\n"
		}
		if !strings.Contains(g.cgoPreamble, "stdint.h") {
			headers += "#include <stdint.h>\n"
		}
		g.buf.WriteString(headers)
		g.buf.WriteString(strings.TrimSpace(g.cgoPreamble))
		g.buf.WriteString("\n*/\n")
		g.emitf("import \"C\"\n\n")
	}

	// Imports (now we know if fmt is needed)
	g.emitImports()

	// Body
	g.writeStr(bodyStr)

	if len(g.errs) > 0 {
		return g.buf.String(), fmt.Errorf("%s", strings.Join(g.errs, "\n"))
	}
	return g.buf.String(), nil
}

// ---------------------------------------------------------------------------
// Pre-scan: gather type declarations, function return types
// ---------------------------------------------------------------------------

func (g *Generator) prescan(mod *ast.Module) {
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			switch d.Kind.(type) {
			case *ast.ADTTypeKind, *ast.GADTTypeKind:
				g.adts[d.Name] = d
				goName := g.goName(d.Name)
				g.goopToGo[d.Name] = goName
			case *ast.ExtensibleTypeKind:
				g.adts[d.Name] = d
				g.goopToGo[d.Name] = g.goName(d.Name)
			case *ast.RecordTypeKind:
				g.records[d.Name] = d
				goName := g.goName(d.Name)
				g.goopToGo[d.Name] = goName
			case *ast.OpaqueTypeKind:
				// Opaque linear type — erased in Go output.
				// Register the Go name so references resolve correctly.
				g.opaqueTypes[d.Name] = d
				goName := g.goName(d.Name)
				g.goopToGo[d.Name] = goName
			case *ast.AliasTypeKind:
				// Aliases map to their underlying type
			case *ast.NewtypeTypeKind:
				g.newtypes[d.Name] = d
				goName := newtypeGoName(d.Name)
				g.goopToGo[d.Name] = goName
			}
		case *ast.ClassDecl:
			g.goopToGo[d.Name] = g.goName(d.Name)
			g.classes[d.Name] = d
		case *ast.ExtensibleVariantDecl:
			g.extensibleCases[d.TypeName] = append(g.extensibleCases[d.TypeName], d.Cases...)
		case *ast.LetDecl:
			for _, b := range d.Bindings {
				emitName := b.Name
				if !d.ActivePattern {
					if !d.Private && b.Name != "main" {
						emitName = exported(b.Name)
					}
					g.goopToGo[b.Name] = emitName
				}
				if b.RetType != nil {
					g.funcRetType[b.Name] = g.typeToGo(b.RetType)
				}
				if containsCPSPerform(b.Body) {
					// Effectful functions return an effect request until a
					// surrounding handler resumes its continuation.
					g.funcRetType[b.Name] = "interface{}"
				}
				// Filter empty-named and unit params (OCaml `()` / `_u: unit`
				// lower to zero Go parameters).
				realCount := 0
				for _, p := range b.Params {
					if p.Name != "" && !isUnitParam(p) {
						realCount++
					}
				}
				g.funcParamCount[b.Name] = realCount
				g.funcParamCount[emitName] = realCount
				// Detect row-polymorphic parameters
				for _, p := range b.Params {
					if p.Type != nil {
						if rt, ok := p.Type.(*ast.TRecord); ok && rt.Open {
							var fields []string
							for _, f := range rt.Fields {
								fields = append(fields, f.Name)
							}
							g.rowParams[b.Name] = fields
							g.rowParamName[b.Name] = p.Name
						}
					}
				}
				// Store parameter Go types for partial application
				paramTypes := make([]string, 0)
				for _, p := range b.Params {
					if p.Name != "" {
						if p.Type != nil {
							paramTypes = append(paramTypes, g.typeToGo(p.Type))
						} else {
							paramTypes = append(paramTypes, "interface{}")
						}
					}
				}
				g.funcParamTypes[b.Name] = paramTypes
			}
		}
	}

	// Scan record and ADT field types for option/result/tuple usage.
	for _, d := range mod.Decls {
		td, ok := d.(*ast.TypeDecl)
		if !ok {
			continue
		}
		switch kind := td.Kind.(type) {
		case *ast.RecordTypeKind:
			for _, f := range kind.Fields {
				g.scanUsedTypes(f.Type)
			}
		case *ast.ADTTypeKind:
			for _, c := range kind.Cases {
				if c.Arg != nil {
					g.scanUsedTypes(c.Arg)
				}
			}
		case *ast.GADTTypeKind:
			for _, c := range kind.Cases {
				if c.Arg != nil {
					g.scanUsedTypes(c.Arg)
				}
				if c.Result != nil {
					g.scanUsedTypes(c.Result)
				}
			}
		}
	}

	// Scan for used option/result/tuple types in return type annotations
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for _, b := range ld.Bindings {
				if b.RetType != nil {
					g.scanUsedTypes(b.RetType)
				}
				// Also scan parameter types
				for _, p := range b.Params {
					if p.Type != nil {
						g.scanUsedTypes(p.Type)
					}
				}
				// Scan expression for literals that imply types
				g.scanExprTypes(b.Body)
			}
		}
	}

	// Discover option/result types from the typechecker map (e.g. Map.get → 'v option).
	g.scanTypeMapUsedTypes()
}

func (g *Generator) scanUsedTypes(at ast.Type) {
	if at == nil {
		return
	}
	switch t := at.(type) {
	case *ast.TApp:
		// Check for option/result/list type constructors
		if ident, ok := t.Func.(*ast.TIdent); ok {
			switch ident.Name {
			case "option":
				elemGo := g.typeToGo(t.Arg)
				goType := "Option" + optionTypeSuffix(elemGo)
				g.usedOption[goType] = elemGo
			case "result":
				elemGo := g.typeToGo(t.Arg)
				goType := "Result" + exported(elemGo)
				// Store [okType, errType] separately
				okType, errType := "interface{}", "interface{}"
				if tup, ok := t.Arg.(*ast.TTuple); ok && len(tup.Elems) >= 2 {
					okType = g.typeToGo(tup.Elems[0])
					errType = g.typeToGo(tup.Elems[1])
				}
				g.usedResult[goType] = []string{okType, errType}
			}
		}
		g.scanUsedTypes(t.Func)
		g.scanUsedTypes(t.Arg)
	case *ast.TTuple:
		if len(t.Elems) >= 2 {
			goType, parts := g.tupleGoName(t)
			g.usedTuple[goType] = parts
		}
		for _, e := range t.Elems {
			g.scanUsedTypes(e)
		}
	case *ast.TFun:
		g.scanUsedTypes(t.From)
		g.scanUsedTypes(t.To)
	case *ast.TRecord:
		for _, f := range t.Fields {
			g.scanUsedTypes(f.Type)
		}
	case *ast.RefinementType:
		g.scanUsedTypes(t.Inner)
	case *ast.TChan:
		g.scanUsedTypes(t.Elem)
	case *ast.TMap:
		g.scanUsedTypes(t.Key)
		g.scanUsedTypes(t.Val)
	}
}

func (g *Generator) scanTypeMapUsedTypes() {
	if g.typeMap == nil {
		return
	}
	for _, t := range g.typeMap {
		g.scanInternalUsedTypes(t)
	}
	if g.varTypeMap != nil {
		for _, t := range g.varTypeMap {
			g.scanInternalUsedTypes(t)
		}
	}
}

func (g *Generator) scanInternalUsedTypes(t types.Type) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *types.TCon:
		if t.Name == "option" && len(t.Args) > 0 {
			elemGo := g.internalTypeToGo(t.Args[0])
			goType := "Option" + optionTypeSuffix(elemGo)
			g.usedOption[goType] = elemGo
		}
		if t.Name == "result" && len(t.Args) > 0 {
			okType, errType := "interface{}", "interface{}"
			if len(t.Args) >= 1 {
				okType = g.internalTypeToGo(t.Args[0])
			}
			if len(t.Args) >= 2 {
				errType = g.internalTypeToGo(t.Args[1])
			}
			goType := "Result" + optionTypeSuffix(okType)
			g.usedResult[goType] = []string{okType, errType}
		}
		for _, a := range t.Args {
			g.scanInternalUsedTypes(a)
		}
	case *types.TMap:
		g.scanInternalUsedTypes(t.Key)
		g.scanInternalUsedTypes(t.Val)
	case *types.TChan:
		g.scanInternalUsedTypes(t.Elem)
	case *types.TFun:
		g.scanInternalUsedTypes(t.From)
		g.scanInternalUsedTypes(t.To)
	case *types.TTuple:
		for _, e := range t.Elems {
			g.scanInternalUsedTypes(e)
		}
	case *types.TPtr:
		g.scanInternalUsedTypes(t.Elem)
	case *types.TGoSlice:
		g.scanInternalUsedTypes(t.Elem)
	}
}

func (g *Generator) scanExprTypes(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.IfExpr:
		g.scanExprTypes(e.ThenBranch)
		g.scanExprTypes(e.ElseBranch)
	case *ast.MatchExpr:
		for _, arm := range e.Arms {
			g.scanExprTypes(arm.Body)
		}
	case *ast.LetInExpr:
		for _, b := range e.Bindings {
			g.scanExprTypes(b.Body)
		}
		g.scanExprTypes(e.Body)
	case *ast.FunExpr:
		g.scanExprTypes(e.Body)
	case *ast.BinaryExpr:
		g.scanExprTypes(e.Left)
		g.scanExprTypes(e.Right)
	case *ast.AppExpr:
		g.scanExprTypes(e.Func)
		g.scanExprTypes(e.Arg)
	case *ast.TupleExpr:
		for _, el := range e.Elems {
			g.scanExprTypes(el)
		}
	}
}

// ---------------------------------------------------------------------------
// Name mapping helpers
// ---------------------------------------------------------------------------

func (g *Generator) goName(c0Name string) string {
	if mapped, ok := g.goopToGo[c0Name]; ok {
		return mapped
	}
	return c0Name
}

func exported(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func unexported(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[size:]
}

func goPkgName(modulePath string) string {
	// Last segment, lowercased
	parts := strings.Split(modulePath, ".")
	return strings.ToLower(parts[len(parts)-1])
}

func goFileName(modulePath string) string {
	parts := strings.Split(modulePath, ".")
	return strings.ToLower(parts[len(parts)-1]) + ".go"
}

// ---------------------------------------------------------------------------
// Type → Go type string
// ---------------------------------------------------------------------------

// flattenFunFrom extracts the argument types from a curried function type chain.
func (g *Generator) flattenFunFrom(t *ast.TFun) []string {
	var result []string
	current := ast.Type(t)
	for {
		if fn, ok := current.(*ast.TFun); ok {
			result = append(result, g.typeToGo(fn.From))
			current = fn.To
		} else {
			break
		}
	}
	return result
}

// lastReturnType extracts the final (non-function) return type from a curried
// function type chain.  E.g. for float -> float -> bool, returns "bool".
func (g *Generator) lastReturnType(t *ast.TFun) string {
	current := ast.Type(t.To)
	for {
		if fn, ok := current.(*ast.TFun); ok {
			current = fn.To
		} else {
			return g.typeToGo(current)
		}
	}
}

func (g *Generator) typeToGo(at ast.Type) string {
	if at == nil {
		return "struct{}"
	}
	switch t := at.(type) {
	case *ast.TIdent:
		switch t.Name {
		case "int":
			return "int"
		case "float":
			return "float64"
		case "bool":
			return "bool"
		case "string":
			return "string"
		case "unit":
			return "struct{}"
		case "any":
			return "interface{}"
		case "list":
			return "list"
		case "option":
			return "option"
		case "result":
			return "result"
		case "owned_chan":
			return "owned_chan"
		case "error":
			return "error"
		default:
			if qualified, ok := g.goTypeQual[t.Name]; ok {
				return qualified
			}
			return g.goName(t.Name)
		}
	case *ast.TApp:
		funcName := g.typeToGo(t.Func)
		argName := g.typeToGo(t.Arg)
		switch funcName {
		case "list":
			return "[]" + argName
		case "array":
			return "[]" + argName
		case "option":
			return "Option" + optionTypeSuffix(argName)
		case "ref":
			return "*" + argName
		case "result":
			// result applied to a tuple → result<A, B>
			return "Result" + exported(argName)
		case "owned_chan":
			return "*C0Chan"
		default:
			return funcName + "_" + argName
		}
	case *ast.TFun:
		// Fully flatten curried function: float -> float -> bool → func(float64, float64) bool
		allParams := g.flattenFunFrom(t)
		finalReturn := g.lastReturnType(t)
		return fmt.Sprintf("func(%s) %s", strings.Join(allParams, ", "), finalReturn)
	case *ast.TTuple:
		name, _ := g.tupleGoName(t)
		return name
	case *ast.TVar:
		return "interface{}"
	case *ast.TRecord:
		return g.goName(g.recordNameFromType(t))
	case *ast.TChan:
		return "*C0Chan"
	case *ast.TMap:
		return "map[" + g.typeToGo(t.Key) + "]" + g.typeToGo(t.Val)
	case *ast.TPtr:
		return "*" + g.typeToGo(t.Elem)
	case *ast.TGoSlice:
		return "[]" + g.typeToGo(t.Elem)
	case *ast.TVariadic:
		return "..." + g.typeToGo(t.Elem)
	case *ast.RefinementType:
		// Refinement types are transparent — only the inner type matters
		return g.typeToGo(t.Inner)
	default:
		return "interface{}"
	}
}

// optionTypeSuffix turns an element Go type into a valid identifier suffix.
// External types (for example io.Writer) cannot be used verbatim in an
// identifier such as OptionIo.Writer.
func optionTypeSuffix(goType string) string {
	if dot := strings.LastIndex(goType, "."); dot >= 0 {
		goType = goType[dot+1:]
	}
	var b strings.Builder
	for _, r := range goType {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return exported(b.String())
}

func (g *Generator) recordNameFromType(t *ast.TRecord) string {
	// Try to find which declared record type matches these fields
	for name, td := range g.records {
		if rk, ok := td.Kind.(*ast.RecordTypeKind); ok {
			if len(rk.Fields) == len(t.Fields) {
				match := true
				for i, f := range rk.Fields {
					if i >= len(t.Fields) || f.Name != t.Fields[i].Name {
						match = false
						break
					}
				}
				if match {
					return name
				}
			}
		}
	}
	return "Record"
}

// ---------------------------------------------------------------------------
// Source map: //line directives
// ---------------------------------------------------------------------------

func (g *Generator) emitLine(line, col int) {
	if g.srcFile != "" {
		g.emitf("//line %s:%d:%d\n", g.srcFile, line, col)
	}
	// After a //line directive, the next Go line maps to the given Goop line.
	// Record this remapping so the source map reflects it.
	g.recordMapping(line, col)
}

// ---------------------------------------------------------------------------
// Output helpers — all buffer writes go through writeStr for position tracking
// ---------------------------------------------------------------------------

// writeStr appends s to the output buffer, updating the Go line/column tracker.
func (g *Generator) writeStr(s string) {
	for _, r := range s {
		if r == '\n' {
			g.goLine++
			g.goCol = 1
		} else {
			g.goCol++
		}
	}
	g.buf.WriteString(s)
}

func (g *Generator) emitf(format string, args ...any) {
	s := g.indentStr() + fmt.Sprintf(format, args...)
	g.writeStr(s)
}

func (g *Generator) emit(s string) {
	g.writeStr(g.indentStr() + s)
}

func (g *Generator) indentStr() string {
	return strings.Repeat("\t", g.indent)
}

// ---------------------------------------------------------------------------
// Imports
// ---------------------------------------------------------------------------

// collectImports gathers go and goop imports and export qualifiers.
func (g *Generator) collectImports(mod *ast.Module) {
	root := findProjectRootFromFile(g.srcFile)
	var resolver *modresolve.Resolver
	var deps map[string]*ast.Module
	if g.cfg != nil {
		resolver = modresolve.New(g.cfg, nil, root)
		deps, _ = resolver.LoadModuleGraph(g.srcFile, mod)
	}

	for _, spec := range mod.Imports {
		switch spec.Kind {
		case ast.ImportGo:
			g.collectGoImport(spec)
		case ast.ImportGoop:
			if resolver == nil {
				continue
			}
			resolved, err := resolver.ResolveGoopPath(spec.Path)
			if err != nil {
				continue
			}
			alias := modresolve.ImportAlias(spec, resolved)
			pkg := resolved.PkgName
			if alias != "" && alias != "." {
				pkg = alias
			}
			if resolved.GoImportPath != "" {
				if alias == "." {
					g.importPkgs[resolved.GoImportPath] = resolved.PkgName
				} else if g.goPkg != pkg {
					g.importPkgs[resolved.GoImportPath] = pkg
					g.resolvedImports[spec.Path] = resolved.GoImportPath
				}
			}
			if alias == "." {
				dep := deps[resolved.GoImportPath]
				if dep != nil {
					g.collectDotExports(dep, resolved.PkgName)
				}
			}
		}
	}

	for _, d := range mod.Decls {
		if ge, ok := d.(*ast.LangEmbedDecl); ok {
			if ge.Lang == "c" {
				g.needCgo = true
				if ge.Body != "" {
					if g.cgoPreamble != "" {
						g.cgoPreamble += "\n"
					}
					g.cgoPreamble += ge.Body
				}
			}
			for _, ev := range ge.Vals {
				g.externNames[ev.Name] = ev.Name
				g.externRetTypes[ev.Name] = ev.Type
				g.markExternResultCoerce(ev.Name, ev.Type)
				if ret := finalReturnASTType(ev.Type); ret != nil {
					g.scanUsedTypes(ret)
				}
			}
		}
	}
}

func (g *Generator) collectGoImport(spec ast.ImportSpec) {
	if spec.Path != "" {
		g.externImports[spec.Path] = packageNameFromPath2(spec.Path)
	}
	for _, ev := range spec.Vals {
		pkgName := packageNameFromPath2(spec.Path)
		if ev.Kind == ast.ExternMethod {
			g.externMethods[ev.Name] = externMethod{
				recvGoType: g.typeToGo(ev.RecvType),
				recvGoop:   goopTypeName(ev.RecvType),
				methodName: ev.Name,
				pkg:        pkgName,
			}
			g.externRetTypes[ev.Name] = ev.Type
			if !spec.Raw {
				g.markExternResultCoerce(ev.Name, ev.Type)
			}
			continue
		}
		if ev.Kind == ast.ExternField {
			recvName := goopTypeName(ev.RecvType)
			if recvName != "" {
				if g.externFields[recvName] == nil {
					g.externFields[recvName] = make(map[string]string)
				}
				g.externFields[recvName][ev.Name] = ev.Name
			}
			g.externRetTypes[ev.Name] = ev.Type
			continue
		}
		var qualified string
		if spec.Path == "" {
			qualified = ev.Name
		} else {
			qualified = pkgName + "." + ev.Name
		}
		g.externNames[ev.Name] = qualified
		g.externRetTypes[ev.Name] = ev.Type
		if !spec.Raw {
			g.markExternResultCoerce(ev.Name, ev.Type)
		}
		if ret := finalReturnASTType(ev.Type); ret != nil {
			g.scanUsedTypes(ret)
		}
	}
	for _, et := range spec.Types {
		g.goTypeQual[et.Name] = packageNameFromPath2(spec.Path) + "." + et.Name
	}
}

// markExternResultCoerce records that calls to name should wrap a Go (T, error)
// return into a Goop result value (H6).
func (g *Generator) markExternResultCoerce(name string, typ ast.Type) {
	if tup := tupleErrorReturn(typ); tup != nil {
		g.externResultCoerce[name] = true
		g.registerResultFromTupleError(tup)
	}
}

func isASTErrorType(t ast.Type) bool {
	id, ok := t.(*ast.TIdent)
	return ok && id.Name == "error"
}

// tupleErrorReturn returns the (T, error) final return of typ, or nil.
func tupleErrorReturn(typ ast.Type) *ast.TTuple {
	ret := finalReturnASTType(typ)
	tup, ok := ret.(*ast.TTuple)
	if !ok || len(tup.Elems) != 2 || !isASTErrorType(tup.Elems[1]) {
		return nil
	}
	return tup
}

func (g *Generator) registerResultFromTupleError(tup *ast.TTuple) {
	okType := g.typeToGo(tup.Elems[0])
	errType := g.typeToGo(tup.Elems[1])
	tupleGo, _ := g.tupleGoName(tup)
	resType := "Result" + exported(tupleGo)
	g.usedResult[resType] = []string{okType, errType}
}

func (g *Generator) resultGoTypeForTupleError(tup *ast.TTuple) string {
	tupleGo, _ := g.tupleGoName(tup)
	return "Result" + exported(tupleGo)
}

func goopTypeName(t ast.Type) string {
	switch t := t.(type) {
	case *ast.TIdent:
		return t.Name
	case *ast.TPtr:
		return goopTypeName(t.Elem)
	}
	return ""
}

func (g *Generator) collectDotExports(dep *ast.Module, goPkg string) {
	for _, d := range dep.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			if d.Private {
				continue
			}
			for _, b := range d.Bindings {
				g.openExports[b.Name] = goPkg
				emitName := b.Name
				if b.Name != "main" {
					emitName = exported(b.Name)
				}
				realCount := 0
				paramTypes := make([]string, 0)
				for _, p := range b.Params {
					if p.Name != "" && !isUnitParam(p) {
						realCount++
						if p.Type != nil {
							paramTypes = append(paramTypes, g.typeToGo(p.Type))
						} else {
							paramTypes = append(paramTypes, "interface{}")
						}
					}
				}
				g.funcParamCount[b.Name] = realCount
				g.funcParamCount[emitName] = realCount
				g.funcParamTypes[b.Name] = paramTypes
				if b.RetType != nil {
					g.funcRetType[b.Name] = g.typeToGo(b.RetType)
				}
				g.goopToGo[b.Name] = emitName
			}
		case *ast.TypeDecl:
			if d.Private {
				continue
			}
			g.openExports[d.Name] = goPkg
			if rk, ok := d.Kind.(*ast.RecordTypeKind); ok {
				g.importedRecords[d.Name] = d
				g.goopToGo[d.Name] = exported(d.Name)
				g.registerImportedOptionFields(rk, goPkg)
			}
		}
	}
}

// registerImportedOptionFields maps option field Go types to the package that
// owns the record, so Some/None lower to pkg.NewOptionTSome (not local undefs).
func (g *Generator) registerImportedOptionFields(rk *ast.RecordTypeKind, goPkg string) {
	for _, f := range rk.Fields {
		g.registerImportedOptionType(f.Type, goPkg)
	}
}

func (g *Generator) registerImportedOptionType(t ast.Type, goPkg string) {
	if t == nil {
		return
	}
	switch typ := t.(type) {
	case *ast.TApp:
		if ident, ok := typ.Func.(*ast.TIdent); ok && ident.Name == "option" {
			elemGo := g.typeToGo(typ.Arg)
			goType := "Option" + optionTypeSuffix(elemGo)
			g.importedOptions[goType] = goPkg
			return
		}
		g.registerImportedOptionType(typ.Func, goPkg)
		g.registerImportedOptionType(typ.Arg, goPkg)
	case *ast.TTuple:
		for _, e := range typ.Elems {
			g.registerImportedOptionType(e, goPkg)
		}
	case *ast.TFun:
		g.registerImportedOptionType(typ.From, goPkg)
		g.registerImportedOptionType(typ.To, goPkg)
	case *ast.TRecord:
		for _, f := range typ.Fields {
			g.registerImportedOptionType(f.Type, goPkg)
		}
	case *ast.RefinementType:
		g.registerImportedOptionType(typ.Inner, goPkg)
	}
}

// resolveCallTarget returns the Go qualified name for a call target,
// including package prefix for open-imported lets.
func (g *Generator) resolveCallTarget(name string) string {
	if pkg, ok := g.openExports[name]; ok {
		return pkg + "." + exported(name)
	}
	return g.goName(name)
}

// resolveOpens is deprecated; use collectImports.
func (g *Generator) resolveOpens(mod *ast.Module) {
	_ = mod
}

// collectExterns is deprecated; use collectImports.
func (g *Generator) collectExterns(mod *ast.Module) {
	_ = mod
}

func (g *Generator) collectExternDecl(ed *ast.ExternDecl) {
	if ed.Lang != "go" {
		return
	}
	pkgName := packageNameFromPath2(ed.Path)
	if ed.Path != "" {
		g.externImports[ed.Path] = pkgName
	}
	for _, ev := range ed.Vals {
		var qualified string
		if ed.Path == "" {
			qualified = ev.Name
		} else {
			qualified = pkgName + "." + ev.Name
		}
		g.externNames[ev.Name] = qualified
		g.externRetTypes[ev.Name] = ev.Type
		if ret := finalReturnASTType(ev.Type); ret != nil {
			g.scanUsedTypes(ret)
		}
	}
}

// finalReturnASTType walks a curried function type to its final return type.
func finalReturnASTType(t ast.Type) ast.Type {
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

func (g *Generator) tupleGoName(t *ast.TTuple) (string, []string) {
	parts := make([]string, len(t.Elems))
	for i, e := range t.Elems {
		parts[i] = g.typeToGo(e)
	}
	if len(parts) == 2 {
		return parts[0] + "_and_" + parts[1], parts
	}
	return strings.Join(parts, "_and_"), parts
}

func (g *Generator) externReturnTuple(funcName string) *ast.TTuple {
	t, ok := g.externRetTypes[funcName]
	if !ok {
		return nil
	}
	// H6: (T, error) is wrapped as result, not as a product tuple.
	if g.externResultCoerce[funcName] {
		return nil
	}
	ret := finalReturnASTType(t)
	tup, ok := ret.(*ast.TTuple)
	if !ok || len(tup.Elems) < 2 {
		return nil
	}
	return tup
}

// externReturnErrorTuple returns the (T, error) declaration when H6 coercion applies.
func (g *Generator) externReturnErrorTuple(funcName string) *ast.TTuple {
	if !g.externResultCoerce[funcName] {
		return nil
	}
	t, ok := g.externRetTypes[funcName]
	if !ok {
		return nil
	}
	return tupleErrorReturn(t)
}

func (g *Generator) collectOpenExports(mod *ast.Module) {
	_ = mod
}

func findProjectRootFromFile(srcFile string) string {
	dir, err := filepath.Abs(filepath.Dir(srcFile))
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "goop.toml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "std")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func localGoopPathForImport(projectRoot, goImport string) string {
	const prefix = "github.com/Macho0x/Goop/"
	if !strings.HasPrefix(goImport, prefix) {
		return ""
	}
	rel := strings.TrimPrefix(goImport, prefix)
	pkg := filepath.Base(rel)
	return filepath.Join(projectRoot, filepath.FromSlash(rel), pkg+modresolve.SourceExt)
}

func (g *Generator) emitImports() {
	imports := make(map[string]string) // path → alias
	if g.needFmt {
		imports["fmt"] = "fmt"
	}
	// Add resolved imports from open statements
	for _, goPath := range g.resolvedImports {
		pkg := g.importPkgs[goPath]
		// If the Go package name matches the path's last segment, no alias needed
		if packageNameFromPath2(goPath) == pkg {
			imports[goPath] = pkg
		} else {
			imports[goPath] = pkg
		}
	}
	// Add extern imports
	for path, pkg := range g.externImports {
		imports[path] = pkg
	}
	// Add prelude imports (strconv, etc.)
	for path, pkg := range g.importPkgs {
		if path != "fmt" && path != "" {
			imports[path] = pkg
		}
	}
	// Add HTTP/JSON imports if needed
	if g.usedHTTP {
		imports["net/http"] = "http"
		imports["encoding/json"] = "json"
		imports["io"] = "io"
		imports["strconv"] = "strconv"
		imports["time"] = "time"
	}
	if g.needsLazyRuntime {
		imports["sync"] = "sync"
	}
	if g.needUnsafe {
		imports["unsafe"] = "unsafe"
	}
	if len(imports) == 0 {
		return
	}
	if len(imports) == 1 {
		for path := range imports {
			g.emitf("import %q\n\n", path)
		}
		return
	}
	g.emitf("import (\n")
	g.indent++
	for path := range imports {
		g.emitf("%q\n", path)
	}
	g.indent--
	g.emitf(")\n\n")
}

// packageNameFromPath2 extracts the last segment of a Go import path.
func packageNameFromPath2(path string) string {
	segments := strings.Split(path, "/")
	return segments[len(segments)-1]
}

// ---------------------------------------------------------------------------
// Tuple type generation
// ---------------------------------------------------------------------------

func (g *Generator) emitTupleTypes() {
	for name, fieldTypes := range g.usedTuple {
		// Generate struct for each unique tuple type
		g.emitf("type %s struct {\n", name)
		g.indent++
		for i, ft := range fieldTypes {
			g.emitf("F%d %s\n", i, ft)
		}
		g.indent--
		g.emitf("}\n\n")
	}
}

// ---------------------------------------------------------------------------
// Option type generation
// ---------------------------------------------------------------------------

func (g *Generator) emitOptionTypes() {
	for goType, elemGo := range g.usedOption {
		if _, imported := g.importedOptions[goType]; imported {
			continue
		}
		if qualified, ok := g.goTypeQual[elemGo]; ok {
			elemGo = qualified
		}
		short := strings.TrimPrefix(goType, "Option")
		tagType := "OptionTag" + short
		g.emitf("// Option type: %s\n", goType)
		g.emitf("type %s byte\n\n", tagType)
		g.emitf("const (\n")
		g.indent++
		g.emitf("%sNone %s = iota\n", unexported(tagType), tagType)
		g.emitf("%sSome\n", unexported(tagType))
		g.indent--
		g.emitf(")\n\n")

		g.emitf("type %s struct {\n", goType)
		g.indent++
		g.emitf("tag  %s\n", tagType)
		g.emitf("some %s\n", elemGo)
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func New%sNone() %s {\n", goType, goType)
		g.indent++
		g.emitf("return %s{tag: %sNone}\n", goType, unexported(tagType))
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func New%sSome(v %s) %s {\n", goType, elemGo, goType)
		g.indent++
		g.emitf("return %s{tag: %sSome, some: v}\n", goType, unexported(tagType))
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func (o %s) IsSome() bool { return o.tag == %sSome }\n\n", goType, unexported(tagType))
		g.emitf("func (o %s) MustSome() %s {\n", goType, elemGo)
		g.indent++
		g.emitf("if o.tag != %sSome { panic(\"Option.Some on None\") }\n", unexported(tagType))
		g.emitf("return o.some\n")
		g.indent--
		g.emitf("}\n\n")
		g.emitf("func (o %s) SomeOr(def %s) %s {\n", goType, elemGo, elemGo)
		g.indent++
		g.emitf("if o.tag == %sSome { return o.some }\n", unexported(tagType))
		g.emitf("return def\n")
		g.indent--
		g.emitf("}\n\n")
	}
}

// ---------------------------------------------------------------------------
// Result type generation
// ---------------------------------------------------------------------------

func (g *Generator) emitResultTypes() {
	for goType, parts := range g.usedResult {
		short := strings.TrimPrefix(goType, "Result")
		tagType := "ResultTag" + short

		okType, errType := "interface{}", "interface{}"
		if len(parts) >= 2 {
			okType = parts[0]
			errType = parts[1]
		}

		g.emitf("// Result type: %s\n", goType)
		g.emitf("type %s byte\n\n", tagType)
		g.emitf("const (\n")
		g.indent++
		g.emitf("%sOk %s = iota\n", unexported(tagType), tagType)
		g.emitf("%sErr\n", unexported(tagType))
		g.indent--
		g.emitf(")\n\n")

		g.emitf("type %s struct {\n", goType)
		g.indent++
		g.emitf("tag  %s\n", tagType)
		g.emitf("ok   %s\n", okType)
		g.emitf("err  %s\n", errType)
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func New%sOk(v %s) %s {\n", goType, okType, goType)
		g.indent++
		g.emitf("return %s{tag: %sOk, ok: v}\n", goType, unexported(tagType))
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func New%sErr(e %s) %s {\n", goType, errType, goType)
		g.indent++
		g.emitf("return %s{tag: %sErr, err: e}\n", goType, unexported(tagType))
		g.indent--
		g.emitf("}\n\n")

		g.emitf("func (r %s) IsOk() bool { return r.tag == %sOk }\n\n", goType, unexported(tagType))
		g.emitf("func (r %s) MustOk() %s {\n", goType, okType)
		g.indent++
		g.emitf("if r.tag != %sOk { panic(\"Result.Ok on Err\") }\n", unexported(tagType))
		g.emitf("return r.ok\n")
		g.indent--
		g.emitf("}\n\n")
		g.emitf("func (r %s) MustErr() %s {\n", goType, errType)
		g.indent++
		g.emitf("if r.tag != %sErr { panic(\"Result.Err on Ok\") }\n", unexported(tagType))
		g.emitf("return r.err\n")
		g.indent--
		g.emitf("}\n\n")
		g.emitf("func (r %s) OkOr(def %s) %s {\n", goType, okType, okType)
		g.indent++
		g.emitf("if r.tag == %sOk { return r.ok }\n", unexported(tagType))
		g.emitf("return def\n")
		g.indent--
		g.emitf("}\n\n")
	}
}

// ---------------------------------------------------------------------------
// Record type generation
// ---------------------------------------------------------------------------

func (g *Generator) emitRecordTypes() {
	for _, td := range g.records {
		rk, ok := td.Kind.(*ast.RecordTypeKind)
		if !ok {
			continue
		}
		goName := g.goName(td.Name)
		g.emitf("type %s struct {\n", goName)
		g.indent++
		for _, f := range rk.Fields {
			g.emitf("%s %s\n", exported(f.Name), g.typeToGo(f.Type))
		}
		g.indent--
		g.emitf("}\n\n")
	}
}

// ---------------------------------------------------------------------------
// ADT type generation
// ---------------------------------------------------------------------------

func (g *Generator) emitADTTypes() {
	for _, td := range g.adts {
		if g.isZeroCostBrand(td) {
			g.emitZeroCostBrand(td)
			continue
		}
		var cases []ast.ADTCase
		switch k := td.Kind.(type) {
		case *ast.ADTTypeKind:
			cases = k.Cases
		case *ast.GADTTypeKind:
			for _, c := range k.Cases {
				cases = append(cases, ast.ADTCase{Name: c.Name, Arg: c.Arg})
			}
		case *ast.ExtensibleTypeKind:
			cases = append(cases, g.extensibleCases[td.Name]...)
		default:
			continue
		}
		goName := g.goName(td.Name)
		iface := "is" + goName

		g.emitf("type %s interface {\n", goName)
		g.indent++
		g.emitf("%s()\n", iface)
		g.indent--
		g.emitf("}\n\n")

		for _, c := range cases {
			varName := goName + exported(c.Name)
			g.emitf("type %s struct {\n", varName)
			g.indent++
			if c.Arg != nil {
				g.emitVariantFields(c.Arg)
			}
			g.indent--
			g.emitf("}\n\n")

			g.emitf("func (%s) %s() {}\n\n", varName, iface)
			g.emitConstructorFunc(goName, varName, c)
		}
	}
}

// emitZeroCostBrand lowers a single-constructor primitive brand to a Go
// defined type — no interface, no variant struct, no boxing.
// See docs/design/21-branded-ids.md.
func (g *Generator) emitZeroCostBrand(td *ast.TypeDecl) {
	goName := g.goName(td.Name)
	rep := g.zeroCostBrandRep(td)
	g.emitf("type %s %s\n\n", goName, rep)
	for _, c := range adtCasesOf(td) {
		g.emitBrandConstructorFunc(goName, rep, c)
	}
}

func (g *Generator) emitBrandConstructorFunc(goName, rep string, c ast.ADTCase) {
	g.emitf("func New%s%s(v %s) %s {\n", goName, exported(c.Name), rep, goName)
	g.indent++
	g.emitf("return %s(v)\n", goName)
	g.indent--
	g.emitf("}\n\n")
}

func (g *Generator) emitNewtypeTypes() {
	for _, td := range g.newtypes {
		nk, ok := td.Kind.(*ast.NewtypeTypeKind)
		if !ok {
			continue
		}
		goName := newtypeGoName(td.Name)
		rep := g.typeToGo(nk.Rep)
		g.emitf("type %s %s\n\n", goName, rep)
	}
}

func (g *Generator) emitVariantFields(arg ast.Type) {
	switch t := arg.(type) {
	case *ast.TRecord:
		for _, f := range t.Fields {
			g.emitf("%s %s\n", exported(f.Name), g.typeToGo(f.Type))
		}
	case *ast.TIdent:
		g.emitf("Value %s\n", g.typeToGo(t))
	case *ast.TTuple:
		for i, e := range t.Elems {
			g.emitf("F%d %s\n", i, g.typeToGo(e))
		}
	default:
		g.emitf("Value interface{}\n")
	}
}

// ---------------------------------------------------------------------------
// Opaque type generation (linear types — erased in Go)
// ---------------------------------------------------------------------------

func (g *Generator) emitOpaqueTypes() {
	for _, d := range g.opaqueTypes {
		goName := g.goName(d.Name)
		// Opaque linear types are erased — generate a placeholder type
		// alias that the Go type system will accept.
		g.emitf("type %s = interface{}\n\n", goName)
	}
}

func (g *Generator) emitConstructorFunc(adtGoName, varName string, c ast.ADTCase) {
	g.emitf("func New%s", varName)
	// Parameters
	hasParams := false
	if c.Arg != nil {
		params := g.variantFieldParams(c.Arg)
		if len(params) > 0 {
			g.buf.WriteString("(")
			hasParams = true
			for i, p := range params {
				if i > 0 {
					g.buf.WriteString(", ")
				}
				g.buf.WriteString(p.name + " " + p.goType)
			}
			g.buf.WriteString(")")
		}
	}
	if !hasParams {
		g.buf.WriteString("()")
	}
	g.buf.WriteString(" " + adtGoName + " {\n")
	g.indent++
	g.emitf("return %s{", varName)
	if c.Arg != nil {
		args := g.variantFieldArgs(c.Arg)
		for i, a := range args {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			g.buf.WriteString(a.field + ": " + a.param)
		}
	}
	g.buf.WriteString("}\n")
	g.indent--
	g.emitf("}\n\n")
}

type fieldParam struct {
	name, goType, field, param string
}

func (g *Generator) variantFieldParams(arg ast.Type) []fieldParam {
	var result []fieldParam
	switch t := arg.(type) {
	case *ast.TRecord:
		for _, f := range t.Fields {
			result = append(result, fieldParam{
				name:   unexported(f.Name),
				goType: g.typeToGo(f.Type),
				field:  exported(f.Name),
				param:  unexported(f.Name),
			})
		}
	case *ast.TIdent:
		result = append(result, fieldParam{
			name:   "v",
			goType: g.typeToGo(t),
			field:  "Value",
			param:  "v",
		})
	case *ast.TTuple:
		for i, e := range t.Elems {
			n := fmt.Sprintf("f%d", i)
			result = append(result, fieldParam{
				name:   n,
				goType: g.typeToGo(e),
				field:  fmt.Sprintf("F%d", i),
				param:  n,
			})
		}
	}
	return result
}

func (g *Generator) variantFieldArgs(arg ast.Type) []fieldParam {
	return g.variantFieldParams(arg)
}

func (g *Generator) variantFieldName(arg ast.Type, adtName, caseName string) string {
	if rt, ok := arg.(*ast.TRecord); ok {
		// For records, emitVariantFields expands fields directly into the struct.
		// If the record matches a known type, use that name.
		// Otherwise, use empty string (fields are directly accessible).
		name := g.recordNameFromType(rt)
		if name != "Record" {
			return name
		}
		// Inline record: fields are expanded directly — no wrapper field name.
		return ""
	}
	// For non-record payloads, the field is always "Value"
	return "Value"
}

// ---------------------------------------------------------------------------
// Top-level let declarations
// ---------------------------------------------------------------------------

// patternVarName returns the variable name bound by a simple pattern.
func (g *Generator) patternVarName(p ast.Pattern) string {
	if ip, ok := p.(*ast.IdentPattern); ok {
		return ip.Name
	}
	return "_"
}

func (g *Generator) emitLetDecl(d *ast.LetDecl) {
	// Active patterns get a special function name
	if d.ActivePattern {
		for _, b := range d.Bindings {
			goFuncName := "__active_" + b.Name
			g.currentFunc = b.Name
			// Emit as a regular function with the active-pattern name
			g.emitf("func %s", goFuncName)
			g.emitParams(b.Params)
			// Return type is always an option type
			retGo := "interface{}"
			if b.RetType != nil {
				retGo = g.typeToGo(b.RetType)
			}
			g.buf.WriteString(" " + retGo)
			g.buf.WriteString(" {\n")
			g.indent++
			if retGo != "struct{}" {
				g.emitReturnExpr(b.Body)
			} else {
				g.emitExpr(b.Body, true)
			}
			g.indent--
			g.emitf("}\n\n")
		}
		return
	}

	for _, b := range d.Bindings {
		g.currentFunc = b.Name
		funcName := b.Name
		g.funcPrivate[funcName] = d.Private

		// Record source mapping: we don't have the AST source location directly,
		// so we use the current Go output position and approximate Goop line from
		// the function name context.
		g.recordMapping(g.goLine, 0) // approximate Goop line = current Go line

		// Filter out empty-named params (from `()` parsing quirk) and unit
		// params (`_u: unit`) — they are not Go parameters.
		realParams := make([]ast.Param, 0, len(b.Params))
		for _, p := range b.Params {
			if p.Name != "" && !isUnitParam(p) {
				realParams = append(realParams, p)
			}
		}

		hasFunParams := len(b.Params) > 0

		// Non-private top-level lets are Go-exported (capitalized) so other
		// packages can call them via Module.name → pkg.Name.
		// Exception: Go's program entry point must remain lowercase `main`.
		emitName := b.Name
		if !d.Private {
			if b.Name == "main" {
				emitName = "main"
			} else {
				emitName = exported(b.Name)
			}
		}
		g.goopToGo[b.Name] = emitName
		funcName = emitName

		if !hasFunParams && !isCompoundExpr(b.Body) {
			// Simple value binding
			if d.Mutable {
				g.emitf("var %s = ", funcName)
			} else {
				g.emitf("var %s = ", funcName)
			}
			g.emitExpr(b.Body, false)
			g.buf.WriteString("\n\n")
		} else if !hasFunParams && g.isChanMakeExpr(b.Body) {
			// let ch = Chan.make () → ch := C0ChanMake()
			g.usedChan = true
			g.emitf("%s := C0ChanMake()\n", funcName)
		} else if !hasFunParams {
			// Compound value binding: emit as a func that returns the value
			g.emitf("func %s()", funcName)
			retType := ""
			if b.RetType != nil {
				retType = g.typeToGo(b.RetType)
			}
			if retType != "" && retType != "struct{}" {
				g.buf.WriteString(" " + retType)
			}
			g.buf.WriteString(" {\n")
			g.indent++
			if retType != "" && retType != "struct{}" {
				g.emitReturnExpr(b.Body)
			} else {
				g.emitExpr(b.Body, true)
			}
			g.indent--
			g.emitf("}\n\n")
		} else {
			// Function
			g.emitf("func %s", funcName)
			g.emitParams(realParams)
			hasReturn := b.RetType != nil && g.typeToGo(b.RetType) != "struct{}"
			retGo := ""
			if g.funcRetType[b.Name] == "interface{}" && containsCPSPerform(b.Body) {
				hasReturn = true
				retGo = "interface{}"
			} else if hasReturn {
				retGo = g.typeToGo(b.RetType)
			} else if g.varTypeMap != nil {
				if t, ok := g.varTypeMap[b.Name]; ok {
					retGo = g.funcReturnGo(t)
					if retGo != "" && retGo != "struct{}" && retGo != "interface{}" {
						hasReturn = true
					}
				}
			}

			// Check for return type refinement → use named return value
			hasPostcondition := false
			var postRT *ast.RefinementType
			if hasReturn {
				if b.RetType != nil {
					if rt, ok := b.RetType.(*ast.RefinementType); ok {
						hasPostcondition = true
						postRT = rt
						g.buf.WriteString(" (result " + g.typeToGo(b.RetType) + ")")
					} else {
						g.buf.WriteString(" " + retGo)
					}
				} else {
					g.buf.WriteString(" " + retGo)
				}
				g.funcRetType[funcName] = retGo
			}

			g.buf.WriteString(" {\n")
			g.indent++

			// Emit postcondition check via defer
			if hasPostcondition {
				g.emitPostconditionCheck(funcName, postRT)
			}

			// Record refinement-annotated parameters and emit precondition checks.
			// Proven call sites can skip checks (checked via g.provenSites lookup),
			// but the in-body check serves as a safety net for all callers.
			var refined []refinedParam
			for i, p := range realParams {
				if rt, ok := p.Type.(*ast.RefinementType); ok {
					refined = append(refined, refinedParam{index: i, name: p.Name, rt: rt})
				}
			}
			if len(refined) > 0 {
				g.refinementParams[funcName] = refined
			}

			// Emit precondition checks for exported functions or when not all call sites proven.
			emitEntryGuards := g.shouldEmitEntryGuards(funcName, d.Private)
			if emitEntryGuards {
				for _, rp := range refined {
					g.emitPreconditionCheck(funcName, rp.name, rp.rt)
				}
			}

			if hasReturn {
				g.emitReturnExpr(b.Body)
			} else {
				g.emitExpr(b.Body, true)
			}
			g.indent--
			g.emitf("}\n\n")
		}
		g.currentFunc = ""
	}
}

func isCompoundExpr(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.LitExpr, *ast.IdentExpr, *ast.ConstructorExpr,
		*ast.RecordExpr, *ast.ListExpr, *ast.TupleExpr:
		return false
	case *ast.AppExpr:
		if _, ok := e.Func.(*ast.ConstructorExpr); ok {
			return false
		}
	}
	return true
}

func newtypeGoName(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = exported(p)
		}
	}
	return strings.Join(parts, "")
}

// isChanMakeExpr checks whether an expression is a Chan.make () or OwnedChan.make () call.
func (g *Generator) isChanMakeExpr(e ast.Expr) bool {
	app, ok := e.(*ast.AppExpr)
	if !ok {
		return false
	}
	field, ok := app.Func.(*ast.FieldAccessExpr)
	if !ok {
		return false
	}
	qualified := g.fieldAccessName(field)
	return qualified == "Chan.make" || qualified == "OwnedChan.make"
}

func (g *Generator) emitImplementsDecl(d *ast.ImplementsDecl) {
	receiverType := g.goName(d.ForType)
	for _, method := range d.Methods {
		if len(method.Params) == 0 {
			continue
		}
		g.currentFunc = method.Name
		if method.RetType != nil {
			g.funcRetType[method.Name] = g.typeToGo(method.RetType)
		}
		receiver := method.Params[0]
		g.emitf("func (%s *%s) %s", receiver.Name, receiverType, method.Name)
		g.emitParams(method.Params[1:])
		if method.RetType != nil && g.typeToGo(method.RetType) != "struct{}" {
			g.buf.WriteString(" " + g.typeToGo(method.RetType))
		}
		g.buf.WriteString(" {\n")
		g.indent++
		if method.RetType != nil && g.typeToGo(method.RetType) != "struct{}" {
			g.emitReturnExpr(method.Body)
		} else {
			g.emitExpr(method.Body, true)
		}
		g.indent--
		g.emitf("}\n\n")
	}
	if iface, ok := g.goTypeQual[d.Interface]; ok {
		g.emitf("var _ %s = (*%s)(nil)\n\n", iface, receiverType)
	}
}

func (g *Generator) emitGoSliceHelpers() {
	g.emit("func any_of[T any](x T) interface{} { return x }\n\n")
	g.emit("func go_slice_len[T any](xs []T) int { return len(xs) }\n\n")
	g.emit("func go_slice_append[T any](xs []T, x T) []T { return append(xs, x) }\n\n")
	g.emit("func go_slice_get[T any](xs []T, i int) T { return xs[i] }\n\n")
	g.emit("func go_slice_of_list[T any](xs []T) []T { return xs }\n\n")
	g.emit("func list_of_go_slice[T any](xs []T) []T { return xs }\n\n")
}

func isUnitParam(p ast.Param) bool {
	if p.Type == nil {
		return false
	}
	if id, ok := p.Type.(*ast.TIdent); ok && id.Name == "unit" {
		return true
	}
	return false
}

// isUnitTyped reports whether e has type unit (including the () literal).
// Unit-returning Go methods (Lock/Unlock/Info/Reset) erase to void and must be
// emitted as statements, not `_ = expr` / `x := expr` bindings.
func (g *Generator) isUnitTyped(e ast.Expr) bool {
	if e == nil {
		return false
	}
	if lit, ok := e.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
		return true
	}
	if t := g.typeOf(e); t != nil {
		if p, ok := t.(*types.Prim); ok && p.Name == "unit" {
			return true
		}
		if g.internalTypeToGo(t) == "struct{}" {
			return true
		}
	}
	if name := g.callRootName(e); name != "" {
		if rt, ok := g.funcRetType[name]; ok && rt == "struct{}" {
			return true
		}
		if t, ok := g.externRetTypes[name]; ok && astFinalReturnIsUnit(t) {
			return true
		}
	}
	return false
}

// callRootName returns the extern/local function or method name at the root of
// an application chain (e.g. log.Info "x" → "Info", Fail s msg → "Fail").
func (g *Generator) callRootName(e ast.Expr) string {
	cur := e
	for {
		switch n := cur.(type) {
		case *ast.AppExpr:
			cur = n.Func
		case *ast.FieldAccessExpr:
			return n.Field
		case *ast.IdentExpr:
			return n.Name
		case *ast.ConstructorExpr:
			return n.Name
		default:
			return ""
		}
	}
}

func astFinalReturnIsUnit(t ast.Type) bool {
	for t != nil {
		fn, ok := t.(*ast.TFun)
		if !ok {
			break
		}
		t = fn.To
	}
	id, ok := t.(*ast.TIdent)
	return ok && id.Name == "unit"
}

func (g *Generator) emitParams(params []ast.Param) {
	g.buf.WriteString("(")
	first := true
	for i, p := range params {
		// Expand open record params into individual field params
		if p.Type != nil {
			if rt, ok := p.Type.(*ast.TRecord); ok && rt.Open {
				for _, f := range rt.Fields {
					if !first {
						g.buf.WriteString(", ")
					}
					first = false
					g.buf.WriteString(p.Name + "_" + f.Name)
					g.buf.WriteString(" ")
					g.buf.WriteString(g.typeToGo(f.Type))
				}
				continue
			}
		}
		if !first {
			g.buf.WriteString(", ")
		}
		first = false
		g.buf.WriteString(p.Name)
		g.buf.WriteString(" ")
		if p.Type != nil {
			typeName := g.typeToGo(p.Type)
			if p.Variadic && !strings.HasPrefix(typeName, "...") {
				typeName = "..." + typeName
			}
			g.buf.WriteString(typeName)
		} else if g.currentFunc != "" && g.varTypeMap != nil {
			if t, ok := g.varTypeMap[g.currentFunc]; ok {
				if pt := nthFunParam(t, i); pt != nil {
					g.buf.WriteString(g.internalTypeToGo(pt))
				} else {
					g.buf.WriteString("interface{}")
				}
			} else {
				g.buf.WriteString("interface{}")
			}
		} else {
			g.buf.WriteString("interface{}")
		}
	}
	g.buf.WriteString(")")
}

func nthFunParam(t types.Type, n int) types.Type {
	for i := 0; i <= n; i++ {
		fn, ok := t.(*types.TFun)
		if !ok {
			return nil
		}
		if i == n {
			return fn.From
		}
		t = fn.To
	}
	return nil
}

func (g *Generator) funcReturnGo(t types.Type) string {
	for {
		fn, ok := t.(*types.TFun)
		if !ok {
			return g.internalTypeToGo(t)
		}
		t = fn.To
	}
}

// ---------------------------------------------------------------------------
// Expression emission
// ---------------------------------------------------------------------------

func (g *Generator) emitExpr(e ast.Expr, isStmt bool) {
	switch e := e.(type) {
	case *ast.LitExpr:
		g.emitLit(e)
	case *ast.NullExpr:
		// Prefer typed nil when the expression type is known.
		if g.currentFunc != "" {
			if rt, ok := g.funcRetType[g.currentFunc]; ok && rt == "error" {
				g.buf.WriteString("error(nil)")
				return
			}
		}
		if g.typeMap != nil {
			if t, ok := g.typeMap[e]; ok {
				goT := g.internalTypeToGo(t)
				if goT == "error" {
					g.buf.WriteString("error(nil)")
					return
				}
				if strings.HasPrefix(goT, "*") || strings.Contains(goT, "interface") {
					g.buf.WriteString("(" + goT + ")(nil)")
					return
				}
				if goT != "" && goT != "interface{}" {
					g.buf.WriteString("(*" + goT + ")(nil)")
					return
				}
			}
		}
		g.buf.WriteString("nil")
	case *ast.PtrOfExpr:
		g.buf.WriteString("&(")
		g.emitExpr(e.Inner, false)
		g.buf.WriteString(")")
	case *ast.IsNullExpr:
		g.buf.WriteString("(")
		g.emitExpr(e.Inner, false)
		g.buf.WriteString(" == nil)")
	case *ast.SpreadExpr:
		g.emitExpr(e.Inner, false)
		g.buf.WriteString("...")
	case *ast.IdentExpr:
		g.emitIdent(e)
	case *ast.ConstructorExpr:
		g.emitConstructor(e)
	case *ast.AppExpr:
		g.emitApp(e, isStmt)
	case *ast.IfExpr:
		g.emitIf(e, isStmt)
	case *ast.MatchExpr:
		g.emitMatch(e)
	case *ast.LetInExpr:
		g.emitLetIn(e)
	case *ast.FunExpr:
		g.emitFun(e)
	case *ast.BinaryExpr:
		g.emitBinary(e)
	case *ast.PipeExpr:
		g.emitPipe(e)
	case *ast.QuestionExpr:
		g.emitQuestion(e)
	case *ast.RecordExpr:
		g.emitRecord(e)
	case *ast.RecordUpdateExpr:
		g.emitRecordUpdate(e)
	case *ast.FieldAccessExpr:
		g.emitFieldAccess(e)
	case *ast.MethodSendExpr:
		g.emitMethodSend(e)
	case *ast.TupleExpr:
		g.emitTuple(e)
	case *ast.ListExpr:
		g.emitList(e)
	case *ast.ParenExpr:
		if _, ok := e.Inner.(*ast.SpreadExpr); ok {
			g.emitExpr(e.Inner, false)
			return
		}
		g.buf.WriteString("(")
		g.emitExpr(e.Inner, false)
		g.buf.WriteString(")")
	case *ast.IndexExpr:
		g.emitIndex(e)
	case *ast.AssignExpr:
		g.emitAssign(e, isStmt)
	case *ast.ForExpr:
		g.emitFor(e, isStmt)
	case *ast.BeginExpr:
		g.emitBegin(e, isStmt)
	case *ast.WhileExpr:
		g.emitWhile(e, isStmt)
	case *ast.FunctionExpr:
		g.emitFunction(e)
	case *ast.RefExpr:
		g.emitRef(e)
	case *ast.DerefExpr:
		g.emitDeref(e)
	case *ast.TryExpr:
		g.emitTry(e, isStmt)
	case *ast.RaiseExpr:
		g.emitRaise(e)
	case *ast.AssertExpr:
		g.emitAssertExpr(e, isStmt)
	case *ast.LazyExpr:
		g.emitLazy(e)
	case *ast.PerformExpr:
		g.emitPerform(e)
	case *ast.ArrayLitExpr:
		g.emitArrayLit(e)
	case *ast.PolyvarExpr:
		g.emitPolyvar(e)
	case *ast.ObjectExpr:
		g.emitObjectExpr(e)
	case *ast.NewExpr:
		g.emitNewExpr(e)
	case *ast.LabelledArgExpr:
		g.emitLabelledArg(e)
	case *ast.LetModuleExpr:
		// Flatten: emit body only (decls already typechecked / opened)
		g.emitExpr(e.Body, isStmt)
	case *ast.LetOpenExpr:
		g.emitExpr(e.Body, isStmt)
	case *ast.LocalOpenExpr:
		g.emitExpr(e.Body, isStmt)
	case *ast.ContinueExpr:
		// continue k x  →  k(x)
		g.emitExpr(e.Cont, false)
		g.buf.WriteString("(")
		g.emitExpr(e.Arg, false)
		g.buf.WriteString(")")
	case *ast.DiscontinueExpr:
		g.buf.WriteString("(func() interface{} { panic(")
		g.emitExpr(e.Exn, false)
		g.buf.WriteString("); return nil })()")
	case *ast.ModuleAppExpr:
		g.buf.WriteString("struct{}{}")
	case *ast.PackModuleExpr:
		if isStmt {
			g.buf.WriteString("_ = struct{}{}")
		} else {
			g.buf.WriteString("struct{}{}")
		}
	case *ast.UnpackModuleExpr:
		if isStmt {
			g.buf.WriteString("_ = struct{}{}")
		} else {
			g.buf.WriteString("struct{}{}")
		}
	case *ast.IsExpr:
		g.emitIsExpr(e)
	case *ast.AsMatchExpr:
		g.emitAsMatchExpr(e)
	case *ast.GoExpr:
		g.emitGoExpr(e)
	case *ast.SelectExpr:
		g.emitSelectExpr(e)
	case *ast.UsingExpr:
		g.emitUsing(e, isStmt)
	case *ast.RegionExpr:
		g.emitRegion(e)
	default:
		// Hard error, not a silent `/* TODO */` in the generated Go:
		// an unhandled expression must fail at Goop compile time with
		// a source position, not at Go compile time without one.
		g.errorfAt(ast.ExprLoc(e), "codegen: unhandled expression type %T", e)
	}
}

func (g *Generator) emitLit(e *ast.LitExpr) {
	switch e.Kind {
	case token.INT:
		g.buf.WriteString(fmt.Sprintf("%v", e.Value))
	case token.FLOAT:
		// Use a format that Go sees as float (add .0 for whole numbers)
		s := fmt.Sprintf("%v", e.Value)
		if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
			s += ".0"
		}
		g.buf.WriteString(s)
	case token.STRING:
		g.buf.WriteString(fmt.Sprintf("%q", e.Value))
	case token.TRUE:
		g.buf.WriteString("true")
	case token.FALSE:
		g.buf.WriteString("false")
	case token.UNIT:
		g.buf.WriteString("struct{}{}")
	}
}

func (g *Generator) emitIdent(e *ast.IdentExpr) {
	if e.Name == "__goop_perform" || e.Name == "__goop_handle" {
		g.needsEffectRuntime = true
	}
	// Check for extern-qualified names
	if qualified, ok := g.externNames[e.Name]; ok {
		g.buf.WriteString(qualified)
		return
	}
	if pkg, ok := g.openExports[e.Name]; ok {
		g.buf.WriteString(pkg + "." + exported(e.Name))
		return
	}
	// Use mapped Go name for exported lets (colorize → Colorize).
	g.buf.WriteString(g.goName(e.Name))
}

func (g *Generator) emitConstructor(e *ast.ConstructorExpr) {
	// Open-module exported capitalized lets — same flatten/partial-app path
	// as local lets (imported Capitalized names are Constructor tokens).
	if pkg, ok := g.openExports[e.Name]; ok {
		if count, ok := g.funcParamCount[e.Name]; ok {
			goN := pkg + "." + exported(e.Name)
			if e.Arg == nil {
				g.buf.WriteString(goN)
				return
			}
			if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				g.buf.WriteString(goN + "()")
				return
			}
			if count > 1 {
				g.emitPartialApp(e.Name, []ast.Expr{e.Arg}, count)
				return
			}
			g.buf.WriteString(goN)
			g.buf.WriteString("(")
			g.emitExpr(e.Arg, false)
			g.buf.WriteString(")")
			return
		}
		// Non-function open export (e.g. type name used as constructor token).
		g.buf.WriteString(pkg + "." + exported(e.Name))
		if e.Arg != nil {
			if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				g.buf.WriteString("()")
				return
			}
			g.buf.WriteString("(")
			g.emitExpr(e.Arg, false)
			g.buf.WriteString(")")
		}
		return
	}
	// Check for extern-qualified names first
	if _, ok := g.externNames[e.Name]; ok {
		if tup := g.externReturnErrorTuple(e.Name); tup != nil && e.Arg != nil {
			if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				g.emitExternResultCall(&ast.IdentExpr{Name: e.Name}, nil, tup, false)
				return
			}
			g.emitExternResultCall(&ast.IdentExpr{Name: e.Name}, []ast.Expr{e.Arg}, tup, false)
			return
		}
		if tup := g.externReturnTuple(e.Name); tup != nil && e.Arg != nil {
			if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				g.emitExternTupleCall(&ast.IdentExpr{Name: e.Name}, nil, tup, false)
				return
			}
			g.emitExternTupleCall(&ast.IdentExpr{Name: e.Name}, []ast.Expr{e.Arg}, tup, false)
			return
		}
		if qualified, ok := g.externNames[e.Name]; ok {
			g.buf.WriteString(qualified)
			if e.Arg != nil {
				if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
					// OCaml `f ()` on a zero-arg Go func → f(), not f(struct{}{})
					g.buf.WriteString("()")
					return
				}
				g.buf.WriteString("(")
				g.emitExpr(e.Arg, false)
				g.buf.WriteString(")")
			}
			return
		}
	}
	// Capitalized user lets are tokenized as constructors (`Add 2`).
	if count, ok := g.funcParamCount[e.Name]; ok {
		goN := g.goName(e.Name)
		if e.Arg == nil {
			g.buf.WriteString(goN)
			return
		}
		if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			g.buf.WriteString(goN + "()")
			return
		}
		if count > 1 {
			g.emitPartialApp(e.Name, []ast.Expr{e.Arg}, count)
			return
		}
		g.buf.WriteString(goN)
		g.buf.WriteString("(")
		g.emitExpr(e.Arg, false)
		g.buf.WriteString(")")
		return
	}
	// Check user-defined ADTs first (they take priority over built-in names)
	varName := g.findVariantStruct(e.Name, e.TypePrefix)
	if varName != "" {
		g.buf.WriteString("New" + varName)
		g.buf.WriteString("(")
		if e.Arg != nil {
			if rec, ok := e.Arg.(*ast.RecordExpr); ok && g.isVariantRecordPayload(e.Name) {
				// Inline record fields — expand as individual constructor args
				for i, f := range rec.Fields {
					if i > 0 {
						g.buf.WriteString(", ")
					}
					g.emitExpr(f.Value, false)
				}
			} else {
				g.emitExpr(e.Arg, false)
			}
		}
		g.buf.WriteString(")")
		return
	}

	switch e.Name {
	case "None":
		optType := g.resolveOptionGoType(e)
		g.buf.WriteString(g.qualifyOptionCtor("New" + optType + "None()"))
		return
	case "Some":
		optType := g.resolveOptionGoType(e)
		g.buf.WriteString(g.qualifyOptionCtor("New" + optType + "Some("))
		g.emitExpr(e.Arg, false)
		g.buf.WriteString(")")
		return
	case "Ok":
		resType := g.findResultType()
		g.buf.WriteString("New" + resType + "Ok(")
		g.emitExpr(e.Arg, false)
		g.buf.WriteString(")")
		return
	case "Error":
		resType := g.findResultType()
		g.buf.WriteString("New" + resType + "Err(")
		g.emitExpr(e.Arg, false)
		g.buf.WriteString(")")
		return
	default:
		// Unknown constructor — emit as plain call
		g.buf.WriteString(e.Name)
		if e.Arg != nil {
			g.buf.WriteString("(")
			g.emitExpr(e.Arg, false)
			g.buf.WriteString(")")
		}
	}
}

func (g *Generator) isListMatch(e *ast.MatchExpr) bool {
	for _, arm := range e.Arms {
		switch arm.Pattern.(type) {
		case *ast.ListPattern, *ast.ConsPattern:
			return true
		}
	}
	return false
}

func (g *Generator) emitListMatch(e *ast.MatchExpr) {
	// Emit as if/else chain: if len == 0 { ... } else { first := xs[0]; ... }
	g.varCounter++
	tmp := fmt.Sprintf("_l%d", g.varCounter)
	g.emitf("%s := ", tmp)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	for i, arm := range e.Arms {
		prefix := "if "
		if i > 0 {
			prefix = "} else if "
		}
		switch p := arm.Pattern.(type) {
		case *ast.ListPattern:
			if len(p.Elems) == 0 {
				g.emitf("%slen(%s) == 0 {\n", prefix, tmp)
				g.indent++
				g.emitReturnExpr(arm.Body)
				g.indent--
			}
		case *ast.ConsPattern:
			g.emitf("%slen(%s) > 0 {\n", prefix, tmp)
			g.indent++
			if _, isWild := p.Head.(*ast.WildcardPattern); !isWild {
				g.emitf("%s := %s[0]\n", g.patternVarName(p.Head), tmp)
			}
			if _, isWild := p.Tail.(*ast.WildcardPattern); !isWild {
				g.emitf("%s := %s[1:]\n", g.patternVarName(p.Tail), tmp)
			}
			g.emitReturnExpr(arm.Body)
			g.indent--
		case *ast.WildcardPattern, *ast.IdentPattern:
			if prefix == "if " {
				g.emitf("{\n")
			} else {
				g.emitf("} else {\n")
			}
			g.indent++
			if ip, ok := p.(*ast.IdentPattern); ok {
				g.emitf("%s := %s\n", ip.Name, tmp)
			}
			g.emitReturnExpr(arm.Body)
			g.indent--
			g.emitf("}\n")
		}
	}
	// Close the final if/else block and add a fallback panic for Go's exhaustiveness check
	g.emitf("}\n")
	g.emitf("panic(\"unreachable: unhandled list pattern\")\n")
}

func (g *Generator) isTupleMatch(e *ast.MatchExpr) bool {
	for _, arm := range e.Arms {
		if _, ok := arm.Pattern.(*ast.TuplePattern); ok {
			return true
		}
	}
	return false
}

func (g *Generator) emitTupleMatch(e *ast.MatchExpr) {
	g.varCounter++
	tmp := fmt.Sprintf("_t%d", g.varCounter)
	g.emitf("%s := ", tmp)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	for i, arm := range e.Arms {
		tp, ok := arm.Pattern.(*ast.TuplePattern)
		if !ok {
			continue
		}
		prefix := "if "
		if i > 0 {
			prefix = "} else if "
		}
		cond := g.tuplePatternCondition(tmp, tp)
		if arm.Guard != nil {
			cond = cond + " && "
		}
		if arm.Guard != nil {
			var gbuf strings.Builder
			old := g.buf
			g.buf = gbuf
			g.emitExpr(arm.Guard, false)
			guard := g.buf.String()
			g.buf = old
			if strings.HasSuffix(cond, " && ") {
				cond += guard
			} else if cond == "true" {
				cond = guard
			} else {
				cond = cond + guard
			}
		}
		g.emitf("%s%s {\n", prefix, cond)
		g.indent++
		for j, pat := range tp.Elems {
			if _, ok := pat.(*ast.IdentPattern); ok {
				g.emitPatternBinding(pat, fmt.Sprintf("%s.F%d", tmp, j))
			}
		}
		g.emitReturnExpr(arm.Body)
		g.indent--
	}
	g.emitf("}\n")
	g.emitf("panic(\"unreachable: unhandled tuple pattern\")\n")
}

func (g *Generator) tuplePatternCondition(tmp string, p *ast.TuplePattern) string {
	var parts []string
	for j, pat := range p.Elems {
		field := fmt.Sprintf("%s.F%d", tmp, j)
		switch pat := pat.(type) {
		case *ast.LitPattern:
			var buf strings.Builder
			old := g.buf
			g.buf = buf
			g.emitLit(&ast.LitExpr{Value: pat.Value, Kind: pat.Kind})
			lit := g.buf.String()
			g.buf = old
			parts = append(parts, field+" == "+lit)
		case *ast.IdentPattern, *ast.WildcardPattern:
			continue
		default:
			continue
		}
	}
	if len(parts) == 0 {
		return "true"
	}
	return strings.Join(parts, " && ")
}

func (g *Generator) isResultMatch(e *ast.MatchExpr) bool {
	// Check if all patterns are Ok/Error constructors (result type match)
	if len(e.Arms) == 0 {
		return false
	}
	for _, arm := range e.Arms {
		cp, ok := arm.Pattern.(*ast.ConstructorPattern)
		if !ok {
			return false
		}
		if cp.Name != "Ok" && cp.Name != "Error" {
			return false
		}
	}
	return true
}

func (g *Generator) isOptionMatch(e *ast.MatchExpr) bool {
	if len(e.Arms) == 0 {
		return false
	}
	if t := g.typeOf(e.Scrutinee); t != nil {
		switch tt := t.(type) {
		case *types.TCon:
			if tt.Name == "option" {
				return true
			}
		case *types.TAdt:
			if tt.Name == "option" {
				return true
			}
		}
	}
	sawOptionCtor := false
	for _, arm := range e.Arms {
		switch p := arm.Pattern.(type) {
		case *ast.ConstructorPattern:
			if p.Name != "Some" && p.Name != "None" {
				return false
			}
			// User ADT owns Some/None → use ADT type-switch path instead.
			if g.findVariantStruct(p.Name, p.TypePrefix) != "" {
				return false
			}
			sawOptionCtor = true
		case *ast.WildcardPattern:
			continue
		default:
			return false
		}
	}
	// Wildcard-only matches (e.g. `function | _ -> …`) are not option matches.
	return sawOptionCtor
}

func (g *Generator) emitOptionMatch(e *ast.MatchExpr) {
	// Emit as: tmp := scrutinee; if tmp.IsSome() { <Some arms> } else { <None arms> }
	g.varCounter++
	tmp := fmt.Sprintf("_tmp%d", g.varCounter)
	g.emitf("%s := ", tmp)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	var someArms, noneArms []ast.MatchArm
	for _, arm := range e.Arms {
		switch p := arm.Pattern.(type) {
		case *ast.ConstructorPattern:
			switch p.Name {
			case "Some":
				someArms = append(someArms, arm)
			case "None":
				noneArms = append(noneArms, arm)
			}
		case *ast.WildcardPattern:
			noneArms = append(noneArms, arm)
		}
	}

	if len(someArms) > 0 {
		g.emitf("if %s.IsSome() {\n", tmp)
		g.indent++
		g.emitOptionArms(someArms, tmp, true)
		g.indent--
		if len(noneArms) > 0 {
			g.emitf("} else {\n")
			g.indent++
			g.emitOptionArms(noneArms, tmp, false)
			g.indent--
		}
		g.emitf("}\n")
	} else if len(noneArms) > 0 {
		g.emitf("if !%s.IsSome() {\n", tmp)
		g.indent++
		g.emitOptionArms(noneArms, tmp, false)
		g.indent--
		g.emitf("}\n")
	}
}

func (g *Generator) emitOptionArms(arms []ast.MatchArm, tmp string, isSome bool) {
	if len(arms) == 0 {
		return
	}
	if !isSome {
		// None / wildcard arms: no payload.
		if len(arms) == 1 {
			g.emitReturnExpr(arms[0].Body)
			return
		}
		for i, arm := range arms {
			if i > 0 {
				g.emitf("} else ")
			}
			if arm.Guard != nil {
				if i == 0 {
					g.emitf("if ")
				} else {
					g.buf.WriteString("if ")
				}
				g.emitExpr(arm.Guard, false)
				g.buf.WriteString(" {\n")
				g.indent++
				g.emitReturnExpr(arm.Body)
				g.indent--
			} else if i > 0 {
				g.buf.WriteString("{\n")
				g.indent++
				g.emitReturnExpr(arm.Body)
				g.indent--
			} else {
				g.emitReturnExpr(arm.Body)
			}
		}
		if len(arms) > 1 || arms[0].Guard != nil {
			g.emitf("}\n")
		}
		return
	}

	g.varCounter++
	ev := fmt.Sprintf("_e%d", g.varCounter)
	g.emitf("%s := %s.MustSome()\n", ev, tmp)
	// Bind once from the first arm so when-guards can reference the name.
	if cp, ok := arms[0].Pattern.(*ast.ConstructorPattern); ok && cp.Arg != nil {
		g.emitPatternBinding(cp.Arg, ev)
	} else {
		g.emitf("_ = %s\n", ev)
	}

	if len(arms) == 1 && arms[0].Guard == nil {
		g.emitReturnExpr(arms[0].Body)
		return
	}

	for i, arm := range arms {
		if i > 0 {
			g.emitf("} else ")
		}
		if arm.Guard != nil {
			if i == 0 {
				g.emitf("if ")
			} else {
				g.buf.WriteString("if ")
			}
			g.emitExpr(arm.Guard, false)
			g.buf.WriteString(" {\n")
			g.indent++
			g.emitReturnExpr(arm.Body)
			g.indent--
		} else if i > 0 {
			g.buf.WriteString("{\n")
			g.indent++
			g.emitReturnExpr(arm.Body)
			g.indent--
		} else {
			g.emitReturnExpr(arm.Body)
		}
	}
	if len(arms) > 1 || arms[0].Guard != nil {
		g.emitf("}\n")
	}
}

func (g *Generator) emitResultMatch(e *ast.MatchExpr) {
	// Emit as: tmp := scrutinee; if tmp.IsOk() { <Ok arms> } else { <Error arms> }
	g.varCounter++
	tmp := fmt.Sprintf("_tmp%d", g.varCounter)
	g.emitf("%s := ", tmp)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	// Group Ok and Error arms
	var okArms, errArms []ast.MatchArm
	for _, arm := range e.Arms {
		cp := arm.Pattern.(*ast.ConstructorPattern)
		switch cp.Name {
		case "Ok":
			okArms = append(okArms, arm)
		case "Error":
			errArms = append(errArms, arm)
		}
	}

	if len(okArms) > 0 {
		g.emitf("if %s.IsOk() {\n", tmp)
		g.indent++
		g.emitResultArms(okArms, tmp)
		g.indent--
		if len(errArms) > 0 {
			g.emitf("} else {\n")
			g.indent++
			g.emitResultArms(errArms, tmp)
			g.indent--
		}
		g.emitf("}\n")
	} else if len(errArms) > 0 {
		g.emitf("if !%s.IsOk() {\n", tmp)
		g.indent++
		g.emitResultArms(errArms, tmp)
		g.indent--
		g.emitf("}\n")
	}
}

func (g *Generator) emitResultArms(arms []ast.MatchArm, tmp string) {
	armKind := "Ok"
	if len(arms) > 0 {
		if cp, ok := arms[0].Pattern.(*ast.ConstructorPattern); ok {
			armKind = cp.Name
		}
	}
	extractor := tmp + ".MustOk()"
	if armKind == "Error" {
		extractor = tmp + ".MustErr()"
	}

	// If only one arm with a simple pattern (no nested constructor), emit directly
	if len(arms) == 1 {
		cp := arms[0].Pattern.(*ast.ConstructorPattern)
		if cp.Arg != nil && isSimplePattern(cp.Arg) {
			g.emitPatternBinding(cp.Arg, extractor)
		} else if cp.Arg != nil {
			// Nested constructor pattern: emit as type switch
			g.emitResultNestedArms(arms, extractor)
			return
		}
		g.emitReturnExpr(arms[0].Body)
		return
	}

	// Multiple arms
	g.emitResultNestedArms(arms, extractor)
}

func isSimplePattern(p ast.Pattern) bool {
	switch p.(type) {
	case *ast.IdentPattern, *ast.WildcardPattern:
		return true
	}
	return false
}

func (g *Generator) emitResultNestedArms(arms []ast.MatchArm, prefix string) {
	// Emit as type switch on the extracted value
	g.varCounter++
	ev := fmt.Sprintf("_e%d", g.varCounter)
	g.emitf("%s := %s\n", ev, prefix)
	g.emitf("switch %s := %s.(type) {\n", ev, ev)
	g.indent++
	for _, arm := range arms {
		cp := arm.Pattern.(*ast.ConstructorPattern)
		// The inner pattern determines the variant struct name
		innerName := ""
		if cp.Arg != nil {
			if icp, ok := cp.Arg.(*ast.ConstructorPattern); ok {
				innerName = icp.Name
			}
		}
		// A zero-cost brand is carried as its own defined type, not a
		// variant struct, so the type switch matches on the brand itself.
		structName, _ := g.zeroCostBrandCtor(innerName, "")
		if structName == "" {
			structName = g.findVariantStruct(innerName, "")
		}
		if structName == "" {
			structName = g.goName(innerName)
		}
		if structName == "" {
			structName = cp.Name
		}
		g.emitf("case %s:\n", structName)
		g.indent++
		if cp.Arg != nil {
			g.emitPatternBinding(cp.Arg, ev)
		}
		g.emitReturnExpr(arm.Body)
		g.indent--
	}
	g.emitf("default:\n")
	g.indent++
	g.emitf("panic(\"unreachable: unhandled result variant\")\n")
	g.indent--
	g.indent--
	g.emitf("}\n")
}

func (g *Generator) findOptionType() string {
	if ret, ok := g.funcRetType[g.currentFunc]; ok && strings.HasPrefix(ret, "Option") {
		return ret
	}
	return "OptionT"
}

func (g *Generator) resolveOptionGoType(e ast.Expr) string {
	if g.optionCtorHint != "" {
		return g.optionCtorHint
	}
	if g.typeMap != nil && e != nil {
		if t, ok := g.typeMap[e]; ok {
			goT := g.internalTypeToGo(t)
			if strings.HasPrefix(goT, "Option") {
				return goT
			}
		}
	}
	return g.findOptionType()
}

// qualifyOptionCtor prefixes NewOptionT{Some,None} with the owning package
// when that option type comes from an open-imported record field.
func (g *Generator) qualifyOptionCtor(ctorCall string) string {
	// ctorCall is "NewOptionFooSome(" or "NewOptionFooNone()"
	name := ctorCall
	if i := strings.IndexAny(name, "()"); i >= 0 {
		name = name[:i]
	}
	optType := strings.TrimPrefix(name, "New")
	optType = strings.TrimSuffix(optType, "Some")
	optType = strings.TrimSuffix(optType, "None")
	if pkg, ok := g.importedOptions[optType]; ok {
		return pkg + "." + ctorCall
	}
	return ctorCall
}

func (g *Generator) findResultType() string {
	if ret, ok := g.funcRetType[g.currentFunc]; ok && strings.HasPrefix(ret, "Result") {
		return ret
	}
	return "ResultT"
}

func (g *Generator) emitApp(e *ast.AppExpr, isStmt bool) {
	// CPS-lowered perform: __goop_perform(Flip ()) → __goop_perform("Flip")
	if id, ok := e.Func.(*ast.IdentExpr); ok && id.Name == "__goop_perform" {
		g.needsEffectRuntime = true
		tag := effectOpTag(e.Arg)
		g.buf.WriteString(fmt.Sprintf("__goop_perform(%q)", tag))
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}

	// Function application
	// Check if this is a method-like call on a constructor (e.g., Console.print_line)
	if send, ok := e.Func.(*ast.MethodSendExpr); ok {
		args := g.collectArgs(e)
		g.emitExpr(send.Target, false)
		g.buf.WriteString(".")
		g.buf.WriteString(exported(send.Method))
		g.buf.WriteString("(")
		for i, arg := range args[1:] {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			g.emitExpr(arg, false)
		}
		g.buf.WriteString(")")
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}
	if field, ok := e.Func.(*ast.FieldAccessExpr); ok {
		if method, ok := g.externMethods[field.Field]; ok {
			args := g.collectArgs(e)[1:]
			receiver := field.Left
			if fieldAccessMatchesType(field, method.recvGoop) && len(args) > 0 {
				receiver = args[0]
				args = args[1:]
			}
			if tup := g.externReturnErrorTuple(field.Field); tup != nil {
				g.emitExternMethodResultCall(receiver, method.methodName, args, tup, isStmt)
				return
			}
			g.emitExpr(receiver, false)
			g.buf.WriteString(".")
			g.buf.WriteString(method.methodName)
			g.buf.WriteString("(")
			first := true
			for _, arg := range args {
				if lit, ok := arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
					continue
				}
				if !first {
					g.buf.WriteString(", ")
				}
				first = false
				g.emitExpr(arg, false)
			}
			g.buf.WriteString(")")
			if isStmt {
				g.buf.WriteString("\n")
			}
			return
		}
		// Qualified free extern: time.Now (), fmt.Sprintf, etc.
		if _, isExtern := g.externNames[field.Field]; isExtern {
			args := g.collectArgs(e)[1:]
			if tup := g.externReturnErrorTuple(field.Field); tup != nil {
				g.emitExternResultCall(&ast.IdentExpr{Name: field.Field}, args, tup, isStmt)
				return
			}
			if tup := g.externReturnTuple(field.Field); tup != nil {
				g.emitExternTupleCall(&ast.IdentExpr{Name: field.Field}, args, tup, isStmt)
				return
			}
			if q, ok := g.externNames[field.Field]; ok {
				g.buf.WriteString(q)
			} else {
				g.emitFieldAccess(field)
			}
			g.buf.WriteString("(")
			first := true
			for _, arg := range args {
				if lit, ok := arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
					continue
				}
				if !first {
					g.buf.WriteString(", ")
				}
				first = false
				g.emitExpr(arg, false)
			}
			g.buf.WriteString(")")
			if isStmt {
				g.buf.WriteString("\n")
			}
			return
		}
		// Check prelude for qualified names like Console.print_line
		qualifiedName := g.fieldAccessName(field)
		if qualifiedName != "" {
			if b := g.prelude.Lookup(qualifiedName); b != nil {
				g.emitPreludeCall(b, []ast.Expr{e.Arg}, e, isStmt)
				if isStmt {
					g.buf.WriteString("\n")
				}
				return
			}
		}
		// qualified call like colors.gray () / Console.print_line
		g.emitFieldAccess(field)
		g.buf.WriteString("(")
		if lit, ok := e.Arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			// unit erased — zero-arg Go call
		} else {
			g.emitExpr(e.Arg, false)
		}
		g.buf.WriteString(")")
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}
	if cons, ok := e.Func.(*ast.ConstructorExpr); ok && cons.TypePrefix != "" {
		if method, ok := g.externMethods[cons.Name]; ok && cons.TypePrefix == method.recvGoop {
			args := g.collectArgs(e)[1:]
			if cons.Arg != nil {
				args = append([]ast.Expr{cons.Arg}, args...)
			}
			if len(args) == 0 {
				g.errorfAt(e.Loc, "codegen: missing receiver in call to Go method %s.%s", method.recvGoType, method.methodName)
				return
			}
			g.emitExpr(args[0], false)
			g.buf.WriteString(".")
			g.buf.WriteString(method.methodName)
			g.buf.WriteString("(")
			first := true
			for _, arg := range args[1:] {
				if lit, ok := arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
					continue
				}
				if !first {
					g.buf.WriteString(", ")
				}
				first = false
				g.emitExpr(arg, false)
			}
			g.buf.WriteString(")")
			if isStmt {
				g.buf.WriteString("\n")
			}
			return
		}
	}

	// Regular function application: f(x, y, ...)
	// For curried calls, collect all arguments
	args := g.collectArgs(e)

	funcExpr := args[0].(ast.Expr)
	args = args[1:]

	// Flatten ConstructorExpr with embedded Arg for user lets and externs.
	// The parser represents "Add 2 3" / "Mod a b" as
	// AppExpr(Func=ConstructorExpr(Add, Arg=2), Arg=3), so collectArgs only
	// sees [ConstructorExpr(Add, 2), 3]. Un-embed the first arg so all args
	// are in one flat list and we emit Add(2, 3) rather than Add(2)(3).
	if cons, ok := funcExpr.(*ast.ConstructorExpr); ok && cons.Arg != nil {
		if g.shouldFlattenCtorCall(cons.Name) {
			funcExpr = &ast.IdentExpr{Name: cons.Name, Loc: cons.Loc}
			args = append([]ast.Expr{cons.Arg}, args...)
		}
	}

	// Check for prelude lowering before partial application/user function
	funcName := g.getExprName(funcExpr)
	if funcName == "" {
		// Also recognize qualified names like Chan.send as prelude calls
		if field, ok := funcExpr.(*ast.FieldAccessExpr); ok {
			funcName = g.fieldAccessName(field)
		}
	}
	if b := g.prelude.Lookup(funcName); b != nil {
		g.emitPreludeCall(b, args, e, isStmt)
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}

	// Check for partial application
	if funcName == "" {
		if cons, ok := funcExpr.(*ast.ConstructorExpr); ok && cons.Arg == nil {
			funcName = cons.Name
		}
	}
	if funcName != "" {
		paramCount := g.funcParamCount[funcName]
		if paramCount == 0 {
			// Also try exported Go name key
			if goName, ok := g.goopToGo[funcName]; ok {
				paramCount = g.funcParamCount[goName]
			}
		}
		if paramCount > 0 && len(args) < paramCount {
			// Partial application: emit closure
			g.emitPartialApp(funcName, args, paramCount)
			return
		}
	}

	// Expand row-polymorphic arguments: for each arg that corresponds to
	// a row param, extract individual fields from the record literal.
	if funcName != "" && len(g.rowParams[funcName]) > 0 {
		g.emitExpr(funcExpr, false)
		g.buf.WriteString("(")
		rowFields := g.rowParams[funcName]
		first := true
		for i, arg := range args {
			if i >= len(rowFields) {
				// Extra args after row param
				if !first {
					g.buf.WriteString(", ")
				}
				first = false
				g.emitExpr(arg, false)
				continue
			}
			// Row param arg: extract fields from the record expression
			if rec, ok := arg.(*ast.RecordExpr); ok {
				for _, rf := range rowFields {
					if !first {
						g.buf.WriteString(", ")
					}
					first = false
					found := false
					for _, f := range rec.Fields {
						if f.Name == rf {
							if f.Value != nil {
								g.emitExpr(f.Value, false)
							} else {
								g.buf.WriteString(f.Name)
							}
							found = true
							break
						}
					}
					if !found {
						g.errorfAt(ast.ExprLoc(arg), "codegen: record literal is missing field %q required by row parameter of %s", rf, funcName)
					}
				}
			} else {
				// Non-record arg for row param — emit as-is
				if !first {
					g.buf.WriteString(", ")
				}
				first = false
				g.emitExpr(arg, false)
			}
		}
		g.buf.WriteString(")")
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}

	// Extern call returning (T, error) → result wrapper (H6).
	if funcName != "" {
		if tup := g.externReturnErrorTuple(funcName); tup != nil {
			g.emitExternResultCall(funcExpr, args, tup, isStmt)
			return
		}
	}

	// Extern call returning a 2+ tuple → multi-value Go assignment wrapped in struct.
	if funcName != "" {
		if tup := g.externReturnTuple(funcName); tup != nil {
			g.emitExternTupleCall(funcExpr, args, tup, isStmt)
			return
		}
	}

	if g.needsCallSiteGuards(e, funcName, args) && !isStmt {
		g.emitGuardedCallExpr(funcExpr, args, funcName, e)
		return
	}

	g.emitCallSiteRefinementGuards(e, funcName, args)

	g.emitExpr(funcExpr, false)
	g.buf.WriteString("(")
	// When the function has zero Go params (because its only param was unit `()`),
	// skip emitting arguments — don't emit `struct{}{}`.
	if funcName != "" {
		if paramCnt, ok := g.funcParamCount[funcName]; ok && paramCnt == 0 {
			goto afterArgs
		}
		// Also try exported name.
		if goName, ok := g.goopToGo[funcName]; ok {
			if paramCnt, ok := g.funcParamCount[goName]; ok && paramCnt == 0 {
				goto afterArgs
			}
		}
	}
	// Strip unit arguments everywhere — Goop erases `unit` parameters, so
	// OCaml `f ()` / `M.f ()` must not pass `struct{}{}` (including cross-
	// package Goop calls where funcParamCount is unknown).
	{
		var filtered []ast.Expr
		for _, a := range args {
			if lit, ok := a.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				continue
			}
			filtered = append(filtered, a)
		}
		args = filtered
	}
	for i, arg := range args {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(arg.(ast.Expr), false)
	}
afterArgs:
	g.buf.WriteString(")")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) getExprName(e ast.Expr) string {
	if ident, ok := e.(*ast.IdentExpr); ok {
		return ident.Name
	}
	if cons, ok := e.(*ast.ConstructorExpr); ok && cons.Arg == nil {
		return cons.Name
	}
	return ""
}

// shouldFlattenCtorCall reports whether a ConstructorExpr with an embedded
// first argument is actually a capitalized function/extern application
// (OCaml juxtaposition), not an ADT / option / result constructor.
func (g *Generator) shouldFlattenCtorCall(name string) bool {
	switch name {
	case "None", "Some", "Ok", "Error", "Err":
		return false
	}
	if g.findVariantStruct(name, "") != "" {
		return false
	}
	if _, ok := g.externNames[name]; ok {
		return true
	}
	if _, ok := g.openExports[name]; ok {
		if _, ok := g.funcParamCount[name]; ok {
			return true
		}
	}
	if _, ok := g.funcParamCount[name]; ok {
		return true
	}
	if goName, ok := g.goopToGo[name]; ok {
		if _, ok := g.funcParamCount[goName]; ok {
			return true
		}
	}
	return false
}

func (g *Generator) emitExternTupleCall(funcExpr ast.Expr, args []ast.Expr, tup *ast.TTuple, isStmt bool) {
	tupleGo, _ := g.tupleGoName(tup)
	g.buf.WriteString("func() " + tupleGo + " {\n")
	g.indent++
	g.emitf("var __t %s\n", tupleGo)
	g.buf.WriteString("__t.F0")
	for i := 1; i < len(tup.Elems); i++ {
		g.buf.WriteString(fmt.Sprintf(", __t.F%d", i))
	}
	g.buf.WriteString(" = ")
	g.emitExpr(funcExpr, false)
	g.buf.WriteString("(")
	for i, arg := range args {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(arg.(ast.Expr), false)
	}
	g.buf.WriteString(")\n")
	g.emitf("return __t\n")
	g.indent--
	g.buf.WriteString("}()")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

// emitExternResultCall wraps a Go (T, error) call as a Goop result value (H6).
func (g *Generator) emitExternResultCall(funcExpr ast.Expr, args []ast.Expr, tup *ast.TTuple, isStmt bool) {
	g.registerResultFromTupleError(tup)
	resType := g.resultGoTypeForTupleError(tup)
	okGo := g.typeToGo(tup.Elems[0])
	g.buf.WriteString("func() " + resType + " {\n")
	g.indent++
	g.emitf("var __v %s\n", okGo)
	g.emitf("var __e error\n")
	g.buf.WriteString("__v, __e = ")
	g.emitExpr(funcExpr, false)
	g.buf.WriteString("(")
	first := true
	for _, arg := range args {
		if lit, ok := arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			continue
		}
		if !first {
			g.buf.WriteString(", ")
		}
		first = false
		g.emitExpr(arg, false)
	}
	g.buf.WriteString(")\n")
	g.emitf("if __e != nil {\n")
	g.indent++
	g.emitf("return New%sErr(__e)\n", resType)
	g.indent--
	g.emitf("}\n")
	g.emitf("return New%sOk(__v)\n", resType)
	g.indent--
	g.buf.WriteString("}()")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

// emitExternMethodResultCall wraps recv.Method(args) (T, error) as a result (H6).
func (g *Generator) emitExternMethodResultCall(receiver ast.Expr, method string, args []ast.Expr, tup *ast.TTuple, isStmt bool) {
	g.registerResultFromTupleError(tup)
	resType := g.resultGoTypeForTupleError(tup)
	okGo := g.typeToGo(tup.Elems[0])
	g.buf.WriteString("func() " + resType + " {\n")
	g.indent++
	g.emitf("var __v %s\n", okGo)
	g.emitf("var __e error\n")
	g.buf.WriteString("__v, __e = ")
	g.emitExpr(receiver, false)
	g.buf.WriteString(".")
	g.buf.WriteString(method)
	g.buf.WriteString("(")
	first := true
	for _, arg := range args {
		if lit, ok := arg.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			continue
		}
		if !first {
			g.buf.WriteString(", ")
		}
		first = false
		g.emitExpr(arg, false)
	}
	g.buf.WriteString(")\n")
	g.emitf("if __e != nil {\n")
	g.indent++
	g.emitf("return New%sErr(__e)\n", resType)
	g.indent--
	g.emitf("}\n")
	g.emitf("return New%sOk(__v)\n", resType)
	g.indent--
	g.buf.WriteString("}()")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitPartialApp(funcName string, givenArgs []ast.Expr, totalParams int) {
	remaining := totalParams - len(givenArgs)
	goFuncName := g.resolveCallTarget(funcName)

	// Determine return type from function's return type (now correctly flattened)
	retType := "interface{}"
	if rt, ok := g.funcRetType[funcName]; ok {
		retType = rt
	}

	// Get the types of the remaining parameters
	paramTypes := g.funcParamTypes[funcName]
	remainingTypes := paramTypes[len(givenArgs):] // types of params not yet given

	g.buf.WriteString("func(")
	for i := 0; i < remaining; i++ {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.buf.WriteString(fmt.Sprintf("_p%d", i+1))
		g.buf.WriteString(" ")
		// Use stored param type if available, otherwise interface{}
		if i < len(remainingTypes) {
			g.buf.WriteString(remainingTypes[i])
		} else {
			g.buf.WriteString("interface{}")
		}
	}
	g.buf.WriteString(") " + retType + " {\n")
	g.indent++
	g.emitf("return %s(", goFuncName)
	for i, arg := range givenArgs {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(arg, false)
	}
	for i := 0; i < remaining; i++ {
		if len(givenArgs) > 0 || i > 0 {
			g.buf.WriteString(", ")
		}
		g.buf.WriteString(fmt.Sprintf("_p%d", i+1))
	}
	g.buf.WriteString(")\n")
	g.indent--
	g.emitf("}")
}

// emitPreludeCall emits Go code for a call to a prelude function.
func (g *Generator) emitPreludeCall(b *prelude.Binding, args []ast.Expr, callExpr ast.Expr, isStmt bool) {
	l := b.Lowering

	// Custom lowering templates
	switch l.Custom {
	case "assert":
		// assert cond → if !cond { panic("assertion failed") }
		g.buf.WriteString("if !(")
		if len(args) > 0 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(") { panic(\"assertion failed\") }")
		return
	case "assert_equal":
		// assert_equal a b → if a != b { panic("assert_equal failed") }
		if len(args) >= 2 {
			g.buf.WriteString("if ")
			g.emitExpr(args[0], false)
			g.buf.WriteString(" != ")
			g.emitExpr(args[1], false)
			g.buf.WriteString(" { panic(\"assert_equal failed\") }")
		}
		return

	case "chan_make":
		// Chan.make () → C0ChanMake()
		g.usedChan = true
		g.buf.WriteString("C0ChanMake()")
		return

	case "chan_send":
		// Chan.send ch v → C0ChanSend(ch, v)
		g.usedChan = true
		g.buf.WriteString("C0ChanSend(")
		if len(args) >= 2 {
			g.emitExpr(args[0], false)
			g.buf.WriteString(", ")
			g.emitExpr(args[1], false)
		}
		g.buf.WriteString(")")
		return

	case "chan_recv":
		// Chan.recv ch → C0ChanRecv(ch).(T)
		g.usedChan = true
		g.buf.WriteString("C0ChanRecv(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(")")
		// Add type assertion if the concrete element type is known.
		elemGo := g.resolveChanElemType(args)
		if elemGo == "" || elemGo == "interface{}" {
			// Fallback: the call expression's return type IS the element type
			if g.typeMap != nil && callExpr != nil {
				if retType, ok := g.typeMap[callExpr]; ok {
					elemGo = g.internalTypeToGo(retType)
				}
			}
		}
		if elemGo != "" && elemGo != "interface{}" {
			g.buf.WriteString(".(" + elemGo + ")")
		}
		return

	case "chan_close":
		// Chan.close ch → C0ChanClose(ch)
		g.usedChan = true
		g.buf.WriteString("C0ChanClose(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(")")
		return

	case "map_make":
		mapGo := "map[interface{}]interface{}"
		if g.typeMap != nil && callExpr != nil {
			if t, ok := g.typeMap[callExpr]; ok {
				if mg := g.internalTypeToGo(t); strings.HasPrefix(mg, "map[") {
					mapGo = mg
				}
			}
		}
		g.buf.WriteString("make(" + mapGo + ")")
		return

	case "map_get":
		valGo, optType := "interface{}", "OptionT"
		if g.typeMap != nil && callExpr != nil {
			if t, ok := g.typeMap[callExpr]; ok {
				if tc, ok := t.(*types.TCon); ok && tc.Name == "option" && len(tc.Args) > 0 {
					valGo = g.internalTypeToGo(tc.Args[0])
					optType = "Option" + optionTypeSuffix(valGo)
					g.usedOption[optType] = valGo
				} else if goT := g.internalTypeToGo(t); strings.HasPrefix(goT, "Option") {
					optType = goT
					valGo = g.usedOption[optType]
					if valGo == "" {
						valGo = "interface{}"
					}
				}
			}
		}
		g.buf.WriteString("func() " + optType + " {\n")
		g.indent++
		g.emitf("__mv, __mok := ")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString("[")
		if len(args) >= 2 {
			g.emitExpr(args[1], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString("]\n")
		g.emitf("if __mok { return %s(__mv) }\n", g.qualifyOptionCtor("New"+optType+"Some"))
		g.emitf("return %s\n", g.qualifyOptionCtor("New"+optType+"None()"))
		g.indent--
		g.buf.WriteString("}()")
		return

	case "map_add":
		if isStmt {
			if len(args) >= 3 {
				g.emitf("")
				g.emitExpr(args[0], false)
				g.buf.WriteString("[")
				g.emitExpr(args[1], false)
				g.buf.WriteString("] = ")
				g.emitExpr(args[2], false)
			}
			return
		}
		g.buf.WriteString("func() struct{} {\n")
		g.indent++
		if len(args) >= 3 {
			g.emitf("")
			g.emitExpr(args[0], false)
			g.buf.WriteString("[")
			g.emitExpr(args[1], false)
			g.buf.WriteString("] = ")
			g.emitExpr(args[2], false)
			g.buf.WriteString("\n")
		}
		g.emitf("return struct{}{}\n")
		g.indent--
		g.buf.WriteString("}()")
		return

	case "map_remove":
		if isStmt {
			g.emitf("delete(")
			if len(args) >= 1 {
				g.emitExpr(args[0], false)
			} else {
				g.buf.WriteString("nil")
			}
			g.buf.WriteString(", ")
			if len(args) >= 2 {
				g.emitExpr(args[1], false)
			} else {
				g.buf.WriteString("nil")
			}
			g.buf.WriteString(")")
			return
		}
		g.buf.WriteString("func() struct{} {\n")
		g.indent++
		g.emitf("delete(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString(", ")
		if len(args) >= 2 {
			g.emitExpr(args[1], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString(")\n")
		g.emitf("return struct{}{}\n")
		g.indent--
		g.buf.WriteString("}()")
		return

	case "map_mem":
		g.buf.WriteString("func() bool { _, __ok := ")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString("[")
		if len(args) >= 2 {
			g.emitExpr(args[1], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString("]; return __ok }()")
		return

	case "map_size":
		g.buf.WriteString("len(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString(")")
		return

	case "array_make":
		g.varCounter++
		arrName := fmt.Sprintf("_arr%d", g.varCounter)
		elemGo := "interface{}"
		if len(args) >= 2 && g.typeMap != nil {
			if t, ok := g.typeMap[args[1]]; ok {
				elemGo = g.internalTypeToGo(t)
			}
		}
		if elemGo == "interface{}" && g.typeMap != nil && callExpr != nil {
			if retType, ok := g.typeMap[callExpr]; ok {
				if tc, ok := retType.(*types.TCon); ok && tc.Name == "array" && len(tc.Args) > 0 {
					elemGo = g.internalTypeToGo(tc.Args[0])
				}
			}
		}
		g.buf.WriteString("func() []" + elemGo + " {\n")
		g.indent++
		g.emitf("%s := make([]%s, ", arrName, elemGo)
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("0")
		}
		g.buf.WriteString(")\n")
		g.emitf("for _i := 0; _i < len(%s); _i++ {\n", arrName)
		g.indent++
		g.emitf("%s[_i] = ", arrName)
		if len(args) >= 2 {
			g.emitExpr(args[1], false)
		} else {
			g.buf.WriteString("var zero " + elemGo)
		}
		g.buf.WriteString("\n")
		g.indent--
		g.emitf("}\n")
		g.emitf("return %s\n", arrName)
		g.indent--
		g.buf.WriteString("}()")
		return

	case "string_sub":
		// String.sub s start len → s[start : start+len]
		if len(args) >= 3 {
			g.emitExpr(args[0], false)
			g.buf.WriteString("[")
			g.emitExpr(args[1], false)
			g.buf.WriteString(":")
			g.emitExpr(args[1], false)
			g.buf.WriteString("+")
			g.emitExpr(args[2], false)
			g.buf.WriteString("]")
		}
		return

	case "ref_make":
		elemGo := "interface{}"
		if len(args) >= 1 && g.typeMap != nil {
			if t, ok := g.typeMap[args[0]]; ok {
				elemGo = g.internalTypeToGo(t)
			}
		}
		if elemGo == "interface{}" && g.typeMap != nil && callExpr != nil {
			if retType, ok := g.typeMap[callExpr]; ok {
				if tc, ok := retType.(*types.TCon); ok && tc.Name == "ref" && len(tc.Args) > 0 {
					elemGo = g.internalTypeToGo(tc.Args[0])
				}
			}
		}
		g.buf.WriteString("func() *" + elemGo + " { v := ")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("*(new(" + elemGo + "))")
		}
		g.buf.WriteString("; return &v }()")
		return

	case "lazy_force":
		g.needsLazyRuntime = true
		g.buf.WriteString("__goop_lazy_force(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString(")")
		return

	case "lazy_from_val":
		g.needsLazyRuntime = true
		g.buf.WriteString("__goop_lazy_from_val(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		} else {
			g.buf.WriteString("nil")
		}
		g.buf.WriteString(")")
		return

	case "owned_chan_make":
		// OwnedChan.make () → C0ChanMake()
		g.usedChan = true
		g.buf.WriteString("C0ChanMake()")
		return

	case "owned_chan_send":
		// OwnedChan.send ch v → C0ChanSend(ch, v)
		g.usedChan = true
		g.buf.WriteString("C0ChanSend(")
		if len(args) >= 2 {
			g.emitExpr(args[0], false)
			g.buf.WriteString(", ")
			g.emitExpr(args[1], false)
		}
		g.buf.WriteString(")")
		return

	case "owned_chan_recv":
		// OwnedChan.recv ch → C0ChanRecv(ch).(T)
		g.usedChan = true
		g.buf.WriteString("C0ChanRecv(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(")")
		// Add type assertion if the concrete element type is known.
		elemGo := g.resolveChanElemType(args)
		if elemGo == "" || elemGo == "interface{}" {
			if g.typeMap != nil && callExpr != nil {
				if retType, ok := g.typeMap[callExpr]; ok {
					elemGo = g.internalTypeToGo(retType)
				}
			}
		}
		if elemGo != "" && elemGo != "interface{}" {
			g.buf.WriteString(".(" + elemGo + ")")
		}
		return

	case "owned_chan_close":
		// OwnedChan.close ch → C0ChanClose(ch)
		g.usedChan = true
		g.buf.WriteString("C0ChanClose(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(")")
		return

	case "http_get_string":
		g.usedHTTP = true
		g.buf.WriteString("httpGetString(")
		if len(args) >= 1 {
			g.emitExpr(args[0], false)
		}
		g.buf.WriteString(")")
		return

	case "json_extract_floats":
		g.usedHTTP = true
		g.buf.WriteString("jsonExtractFloats(")
		if len(args) >= 2 {
			g.emitExpr(args[0], false)
			g.buf.WriteString(", ")
			g.emitExpr(args[1], false)
		}
		g.buf.WriteString(")")
		return

	case "json_extract_strings":
		g.usedHTTP = true
		g.buf.WriteString("jsonExtractStrings(")
		if len(args) >= 2 {
			g.emitExpr(args[0], false)
			g.buf.WriteString(", ")
			g.emitExpr(args[1], false)
		}
		g.buf.WriteString(")")
		return
	}

	// Operator lowering: e.g., string_concat → a + b
	if l.Operator != "" {
		if len(args) >= 2 {
			// Emit as binary operator between first two arguments
			g.emitExpr(args[0], false)
			g.buf.WriteString(" " + l.Operator + " ")
			g.emitExpr(args[1], false)
		} else {
			g.emitExpr(args[0], false)
		}
		return
	}

	// Standard function call lowering
	if l.Func != "" {
		// Add import if needed
		if l.Pkg != "" {
			g.needFmt = (l.Pkg == "fmt") || g.needFmt
			g.importPkgs[l.Pkg] = l.Pkg
		}

		// Handle wrapped calls (e.g., strconv.FormatFloat with Sprintf)
		if l.Wrap == "fmt.Sprintf" {
			g.buf.WriteString("fmt.Sprintf(\"%v\"")
			g.needFmt = true
			if len(args) > 0 {
				g.buf.WriteString(", ")
				g.emitExpr(args[0], false)
			}
			g.buf.WriteString(")")
			return
		}

		g.buf.WriteString(l.Func)
		g.buf.WriteString("(")
		for i, arg := range args {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			g.emitExpr(arg, false)
		}
		g.buf.WriteString(")")
	}
}

func (g *Generator) collectArgs(e ast.Expr) []ast.Expr {
	var result []ast.Expr
	current := e
	for {
		if app, ok := current.(*ast.AppExpr); ok {
			result = append([]ast.Expr{app.Arg}, result...)
			current = app.Func
		} else {
			result = append([]ast.Expr{current}, result...)
			break
		}
	}
	return result
}

// isNotPattern checks if an IfExpr matches the desugaring of `not expr`:
//
//	if condition then false else true
func isNotPattern(e *ast.IfExpr) bool {
	falseLit, thenOk := e.ThenBranch.(*ast.LitExpr)
	trueLit, elseOk := e.ElseBranch.(*ast.LitExpr)
	return thenOk && elseOk &&
		falseLit.Kind == token.FALSE && falseLit.Value == false &&
		trueLit.Kind == token.TRUE && trueLit.Value == true
}

func (g *Generator) emitIf(e *ast.IfExpr, isStmt bool) {
	// Detect the `not` desugar pattern: if condition then false else true → !condition
	if isNotPattern(e) {
		g.buf.WriteString("!(")
		g.emitExpr(e.Cond, false)
		g.buf.WriteString(")")
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}

	// Expression form (e.g. `let x = if c then a else b`): wrap in an IIFE so
	// the result can appear on the RHS of `:=` / in call args. OCaml if-as-
	// expression; Go needs a block returning a value.
	if !isStmt {
		retGo := "interface{}"
		if g.typeMap != nil {
			if t, ok := g.typeMap[e]; ok {
				retGo = g.internalTypeToGo(t)
			}
		}
		g.varCounter++
		tmpFn := fmt.Sprintf("__ife%d", g.varCounter)
		oldFunc := g.currentFunc
		g.currentFunc = tmpFn
		g.funcRetType[tmpFn] = retGo
		g.buf.WriteString("func() " + retGo + " {\n")
		g.indent++
		g.emitf("if ")
		g.emitExpr(e.Cond, false)
		g.buf.WriteString(" {\n")
		g.indent++
		g.emitReturnExpr(e.ThenBranch)
		g.indent--
		g.emitf("}\n")
		g.emitReturnExpr(e.ElseBranch)
		g.indent--
		g.buf.WriteString("}()")
		delete(g.funcRetType, tmpFn)
		g.currentFunc = oldFunc
		return
	}

	// Statement / return-position form: branches emit their own returns.
	g.emitf("if ")
	g.emitExpr(e.Cond, false)
	g.buf.WriteString(" {\n")
	g.indent++
	g.emitReturnExpr(e.ThenBranch)
	g.indent--
	g.emitf("} else {\n")
	g.indent++
	g.emitReturnExpr(e.ElseBranch)
	g.indent--
	g.emitf("}\n")
}

func (g *Generator) emitMatch(e *ast.MatchExpr) {
	// Record mapping for the match expression
	g.recordMapping(g.goLine, 0)

	if g.hasEffectHandlerArm(e) {
		g.emitEffectHandlerMatch(e)
		return
	}

	// Check for active patterns
	if g.hasActivePattern(e) {
		g.emitActiveMatch(e)
		return
	}

	// Check for list patterns ([] and ::)
	if g.isListMatch(e) {
		g.emitListMatch(e)
		return
	}

	// Tuple scrutinee: match (a, b) with | (x, y) -> ...
	if g.isTupleMatch(e) {
		g.emitTupleMatch(e)
		return
	}

	// Check if this is a result type match (concrete struct, not interface)
	if g.isResultMatch(e) {
		g.emitResultMatch(e)
		return
	}
	// Builtin option match (concrete Option* struct, not ADT interface)
	if g.isOptionMatch(e) {
		g.emitOptionMatch(e)
		return
	}
	for _, arm := range e.Arms {
		if _, ok := arm.Pattern.(*ast.PolyvarPattern); ok {
			g.emitPolyvarMatch(e)
			return
		}
	}

	// Zero-cost brands are Go defined types, and whole-value patterns have
	// nothing to dispatch on: both lower without a type switch.
	if rep, ok := g.directMatchRep(e); ok {
		g.emitDirectMatch(e, rep)
		return
	}

	// Store scrutinee in a temp for use in pattern bindings
	g.varCounter++
	scrutVar := fmt.Sprintf("_s%d", g.varCounter)
	g.emitf("%s := ", scrutVar)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	g.emitf("switch v := %s.(type) {\n", scrutVar)
	g.indent++

	// Group consecutive arms by struct name to avoid duplicate case labels
	groups := groupMatchArms(e.Arms, g)
	hasDefault := false

	for _, grp := range groups {
		if grp.structName == "" || grp.structName == "default" {
			hasDefault = true
			g.emitf("default:\n")
			g.indent++
			g.emitf("_ = v\n")
			for _, arm := range grp.arms {
				if ip, ok := arm.Pattern.(*ast.IdentPattern); ok {
					g.emitf("%s := %s\n", ip.Name, scrutVar)
				}
				if len(grp.arms) == 1 {
					g.emitReturnExpr(arm.Body)
				}
			}
			g.indent--
			continue
		}

		g.emitf("case %s:\n", grp.structName)
		g.indent++

		// Emit pattern bindings from the first arm (or _ = v to suppress unused warning)
		if cp, ok := grp.arms[0].Pattern.(*ast.ConstructorPattern); ok && cp.Arg != nil {
			// Use the variant's struct field name to access the payload (if not expanded inline)
			fieldName := g.adtVariantField(cp.Name)
			prefix := "v"
			if fieldName != "" {
				prefix = "v." + fieldName
			}
			g.emitPatternBinding(cp.Arg, prefix)
		} else {
			g.emitf("_ = v\n")
		}

		g.emitArmBodies(grp.arms)
		g.indent--
	}

	if !hasDefault {
		g.emitf("default:\n")
		g.indent++
		g.emitf("panic(\"unreachable: unhandled variant\")\n")
		g.indent--
	}

	g.indent--
	g.emitf("}\n")
}

// directMatchRep reports whether a match can skip the interface type switch.
// It can when every constructor pattern belongs to one zero-cost brand — rep
// is then the brand's representation type, used for the unwrap conversion —
// or when no arm inspects a constructor at all, in which case rep is empty.
func (g *Generator) directMatchRep(e *ast.MatchExpr) (string, bool) {
	if len(e.Arms) == 0 {
		return "", false
	}
	rep := ""
	for _, arm := range e.Arms {
		switch p := arm.Pattern.(type) {
		case *ast.ConstructorPattern:
			_, r := g.zeroCostBrandCtor(p.Name, p.TypePrefix)
			if r == "" {
				return "", false
			}
			rep = r
		case *ast.WildcardPattern, *ast.IdentPattern:
			continue
		default:
			return "", false
		}
	}
	return rep, true
}

// emitDirectMatch lowers a match that needs no interface dispatch: a
// zero-cost brand unwraps by conversion (docs/design/21-branded-ids.md) and
// whole-value patterns bind the scrutinee as-is.
func (g *Generator) emitDirectMatch(e *ast.MatchExpr, rep string) {
	groups := groupMatchArms(e.Arms, g)
	// Every remaining pattern is irrefutable, so the first unguarded arm
	// always matches and nothing after it is reachable.
	for i, grp := range groups {
		if grp.arms[len(grp.arms)-1].Guard == nil {
			groups = groups[:i+1]
			break
		}
	}

	g.varCounter++
	scrutVar := fmt.Sprintf("_s%d", g.varCounter)
	g.emitf("%s := ", scrutVar)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")
	if !directMatchBinds(groups) {
		g.emitf("_ = %s\n", scrutVar)
	}

	for _, grp := range groups {
		if cp, ok := grp.arms[0].Pattern.(*ast.ConstructorPattern); ok {
			if cp.Arg != nil {
				g.emitPatternBinding(cp.Arg, rep+"("+scrutVar+")")
			}
		} else {
			for _, arm := range grp.arms {
				if ip, ok := arm.Pattern.(*ast.IdentPattern); ok {
					g.emitf("%s := %s\n", ip.Name, scrutVar)
				}
			}
		}
		g.emitArmBodies(grp.arms)
	}
}

// directMatchBinds mirrors the bindings emitDirectMatch emits, so the
// scrutinee temp is never left unused.
func directMatchBinds(groups []armGroup) bool {
	for _, grp := range groups {
		if cp, ok := grp.arms[0].Pattern.(*ast.ConstructorPattern); ok {
			if patternBindsName(cp.Arg) {
				return true
			}
			continue
		}
		for _, arm := range grp.arms {
			if _, ok := arm.Pattern.(*ast.IdentPattern); ok {
				return true
			}
		}
	}
	return false
}

func patternBindsName(p ast.Pattern) bool {
	switch p := p.(type) {
	case *ast.IdentPattern:
		return true
	case *ast.RecordPattern:
		return len(p.Fields) > 0
	case *ast.ConstructorPattern:
		return patternBindsName(p.Arg)
	}
	return false
}

// emitArmBodies emits arms that share one pattern binding as an if/else
// chain over their guards.
func (g *Generator) emitArmBodies(arms []ast.MatchArm) {
	for i, arm := range arms {
		if i > 0 {
			g.emitf("} else ")
		}
		if arm.Guard != nil {
			if i > 0 {
				g.buf.WriteString("if ")
			} else {
				g.emitf("if ")
			}
			g.emitExpr(arm.Guard, false)
			g.buf.WriteString(" {\n")
			g.indent++
			g.emitReturnExpr(arm.Body)
			g.indent--
		} else {
			if i > 0 {
				g.buf.WriteString("{\n")
				g.indent++
				g.emitReturnExpr(arm.Body)
				g.indent--
			} else {
				g.emitReturnExpr(arm.Body)
			}
		}
	}
	// Close the if/else chain block
	// Single unguarded arm: no block to close (just return)
	// Single guarded arm: close `if guard {`
	// Multiple arms: close the last `} else {` block
	if len(arms) > 1 || (len(arms) == 1 && arms[0].Guard != nil) {
		g.emitf("}\n")
	}
}

type armGroup struct {
	structName string
	arms       []ast.MatchArm
}

func groupMatchArms(arms []ast.MatchArm, g *Generator) []armGroup {
	var groups []armGroup
	for _, arm := range arms {
		structName := ""
		if cp, ok := arm.Pattern.(*ast.ConstructorPattern); ok {
			structName = g.findVariantStruct(cp.Name, cp.TypePrefix)
		}
		if _, ok := arm.Pattern.(*ast.WildcardPattern); ok {
			structName = "default"
		}
		if _, ok := arm.Pattern.(*ast.IdentPattern); ok {
			structName = "default"
		}
		// Merge with last group if same struct name
		if len(groups) > 0 && groups[len(groups)-1].structName == structName {
			groups[len(groups)-1].arms = append(groups[len(groups)-1].arms, arm)
		} else {
			groups = append(groups, armGroup{structName: structName, arms: []ast.MatchArm{arm}})
		}
	}
	return groups
}

func (g *Generator) emitReturnExpr(e ast.Expr) {
	// Check if the enclosing function has a non-unit return type
	hasRet := false
	if g.currentFunc != "" {
		if rt, ok := g.funcRetType[g.currentFunc]; ok && rt != "struct{}" {
			hasRet = true
		}
	}
	if !hasRet {
		// No return type: emit as a statement.
		// Skip unit literals — they become `struct{}{}` which Go forbids
		// as a bare expression statement.
		if lit, ok := e.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			return
		}
		g.emitExpr(e, true)
		return
	}

	// Emit as a return statement if it's not already a statement
	switch e := e.(type) {
	case *ast.IfExpr:
		// `if c then false else true` lowers to `!(c)` as an expression.
		// In return position that still needs an explicit `return`.
		if isNotPattern(e) {
			g.emitf("return ")
			g.emitIf(e, false)
			g.buf.WriteString("\n")
		} else {
			g.emitIf(e, true)
		}
	case *ast.MatchExpr, *ast.LetInExpr, *ast.AsMatchExpr, *ast.GuardExpr:
		// These emit their own stmt structure
		g.emitExpr(e, true)
	case *ast.TryExpr:
		g.emitf("return ")
		g.emitTry(e, false)
		g.buf.WriteString("\n")
	case *ast.AssignExpr, *ast.ForExpr, *ast.WhileExpr, *ast.AssertExpr:
		g.emitExpr(e, true)
	case *ast.BeginExpr:
		// In return position, emit a statement block whose last stmt
		// returns — do not wrap in an IIFE. Wrapping statement-shaped
		// tails (match/if) as `return <stmts>` is invalid Go.
		g.emitBeginReturnBlock(e)
	default:
		g.emitf("return ")
		g.emitExpr(e, false)
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitPatternBinding(p ast.Pattern, prefix string) {
	switch p := p.(type) {
	case *ast.IdentPattern:
		g.emitf("%s := %s\n", p.Name, prefix)
	case *ast.WildcardPattern:
		// nothing
	case *ast.RecordPattern:
		for _, f := range p.Fields {
			fieldName := prefix + "." + exported(f.Name)
			if f.Pattern != nil {
				g.emitPatternBinding(f.Pattern, fieldName)
			} else {
				g.emitf("%s := %s\n", f.Name, fieldName)
			}
		}
	case *ast.ConstructorPattern:
		// Zero-cost brands unwrap by conversion, not by struct field access.
		if _, rep := g.zeroCostBrandCtor(p.Name, p.TypePrefix); rep != "" {
			if p.Arg != nil {
				g.emitPatternBinding(p.Arg, rep+"("+prefix+")")
			}
			return
		}
		// For nested constructor patterns in result arms, bind via the struct field
		structName := g.findVariantStruct(p.Name, p.TypePrefix)
		if structName != "" {
			// This is an ADT variant — bind using the struct field name
			fieldName := g.adtVariantField(p.Name)
			if p.Arg != nil {
				g.emitPatternBinding(p.Arg, prefix+"."+fieldName)
			}
		} else {
			g.emitPatternBinding(p.Arg, prefix)
		}
	}
}

func adtCasesOf(td *ast.TypeDecl) []ast.ADTCase {
	switch k := td.Kind.(type) {
	case *ast.ADTTypeKind:
		return k.Cases
	case *ast.GADTTypeKind:
		out := make([]ast.ADTCase, len(k.Cases))
		for i, c := range k.Cases {
			out[i] = ast.ADTCase{Name: c.Name, Arg: c.Arg}
		}
		return out
	default:
		return nil
	}
}

// isZeroCostBrand reports whether an ADT declaration qualifies for the
// transparent Go defined-type lowering (docs/design/21-branded-ids.md):
// exactly one constructor carrying a primitive / string-like payload.
func (g *Generator) isZeroCostBrand(td *ast.TypeDecl) bool {
	return g.zeroCostBrandRep(td) != ""
}

// zeroCostBrandRep returns the Go representation type of a zero-cost brand,
// or "" when the ADT must keep the interface + variant struct lowering.
func (g *Generator) zeroCostBrandRep(td *ast.TypeDecl) string {
	k, ok := td.Kind.(*ast.ADTTypeKind)
	if !ok || len(k.Cases) != 1 {
		return ""
	}
	if len(g.extensibleCases[td.Name]) > 0 {
		return ""
	}
	arg := k.Cases[0].Arg
	switch typeIdentName(arg) {
	case "int", "int32", "int64", "float", "float64", "bool", "string", "rune", "byte":
		return g.typeToGo(arg)
	}
	return ""
}

// zeroCostBrandCtor returns the Go defined type and its representation type
// for a constructor owned by a zero-cost brand; both are "" otherwise.
func (g *Generator) zeroCostBrandCtor(ctorName, typePrefix string) (string, string) {
	for _, td := range g.adts {
		if typePrefix != "" && td.Name != typePrefix {
			continue
		}
		for _, c := range adtCasesOf(td) {
			if c.Name != ctorName {
				continue
			}
			if rep := g.zeroCostBrandRep(td); rep != "" {
				return g.goName(td.Name), rep
			}
			return "", ""
		}
	}
	return "", ""
}

func (g *Generator) findVariantStruct(ctorName, typePrefix string) string {
	// Search through ADTs to find which variant struct contains this constructor
	for _, td := range g.adts {
		if typePrefix != "" && td.Name != typePrefix {
			continue
		}
		cases := append(adtCasesOf(td), g.extensibleCases[td.Name]...)
		for _, c := range cases {
			if c.Name == ctorName {
				return g.goName(td.Name) + exported(c.Name)
			}
		}
	}
	return ""
}

// isVariantRecordPayload checks if the named ADT variant constructor
// has an inline record payload (e.g., "Circle of { radius: float }").
// Used to determine whether constructor args should emit record fields
// as individual arguments or pass the record expression as-is.
func (g *Generator) isVariantRecordPayload(ctorName string) bool {
	for _, td := range g.adts {
		cases := append(adtCasesOf(td), g.extensibleCases[td.Name]...)
		for _, c := range cases {
			if c.Name == ctorName && c.Arg != nil {
				_, ok := c.Arg.(*ast.TRecord)
				return ok
			}
		}
	}
	return false
}

// adtVariantField returns the struct field name for a constructor's payload.
func (g *Generator) adtVariantField(ctorName string) string {
	for _, td := range g.adts {
		cases := append(adtCasesOf(td), g.extensibleCases[td.Name]...)
		for _, c := range cases {
			if c.Name == ctorName && c.Arg != nil {
				return g.variantFieldName(c.Arg, td.Name, c.Name)
			}
		}
	}
	return "Value"
}

// isCustomPreludeStmt checks if an expression is a call to a prelude function
// that uses a Custom lowering which emits a statement rather than a value
// expression (e.g., assert, Chan.send, Chan.close).
// Custom lowerings that return values (e.g., Chan.make, Chan.recv) are NOT
// considered statements by this function.
func isCustomPreludeStmt(e ast.Expr) bool {
	app, ok := e.(*ast.AppExpr)
	if !ok {
		return false
	}
	// Collect the full function chain
	funcExpr := app.Func
	for {
		if inner, ok := funcExpr.(*ast.AppExpr); ok {
			funcExpr = inner.Func
		} else {
			break
		}
	}
	// Check both simple names (e.g., assert) and qualified names (e.g., Chan.send)
	pre := prelude.Default()
	var name string
	if ident, ok := funcExpr.(*ast.IdentExpr); ok {
		name = ident.Name
	}
	if field, ok := funcExpr.(*ast.FieldAccessExpr); ok {
		// Reconstruct the qualified name (e.g., "Chan.send")
		if ctor, ok := field.Left.(*ast.ConstructorExpr); ok && ctor.Arg == nil {
			name = ctor.Name + "." + field.Field
		} else if ident, ok := field.Left.(*ast.IdentExpr); ok {
			name = ident.Name + "." + field.Field
		}
	}
	if name == "" {
		return false
	}
	if b := pre.Lookup(name); b != nil && b.Lowering.Custom != "" {
		// Only match statement-emitting custom lowerings (not value-returning ones)
		switch b.Lowering.Custom {
		case "assert", "assert_equal", "chan_send", "chan_close", "owned_chan_send", "owned_chan_close", "map_add", "map_remove":
			return true
		}
	}
	return false
}

func (g *Generator) emitIndex(e *ast.IndexExpr) {
	g.emitExpr(e.Target, false)
	g.buf.WriteString("[")
	g.emitExpr(e.Index, false)
	g.buf.WriteString("]")
}

func (g *Generator) emitAssign(e *ast.AssignExpr, isStmt bool) {
	if e.Coloneq {
		switch target := e.Target.(type) {
		case *ast.DerefExpr:
			g.buf.WriteString("*")
			g.emitExpr(target.Target, false)
		default:
			g.buf.WriteString("*")
			g.emitExpr(e.Target, false)
		}
		g.buf.WriteString(" = ")
		g.emitExpr(e.Value, false)
		if isStmt {
			g.buf.WriteString("\n")
		}
		return
	}
	g.emitExpr(e.Target, false)
	g.buf.WriteString(" = ")
	g.emitExpr(e.Value, false)
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitWhile(e *ast.WhileExpr, isStmt bool) {
	g.buf.WriteString("for ")
	g.emitExpr(e.Cond, false)
	g.buf.WriteString(" {\n")
	g.indent++
	g.emitExpr(e.Body, true)
	g.indent--
	g.emitf("}")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitFunction(e *ast.FunctionExpr) {
	// Emit as an immediately-usable func that pattern-matches its argument.
	retGo := "interface{}"
	argGo := "interface{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			if fn, ok := t.(*types.TFun); ok {
				argGo = g.internalTypeToGo(fn.From)
				retGo = g.internalTypeToGo(fn.To)
			}
		}
	}
	g.buf.WriteString("func(__fn_arg " + argGo + ") " + retGo + " {\n")
	g.indent++
	saved := g.currentFunc
	g.currentFunc = "__fn_local"
	g.funcRetType[g.currentFunc] = retGo
	g.emitMatch(&ast.MatchExpr{
		Scrutinee: &ast.IdentExpr{Name: "__fn_arg", Loc: e.Loc},
		Arms:      e.Arms,
		Loc:       e.Loc,
	})
	g.currentFunc = saved
	g.indent--
	g.emitf("}")
}

func (g *Generator) emitRef(e *ast.RefExpr) {
	elemGo := "interface{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			if tc, ok := t.(*types.TCon); ok && tc.Name == "ref" && len(tc.Args) > 0 {
				elemGo = g.internalTypeToGo(tc.Args[0])
			}
		}
		if elemGo == "interface{}" {
			if t, ok := g.typeMap[e.Value]; ok {
				elemGo = g.internalTypeToGo(t)
			}
		}
	}
	g.buf.WriteString("func() *" + elemGo + " { v := ")
	g.emitExpr(e.Value, false)
	g.buf.WriteString("; return &v }()")
}

func (g *Generator) emitDeref(e *ast.DerefExpr) {
	g.buf.WriteString("*")
	g.emitExpr(e.Target, false)
}

func (g *Generator) emitTry(e *ast.TryExpr, isStmt bool) {
	retGo := "interface{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			retGo = g.internalTypeToGo(t)
		}
	}
	g.buf.WriteString("func() (ret " + retGo + ") {\n")
	g.indent++
	g.emitf("defer func() {\n")
	g.indent++
	g.emitf("if r := recover(); r != nil {\n")
	g.indent++
	if len(e.Arms) > 0 {
		for i, arm := range e.Arms {
			pat := arm.Pattern
			if ep, ok := pat.(*ast.ExceptionPattern); ok {
				pat = ep.Pattern
			}
			cond := "true"
			switch p := pat.(type) {
			case *ast.WildcardPattern, *ast.IdentPattern:
				cond = "true"
			case *ast.ConstructorPattern:
				cond = fmt.Sprintf("r == %s", g.goName(p.Name))
			}
			if i == 0 {
				g.emitf("if %s {\n", cond)
			} else {
				g.emitf("} else if %s {\n", cond)
			}
			g.indent++
			if ip, ok := pat.(*ast.IdentPattern); ok {
				g.emitf("%s := r\n", g.goName(ip.Name))
				_ = ip
			}
			g.emitf("ret = ")
			g.emitExpr(arm.Body, false)
			g.buf.WriteString("\n")
			g.indent--
		}
		g.emitf("}\n")
	}
	g.indent--
	g.emitf("}\n")
	g.indent--
	g.emitf("}()\n")
	if e.Finally != nil {
		g.emitf("defer func() {\n")
		g.indent++
		g.emitExpr(e.Finally, true)
		g.indent--
		g.emitf("}()\n")
	}
	if _, isRaise := e.Body.(*ast.RaiseExpr); isRaise {
		g.emitExpr(e.Body, true)
		g.buf.WriteString("\n")
	} else {
		g.emitf("ret = ")
		g.emitExpr(e.Body, false)
		g.buf.WriteString("\n")
	}
	g.emitf("return\n")
	g.indent--
	g.buf.WriteString("}()")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitLazy(e *ast.LazyExpr) {
	g.needsLazyRuntime = true
	g.buf.WriteString("__goop_lazy_make(func() interface{} { return ")
	g.emitExpr(e.Value, false)
	g.buf.WriteString(" })")
}

func (g *Generator) emitRaise(e *ast.RaiseExpr) {
	g.buf.WriteString("(func() interface{} { panic(")
	g.emitExpr(e.Exn, false)
	g.buf.WriteString("); return nil })()")
}

func (g *Generator) emitAssertExpr(e *ast.AssertExpr, isStmt bool) {
	g.buf.WriteString("if !(")
	g.emitExpr(e.Cond, false)
	g.buf.WriteString(") { panic(\"assert\") }")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

func (g *Generator) emitPerform(e *ast.PerformExpr) {
	g.needsEffectRuntime = true
	g.buf.WriteString(fmt.Sprintf("__goop_perform(%q)", effectOpTag(e.Op)))
}

func effectOpTag(op ast.Expr) string {
	for {
		switch e := op.(type) {
		case *ast.ParenExpr:
			op = e.Inner
			continue
		case *ast.AppExpr:
			op = e.Func
			continue
		case *ast.ConstructorExpr:
			return e.Name
		case *ast.IdentExpr:
			return e.Name
		default:
			return "effect"
		}
	}
}

func (g *Generator) hasEffectHandlerArm(e *ast.MatchExpr) bool {
	for _, a := range e.Arms {
		if a.EffectHandler {
			return true
		}
	}
	return false
}

// emitEffectHandlerMatch lowers effect-handler matches to a minimal runnable form:
// evaluate scrutinee, bind continuation as identity, execute handler body.
func (g *Generator) emitEffectHandlerMatch(e *ast.MatchExpr) {
	g.needsEffectRuntime = true
	g.varCounter++
	scrutVar := fmt.Sprintf("_s%d", g.varCounter)
	g.emitf("%s := ", scrutVar)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	for _, arm := range e.Arms {
		if !arm.EffectHandler {
			continue
		}
		tag := effectPatternTag(arm.Pattern)
		g.emitf("if _eff, _handled := %s.(__goop_eff); _handled && _eff.Tag == %q {\n", scrutVar, tag)
		g.indent++
		cont := arm.ContName
		if cont == "" {
			cont = "_k"
		}
		retGo := g.effectMatchGoType(e)
		argGo := g.effectContinuationArgGoType(arm, retGo)
		if retGo == "interface{}" {
			g.emitf("%s := func(x %s) interface{} { return _eff.Cont(x) }\n", cont, argGo)
		} else {
			g.emitf("%s := func(x %s) %s { return _eff.Cont(x).(%s) }\n", cont, argGo, retGo, retGo)
		}
		g.emitf("_ = %s\n", cont)
		if d, ok := arm.Body.(*ast.DiscontinueExpr); ok {
			g.emitf("panic(")
			g.emitExpr(d.Exn, false)
			g.buf.WriteString(")\n")
		} else {
			g.emitReturnExpr(arm.Body)
		}
		g.indent--
		g.emitf("}\n")
	}
	for _, arm := range e.Arms {
		if arm.EffectHandler {
			continue
		}
		if ip, ok := arm.Pattern.(*ast.IdentPattern); ok {
			g.emitf("%s := %s\n", ip.Name, scrutVar)
			if retGo := g.effectMatchGoType(e); retGo != "interface{}" {
				g.emitf("return %s.(%s)\n", ip.Name, retGo)
				return
			}
		}
		g.emitReturnExpr(arm.Body)
		return
	}
	g.emitf("panic(\"unreachable: effect match\")\n")
}

func (g *Generator) effectMatchGoType(e *ast.MatchExpr) string {
	if g.typeMap != nil {
		if typ, ok := g.typeMap[e]; ok {
			return g.internalTypeToGo(typ)
		}
	}
	return "interface{}"
}

func (g *Generator) effectContinuationArgGoType(arm ast.MatchArm, fallback string) string {
	if c, ok := arm.Body.(*ast.ContinueExpr); ok && g.typeMap != nil {
		if typ, ok := g.typeMap[c.Arg]; ok {
			return g.internalTypeToGo(typ)
		}
	}
	return fallback
}

func effectPatternTag(p ast.Pattern) string {
	if p, ok := p.(*ast.ConstructorPattern); ok {
		return p.Name
	}
	return ""
}

func containsCPSPerform(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.AppExpr:
		if id, ok := e.Func.(*ast.IdentExpr); ok && id.Name == "__goop_perform" {
			return true
		}
		return containsCPSPerform(e.Func) || containsCPSPerform(e.Arg)
	case *ast.BeginExpr:
		for _, stmt := range e.Stmts {
			if containsCPSPerform(stmt) {
				return true
			}
		}
	case *ast.MatchExpr:
		if containsCPSPerform(e.Scrutinee) {
			return true
		}
		for _, arm := range e.Arms {
			if containsCPSPerform(arm.Body) || containsCPSPerform(arm.Guard) {
				return true
			}
		}
	case *ast.LetInExpr:
		for _, binding := range e.Bindings {
			if containsCPSPerform(binding.Body) {
				return true
			}
		}
		return containsCPSPerform(e.Body)
	case *ast.IfExpr:
		return containsCPSPerform(e.Cond) || containsCPSPerform(e.ThenBranch) || containsCPSPerform(e.ElseBranch)
	}
	return false
}

func (g *Generator) emitArrayLit(e *ast.ArrayLitExpr) {
	elemGo := "interface{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			if tc, ok := t.(*types.TCon); ok && tc.Name == "array" && len(tc.Args) > 0 {
				elemGo = g.internalTypeToGo(tc.Args[0])
			}
		}
	}
	g.buf.WriteString("[]" + elemGo + "{")
	for i, el := range e.Elems {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(el, false)
	}
	g.buf.WriteString("}")
}

func (g *Generator) emitPolyvar(e *ast.PolyvarExpr) {
	g.needsPolyvarRuntime = true
	if e.Arg != nil {
		g.buf.WriteString("__goop_polyvar{Tag: ")
		g.buf.WriteString(fmt.Sprintf("%q", e.Tag))
		g.buf.WriteString(", Arg: ")
		g.emitExpr(e.Arg, false)
		g.buf.WriteString("}")
		return
	}
	g.buf.WriteString("__goop_polyvar{Tag: ")
	g.buf.WriteString(fmt.Sprintf("%q", e.Tag))
	g.buf.WriteString("}")
}

func (g *Generator) emitPolyvarMatch(e *ast.MatchExpr) {
	g.needsPolyvarRuntime = true
	g.varCounter++
	tmp := fmt.Sprintf("_pv%d", g.varCounter)
	g.emitf("%s := ", tmp)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString(".(__goop_polyvar)\n")
	for i, arm := range e.Arms {
		prefix := "if "
		if i > 0 {
			prefix = "} else if "
		}
		cond := "true"
		if p, ok := arm.Pattern.(*ast.PolyvarPattern); ok {
			cond = tmp + ".Tag == " + fmt.Sprintf("%q", p.Tag)
		}
		if cond == "true" && i > 0 {
			g.emitf("} else {\n")
		} else {
			g.emitf("%s%s {\n", prefix, cond)
		}
		g.indent++
		if p, ok := arm.Pattern.(*ast.PolyvarPattern); ok && p.Arg != nil {
			g.emitPatternBinding(p.Arg, tmp+".Arg")
		} else if ip, ok := arm.Pattern.(*ast.IdentPattern); ok {
			g.emitf("%s := %s\n", ip.Name, tmp)
		}
		g.emitReturnExpr(arm.Body)
		g.indent--
	}
	g.emitf("}\n")
	g.emitf("panic(\"unreachable: unhandled polymorphic variant\")\n")
}

func (g *Generator) emitFor(e *ast.ForExpr, isStmt bool) {
	g.emitf("for %s := ", e.Var)
	g.emitExpr(e.From, false)
	g.buf.WriteString("; ")
	if e.Down {
		g.emitf("%s >= ", e.Var)
		g.emitExpr(e.To, false)
		g.buf.WriteString("; ")
		g.emitf("%s-- {\n", e.Var)
	} else {
		g.emitf("%s <= ", e.Var)
		g.emitExpr(e.To, false)
		g.buf.WriteString("; ")
		g.emitf("%s++ {\n", e.Var)
	}
	g.indent++
	g.emitExpr(e.Body, true)
	g.indent--
	g.emitf("}")
	if isStmt {
		g.buf.WriteString("\n")
	}
}

// emitBeginReturnBlock emits begin/end as a Go block in return position.
// The last statement uses emitReturnExpr so match/if tails stay valid.
func (g *Generator) emitBeginReturnBlock(e *ast.BeginExpr) {
	g.buf.WriteString("{\n")
	g.indent++
	for i, s := range e.Stmts {
		if i < len(e.Stmts)-1 {
			if lit, ok := s.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				continue
			}
			if containsCPSPerform(s) {
				// Suspend at the surrounding function boundary so an effect
				// handler can invoke the request's continuation.
				g.emitf("return ")
				g.emitExpr(s, false)
				g.buf.WriteString("\n")
				break
			}
			g.emitExpr(s, true)
			g.buf.WriteString("\n")
			continue
		}
		g.emitReturnExpr(s)
	}
	g.indent--
	g.emitf("}\n")
}

func (g *Generator) emitBegin(e *ast.BeginExpr, isStmt bool) {
	if isStmt {
		g.buf.WriteString("{\n")
		g.indent++
		for _, s := range e.Stmts {
			if lit, ok := s.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
				continue
			}
			g.emitExpr(s, true)
			g.buf.WriteString("\n")
		}
		g.indent--
		g.emitf("}\n")
		return
	}
	retGo := "struct{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			retGo = g.internalTypeToGo(t)
		}
	}
	if containsCPSPerform(e) {
		retGo = "interface{}"
	}
	g.buf.WriteString("func() " + retGo + " {\n")
	g.indent++
	for i, s := range e.Stmts {
		if i < len(e.Stmts)-1 {
			if containsCPSPerform(s) {
				// A shallow handler receives the suspended request at the
				// nearest function boundary. Its continuation is carried in
				// __goop_eff and is invoked by `continue`.
				g.emitf("return ")
				g.emitExpr(s, false)
				g.buf.WriteString("\n")
				break
			}
			switch s.(type) {
			case *ast.ForExpr, *ast.AssignExpr:
				g.emitExpr(s, true)
				g.buf.WriteString("\n")
			case *ast.BeginExpr:
				g.emitBegin(s.(*ast.BeginExpr), false)
				g.buf.WriteString("\n")
			default:
				if g.isUnitTyped(s) {
					g.emitExpr(s, true)
					g.buf.WriteString("\n")
				} else {
					g.emitf("_ = ")
					g.emitExpr(s, false)
					g.buf.WriteString("\n")
				}
			}
		} else if retGo != "struct{}" {
			// Statement-shaped tails must emit their own returns; prefixing
			// `return` onto match/if produces illegal Go (`return _s := ...`).
			switch s.(type) {
			case *ast.IfExpr, *ast.MatchExpr, *ast.LetInExpr, *ast.AsMatchExpr, *ast.GuardExpr, *ast.BeginExpr:
				g.emitReturnExpr(s)
			default:
				g.emitf("return ")
				g.emitExpr(s, false)
				g.buf.WriteString("\n")
			}
		} else {
			g.emitExpr(s, true)
			g.buf.WriteString("\n")
			g.emitf("return struct{}{}\n")
		}
	}
	g.indent--
	g.buf.WriteString("}()")
}

func (g *Generator) emitLetIn(e *ast.LetInExpr) {
	for _, b := range e.Bindings {
		// Check if the body contains a ? operator
		if qe, ok := b.Body.(*ast.QuestionExpr); ok {
			// Generate: tmp := expr; if !tmp.IsOk() { return tmp }; x := tmp.MustOk()
			g.varCounter++
			tmp := fmt.Sprintf("_tmp%d", g.varCounter)
			g.emitf("%s := ", tmp)
			g.emitExpr(qe.Left, false)
			g.buf.WriteString("\n")
			retType, hasResult := g.funcRetType[g.currentFunc]
			if hasResult && strings.HasPrefix(retType, "Result") {
				g.emitf("if !%s.IsOk() {\n", tmp)
				g.indent++
				g.emitf("return %s\n", tmp)
				g.indent--
				g.emitf("}\n")
			}
			g.emitf("%s := %s.MustOk()\n", b.Name, tmp)
		} else {
			// Check if the body is a prelude call with a custom lowering
			// that emits a statement (e.g., assert, assert_equal), or an
			// AssertExpr (parsed keyword form of assert).
			// In that case, emit the statement first, then assign struct{}{}.
			if _, isAssert := b.Body.(*ast.AssertExpr); isAssert || isCustomPreludeStmt(b.Body) {
				g.emitExpr(b.Body, true)
				if b.Name != "_" {
					g.emitf("%s := struct{}{}\n", b.Name)
					g.emitf("_ = %s\n", b.Name)
				}
			} else if b.Name == "_" {
				// `let _ = e` must never emit `_ := e` (illegal in Go).
				if g.isUnitTyped(b.Body) {
					g.emitExpr(b.Body, true)
				} else {
					g.emitf("_ = ")
					g.emitExpr(b.Body, false)
				}
				g.buf.WriteString("\n")
			} else if g.isUnitTyped(b.Body) {
				// Unit side-effects (e.g. mu.Lock ()): emit as statement.
				// Do not bind void Go methods for blank / `_…` names.
				// Named bindings (legacy `let dummy = go …`) keep a unit value.
				g.emitExpr(b.Body, true)
				g.buf.WriteString("\n")
				if b.Name != "_" && !strings.HasPrefix(b.Name, "_") {
					g.emitf("%s := struct{}{}\n", b.Name)
					g.emitf("_ = %s\n", b.Name)
				}
			} else if g.isChanMakeExpr(b.Body) {
				// let ch = Chan.make () → ch := C0ChanMake()
				g.usedChan = true
				g.emitf("%s := C0ChanMake()\n", b.Name)
			} else if _, ok := b.Body.(*ast.GoExpr); ok {
				// go is a statement in Go, not an expression — emit without binding.
				g.emitExpr(b.Body, true)
				g.buf.WriteString("\n")
			} else {
				g.emitf("%s := ", b.Name)
				g.emitExpr(b.Body, false)
				g.buf.WriteString("\n")
			}
		}
	}
	// Emit the body. Use return only if the enclosing function has a non-unit return type.
	hasRet := false
	if g.currentFunc != "" {
		if rt, ok := g.funcRetType[g.currentFunc]; ok && rt != "struct{}" {
			hasRet = true
		}
	}
	if hasRet {
		g.emitReturnExpr(e.Body)
	} else {
		// No return type: emit as a statement.
		// Skip unit literals — they become `struct{}{}` which Go forbids
		// as a bare expression statement.
		if lit, ok := e.Body.(*ast.LitExpr); ok && lit.Kind == token.UNIT {
			// nothing — void return
		} else {
			g.emitExpr(e.Body, true)
			g.buf.WriteString("\n")
		}
	}
}

func (g *Generator) emitFun(e *ast.FunExpr) {
	g.buf.WriteString("func(")
	first := true
	for i, p := range e.Params {
		if p.Name == "" {
			continue // unit parameter from fun () ->
		}
		if !first {
			g.buf.WriteString(", ")
		}
		first = false
		g.buf.WriteString(p.Name)
		g.buf.WriteString(" ")
		if p.Type != nil {
			g.buf.WriteString(g.typeToGo(p.Type))
		} else if g.typeMap != nil {
			if t, ok := g.typeMap[e]; ok {
				g.buf.WriteString(g.internalTypeToGo(nthFunParam(t, i)))
			} else {
				g.buf.WriteString("interface{}")
			}
		} else {
			g.buf.WriteString("interface{}")
		}
	}
	retGo := "interface{}"
	if g.typeMap != nil {
		if t, ok := g.typeMap[e]; ok {
			retGo = g.funcReturnGo(t)
		}
	}
	g.buf.WriteString(") " + retGo + " {\n")
	g.indent++
	saved := g.currentFunc
	g.currentFunc = "__fn_local"
	g.funcRetType[g.currentFunc] = retGo
	switch e.Body.(type) {
	case *ast.MatchExpr, *ast.IfExpr, *ast.LetInExpr, *ast.TryExpr:
		g.emitExpr(e.Body, true)
	default:
		g.emitf("return ")
		g.emitExpr(e.Body, false)
		g.buf.WriteString("\n")
	}
	g.currentFunc = saved
	g.indent--
	g.emitf("}")
}

func (g *Generator) emitBinary(e *ast.BinaryExpr) {
	// Handle special operators
	switch e.Op {
	case token.CONS:
		// x :: xs → append([]T{x}, xs...)
		// Determine the element type from the right side's context.
		elemType := g.currentListElemType()
		g.buf.WriteString("append([]" + elemType + "{")
		g.emitExpr(e.Left, false)
		g.buf.WriteString("}, ")
		g.emitExpr(e.Right, false)
		g.buf.WriteString("...)")
		return
	case token.STARDOT:
		// *. → *
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" * ")
		g.emitExpr(e.Right, false)
		return
	case token.PLUSDOT:
		// +. → +
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" + ")
		g.emitExpr(e.Right, false)
		return
	case token.MINUSDOT:
		// -. → -
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" - ")
		g.emitExpr(e.Right, false)
		return
	case token.SLASHDOT:
		// /. → /
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" / ")
		g.emitExpr(e.Right, false)
		return
	case token.CARET:
		// ^ (string concat) → +
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" + ")
		g.emitExpr(e.Right, false)
		return
	case token.EQUALS, token.EQEQ:
		// = / == (equality) → ==
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" == ")
		g.emitExpr(e.Right, false)
		return
	case token.DIAMOND, token.NEQ:
		// <> / != → !=
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" != ")
		g.emitExpr(e.Right, false)
		return
	case token.PIPEOP:
		// x |> f → f(x)  (parser emits BinaryExpr for |>)
		g.emitExpr(e.Right, false)
		g.buf.WriteString("(")
		g.emitExpr(e.Left, false)
		g.buf.WriteString(")")
		return
	case token.MOD, token.PERCENT:
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" % ")
		g.emitExpr(e.Right, false)
		return
	case token.LAND:
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" & ")
		g.emitExpr(e.Right, false)
		return
	case token.LOR:
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" | ")
		g.emitExpr(e.Right, false)
		return
	case token.LXOR:
		g.emitExpr(e.Left, false)
		g.buf.WriteString(" ^ ")
		g.emitExpr(e.Right, false)
		return
	}

	g.emitExpr(e.Left, false)
	g.buf.WriteString(" " + e.Op.String() + " ")
	g.emitExpr(e.Right, false)
}

func (g *Generator) emitPipe(e *ast.PipeExpr) {
	// x |> f → f(x)
	g.emitExpr(e.Right, false)
	g.buf.WriteString("(")
	g.emitExpr(e.Left, false)
	g.buf.WriteString(")")
}

func (g *Generator) emitQuestion(e *ast.QuestionExpr) {
	// e ? → result propagation pattern
	g.varCounter++
	tmp := fmt.Sprintf("_tmp%d", g.varCounter)
	g.emitExpr(e.Left, false)
	g.buf.WriteString("\n")
	// The assignment to tmp is done INSIDE the let-in emitter,
	// so we just generate the value expression here.
	// Actually, the QuestionExpr IS the right-hand side of a let binding.
	// We generate: tmp := expr; if !tmp.IsOk() { return tmp }; x := tmp.MustOk()
	// But the let-binding already generates `x := ...`, so we need to
	// generate the full expanded form here and not also emit `x :=` outside.
	_ = tmp
}

// emitIsExpr generates Go code for `expr is pattern` (match macro).
// Uses an immediately-invoked function expression for the type switch.
func (g *Generator) emitIsExpr(e *ast.IsExpr) {
	fakeMatch := &ast.MatchExpr{
		Scrutinee: e.Left,
		Arms: []ast.MatchArm{
			{Pattern: e.Pattern, Body: &ast.LitExpr{Value: true, Kind: token.TRUE}},
			{Pattern: &ast.WildcardPattern{}, Body: &ast.LitExpr{Value: false, Kind: token.FALSE}},
		},
	}
	g.buf.WriteString("func() bool {\n")
	g.indent++
	g.emitMatch(fakeMatch)
	g.indent--
	g.emitf("}()")
}

// emitAsMatchExpr generates Go for `expr as pattern -> then else elseExpr`.
func (g *Generator) emitAsMatchExpr(e *ast.AsMatchExpr) {
	fakeMatch := &ast.MatchExpr{
		Scrutinee: e.Left,
		Arms: []ast.MatchArm{
			{Pattern: e.Pattern, Body: e.Body},
			{Pattern: &ast.WildcardPattern{}, Body: e.ElseBody},
		},
	}
	g.emitMatch(fakeMatch)
}

func (g *Generator) emitGoExpr(e *ast.GoExpr) {
	g.buf.WriteString("go ")
	// Unwrap ParenExpr to check for FunExpr inside: `go (fun () -> ...)`
	inner := e.Expr
	if pe, ok := inner.(*ast.ParenExpr); ok {
		inner = pe.Inner
	}
	if fe, ok := inner.(*ast.FunExpr); ok {
		g.buf.WriteString("func() {\n")
		g.indent++
		g.emitExpr(fe.Body, true)
		g.indent--
		g.buf.WriteString("\n}()")
		return
	}
	g.buf.WriteString("func() { ")
	g.emitExpr(inner, false)
	g.buf.WriteString(" }()")
}

func (g *Generator) emitSelectExpr(e *ast.SelectExpr) {
	g.usedChan = true
	g.emitf("select {\n")
	g.indent++
	for _, c := range e.Cases {
		// C0Chan wraps the real Go channel in .ch; values are interface{}.
		tmp := fmt.Sprintf("_sel%d", g.varCounter)
		g.varCounter++
		g.emitf("case %s := <-", tmp)
		g.emitExpr(c.Recv, false)
		g.buf.WriteString(".ch:\n")
		g.indent++
		if c.Bind != "" {
			elemGo := "interface{}"
			if g.typeMap != nil {
				if t, ok := g.typeMap[c.Recv]; ok {
					if ch, ok := t.(*types.TChan); ok {
						elemGo = g.internalTypeToGo(ch.Elem)
					}
				}
			}
			if elemGo != "" && elemGo != "interface{}" {
				g.emitf("%s := %s.(%s)\n", c.Bind, tmp, elemGo)
			} else {
				g.emitf("%s := %s\n", c.Bind, tmp)
			}
		}
		g.emitReturnExpr(c.Body)
		g.indent--
	}
	if e.Default != nil {
		g.emitf("default:\n")
		g.indent++
		g.emitReturnExpr(e.Default)
		g.indent--
	}
	g.indent--
	g.emitf("}\n")
}

func (g *Generator) emitUsing(e *ast.UsingExpr, isStmt bool) {
	// using x = e in body → x := e; defer Close(x); body
	// The cleanup function Close must be in scope (v1 convention).
	varName := "v"
	if ip, ok := e.Pattern.(*ast.IdentPattern); ok {
		varName = ip.Name
	}

	if !isStmt {
		g.buf.WriteString("func() interface{} {\n")
		g.indent++
	}

	g.emitf("%s := ", varName)
	g.emitExpr(e.Expr, false)
	g.buf.WriteString("\n")
	g.emitf("defer Close(%s)\n", varName)
	g.emitReturnExpr(e.Body)

	if !isStmt {
		g.indent--
		g.emitf("}()\n")
	}
}

// emitRegion emits a `region { ops }` block inline (no closure wrapper).
// Each let! binding becomes: varName := expr; defer Close(varName)
// Regular let bindings become: varName := expr (no defer).
// The result is the value of the final return/return!/body expression.
func (g *Generator) emitRegion(e *ast.RegionExpr) {
	for _, op := range e.Ops {
		switch o := op.(type) {
		case *ast.LetBangOp:
			varName := "v"
			if ip, ok := o.Pattern.(*ast.IdentPattern); ok {
				varName = ip.Name
			}
			g.emitf("%s := ", varName)
			g.emitExpr(o.Expr, false)
			g.buf.WriteString("\n")
			g.emitf("defer Close(%s)\n", varName)
		case *ast.LetOp:
			varName := "v"
			if ip, ok := o.Pattern.(*ast.IdentPattern); ok {
				varName = ip.Name
			}
			g.emitf("%s := ", varName)
			g.emitExpr(o.Expr, false)
			g.buf.WriteString("\n")
		case *ast.DoBangOp:
			g.emitExpr(o.Expr, true)
			g.buf.WriteString("\n")
		case *ast.ReturnOp:
			g.emitReturnExpr(o.Expr)
		case *ast.ReturnBangOp:
			g.emitReturnExpr(o.Expr)
		case *ast.BodyOp:
			g.emitReturnExpr(o.Expr)
		}
	}
}

func (g *Generator) emitRecord(e *ast.RecordExpr) {
	// Go struct literal when typechecker assigned an imported Go named struct.
	if t := g.typeOf(e); t != nil {
		if named, ok := types.Apply(types.EmptySubst(), t).(*types.TGoNamed); ok && !named.Interface {
			g.emitGoStructLiteral(e, named)
			return
		}
	}
	// Determine the record type from fields
	recName := g.inferRecordName(e)
	g.buf.WriteString(recName + "{")
	fieldTypes := g.recordFieldASTTypes(recName)
	for i, f := range e.Fields {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.buf.WriteString(exported(f.Name) + ": ")
		if f.Value != nil {
			prevHint := g.optionCtorHint
			if ft, ok := fieldTypes[f.Name]; ok {
				g.optionCtorHint = g.optionGoTypeFromAST(ft)
			}
			g.emitExpr(f.Value, false)
			g.optionCtorHint = prevHint
		} else {
			// Field punning
			g.buf.WriteString(f.Name)
		}
	}
	g.buf.WriteString("}")
}

func (g *Generator) emitGoStructLiteral(e *ast.RecordExpr, named *types.TGoNamed) {
	pkg := named.Pkg
	if pkg == "" {
		pkg = g.pkgAliasForGoName(named.Name)
	}
	if pkg != "" {
		g.buf.WriteString(pkg + ".")
	}
	g.buf.WriteString(named.Name + "{")
	for i, f := range e.Fields {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.buf.WriteString(exported(f.Name) + ": ")
		if f.Value != nil {
			g.emitExpr(f.Value, false)
		} else {
			g.buf.WriteString(f.Name)
		}
	}
	g.buf.WriteString("}")
}

// pkgAliasForGoName finds the Go import package name for an opaque type.
func (g *Generator) pkgAliasForGoName(typeName string) string {
	for path, pkg := range g.externImports {
		switch {
		case path == "log/slog" && (typeName == "HandlerOptions" || typeName == "Level" || typeName == "Leveler" || typeName == "Handler"):
			return pkg
		case path == "sync" && typeName == "Mutex":
			return pkg
		case path == "bytes" && typeName == "Buffer":
			return pkg
		default:
			_ = path
		}
	}
	if named := typeName; named != "" {
		for path, pkg := range g.externImports {
			if pkg == "slog" || pkg == "sync" || pkg == "bytes" {
				return pkg
			}
			_ = path
		}
	}
	return ""
}

// recordFieldASTTypes returns field name → AST type for a local or imported record.
func (g *Generator) recordFieldASTTypes(recName string) map[string]ast.Type {
	out := make(map[string]ast.Type)
	base := recName
	if i := strings.LastIndex(recName, "."); i >= 0 {
		base = recName[i+1:]
	}
	for name, td := range g.records {
		if g.goName(name) == base || exported(name) == base {
			if rk, ok := td.Kind.(*ast.RecordTypeKind); ok {
				for _, f := range rk.Fields {
					out[f.Name] = f.Type
				}
			}
			return out
		}
	}
	for name, td := range g.importedRecords {
		if exported(name) == base {
			if rk, ok := td.Kind.(*ast.RecordTypeKind); ok {
				for _, f := range rk.Fields {
					out[f.Name] = f.Type
				}
			}
			return out
		}
	}
	return out
}

func (g *Generator) optionGoTypeFromAST(t ast.Type) string {
	if t == nil {
		return ""
	}
	if app, ok := t.(*ast.TApp); ok {
		if ident, ok := app.Func.(*ast.TIdent); ok && ident.Name == "option" {
			return "Option" + optionTypeSuffix(g.typeToGo(app.Arg))
		}
	}
	return ""
}

func recordFieldNamesMatch(rk *ast.RecordTypeKind, fields []ast.RecordField) bool {
	if len(rk.Fields) != len(fields) {
		return false
	}
	lit := make(map[string]bool, len(fields))
	for _, f := range fields {
		lit[f.Name] = true
	}
	for _, f := range rk.Fields {
		if !lit[f.Name] {
			return false
		}
	}
	return true
}

func (g *Generator) inferRecordName(e *ast.RecordExpr) string {
	// Local records first (same-package wins on name collision).
	for name, td := range g.records {
		rk := td.Kind.(*ast.RecordTypeKind)
		if recordFieldNamesMatch(rk, e.Fields) {
			return g.goName(name)
		}
	}
	// Open-imported records → pkg.ExportedName{...}
	for name, td := range g.importedRecords {
		rk := td.Kind.(*ast.RecordTypeKind)
		if recordFieldNamesMatch(rk, e.Fields) {
			if pkg, ok := g.openExports[name]; ok {
				return pkg + "." + exported(name)
			}
			return exported(name)
		}
	}
	return "struct{...}"
}

func (g *Generator) emitRecordUpdate(e *ast.RecordUpdateExpr) {
	// { r with x = e } → reconstruct struct
	// Determine the record type from the base expression
	recName := g.inferRecordUpdateName(e)
	g.buf.WriteString(recName + "{\n")
	g.indent++
	// Copy all fields from base: for each known field, emit Field: base.Field
	// Then override with specified fields
	// Simplified: emit all known fields with base values, then overrides
	td := g.findRecordType(recName)
	if td != nil {
		rk := td.Kind.(*ast.RecordTypeKind)
		for _, f := range rk.Fields {
			// Check if this field is being overridden
			overridden := false
			for _, of := range e.Fields {
				if of.Name == f.Name {
					overridden = true
					break
				}
			}
			if overridden {
				continue
			}
			g.emitf("%s: ", exported(f.Name))
			g.emitExpr(e.Base, false)
			g.buf.WriteString("." + exported(f.Name) + ",\n")
		}
	}
	// Emit overridden fields
	for _, f := range e.Fields {
		g.emitf("%s: ", exported(f.Name))
		prevHint := g.optionCtorHint
		if td != nil {
			rk := td.Kind.(*ast.RecordTypeKind)
			for _, rf := range rk.Fields {
				if rf.Name == f.Name {
					g.optionCtorHint = g.optionGoTypeFromAST(rf.Type)
					break
				}
			}
		}
		g.emitExpr(f.Value, false)
		g.optionCtorHint = prevHint
		g.buf.WriteString(",\n")
	}
	// Remove trailing comma by seeking back
	g.indent--
	g.emitf("}")
}

func (g *Generator) inferRecordUpdateName(e *ast.RecordUpdateExpr) string {
	// Prefer inferred type of the base expression.
	if t := g.typeOf(e.Base); t != nil {
		if tr, ok := t.(*types.TRecord); ok {
			if name := g.internalRecordToGo(tr); name != "interface{}" {
				return name
			}
		}
	}
	if ident, ok := e.Base.(*ast.IdentExpr); ok {
		if g.varTypeMap != nil {
			if t, ok := g.varTypeMap[ident.Name]; ok {
				if tr, ok := t.(*types.TRecord); ok {
					if name := g.internalRecordToGo(tr); name != "interface{}" {
						return name
					}
				}
			}
		}
		for name := range g.records {
			if g.goName(name) == g.goName(ident.Name) || name == ident.Name {
				return g.goName(name)
			}
		}
		for name := range g.importedRecords {
			if name == ident.Name || exported(name) == ident.Name {
				if pkg, ok := g.openExports[name]; ok {
					return pkg + "." + exported(name)
				}
				return exported(name)
			}
		}
	}
	return "struct{...}"
}

func (g *Generator) findRecordType(goName string) *ast.TypeDecl {
	for name, td := range g.records {
		if g.goName(name) == goName {
			return td
		}
	}
	for name, td := range g.importedRecords {
		qual := exported(name)
		if pkg, ok := g.openExports[name]; ok {
			qual = pkg + "." + exported(name)
		}
		if qual == goName || exported(name) == goName {
			return td
		}
	}
	return nil
}

// fieldAccessName returns the dotted name for a simple field access expression
// like Console.print_line, or empty string if it's complex.
func (g *Generator) fieldAccessName(e *ast.FieldAccessExpr) string {
	if ctor, ok := e.Left.(*ast.ConstructorExpr); ok && ctor.Arg == nil {
		return ctor.Name + "." + e.Field
	}
	if ident, ok := e.Left.(*ast.IdentExpr); ok {
		return ident.Name + "." + e.Field
	}
	return ""
}

func (g *Generator) emitFieldAccess(e *ast.FieldAccessExpr) {
	// If left is an ident and the current function has a row parameter
	// with that name, access the expanded Go param directly.
	if ident, ok := e.Left.(*ast.IdentExpr); ok {
		if rowName, hasRow := g.rowParamName[g.currentFunc]; hasRow && rowName == ident.Name {
			g.buf.WriteString(ident.Name + "_" + e.Field)
			return
		}
	}
	// If left is a constructor (like Console), emit specially
	if ctor, ok := e.Left.(*ast.ConstructorExpr); ok {
		if ctor.Name == "Console" && ctor.Arg == nil {
			if e.Field == "print_line" {
				g.buf.WriteString("fmt.Println")
				g.needFmt = true
				return
			}
		}
	}
	g.emitExpr(e.Left, false)
	g.buf.WriteString(".")
	if _, ok := g.externMethods[e.Field]; ok {
		g.buf.WriteString(g.externMethods[e.Field].methodName)
		return
	}
	if recvName := goopTypeNameFromInternal(g.typeOf(e.Left)); recvName != "" {
		if fieldName := g.externFields[recvName][e.Field]; fieldName != "" {
			g.buf.WriteString(fieldName)
			return
		}
	}
	g.buf.WriteString(exported(e.Field))
}

func goopTypeNameFromInternal(t types.Type) string {
	switch t := t.(type) {
	case *types.TGoNamed:
		return t.Name
	case *types.TPtr:
		return goopTypeNameFromInternal(t.Elem)
	}
	return ""
}

func fieldAccessMatchesType(e *ast.FieldAccessExpr, typeName string) bool {
	switch left := e.Left.(type) {
	case *ast.IdentExpr:
		return left.Name == typeName
	case *ast.ConstructorExpr:
		return left.Name == typeName && left.Arg == nil
	}
	return false
}

func (g *Generator) emitMethodSend(e *ast.MethodSendExpr) {
	g.emitExpr(e.Target, false)
	g.buf.WriteString(".")
	g.buf.WriteString(exported(e.Method))
	g.buf.WriteString("()")
}

func (g *Generator) emitTuple(e *ast.TupleExpr) {
	goType := g.tupleGoTypeFromExpr(e)
	if goType == "" {
		parts := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			var buf strings.Builder
			old := g.buf
			g.buf = buf
			g.emitExpr(el, false)
			parts[i] = g.buf.String()
			g.buf = old
		}
		goType = "Tuple" + strings.Join(parts, "")
	}
	g.buf.WriteString(goType + "{")
	for i, el := range e.Elems {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.buf.WriteString("F" + fmt.Sprintf("%d", i) + ": ")
		g.emitExpr(el, false)
	}
	g.buf.WriteString("}")
}

func (g *Generator) tupleGoTypeFromExpr(e *ast.TupleExpr) string {
	if g.typeMap == nil {
		return ""
	}
	t, ok := g.typeMap[e]
	if !ok {
		return ""
	}
	if tt, ok := t.(*types.TTuple); ok {
		parts := make([]string, len(tt.Elems))
		for i, el := range tt.Elems {
			parts[i] = g.internalTypeToGo(el)
		}
		for name, elems := range g.usedTuple {
			if len(elems) == len(parts) {
				match := true
				for i, p := range parts {
					if elems[i] != p {
						match = false
						break
					}
				}
				if match {
					return name
				}
			}
		}
	}
	return ""
}

func (g *Generator) emitList(e *ast.ListExpr) {
	if len(e.Elems) == 0 {
		g.buf.WriteString("nil")
		return
	}
	// Determine element type from enclosing function's return type if possible
	elemType := g.currentListElemType()
	g.buf.WriteString("[]" + elemType + "{")
	for i, el := range e.Elems {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(el, false)
	}
	g.buf.WriteString("}")
}

func (g *Generator) currentListElemType() string {
	if ret, ok := g.funcRetType[g.currentFunc]; ok {
		if strings.HasPrefix(ret, "[]") {
			return ret[2:]
		}
	}
	return "interface{}"
}

// ---------------------------------------------------------------------------
// Active pattern match lowering
// ---------------------------------------------------------------------------

func (g *Generator) hasActivePattern(e *ast.MatchExpr) bool {
	for _, arm := range e.Arms {
		if cp, ok := arm.Pattern.(*ast.ConstructorPattern); ok {
			if active.GlobalRegistry.IsActivePattern(cp.Name) {
				return true
			}
		}
	}
	return false
}

func (g *Generator) emitActiveMatch(e *ast.MatchExpr) {
	// Lower to flat if statements: for each active pattern arm,
	//   if _opt := __active_Positive(v); _opt.IsSome() { ... }
	// Guarded arms: if guard fails, fall through to next arm.
	// Wildcard arm: emit as final else or bare return.
	g.varCounter++
	scrutVar := fmt.Sprintf("_a%d", g.varCounter)
	g.emitf("%s := ", scrutVar)
	g.emitExpr(e.Scrutinee, false)
	g.buf.WriteString("\n")

	for i, arm := range e.Arms {
		switch p := arm.Pattern.(type) {
		case *ast.ConstructorPattern:
			if ap := active.GlobalRegistry.Lookup(p.Name); ap != nil {
				goFunc := ap.GoFuncName
				// Store the option result in a temp to avoid double calls
				g.varCounter++
				optVar := fmt.Sprintf("_opt%d", g.varCounter)
				g.emitf("if %s := %s(%s); %s.IsSome() {\n",
					optVar, goFunc, scrutVar, optVar)
				g.indent++
				if p.Arg != nil {
					varName := g.patternVarName(p.Arg)
					// Use = for blank identifier (Go 1.22 doesn't allow _ := expr alone)
					if varName == "_" {
						g.emitf("_ = %s.MustSome()\n", optVar)
					} else {
						g.emitf("%s := %s.MustSome()\n", varName, optVar)
						// Suppress "declared and not used"
						g.emitf("_ = %s\n", varName)
					}
				}
				if arm.Guard != nil {
					g.emitf("if ")
					g.emitExpr(arm.Guard, false)
					g.buf.WriteString(" {\n")
					g.indent++
					g.emitReturnExpr(arm.Body)
					g.indent--
					g.emitf("}\n")
				} else {
					g.emitReturnExpr(arm.Body)
				}
				g.indent--
				g.emitf("}\n")
			}
		case *ast.WildcardPattern:
			// Wildcard: emit as the final return
			g.emitReturnExpr(arm.Body)
			return // done — nothing after wildcard
		case *ast.IdentPattern:
			// Variable pattern: bind and return
			g.emitf("%s := %s\n", p.Name, scrutVar)
			g.emitReturnExpr(arm.Body)
			return
		}
		_ = i
	}

	// If we get here, there was no wildcard. Emit a panic as fallback.
	g.emitf("panic(\"unreachable: unhandled active pattern\")\n")
}

// ---------------------------------------------------------------------------
// Refinement contract checks
// ---------------------------------------------------------------------------

// emitPreconditionCheck emits a runtime check for a parameter refinement:
//
//	if !(<pred>) { panic("<func>: precondition violated: <pred text>") }
func (g *Generator) emitPreconditionCheck(funcName, paramName string, rt *ast.RefinementType) {
	g.emitRefinementGuard(funcName, rt.Pred, "precondition violated")
}

// needsCallSiteGuards reports whether unproven refinement checks must be emitted.
func (g *Generator) needsCallSiteGuards(app *ast.AppExpr, funcName string, args []ast.Expr) bool {
	if funcName == "" || g.provenSites[app] {
		return false
	}
	refined, ok := g.refinementParams[funcName]
	if !ok || len(refined) == 0 {
		return false
	}
	paramCount := g.funcParamCount[funcName]
	if paramCount > 0 && len(args) < paramCount {
		return false
	}
	return true
}

// emitGuardedCallExpr wraps a call in an IIFE when guards are needed in expression position.
func (g *Generator) emitGuardedCallExpr(funcExpr ast.Expr, args []ast.Expr, funcName string, app *ast.AppExpr) {
	retGo := g.funcRetType[funcName]
	unitRet := retGo == "" || retGo == "struct{}"
	if unitRet {
		g.buf.WriteString("(func() {\n")
	} else {
		g.buf.WriteString("(func() " + retGo + " {\n")
	}
	g.indent++
	g.emitCallSiteRefinementGuards(app, funcName, args)
	if !unitRet {
		g.buf.WriteString("return ")
	}
	g.emitExpr(funcExpr, false)
	g.buf.WriteString("(")
	for i, arg := range args {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitExpr(arg.(ast.Expr), false)
	}
	g.buf.WriteString(")")
	if unitRet {
		g.buf.WriteString("\n")
	}
	g.indent--
	g.buf.WriteString("\n})()")
}

// emitCallSiteRefinementGuards emits inline checks before a call when refinements
// were not proven at compile time.
func (g *Generator) emitCallSiteRefinementGuards(app *ast.AppExpr, funcName string, args []ast.Expr) {
	if !g.needsCallSiteGuards(app, funcName, args) {
		return
	}
	refined := g.refinementParams[funcName]
	for _, rp := range refined {
		if rp.index >= len(args) {
			continue
		}
		actualArg := args[rp.index]
		pred := refine.SubstituteIdent(rp.rt.Pred, rp.name, actualArg)
		g.emitRefinementGuard(funcName, pred, "precondition violated")
	}
}

func (g *Generator) shouldEmitEntryGuards(funcName string, isPrivate bool) bool {
	if !isPrivate && isExportedGoopName(funcName) {
		return true
	}
	if g.funcAllProven != nil && !g.funcAllProven[funcName] {
		return true
	}
	return false
}

func isExportedGoopName(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func (g *Generator) emitRefinementGuard(funcName string, pred ast.Expr, kind string) {
	var predBuf strings.Builder
	origBuf := g.buf
	g.buf = predBuf
	g.emitExpr(pred, false)
	predGo := g.buf.String()
	g.buf = origBuf

	g.emitf("if !(%s) { panic(\"%s: %s: %s\") }\n",
		predGo, funcName, kind, g.exprToSource(pred))
}

// emitPostconditionCheck emits a defer check for a return type refinement:
//
//	defer func() { if !(<pred>) { panic("<func>: postcondition violated") } }()
func (g *Generator) emitPostconditionCheck(funcName string, rt *ast.RefinementType) {
	var predBuf strings.Builder
	origBuf := g.buf
	g.buf = predBuf
	g.emitExpr(rt.Pred, false)
	predGo := g.buf.String()
	g.buf = origBuf

	g.emitf("defer func() {\n")
	g.indent++
	g.emitf("if !(%s) { panic(\"%s: postcondition violated: %s\") }\n",
		predGo, funcName, g.predicateSource(rt))
	g.indent--
	g.emitf("}()\n")
}

// predicateSource returns a human-readable form of the refinement predicate.
func (g *Generator) predicateSource(rt *ast.RefinementType) string {
	return g.exprToSource(rt.Pred)
}

// exprToSource recursively converts an expression to a readable source form.
func (g *Generator) exprToSource(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.BinaryExpr:
		left := g.exprToSource(e.Left)
		right := g.exprToSource(e.Right)
		op := e.Op.String()
		return left + " " + op + " " + right
	case *ast.IdentExpr:
		return e.Name
	case *ast.LitExpr:
		return fmt.Sprintf("%v", e.Value)
	default:
		return ast.ExprString(e)
	}
}

// resolveChanElemType attempts to determine the Go type of a channel's
// element by inspecting the TypeMap. It relies on the type checker having
// fully resolved all type variables (see CheckWithTypes in typecheck.go).
func (g *Generator) resolveChanElemType(args []ast.Expr) string {
	if g.typeMap == nil || len(args) == 0 {
		return ""
	}
	// Look up the channel argument's type in the TypeMap.
	// After type checking, this should be a concrete *types.TChan.
	if chanType, ok := g.typeMap[args[0]].(*types.TChan); ok {
		elemGo := g.internalTypeToGo(chanType.Elem)
		if elemGo != "interface{}" {
			return elemGo
		}
	}
	// Also handle owned_chan (TAdt) — same *C0Chan Go type.
	if adtType, ok := g.typeMap[args[0]].(*types.TAdt); ok && adtType.Name == "owned_chan" {
		if len(adtType.Params) > 0 {
			elemGo := g.internalTypeToGo(adtType.Params[0])
			if elemGo != "interface{}" {
				return elemGo
			}
		}
	}
	// Fallback: if the argument is an identifier, look it up in the VarTypeMap
	if ident, ok := args[0].(*ast.IdentExpr); ok {
		if t, ok := g.varTypeMap[ident.Name]; ok {
			if tc, ok := t.(*types.TChan); ok {
				elemGo := g.internalTypeToGo(tc.Elem)
				if elemGo != "interface{}" {
					return elemGo
				}
			}
			// Also handle owned_chan in VarTypeMap
			if adt, ok := t.(*types.TAdt); ok && adt.Name == "owned_chan" {
				if len(adt.Params) > 0 {
					elemGo := g.internalTypeToGo(adt.Params[0])
					if elemGo != "interface{}" {
						return elemGo
					}
				}
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Channel wrapper helpers
// ---------------------------------------------------------------------------

// emitChanHelpers emits the C0Chan wrapper struct and its helper functions
// if any channel operations are used in the module.
func (g *Generator) emitChanHelpers() {
	if !g.usedChan {
		return
	}
	g.emitf("// Goop file-operations‑safe channel wrapper with close‑time tracking\n")
	g.emitf("type C0Chan struct {\n")
	g.indent++
	g.emitf("ch     chan interface{}\n")
	g.emitf("closed bool\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func C0ChanMake() *C0Chan {\n")
	g.indent++
	g.emitf("return &C0Chan{ch: make(chan interface{})}\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func C0ChanSend(c *C0Chan, v interface{}) {\n")
	g.indent++
	g.emitf("if c.closed { panic(\"Chan.send: channel is closed\") }\n")
	g.emitf("c.ch <- v\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func C0ChanRecv(c *C0Chan) interface{} {\n")
	g.indent++
	g.emitf("return <-c.ch\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func C0ChanClose(c *C0Chan) {\n")
	g.indent++
	g.emitf("if c.closed { panic(\"Chan.close: channel already closed\") }\n")
	g.emitf("c.closed = true\n")
	g.emitf("close(c.ch)\n")
	g.indent--
	g.emitf("}\n\n")
}

// ---------------------------------------------------------------------------
// HTTP/JSON helpers
// ---------------------------------------------------------------------------

// emitHTTPHelpers emits Go helper functions for HTTP and JSON operations
// if any HTTP/JSON prelude functions are used in the module.
func (g *Generator) emitHTTPHelpers() {
	if !g.usedHTTP {
		return
	}
	g.emitf("// HTTP/JSON helpers for Goop prelude\n")
	g.emitf("func httpGetString(url string) string {\n")
	g.indent++
	g.emitf("client := &http.Client{Timeout: 15 * time.Second}\n")
	g.emitf("resp, err := client.Get(url)\n")
	g.emitf("if err != nil { panic(\"HTTP error: \" + err.Error()) }\n")
	g.emitf("defer resp.Body.Close()\n")
	g.emitf("body, err := io.ReadAll(resp.Body)\n")
	g.emitf("if err != nil { panic(\"HTTP read error: \" + err.Error()) }\n")
	g.emitf("return string(body)\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func jsonExtractFloats(jsonStr string, index int) []float64 {\n")
	g.indent++
	g.emitf("var raw []any\n")
	g.emitf("if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil { panic(\"JSON error: \" + err.Error()) }\n")
	g.emitf("result := make([]float64, 0, len(raw))\n")
	g.emitf("for _, item := range raw {\n")
	g.indent++
	g.emitf("arr, ok := item.([]any)\n")
	g.emitf("if !ok || len(arr) <= index { continue }\n")
	g.emitf("s, ok := arr[index].(string)\n")
	g.emitf("if !ok { continue }\n")
	g.emitf("f, err := strconv.ParseFloat(s, 64)\n")
	g.emitf("if err != nil { continue }\n")
	g.emitf("result = append(result, f)\n")
	g.indent--
	g.emitf("}\n")
	g.emitf("return result\n")
	g.indent--
	g.emitf("}\n\n")

	g.emitf("func jsonExtractStrings(jsonStr string, index int) []string {\n")
	g.indent++
	g.emitf("var raw []any\n")
	g.emitf("if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil { panic(\"JSON error: \" + err.Error()) }\n")
	g.emitf("result := make([]string, 0, len(raw))\n")
	g.emitf("for _, item := range raw {\n")
	g.indent++
	g.emitf("arr, ok := item.([]any)\n")
	g.emitf("if !ok || len(arr) <= index { continue }\n")
	g.emitf("s, ok := arr[index].(string)\n")
	g.emitf("if !ok { continue }\n")
	g.emitf("result = append(result, s)\n")
	g.indent--
	g.emitf("}\n")
	g.emitf("return result\n")
	g.indent--
	g.emitf("}\n\n")
}

// ---------------------------------------------------------------------------
// Extern declarations
// ---------------------------------------------------------------------------

func (g *Generator) emitExternDecl(d *ast.ExternDecl) {
	g.emitf("// extern %q %q\n", d.Lang, d.Path)
	for _, ev := range d.Vals {
		g.emitf("// val %s : %s\n", ev.Name, g.typeToGo(ev.Type))
	}
	g.buf.WriteString("\n")
}

func (g *Generator) emitLangEmbedDecl(d *ast.LangEmbedDecl) {
	switch d.Lang {
	case "go":
		if d.Body != "" {
			g.buf.WriteString("\n")
			g.buf.WriteString(d.Body)
			g.buf.WriteString("\n\n")
		}
		for _, ev := range d.Vals {
			g.emitf("// val %s : %s\n", ev.Name, g.typeToGo(ev.Type))
		}
		g.buf.WriteString("\n")
	case "c":
		for _, ev := range d.Vals {
			g.emitCgoWrapper(ev)
		}
	}
}

// emitCgoWrapper emits a Go function that marshals Goop primitives to/from C.
func (g *Generator) emitCgoWrapper(ev ast.ExternVal) {
	params, ret := flattenFunType(ev.Type)
	for _, pt := range params {
		if err := cgoSupportedType(pt); err != "" {
			g.errs = append(g.errs, fmt.Sprintf("@[c] val %s: %s", ev.Name, err))
			return
		}
	}
	if err := cgoSupportedType(ret); err != "" {
		g.errs = append(g.errs, fmt.Sprintf("@[c] val %s: %s", ev.Name, err))
		return
	}

	type paramInfo struct {
		name string
		goT  string
		ty   string // type ident name
	}
	var infos []paramInfo
	for i, pt := range params {
		ty := typeIdentName(pt)
		if ty == "unit" {
			continue
		}
		infos = append(infos, paramInfo{
			name: fmt.Sprintf("a%d", i),
			goT:  g.typeToGo(pt),
			ty:   ty,
		})
	}

	var paramDecls []string
	for _, p := range infos {
		paramDecls = append(paramDecls, p.name+" "+p.goT)
	}

	retGo := g.typeToGo(ret)
	retName := typeIdentName(ret)
	retIsUnit := retName == "unit" || retGo == "struct{}"

	g.emitf("func %s(%s) ", ev.Name, strings.Join(paramDecls, ", "))
	if retIsUnit {
		g.buf.WriteString("{\n")
	} else {
		g.buf.WriteString(retGo + " {\n")
	}
	g.indent++

	var callArgs []string
	for _, p := range infos {
		switch p.ty {
		case "string":
			g.needUnsafe = true
			cs := "__cs_" + p.name
			g.emitf("%s := C.CString(%s)\n", cs, p.name)
			g.emitf("defer C.free(unsafe.Pointer(%s))\n", cs)
			callArgs = append(callArgs, cs)
		case "bool":
			callArgs = append(callArgs, "func() C.int { if "+p.name+" { return 1 }; return 0 }()")
		case "int":
			callArgs = append(callArgs, "C.int("+p.name+")")
		case "int32":
			callArgs = append(callArgs, "C.int32_t("+p.name+")")
		case "int64":
			callArgs = append(callArgs, "C.int64_t("+p.name+")")
		case "float", "float64":
			callArgs = append(callArgs, "C.double("+p.name+")")
		default:
			callArgs = append(callArgs, p.name)
		}
	}

	cCall := fmt.Sprintf("C.%s(%s)", ev.Name, strings.Join(callArgs, ", "))
	if retIsUnit {
		g.emitf("%s\n", cCall)
	} else {
		switch retName {
		case "string":
			g.emitf("return C.GoString(%s)\n", cCall)
		case "bool":
			g.emitf("return %s != 0\n", cCall)
		case "int":
			g.emitf("return int(%s)\n", cCall)
		case "int32":
			g.emitf("return int32(%s)\n", cCall)
		case "int64":
			g.emitf("return int64(%s)\n", cCall)
		case "float", "float64":
			g.emitf("return float64(%s)\n", cCall)
		default:
			g.emitf("return %s(%s)\n", retGo, cCall)
		}
	}
	g.indent--
	g.emitf("}\n\n")
}

func flattenFunType(t ast.Type) (params []ast.Type, ret ast.Type) {
	ret = t
	for {
		fn, ok := ret.(*ast.TFun)
		if !ok {
			break
		}
		params = append(params, fn.From)
		ret = fn.To
	}
	return params, ret
}

func typeIdentName(t ast.Type) string {
	if t == nil {
		return ""
	}
	if id, ok := t.(*ast.TIdent); ok {
		return id.Name
	}
	return ""
}

func cgoSupportedType(t ast.Type) string {
	if t == nil {
		return "missing type"
	}
	name := typeIdentName(t)
	switch name {
	case "int", "int32", "int64", "float", "float64", "bool", "string", "unit":
		return ""
	case "":
		return "unsupported @[c] type (use @[go] to call C.* by hand)"
	default:
		return fmt.Sprintf("unsupported @[c] type %q (use @[go] to call C.* by hand)", name)
	}
}
