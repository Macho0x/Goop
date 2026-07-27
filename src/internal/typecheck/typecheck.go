// Package typecheck implements Hindley-Milner style type inference for Goop.
//
// Design decisions:
//   - We use a mutable substitution map updated in-place during unification.
//   - The top-level environment is built from type declarations, then value
//     declarations are checked in order.
//   - Let-polymorphism: let-bindings are generalized (free variables not in
//     the current environment are quantified).
//   - Recursive let-bindings: all bindings in the group share the same
//     fresh type variables, which are unified with their body types.
//   - The `?` operator is special-cased: expr ? with type result<A, B>
//     yields type A.
//   - Pipeline `|>` is desugared to application: `x |> f` ≡ `f x`.
package typecheck

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"goop.dev/compiler/internal/active"
	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/exhaustive"
	"goop.dev/compiler/internal/gosig"
	"goop.dev/compiler/internal/modresolve"
	"goop.dev/compiler/internal/prelude"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

// ---------------------------------------------------------------------------
// TypeError — a type-check error with a source location
// ---------------------------------------------------------------------------

// TypeError carries a message and, when available, a source location.
type TypeError struct {
	Msg string
	Loc token.SourceLoc // may be zero-value if location unknown
}

func (e *TypeError) Error() string {
	if e.Loc.File == "" && e.Loc.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Loc, e.Msg)
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

// Env maps names to type schemes.
type Env struct {
	parent *Env
	names  map[string]*types.Scheme
}

// NewEnv creates a new (potentially nested) environment.
func NewEnv(parent *Env) *Env {
	return &Env{parent: parent, names: make(map[string]*types.Scheme)}
}

// Lookup finds a name in the environment chain.
func (e *Env) Lookup(name string) *types.Scheme {
	for cur := e; cur != nil; cur = cur.parent {
		if s, ok := cur.names[name]; ok {
			return s
		}
	}
	return nil
}

// Bind adds a name to the current scope.
func (e *Env) Bind(name string, s *types.Scheme) {
	e.names[name] = s
}

// InScope returns the set of all free variable IDs in the environment chain
// (used for generalization: variables in scope are NOT quantified).
func (e *Env) InScope() map[int64]bool {
	m := make(map[int64]bool)
	for cur := e; cur != nil; cur = cur.parent {
		for _, s := range cur.names {
			for _, v := range s.Vars {
				m[v.ID] = true
			}
			fv := types.FreeVars(s.Type)
			for id := range fv {
				m[id] = true
			}
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Type checker state
// ---------------------------------------------------------------------------

// Checker holds the mutable state during type checking.
type Checker struct {
	env             *Env              // current environment
	sub             types.Subst       // current substitution
	errs            []error           // accumulated errors
	types           typeinfo.TypeMap  // maps expression AST nodes to their inferred types
	privateNames    map[string]bool   // names marked private in the current module
	blockedNames    map[string]string // private name → defining module path
	importedModule  string            // module being checked (for error messages)
	effectInference bool              // infer effect rows from bodies when true
	mutableVars     map[string]bool   // bindings that may be assigned via <-
	modules         map[string]*moduleExports
	moduleTypes     map[string]*moduleSignature
	functors        map[string]*functorDef
	extensible      map[string]bool
	classes         map[string]*classInfo
	goFields        map[string]map[string]types.Type
	goStructs       map[string]*goStructSchema // Goop type name → struct field schema
	goImportPaths   map[string]string          // Goop type name → Go import path
}

// goStructSchema holds exported (+ promoted) fields for an imported Go struct.
type goStructSchema struct {
	Pkg    string
	Name   string
	Fields []goStructField
}

type goStructField struct {
	GoName string
	Typ    types.Type
}

// pkgFromPath extracts a Go package name from an import path (last segment).
func pkgFromPath(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (c *Checker) bindExternVals(importPath string, vals []ast.ExternVal) {
	c.bindExternValsOpts(importPath, vals, false)
}

func (c *Checker) bindExternValsOpts(importPath string, vals []ast.ExternVal, raw bool) {
	pkgName := pkgFromPath(importPath)
	for _, ev := range vals {
		t := c.convertASTType(ev.Type)
		if ev.Kind == ast.ExternFunc {
			if refined := c.refineExternType(importPath, ev.Name, t); refined != nil {
				t = refined
			}
		} else {
			recv := c.convertASTType(ev.RecvType)
			t = &types.TFun{From: recv, To: t}
			if ev.Kind == ast.ExternField {
				typeName := goNamedTypeName(recv)
				if typeName != "" {
					if c.goFields[typeName] == nil {
						c.goFields[typeName] = make(map[string]types.Type)
					}
					c.goFields[typeName][ev.Name] = c.convertASTType(ev.Type)
				}
			}
		}
		// H6: (T, error) → ('ok, 'err) result unless import go raw.
		if !raw && ev.Kind != ast.ExternField {
			t = coerceTupleErrorToResult(t)
		}
		scheme := types.Mono(t)
		if c.env.Lookup(ev.Name) != nil {
			// Field/method short names often collide with imported type names
			// (e.g. type Value vs Attr.Value). Keep Type.Name qualified binds only.
			if ev.Kind == ast.ExternFunc {
				c.errorf("extern binding %q conflicts with existing name", ev.Name)
			}
		} else {
			c.env.Bind(ev.Name, scheme)
		}
		if importPath != "" && ev.Kind == ast.ExternFunc {
			qualified := pkgName + "." + ev.Name
			c.env.Bind(qualified, scheme)
		}
		if ev.Kind != ast.ExternFunc {
			if typeName := goNamedTypeName(c.convertASTType(ev.RecvType)); typeName != "" {
				c.env.Bind(typeName+"."+ev.Name, scheme)
			}
		}
	}
}

func goNamedTypeName(t types.Type) string {
	switch t := types.Apply(types.EmptySubst(), t).(type) {
	case *types.TGoNamed:
		return t.Name
	case *types.TPtr:
		return goNamedTypeName(t.Elem)
	}
	return ""
}

func (c *Checker) bindExternTypes(importPath string, externTypes []ast.ExternType) {
	pkgName := pkgFromPath(importPath)
	for _, et := range externTypes {
		isInterface := true
		var schema *goStructSchema
		if info, err := gosig.LookupType(importPath, et.Name); err == nil && info != nil {
			switch info.Kind {
			case gosig.TypeKindInterface:
				isInterface = true
			case gosig.TypeKindStruct:
				isInterface = false
				schema = &goStructSchema{Pkg: pkgName, Name: et.Name}
				for _, f := range info.Fields {
					ft := goTypeToC0TypeInPkg(f.Type, importPath)
					if ft == nil {
						ft = &types.TGoNamed{Pkg: pkgName, Name: f.Type, Interface: true}
					}
					schema.Fields = append(schema.Fields, goStructField{GoName: f.Name, Typ: ft})
				}
			default:
				isInterface = false
			}
		}
		c.env.Bind(et.Name, types.Mono(&types.TGoNamed{Pkg: pkgName, Name: et.Name, Interface: isInterface}))
		if c.goImportPaths == nil {
			c.goImportPaths = make(map[string]string)
		}
		c.goImportPaths[et.Name] = importPath
		if schema != nil {
			if c.goStructs == nil {
				c.goStructs = make(map[string]*goStructSchema)
			}
			c.goStructs[et.Name] = schema
		}
	}
}

// Check runs type inference on a complete module.
func Check(mod *ast.Module) []error {
	_, _, errs := CheckWithTypes(mod)
	return errs
}

// CheckWithTypes runs type inference and returns the TypeMap with fully
// resolved types for every expression node, a VarTypeMap with resolved
// types for let-bound variables, plus any errors.
func CheckWithTypes(mod *ast.Module) (typeinfo.TypeMap, typeinfo.VarTypeMap, []error) {
	return CheckWithTypesForFile(mod, "", nil, nil)
}

// CheckWithTypesForFile type-checks mod, resolving import goop dependencies from disk when srcFile is set.
func CheckWithTypesForFile(mod *ast.Module, srcFile string, cfg *config.Config, lock *config.Lockfile) (typeinfo.TypeMap, typeinfo.VarTypeMap, []error) {
	mergeSiblingModules(mod, srcFile)
	var deps map[string]*ast.Module
	var resolver *modresolve.Resolver
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if srcFile != "" {
		root := modresolve.FindProjectRoot(srcFile)
		if root != "" {
			gosig.SetLoadDir(root)
		}
		resolver = modresolve.New(cfg, lock, root)
		var graphErr error
		deps, graphErr = resolver.LoadModuleGraph(srcFile, mod)
		if graphErr != nil {
			return nil, nil, []error{graphErr}
		}
	}
	tm, vtm, errs := checkWithDepsAndImports(mod, deps, resolver, cfg, srcFile)
	return tm, vtm, errs
}

// CheckWithTypesAndDeps type-checks mod after binding exported names from dependency modules.
func CheckWithTypesAndDeps(mod *ast.Module, deps map[string]*ast.Module) (typeinfo.TypeMap, typeinfo.VarTypeMap, []error) {
	return checkWithDepsAndImports(mod, deps, nil, config.DefaultConfig(), "")
}

func checkWithDepsAndImports(mod *ast.Module, deps map[string]*ast.Module, resolver *modresolve.Resolver, cfg *config.Config, srcFile string) (typeinfo.TypeMap, typeinfo.VarTypeMap, []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	c := &Checker{
		env:             NewEnv(nil),
		sub:             types.EmptySubst(),
		types:           make(typeinfo.TypeMap),
		privateNames:    make(map[string]bool),
		blockedNames:    make(map[string]string),
		effectInference: cfg.Check.EffectInference,
		mutableVars:     make(map[string]bool),
		modules:         make(map[string]*moduleExports),
		moduleTypes:     make(map[string]*moduleSignature),
		functors:        make(map[string]*functorDef),
		extensible:      make(map[string]bool),
		classes:         make(map[string]*classInfo),
		goFields:        make(map[string]map[string]types.Type),
		goStructs:       make(map[string]*goStructSchema),
		goImportPaths:   make(map[string]string),
	}
	c.initBuiltins()
	c.importedModule = mod.Name
	if len(mod.Imports) > 0 {
		c.bindImportSpecs(mod.Imports, deps, resolver)
	} else {
		c.bindDependencyExports(deps)
	}
	c.checkModule(mod)

	// Apply the final substitution to all recorded types so they are fully
	// resolved (no free type variables remain that were unified).
	// We iterate until fixpoint because the substitution may contain
	// chains (e.g. A→B→int) that a single pass won't resolve.
	for iter := 0; iter < 100; iter++ {
		// Check if any type still contains an unresolved TVar
		hasTVar := false
		for _, t := range c.types {
			if containsTVar(t) {
				hasTVar = true
				break
			}
		}
		if !hasTVar {
			break
		}
		for expr, t := range c.types {
			c.types[expr] = types.Apply(c.sub, t)
		}
	}

	// Build a VarTypeMap: for each let binding, look up the variable's
	// type scheme in the environment, instantiate, and apply the
	// substitution to get the fully resolved type.
	varTypes := make(typeinfo.VarTypeMap)
	c.collectVarTypes(mod, varTypes)

	return c.types, varTypes, c.errs
}

// containsTVar returns true if the type contains any type variable.
func containsTVar(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t := t.(type) {
	case *types.TVar:
		return true
	case *types.TFun:
		return containsTVar(t.From) || containsTVar(t.To)
	case *types.TTuple:
		for _, e := range t.Elems {
			if containsTVar(e) {
				return true
			}
		}
	case *types.TRecord:
		if t == nil {
			return false
		}
		for _, f := range t.Fields {
			if containsTVar(f.Type) {
				return true
			}
		}
	case *types.TAdt:
		for _, p := range t.Params {
			if containsTVar(p) {
				return true
			}
		}
		for _, v := range t.Variants {
			if v.Arg != nil && containsTVar(v.Arg) {
				return true
			}
		}
	case *types.TCon:
		for _, a := range t.Args {
			if containsTVar(a) {
				return true
			}
		}
	case *types.TChan:
		return containsTVar(t.Elem)
	}
	return false
}

// collectVarTypes populates the VarTypeMap by walking let declarations
// and looking up variable types in the environment.
func (c *Checker) collectVarTypes(mod *ast.Module, varTypes typeinfo.VarTypeMap) {
	c.collectVarTypesDecls(mod.Decls, varTypes)
}

func (c *Checker) collectVarTypesDecls(decls []ast.TopDecl, varTypes typeinfo.VarTypeMap) {
	for _, d := range decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			for _, b := range d.Bindings {
				s := c.env.Lookup(b.Name)
				if s == nil {
					continue
				}
				t := s.Instantiate()
				t = types.Apply(c.sub, t)
				varTypes[b.Name] = t
			}
		case *ast.NestedModuleDecl:
			c.collectVarTypesDecls(d.Decls, varTypes)
			// Also look up exports by name after open may have rebound them
			for _, nd := range d.Decls {
				if ld, ok := nd.(*ast.LetDecl); ok {
					for _, b := range ld.Bindings {
						if _, ok := varTypes[b.Name]; ok {
							continue
						}
						if s := c.env.Lookup(b.Name); s != nil {
							t := types.Apply(c.sub, s.Instantiate())
							varTypes[b.Name] = t
						} else if c.modules != nil {
							if m := c.modules[d.Name]; m != nil {
								if s := m.vals[b.Name]; s != nil {
									varTypes[b.Name] = types.Apply(c.sub, s.Instantiate())
								}
							}
						}
					}
				}
			}
		}
	}
}

func (c *Checker) errorf(format string, args ...any) {
	c.errs = append(c.errs, &TypeError{Msg: fmt.Sprintf(format, args...)})
}

// errorfAt creates a type error with a known source location.
func (c *Checker) errorfAt(loc token.SourceLoc, format string, args ...any) {
	c.errs = append(c.errs, &TypeError{Loc: loc, Msg: fmt.Sprintf(format, args...)})
}

// locOf extracts the source location from an expression node.
func locOf(e ast.Expr) token.SourceLoc {
	return ast.ExprLoc(e)
}

// fresh creates a new type variable and applies the current substitution.
func (c *Checker) fresh(name string) types.Type {
	return types.Fresh(name)
}

// ---------------------------------------------------------------------------
// Built-in types and constructors
// ---------------------------------------------------------------------------

func (c *Checker) initBuiltins() {
	// option ADT: type 'a option = None | Some of 'a
	// result ADT: type ('ok, 'err) result = Ok of 'ok | Error of 'err
	// list type constructor: 'a list with [] and :: constructors

	// Register constructor types for option.
	// None: 'a -> 'a option  (actually just option<'a> since it has no arg)
	// Some: 'a -> 'a option
	a := types.Fresh("'a")
	ok := types.Fresh("'ok")
	err := types.Fresh("'err")

	optType := types.OptionType(a)
	resType := types.ResultType(ok, err)

	// Polymorphic schemes so each Some/None/Ok/Error use gets fresh type variables.
	c.env.Bind("None", types.Generalize(optType, nil))
	c.env.Bind("Some", types.Generalize(&types.TFun{From: a, To: optType}, nil))
	c.env.Bind("Ok", types.Generalize(&types.TFun{From: ok, To: resType}, nil))
	c.env.Bind("Error", types.Generalize(&types.TFun{From: err, To: resType}, nil))

	// Register built-in ADTs for exhaustiveness checking
	exhaustive.RegisterADT("result", []string{"Ok", "Error"})
	exhaustive.RegisterADT("option", []string{"None", "Some"})

	// Register prelude bindings (available to all programs without `open`).
	// These are shadowable — user definitions in the same scope override them.
	pre := prelude.Default()
	for _, b := range pre.Bindings {
		scheme := b.Scheme
		if b.Effects != nil {
			scheme = attachSchemeEffects(scheme, *b.Effects)
		}
		c.env.Bind(b.Name, scheme)
	}

	// Register owned_chan as a built-in linear type for type annotations.
	// This enables `let ch : int owned_chan = OwnedChan.make ()`.
	c.env.Bind("owned_chan", types.Mono(&types.TAdt{Name: "owned_chan", Linear: true}))

	// Built-in exception type for raise / exception decls.
	c.env.Bind("exn", types.Mono(&types.Prim{Name: "exn"}))
}

// ---------------------------------------------------------------------------
// Module checking
// ---------------------------------------------------------------------------

func (c *Checker) checkModule(mod *ast.Module) {
	// First pass: register all type declarations so they can be used in value
	// declarations (no forward references for types yet; they must be declared
	// before use in the source order).

	typeDecls := make(map[string]*types.Scheme)

	for _, d := range mod.Decls {
		td, ok := d.(*ast.TypeDecl)
		if !ok {
			continue
		}
		if td.Private {
			c.markPrivate(td.Name)
			c.checkPrivateName(td.Name)
		}
		scheme := c.convertTypeDecl(td)
		c.env.Bind(td.Name, scheme)
		typeDecls[td.Name] = scheme
	}
	for _, d := range mod.Decls {
		if ext, ok := d.(*ast.ExtensibleVariantDecl); ok {
			c.extendVariant(ext)
		}
	}
	_ = typeDecls

	// bindImportSpecs runs before checkModule when imports are present

	// Second pass: check value declarations (let, @[go]/@[c] vals, exceptions).
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			c.checkLetDecl(d)
		case *ast.ImplementsDecl:
			s := c.env.Lookup(d.ForType)
			if s == nil {
				c.errorf("FFI-IMPL001: unknown implementation type %q", d.ForType)
				continue
			}
			for _, method := range d.Methods {
				if len(method.Params) == 0 {
					c.errorf("FFI-IMPL001: method %q requires a receiver parameter", method.Name)
					continue
				}
				c.checkLetDecl(&ast.LetDecl{Bindings: []ast.LetBinding{method}})
			}
		case *ast.LangEmbedDecl:
			c.bindExternVals("", d.Vals)
		case *ast.TypeDecl:
			// Already handled in first pass
		case *ast.ExtensibleVariantDecl:
			// Registered with its base type in the first pass.
		case *ast.ExceptionDecl:
			c.checkExceptionDecl(d)
		case *ast.EffectDecl, *ast.NestedModuleDecl, *ast.ModuleTypeDecl,
			*ast.OpenModuleDecl, *ast.IncludeDecl, *ast.ClassDecl:
			c.checkTopDeclExtra(d)
		}
	}
}

// ---------------------------------------------------------------------------
// Type declaration conversion
// ---------------------------------------------------------------------------

func (c *Checker) convertTypeDecl(td *ast.TypeDecl) *types.Scheme {
	switch k := td.Kind.(type) {
	case *ast.OpaqueTypeKind:
		// Opaque linear type: no body, just a name
		adt := &types.TAdt{
			Name:     td.Name,
			Linear:   td.Quantity == 1,
			Variants: nil,
		}
		return types.Mono(adt)

	case *ast.RecordTypeKind:
		fields := make([]types.Field, len(k.Fields))
		for i, f := range k.Fields {
			fields[i] = types.Field{Name: f.Name, Type: c.convertASTType(f.Type)}
		}
		t := &types.TRecord{Fields: fields}
		// Quantify type params if present
		if len(td.TypeParams) > 0 {
			vars := make([]*types.TVar, len(td.TypeParams))
			for i, tp := range td.TypeParams {
				vars[i] = types.Fresh(tp)
			}
			// For now, simple ADTs don't substitute params into the body.
			// A full Hindley-Milner system would track this, but for the
			// examples the types are monomorphic.
			if len(vars) > 0 {
				return &types.Scheme{Vars: vars, Type: t}
			}
		}
		return types.Mono(t)

	case *ast.ADTTypeKind:
		variants := make([]types.Variant, len(k.Cases))
		for i, cs := range k.Cases {
			v := types.Variant{Name: cs.Name}
			if cs.Arg != nil {
				v.Arg = c.convertASTType(cs.Arg)
			}
			variants[i] = v
		}
		adt := &types.TAdt{
			Name:     td.Name,
			Params:   nil,
			Variants: variants,
			Linear:   td.Quantity == 1,
		}
		// Register constructors in the environment
		for _, cs := range k.Cases {
			var ctorType types.Type
			if cs.Arg != nil {
				ctorType = &types.TFun{From: c.convertASTType(cs.Arg), To: adt}
			} else {
				ctorType = adt
			}
			c.env.Bind(cs.Name, types.Mono(ctorType))
		}
		return types.Mono(adt)

	case *ast.GADTTypeKind:
		return c.convertGADT(td, k)

	case *ast.ExtensibleTypeKind:
		adt := &types.TAdt{Name: td.Name, Linear: td.Quantity == 1}
		c.extensible[td.Name] = true
		return types.Mono(adt)

	case *ast.AliasTypeKind:
		t := c.convertASTType(k.Alias)
		return types.Mono(t)

	case *ast.NewtypeTypeKind:
		rep := c.convertASTType(k.Rep)
		nt := &types.TNewtype{Name: td.Name, Rep: rep}
		ctor := newtypeCtorName(td.Name)
		c.env.Bind(ctor, types.Mono(&types.TFun{From: rep, To: nt}))
		return types.Mono(nt)
	}
	return types.Mono(types.Unit)
}

func (c *Checker) extendVariant(d *ast.ExtensibleVariantDecl) {
	s := c.env.Lookup(d.TypeName)
	if s == nil {
		c.errorf("cannot extend unknown type %s", d.TypeName)
		return
	}
	adt, ok := s.Type.(*types.TAdt)
	if !ok || !c.extensible[d.TypeName] {
		c.errorf("type %s is not extensible", d.TypeName)
		return
	}
	for _, cs := range d.Cases {
		for _, existing := range adt.Variants {
			if existing.Name == cs.Name {
				c.errorf("constructor %s is already defined", cs.Name)
				continue
			}
		}
		v := types.Variant{Name: cs.Name}
		if cs.Arg != nil {
			v.Arg = c.convertASTType(cs.Arg)
		}
		adt.Variants = append(adt.Variants, v)
		var ctorType types.Type = adt
		if v.Arg != nil {
			ctorType = &types.TFun{From: v.Arg, To: adt}
		}
		c.env.Bind(cs.Name, types.Mono(ctorType))
	}
}

// ---------------------------------------------------------------------------
// AST type → internal type conversion
// ---------------------------------------------------------------------------

func (c *Checker) convertASTType(at ast.Type) types.Type {
	if at == nil {
		return c.fresh("'a")
	}
	switch t := at.(type) {
	case *ast.TIdent:
		// Map primitive type names
		switch t.Name {
		case "int":
			return types.Int
		case "float":
			return types.Float
		case "bool":
			return types.Bool
		case "string":
			return types.String
		case "unit":
			return types.Unit
		case "bytes":
			return types.Bytes
		case "rune":
			return types.Rune
		case "list":
			// Type constructor — args filled by TApp
			return &types.TCon{Name: "list"}
		case "array":
			return &types.TCon{Name: "array"}
		case "ref":
			return &types.TCon{Name: "ref"}
		case "lazy":
			return &types.TCon{Name: "lazy"}
		case "option":
			return &types.TCon{Name: "option"}
		case "result":
			return &types.TCon{Name: "result"}
		case "exn":
			return &types.Prim{Name: "exn"}
		case "error":
			return &types.TError{}
		case "any":
			return &types.Prim{Name: "any"}
		case "owned_chan":
			return &types.TAdt{Name: "owned_chan", Linear: true}
		default:
			// Look up user-defined type
			if s := c.env.Lookup(t.Name); s != nil {
				return s.Type
			}
			// Unknown — could be a module-qualified type; just use as-is
			return &types.Prim{Name: t.Name}
		}
	case *ast.TApp:
		// Type application: TApp(Func, Arg)
		// E.g. TApp(list, order) → list<order>
		//      TApp(result, Tuple(user, error)) → result<user, error>
		funcType := c.convertASTType(t.Func)
		argType := c.convertASTType(t.Arg)

		// If func is a recognized type constructor, fill its args.
		if tc, ok := funcType.(*types.TCon); ok {
			if tup, ok := argType.(*types.TTuple); ok {
				// result(user, error) — flatten the tuple
				tc.Args = append(tc.Args, tup.Elems...)
			} else {
				// list<order>, option<int>
				tc.Args = append(tc.Args, argType)
			}
			return tc
		}
		if tad, ok := funcType.(*types.TAdt); ok {
			if tup, ok := argType.(*types.TTuple); ok {
				tad.Params = append(tad.Params, tup.Elems...)
			} else {
				tad.Params = append(tad.Params, argType)
			}
			return tad
		}
		// Fallback: wrap as generic application
		return &types.TCon{Name: "app", Args: []types.Type{funcType, argType}}

	case *ast.TFun:
		fn := &types.TFun{
			From: c.convertASTType(t.From),
			To:   c.convertASTType(t.To),
		}
		if t.Effects != nil {
			fn.Effects = &types.EffectRow{
				Effects: t.Effects.Effects,
				Open:    t.Effects.Open,
			}
			if t.Effects.Rest != "" {
				fn.Effects.Rest = types.Fresh(t.Effects.Rest)
			}
		}
		return fn
	case *ast.TPtr:
		return &types.TPtr{Elem: c.convertASTType(t.Elem)}
	case *ast.TGoSlice, *ast.TVariadic:
		var elem ast.Type
		switch t := t.(type) {
		case *ast.TGoSlice:
			elem = t.Elem
		case *ast.TVariadic:
			elem = t.Elem
		}
		return &types.TGoSlice{Elem: c.convertASTType(elem)}
	case *ast.TTuple:
		elems := make([]types.Type, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = c.convertASTType(e)
		}
		return &types.TTuple{Elems: elems}
	case *ast.TRecord:
		fields := make([]types.Field, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = types.Field{Name: f.Name, Type: c.convertASTType(f.Type)}
		}
		return &types.TRecord{Fields: fields, Open: t.Open}
	case *ast.TObject:
		methods := make([]types.Field, len(t.Methods))
		for i, m := range t.Methods {
			methods[i] = types.Field{Name: m.Name, Type: c.convertASTType(m.Type)}
		}
		return &types.TRecord{Fields: methods, Open: t.Open}
	case *ast.TPolyVariant:
		variants := make([]types.Variant, len(t.Cases))
		for i, cs := range t.Cases {
			variants[i] = types.Variant{Name: cs.Name}
			if cs.Arg != nil {
				variants[i].Arg = c.convertASTType(cs.Arg)
			}
		}
		return &types.PolyVariant{Variants: variants, Open: t.Open, UpperBound: t.UpperBound}
	case *ast.RefinementType:
		// Refinement types are transparent — only the inner type matters.
		// The where clause is a runtime contract, not a type-level constraint.
		return c.convertASTType(t.Inner)
	case *ast.TChan:
		return &types.TChan{Elem: c.convertASTType(t.Elem)}
	case *ast.TVar:
		// Type variable: 'a → fresh type variable
		return c.fresh(t.Name)
	default:
		return c.fresh("'a")
	}
}

// ---------------------------------------------------------------------------
// Let declaration checking
// ---------------------------------------------------------------------------

func (c *Checker) checkLetDecl(d *ast.LetDecl) {
	if d.Mutable {
		c.errorf("let mutable is removed; use 'ref' instead")
	}
	if d.Private {
		for _, b := range d.Bindings {
			c.markPrivate(b.Name)
			c.checkPrivateName(b.Name)
		}
	}
	// Active pattern: let (|Name|_|) (arg: T) : U option = body
	if d.ActivePattern {
		for _, b := range d.Bindings {
			t := c.checkBinding(b)
			// The type of an active pattern is InputType -> option<OutputType>
			// Store in the active pattern registry
			inputType := types.Fresh("input")
			outputType := types.Fresh("output")
			optType := types.OptionType(outputType)
			fnType := &types.TFun{From: inputType, To: optType}
			c.unify(t, fnType)

			solvedInput := types.Apply(c.sub, inputType)
			solvedOutput := types.Apply(c.sub, outputType)
			goFuncName := "__active_" + b.Name
			active.GlobalRegistry.Register(b.Name, solvedInput, solvedOutput, goFuncName)

			// Also bind as a regular function value
			scheme := types.Generalize(t, c.env.InScope())
			c.env.Bind(b.Name, scheme)
		}
		return
	}

	if d.Rec {
		c.checkLetRec(d)
		return
	}
	for _, b := range d.Bindings {
		t := types.Apply(c.sub, c.checkBinding(b))
		// Generalize and bind (Apply first so multi-value extern results,
		// which infer as a fresh TVar unified to a tuple, are not frozen
		// as polymorphic type variables).
		inScope := c.env.InScope()
		scheme := types.Generalize(t, inScope)
		c.env.Bind(b.Name, scheme)
	}
}

func (c *Checker) checkExceptionDecl(d *ast.ExceptionDecl) {
	exn := &types.Prim{Name: "exn"}
	if d.Arg != nil {
		argType := c.convertASTType(d.Arg)
		c.env.Bind(d.Name, types.Mono(&types.TFun{From: argType, To: exn}))
	} else {
		c.env.Bind(d.Name, types.Mono(exn))
	}
}

func (c *Checker) checkLetRec(d *ast.LetDecl) {
	// Create fresh type variables for all bindings in the group
	freshVars := make([]types.Type, len(d.Bindings))
	for i, b := range d.Bindings {
		fv := c.fresh(b.Name)
		freshVars[i] = fv
		// Bind the fresh variable in the env so the body can see it
		c.env.Bind(b.Name, types.Mono(fv))
	}

	for i, b := range d.Bindings {
		t := c.infer(b.Body)
		// If there are params, wrap in function types
		for j := len(b.Params) - 1; j >= 0; j-- {
			var paramType types.Type
			if b.Params[j].Type != nil {
				paramType = c.convertASTType(b.Params[j].Type)
			} else {
				paramType = c.fresh(b.Params[j].Name)
			}
			t = &types.TFun{From: paramType, To: t, Label: b.Params[j].Label, Optional: b.Params[j].Optional}
		}
		// Attach effect row if specified
		if b.RetEffects != nil && len(b.Params) > 0 {
			if fn, ok := t.(*types.TFun); ok {
				fn.Effects = &types.EffectRow{
					Effects: b.RetEffects.Effects,
					Open:    b.RetEffects.Open,
				}
				if b.RetEffects.Rest != "" {
					fn.Effects.Rest = types.Fresh(b.RetEffects.Rest)
				}
			}
		}
		// Unify with the fresh variable
		c.unify(freshVars[i], t)
	}

	// After all bodies are checked, generalize and re-bind
	for i, b := range d.Bindings {
		solved := types.Apply(c.sub, freshVars[i])
		inScope := c.env.InScope()
		scheme := types.Generalize(solved, inScope)
		c.env.Bind(b.Name, scheme)
	}
}

func (c *Checker) checkBinding(b ast.LetBinding) types.Type {
	// Create a nested scope for params
	saved := c.env
	c.env = NewEnv(c.env)

	// Bind parameters with fresh or annotated types
	var paramTypes []types.Type
	for _, p := range b.Params {
		var pt types.Type
		if p.Type != nil {
			pt = c.convertASTType(p.Type)
		} else {
			pt = c.fresh(p.Name)
		}
		c.env.Bind(p.Name, types.Mono(pt))
		paramTypes = append(paramTypes, pt)
	}

	// Infer body type (propagate return annotation for ptr_of / Go struct literals)
	var bodyType types.Type
	if b.RetType != nil {
		retType := c.convertASTType(b.RetType)
		bodyType = c.inferWithExpected(b.Body, retType)
		c.unify(bodyType, retType)
		bodyType = retType
	} else {
		bodyType = c.infer(b.Body)
	}

	c.env = saved

	// Wrap in function types (curried)
	result := bodyType
	for i := len(paramTypes) - 1; i >= 0; i-- {
		result = &types.TFun{From: paramTypes[i], To: result, Label: b.Params[i].Label, Optional: b.Params[i].Optional}
	}

	// Attach effect row to the outermost function type if specified
	if b.RetEffects != nil && len(paramTypes) > 0 {
		if fn, ok := result.(*types.TFun); ok {
			fn.Effects = &types.EffectRow{
				Effects: b.RetEffects.Effects,
				Open:    b.RetEffects.Open,
			}
			if b.RetEffects.Rest != "" {
				fn.Effects.Rest = types.Fresh(b.RetEffects.Rest)
			}
		}
	}

	result = c.finishBindingEffects(b, result)
	return result
}

// ---------------------------------------------------------------------------
// Expression inference
// ---------------------------------------------------------------------------

func (c *Checker) infer(e ast.Expr) types.Type {
	var t types.Type
	switch e := e.(type) {
	case *ast.LitExpr:
		t = c.inferLit(e)
	case *ast.NullExpr:
		t = &types.TPtr{Elem: c.fresh("null")}
	case *ast.PtrOfExpr:
		t = &types.TPtr{Elem: c.infer(e.Inner)}
	case *ast.IsNullExpr:
		_ = c.infer(e.Inner)
		t = types.Bool
	case *ast.SpreadExpr:
		t = c.infer(e.Inner)
	case *ast.IdentExpr:
		t = c.inferIdent(e)
	case *ast.ConstructorExpr:
		t = c.inferConstructor(e)
	case *ast.AppExpr:
		t = c.inferApp(e)
	case *ast.IfExpr:
		t = c.inferIf(e)
	case *ast.MatchExpr:
		t = c.inferMatch(e)
	case *ast.LetInExpr:
		t = c.inferLetIn(e)
	case *ast.FunExpr:
		t = c.inferFun(e)
	case *ast.BinaryExpr:
		t = c.inferBinary(e)
	case *ast.PipeExpr:
		t = c.inferPipe(e)
	case *ast.QuestionExpr:
		c.errorfAt(e.Loc, "'?' error propagation is not supported in Goop 1.0; use `match` on `('ok, 'err) result` (or `Result.bind`) and annotate the result type if needed")
		t = c.fresh("removed")
	case *ast.RecordExpr:
		t = c.inferRecord(e)
	case *ast.RecordUpdateExpr:
		t = c.inferRecordUpdate(e)
	case *ast.FieldAccessExpr:
		t = c.inferFieldAccess(e)
	case *ast.MethodSendExpr:
		t = c.inferMethodSend(e)
	case *ast.TupleExpr:
		t = c.inferTuple(e)
	case *ast.ListExpr:
		t = c.inferList(e)
	case *ast.ParenExpr:
		t = c.infer(e.Inner)
	case *ast.IndexExpr:
		t = c.inferIndex(e)
	case *ast.AssignExpr:
		t = c.inferAssign(e)
	case *ast.ForExpr:
		t = c.inferFor(e)
	case *ast.BeginExpr:
		t = c.inferBegin(e)
	case *ast.WhileExpr:
		t = c.inferWhile(e)
	case *ast.FunctionExpr:
		t = c.inferFunction(e)
	case *ast.RefExpr:
		t = c.inferRef(e)
	case *ast.DerefExpr:
		t = c.inferDeref(e)
	case *ast.TryExpr:
		t = c.inferTry(e)
	case *ast.RaiseExpr:
		t = c.inferRaise(e)
	case *ast.AssertExpr:
		t = c.inferAssert(e)
	case *ast.LazyExpr:
		t = c.inferLazy(e)
	case *ast.PerformExpr:
		t = c.inferPerform(e)
	case *ast.ArrayLitExpr:
		t = c.inferArrayLit(e)
	case *ast.PolyvarExpr:
		t = c.inferPolyvar(e)
	case *ast.ObjectExpr:
		t = c.inferObject(e)
	case *ast.NewExpr:
		t = c.inferNew(e)
	case *ast.LetModuleExpr:
		t = c.inferLetModule(e)
	case *ast.LetOpenExpr:
		t = c.inferLetOpen(e)
	case *ast.LocalOpenExpr:
		t = c.inferLocalOpen(e)
	case *ast.ContinueExpr:
		t = c.inferContinue(e)
	case *ast.DiscontinueExpr:
		t = c.inferDiscontinue(e)
	case *ast.ModuleAppExpr:
		c.errorfAt(e.Loc, "functor application `%s(%s)` is not supported as an expression in Goop 1.0; use a concrete nested `module` binding, or annotate module interfaces with `sig`/`struct`", e.Func, e.Arg)
		t = types.Unit
	case *ast.PackModuleExpr:
		t = c.inferPackModule(e)
	case *ast.UnpackModuleExpr:
		t = c.inferUnpackModule(e)
	case *ast.LabelledArgExpr:
		t = c.inferLabelledArg(e)
	case *ast.GuardExpr:
		c.errorfAt(e.Loc, "'guard' is not supported in Goop 1.0; use `match` with a `when` guard, and annotate the scrutinee if its type is ambiguous")
		t = c.fresh("removed")
	case *ast.IsExpr:
		c.errorfAt(e.Loc, "'is' pattern tests are not supported in Goop 1.0; use `match` instead, and annotate the scrutinee if its type is ambiguous")
		t = types.Bool
	case *ast.AsMatchExpr:
		c.errorfAt(e.Loc, "expression `as` / `else` macros are not supported in Goop 1.0; use `match` instead, and annotate the scrutinee if its type is ambiguous")
		t = c.fresh("removed")
	case *ast.GoExpr:
		t = c.inferGo(e)
	case *ast.SelectExpr:
		t = c.inferSelect(e)
	case *ast.UsingExpr:
		t = c.inferUsing(e)
	case *ast.RegionExpr:
		c.errorfAt(e.Loc, "`region { … }` is not supported in Goop 1.0; use `try`/`finally` or explicit cleanup, and annotate resource types if inference is ambiguous")
		t = c.fresh("removed")
	case *ast.CompExpr:
		c.errorfAt(e.Loc, "computation expressions (`%s { … }`) are not supported in Goop 1.0; rewrite with `match` on `result`, or `try`/`finally` for cleanup", e.Builder)
		t = c.fresh("removed")
	default:
		// Defensive: every known ast.Expr variant has a case above. Hitting
		// this means a new AST node was added without an infer path.
		c.errorfAt(locOf(e), "internal error: unhandled expression %T in type inference (compiler bug — please report); rewrite using a supported form or add an explicit type annotation on the enclosing binding", e)
		t = types.Unit
	}

	if c.types != nil && t != nil {
		c.types[e] = t
	}
	return t
}

func (c *Checker) inferLit(e *ast.LitExpr) types.Type {
	switch e.Kind {
	case token.INT:
		return types.Int
	case token.FLOAT:
		return types.Float
	case token.STRING:
		return types.String
	case token.TRUE, token.FALSE:
		return types.Bool
	case token.UNIT:
		return types.Unit
	default:
		return types.Unit
	}
}

func (c *Checker) inferIdent(e *ast.IdentExpr) types.Type {
	if mod, blocked := c.blockedNames[e.Name]; blocked {
		c.errorfAt(e.Loc, "cannot access private binding %q from module %s", e.Name, mod)
		return types.Unit
	}
	s := c.env.Lookup(e.Name)
	if s == nil {
		// External/unknown identifier — give it a fresh polymorphic type.
		// This allows the examples to reference external modules (Console,
		// File, Json, etc.) without explicit imports.
		return c.fresh(e.Name)
	}
	return s.Instantiate()
}

func (c *Checker) inferConstructor(e *ast.ConstructorExpr) types.Type {
	var s *types.Scheme
	if e.TypePrefix != "" {
		s = c.env.Lookup(e.TypePrefix + "." + e.Name)
		if s == nil && !c.adtHasConstructor(e.TypePrefix, e.Name) {
			c.errorfAt(e.Loc, "constructor %s.%s is not defined", e.TypePrefix, e.Name)
			return types.Unit
		}
	}
	if s == nil {
		s = c.env.Lookup(e.Name)
	}
	if s == nil {
		// Capital-letter names used as modules/variables are parsed as
		// constructors by the lexer but may be regular identifiers.
		// Fall back to identifier lookup.
		return c.inferIdent(&ast.IdentExpr{Name: e.Name})
	}
	inst := s.Instantiate()

	if e.Arg != nil {
		// Treat as application: ctor(arg)
		funcType := inst
		argType := c.infer(e.Arg)
		resultType := c.fresh("result")
		c.unify(&types.TFun{From: argType, To: resultType}, funcType)
		return resultType
	}
	return inst
}

func (c *Checker) inferApp(e *ast.AppExpr) types.Type {
	funcType := c.infer(e.Func)

	// Bidirectional inference: if the argument is a lambda and we can
	// resolve the function type to a concrete TFun, propagate the expected
	// parameter type into the lambda so the body can use it.
	var argType types.Type
	if fn, ok := e.Arg.(*ast.FunExpr); ok {
		resolvedFunc := types.Apply(c.sub, funcType)
		if tfun, ok := resolvedFunc.(*types.TFun); ok {
			expected := tfun.From
			// Only propagate if expected is concrete (not a TVar).
			if _, isTVar := expected.(*types.TVar); !isTVar {
				argType = c.inferFunExpected(fn, expected)
			} else {
				argType = c.infer(e.Arg)
			}
		} else {
			argType = c.infer(e.Arg)
		}
	} else {
		resolvedFunc := types.Apply(c.sub, funcType)
		if tfun, ok := resolvedFunc.(*types.TFun); ok {
			expected := types.Apply(c.sub, tfun.From)
			if _, isTVar := expected.(*types.TVar); !isTVar {
				argType = c.inferWithExpected(e.Arg, expected)
			} else {
				argType = c.infer(e.Arg)
			}
		} else {
			argType = c.infer(e.Arg)
		}
	}
	// A positional argument skips any outstanding optional labelled
	// parameters. Their defaults are supplied by the callee/codegen.
	if _, labelled := e.Arg.(*ast.LabelledArgExpr); !labelled {
		for {
			fn, ok := types.Apply(c.sub, funcType).(*types.TFun)
			if !ok || !fn.Optional {
				break
			}
			funcType = fn.To
		}
	}

	resultType := c.fresh("result")
	fnType := &types.TFun{From: argType, To: resultType}
	c.unifyAt(e.Loc, funcType, fnType)
	return resultType
}

// inferFunExpected infers a function expression with a known expected type.
// For each unannotated parameter, if the expected type is a TFun, the
// parameter is unified with the expected parameter type BEFORE inferring
// the body. This provides better type information in the body and enables
// inference of lambda parameter types from context.
func (c *Checker) inferFunExpected(e *ast.FunExpr, expected types.Type) types.Type {
	saved := c.env
	c.env = NewEnv(c.env)

	expectedParam := expected // peeled as we process params

	var paramTypes []types.Type
	for _, p := range e.Params {
		var pt types.Type
		if p.Type != nil {
			pt = c.convertASTType(p.Type)
		} else if expectedParam != nil {
			// Try to extract the expected type for this param.
			resolved := types.Apply(c.sub, expectedParam)
			if fn, ok := resolved.(*types.TFun); ok {
				pt = c.fresh(p.Name)
				c.unify(pt, fn.From)
				// Advance to the next expected param (the return type
				// becomes the expected type for the rest of the lambda).
				expectedParam = fn.To
			} else {
				// Expected type is not a function — fall back to fresh.
				pt = c.fresh(p.Name)
				expectedParam = nil
			}
		} else {
			pt = c.fresh(p.Name)
		}
		c.env.Bind(p.Name, types.Mono(pt))
		paramTypes = append(paramTypes, pt)
	}

	bodyType := c.infer(e.Body)
	c.env = saved

	result := bodyType
	for i := len(paramTypes) - 1; i >= 0; i-- {
		result = &types.TFun{From: paramTypes[i], To: result}
	}
	return result
}

func (c *Checker) inferIf(e *ast.IfExpr) types.Type {
	condType := c.infer(e.Cond)
	c.unifyAt(e.Loc, condType, types.Bool)
	thenType := c.infer(e.ThenBranch)
	elseType := c.infer(e.ElseBranch)
	c.unifyAt(e.Loc, thenType, elseType)
	return thenType
}

func (c *Checker) inferMatch(e *ast.MatchExpr) types.Type {
	scrutType := c.infer(e.Scrutinee)
	resultType := c.fresh("match_result")

	hasEffect := false
	for _, arm := range e.Arms {
		if arm.EffectHandler {
			hasEffect = true
			break
		}
	}
	if hasEffect {
		c.inferMatchEffectArms(e, scrutType, resultType)
		return resultType
	}

	for _, arm := range e.Arms {
		// Create a new scope for pattern variables
		saved := c.env
		c.env = NewEnv(c.env)
		c.checkPattern(e.Loc, arm.Pattern, scrutType)
		if arm.Guard != nil {
			guardType := c.infer(arm.Guard)
			c.unifyAt(e.Loc, guardType, types.Bool)
		}
		bodyType := c.infer(arm.Body)
		c.unifyAt(e.Loc, bodyType, resultType)
		c.env = saved
	}
	return resultType
}

func (c *Checker) inferLetIn(e *ast.LetInExpr) types.Type {
	if e.Mutable {
		c.errorfAt(e.Loc, "let mutable is removed; use 'ref' instead")
	}
	// Process as non-recursive let: check bindings, add to env, check body
	for _, b := range e.Bindings {
		t := types.Apply(c.sub, c.checkBinding(b))
		inScope := c.env.InScope()
		scheme := types.Generalize(t, inScope)
		c.env.Bind(b.Name, scheme)
	}
	return c.infer(e.Body)
}

func (c *Checker) inferIndex(e *ast.IndexExpr) types.Type {
	target := c.infer(e.Target)
	index := c.infer(e.Index)
	c.unifyAt(e.Loc, index, types.Int)
	if slice, ok := types.Apply(c.sub, target).(*types.TGoSlice); ok {
		return slice.Elem
	}
	elem := c.unpackArray(e.Loc, target)
	return elem
}

func (c *Checker) inferAssign(e *ast.AssignExpr) types.Type {
	if e.Coloneq {
		return c.inferColoneqAssign(e)
	}
	switch target := e.Target.(type) {
	case *ast.IndexExpr:
		arrType := c.infer(target.Target)
		indexType := c.infer(target.Index)
		c.unifyAt(e.Loc, indexType, types.Int)
		elemType := c.unpackArray(e.Loc, arrType)
		valueType := c.infer(e.Value)
		c.unifyAt(e.Loc, valueType, elemType)
	case *ast.FieldAccessExpr:
		_ = c.infer(target.Left)
		fieldType := c.inferFieldAccess(target)
		valueType := c.infer(e.Value)
		c.unifyAt(e.Loc, valueType, fieldType)
	case *ast.IdentExpr:
		if !c.mutableVars[target.Name] {
			c.errorfAt(e.Loc, "cannot assign to immutable binding %q; use := for refs or <- for arrays/mutable fields", target.Name)
		}
		bound := c.inferIdent(target)
		valueType := c.infer(e.Value)
		c.unifyAt(e.Loc, valueType, bound)
	default:
		c.errorfAt(e.Loc, "invalid assignment target")
	}
	return types.Unit
}

func (c *Checker) inferColoneqAssign(e *ast.AssignExpr) types.Type {
	valueType := c.infer(e.Value)
	switch target := e.Target.(type) {
	case *ast.DerefExpr:
		refType := c.infer(target.Target)
		elem := c.unpackRef(e.Loc, refType)
		c.unifyAt(e.Loc, valueType, elem)
	default:
		refType := c.infer(e.Target)
		elem := c.unpackRef(e.Loc, refType)
		c.unifyAt(e.Loc, valueType, elem)
	}
	return types.Unit
}

func (c *Checker) unpackRef(loc token.SourceLoc, t types.Type) types.Type {
	t = types.Apply(c.sub, t)
	if tc, ok := t.(*types.TCon); ok && tc.Name == "ref" && len(tc.Args) > 0 {
		return tc.Args[0]
	}
	elem := c.fresh("ref_elem")
	c.unifyAt(loc, t, types.RefType(elem))
	return elem
}

func (c *Checker) inferWhile(e *ast.WhileExpr) types.Type {
	cond := c.infer(e.Cond)
	c.unifyAt(e.Loc, cond, types.Bool)
	body := c.infer(e.Body)
	c.unifyAt(e.Loc, body, types.Unit)
	return types.Unit
}

func (c *Checker) inferFunction(e *ast.FunctionExpr) types.Type {
	argType := c.fresh("fn_arg")
	resultType := c.fresh("fn_result")
	for _, arm := range e.Arms {
		saved := c.env
		c.env = NewEnv(c.env)
		c.checkPattern(e.Loc, arm.Pattern, argType)
		if arm.Guard != nil {
			guardType := c.infer(arm.Guard)
			c.unifyAt(e.Loc, guardType, types.Bool)
		}
		bodyType := c.infer(arm.Body)
		c.unifyAt(e.Loc, bodyType, resultType)
		c.env = saved
	}
	return &types.TFun{From: argType, To: resultType}
}

func (c *Checker) inferRef(e *ast.RefExpr) types.Type {
	elem := c.infer(e.Value)
	return types.RefType(elem)
}

func (c *Checker) inferDeref(e *ast.DerefExpr) types.Type {
	refType := c.infer(e.Target)
	return c.unpackRef(e.Loc, refType)
}

func (c *Checker) inferTry(e *ast.TryExpr) types.Type {
	resultType := c.infer(e.Body)
	exn := &types.Prim{Name: "exn"}
	for _, arm := range e.Arms {
		saved := c.env
		c.env = NewEnv(c.env)
		c.checkPattern(e.Loc, arm.Pattern, exn)
		if arm.Guard != nil {
			guardType := c.infer(arm.Guard)
			c.unifyAt(e.Loc, guardType, types.Bool)
		}
		armType := c.infer(arm.Body)
		c.unifyAt(e.Loc, armType, resultType)
		c.env = saved
	}
	if e.Finally != nil {
		fin := c.infer(e.Finally)
		c.unifyAt(e.Loc, fin, types.Unit)
	}
	return resultType
}

func (c *Checker) inferRaise(e *ast.RaiseExpr) types.Type {
	exnType := c.infer(e.Exn)
	c.unifyAt(e.Loc, exnType, &types.Prim{Name: "exn"})
	return c.fresh("raise")
}

func (c *Checker) inferAssert(e *ast.AssertExpr) types.Type {
	cond := c.infer(e.Cond)
	c.unifyAt(e.Loc, cond, types.Bool)
	return types.Unit
}

func (c *Checker) inferLazy(e *ast.LazyExpr) types.Type {
	elem := c.infer(e.Value)
	return types.LazyType(elem)
}

func (c *Checker) inferPerform(e *ast.PerformExpr) types.Type {
	_ = c.infer(e.Op)
	return c.fresh("perform")
}

func (c *Checker) inferArrayLit(e *ast.ArrayLitExpr) types.Type {
	if len(e.Elems) == 0 {
		return types.ArrayType(c.fresh("'a"))
	}
	elemType := c.infer(e.Elems[0])
	for _, el := range e.Elems[1:] {
		c.unifyAt(e.Loc, elemType, c.infer(el))
	}
	return types.ArrayType(elemType)
}

func (c *Checker) inferPolyvar(e *ast.PolyvarExpr) types.Type {
	var argType types.Type
	if e.Arg != nil {
		argType = c.infer(e.Arg)
	}
	return &types.PolyVariant{Variants: []types.Variant{{Name: e.Tag, Arg: argType}}, Open: true}
}

func (c *Checker) inferFor(e *ast.ForExpr) types.Type {
	from := c.infer(e.From)
	to := c.infer(e.To)
	c.unifyAt(e.Loc, from, types.Int)
	c.unifyAt(e.Loc, to, types.Int)
	saved := c.env
	c.env = NewEnv(c.env)
	c.env.Bind(e.Var, types.Mono(types.Int))
	c.infer(e.Body)
	c.env = saved
	return types.Unit
}

func (c *Checker) inferBegin(e *ast.BeginExpr) types.Type {
	var result types.Type = types.Unit
	for _, stmt := range e.Stmts {
		result = c.infer(stmt)
	}
	return result
}

func (c *Checker) unpackArray(loc token.SourceLoc, t types.Type) types.Type {
	if tc, ok := t.(*types.TCon); ok && tc.Name == "array" && len(tc.Args) > 0 {
		return tc.Args[0]
	}
	elem := c.fresh("elem")
	c.unifyAt(loc, t, types.ArrayType(elem))
	return elem
}

func (c *Checker) adtHasConstructor(typeName, ctorName string) bool {
	s := c.env.Lookup(typeName)
	if s == nil {
		return false
	}
	t := types.Apply(c.sub, s.Instantiate())
	adt, ok := t.(*types.TAdt)
	if !ok {
		return false
	}
	for _, v := range adt.Variants {
		if v.Name == ctorName {
			return true
		}
	}
	return false
}

func (c *Checker) inferFun(e *ast.FunExpr) types.Type {
	saved := c.env
	c.env = NewEnv(c.env)

	var paramTypes []types.Type
	for _, p := range e.Params {
		var pt types.Type
		if p.Type != nil {
			pt = c.convertASTType(p.Type)
		} else {
			pt = c.fresh(p.Name)
		}
		c.env.Bind(p.Name, types.Mono(pt))
		paramTypes = append(paramTypes, pt)
	}

	bodyType := c.infer(e.Body)
	c.env = saved

	result := bodyType
	for i := len(paramTypes) - 1; i >= 0; i-- {
		result = &types.TFun{From: paramTypes[i], To: result, Label: e.Params[i].Label, Optional: e.Params[i].Optional}
	}
	return result
}

func (c *Checker) inferBinary(e *ast.BinaryExpr) types.Type {
	left := c.infer(e.Left)
	right := c.infer(e.Right)

	switch e.Op {
	case token.PLUS, token.MINUS, token.STAR, token.SLASH:
		// Arithmetic: both operands must be the same numeric type (int or float)
		c.unifyAt(e.Loc, left, right)
		return left

	case token.MOD, token.LAND, token.LOR, token.LXOR:
		// Integer bitwise / modulo ops
		c.unifyAt(e.Loc, left, types.Int)
		c.unifyAt(e.Loc, right, types.Int)
		return types.Int

	case token.PERCENT:
		c.errorfAt(e.Loc, "'%%' is not supported in Goop 1.0; use 'mod' for integer remainder (e.g. `a mod b`)")
		return types.Int

	case token.STARDOT, token.PLUSDOT, token.MINUSDOT, token.SLASHDOT:
		// Float arithmetic: *. +. -. /. force float
		c.unifyAt(e.Loc, left, types.Float)
		c.unifyAt(e.Loc, right, types.Float)
		return types.Float

	case token.EQUALS, token.EQEQ, token.NEQ, token.DIAMOND:
		// Comparison: both operands same type, result is bool
		// (= and == are equality; != and <> are inequality)
		c.unifyAt(e.Loc, left, right)
		return types.Bool

	case token.LT, token.GT, token.LEQ, token.GEQ:
		// Ordered comparison: both operands same type (int or float), result bool
		c.unifyAt(e.Loc, left, right)
		return types.Bool

	case token.CARET:
		// String concatenation: both strings, result string
		c.unifyAt(e.Loc, left, types.String)
		c.unifyAt(e.Loc, right, types.String)
		return types.String

	case token.AMPAMP, token.PIPEPIPE:
		// Logical: both bool, result bool
		c.unifyAt(e.Loc, left, types.Bool)
		c.unifyAt(e.Loc, right, types.Bool)
		return types.Bool

	case token.CONS:
		// x :: xs — x: A, xs: list<A>, result: list<A>
		c.unifyAt(e.Loc, right, types.ListType(left))
		return right

	case token.PIPEOP:
		// x |> f  ≡  f x  (parser emits BinaryExpr; PipeExpr remains for legacy AST)
		result := c.fresh("pipe")
		c.unifyAt(e.Loc, right, &types.TFun{From: left, To: result})
		return result

	default:
		c.errorfAt(e.Loc, "unsupported binary operator %s; Goop 1.0 supports + - * / mod land lor lxor +. -. *. /. = == != <> < > <= >= ^ && || :: |>; rewrite using those operators, or annotate the enclosing binding with an explicit type and use a library function", e.Op)
		return types.Unit
	}
}

func (c *Checker) inferPipe(e *ast.PipeExpr) types.Type {
	// x |> f  ≡  f x
	left := c.infer(e.Left)
	right := c.infer(e.Right)
	result := c.fresh("pipe")
	c.unifyAt(e.Loc, right, &types.TFun{From: left, To: result})
	return result
}

func (c *Checker) inferQuestion(e *ast.QuestionExpr) types.Type {
	leftType := c.infer(e.Left)
	// Left should be result<A, B>, result is A
	a := c.fresh("ok")
	b := c.fresh("err")
	c.unify(leftType, types.ResultType(a, b))
	if e.Arg != nil {
		// Optional error transformation argument
		_ = c.infer(e.Arg)
	}
	return a
}

func (c *Checker) inferRecord(e *ast.RecordExpr) types.Type {
	fields := make([]types.Field, len(e.Fields))
	for i, f := range e.Fields {
		var ft types.Type
		if f.Value != nil {
			ft = c.infer(f.Value)
		} else {
			// Punning: field name is also variable name
			ft = c.inferIdent(&ast.IdentExpr{Name: f.Name})
		}
		fields[i] = types.Field{Name: f.Name, Type: ft}
	}
	return &types.TRecord{Fields: fields}
}

// inferWithExpected infers e, using expected to guide ptr_of and Go struct literals.
func (c *Checker) inferWithExpected(e ast.Expr, expected types.Type) types.Type {
	expected = types.Apply(c.sub, expected)
	switch e := e.(type) {
	case *ast.PtrOfExpr:
		if p, ok := expected.(*types.TPtr); ok {
			inner := c.inferWithExpected(e.Inner, p.Elem)
			t := &types.TPtr{Elem: inner}
			c.types[e] = t
			return t
		}
	case *ast.RecordExpr:
		if named, ok := expected.(*types.TGoNamed); ok && !named.Interface {
			t := c.inferGoStructLiteral(e, named)
			c.types[e] = t
			return t
		}
	}
	t := c.infer(e)
	c.unify(t, expected)
	return types.Apply(c.sub, expected)
}

func (c *Checker) inferGoStructLiteral(e *ast.RecordExpr, named *types.TGoNamed) types.Type {
	schema := c.goStructs[named.Name]
	if schema == nil {
		c.errorfAt(e.Loc, "no field schema for Go struct %s (import type %s)", named.String(), named.Name)
		return named
	}
	used := make(map[string]bool)
	for _, f := range e.Fields {
		gf, ok := matchGoStructField(schema, f.Name)
		if !ok {
			c.errorfAt(e.Loc, "unknown field %q on Go struct %s", f.Name, named.Name)
			continue
		}
		if used[gf.GoName] {
			c.errorfAt(e.Loc, "duplicate field %q on Go struct %s", f.Name, named.Name)
			continue
		}
		used[gf.GoName] = true
		var vt types.Type
		if f.Value != nil {
			vt = c.infer(f.Value)
		} else {
			vt = c.inferIdent(&ast.IdentExpr{Name: f.Name})
		}
		c.unifyGoField(vt, gf.Typ, named.Name)
	}
	return named
}

func matchGoStructField(schema *goStructSchema, goopName string) (goStructField, bool) {
	var matches []goStructField
	for _, f := range schema.Fields {
		if strings.EqualFold(f.GoName, goopName) {
			matches = append(matches, f)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return goStructField{}, false
}

// unifyGoField unifies a literal field value with the Go struct field type,
// allowing same-package named types that Go considers assignable (Level → Leveler).
func (c *Checker) unifyGoField(val, field types.Type, structTypeName string) {
	val = types.Apply(c.sub, val)
	field = types.Apply(c.sub, field)
	if vn, ok := val.(*types.TGoNamed); ok {
		if fn, ok := field.(*types.TGoNamed); ok && fn.Interface {
			path := c.goImportPaths[structTypeName]
			if path == "" {
				path = c.goImportPaths[fn.Name]
			}
			if path != "" {
				if ok, err := gosig.Assignable(path, vn.Name, fn.Name); err == nil && ok {
					return
				}
			}
			// Same-package interface field: accept named non-interface values.
			if vn.Pkg == fn.Pkg || vn.Pkg == "" || fn.Pkg == "" {
				return
			}
		}
	}
	c.unify(val, field)
}

func (c *Checker) inferRecordUpdate(e *ast.RecordUpdateExpr) types.Type {
	baseType := c.infer(e.Base)
	// Verify that the updated fields exist and have compatible types.
	if rec, ok := baseType.(*types.TRecord); ok {
		for _, f := range e.Fields {
			valType := c.infer(f.Value)
			if ft := rec.Lookup(f.Name); ft != nil {
				c.unify(valType, ft)
			}
		}
	}
	return baseType
}

func (c *Checker) inferFieldAccess(e *ast.FieldAccessExpr) types.Type {
	// Check for prelude-qualified names like Chan.make, Console.print_line.
	// The codegen resolves these through the prelude, so the typechecker
	// must do the same to get correct types for polymorphic prelude calls.
	qualified := c.fieldAccessName(e)
	if qualified != "" {
		if s := c.env.Lookup(qualified); s != nil {
			return s.Instantiate()
		}
	}

	leftType := c.infer(e.Left)
	if fields := c.goFields[goNamedTypeName(types.Apply(c.sub, leftType))]; fields != nil {
		if fieldType := fields[e.Field]; fieldType != nil {
			return fieldType
		}
		c.errorfAt(e.Loc, "Go type has no imported field %s", e.Field)
	}
	// For field access, we only need the field to exist in the record.
	// We don't require the records to be identical.
	resultType := c.fresh(e.Field)

	// If the left side is already a known record, look up the field.
	if rec, ok := leftType.(*types.TRecord); ok {
		if ft := rec.Lookup(e.Field); ft != nil {
			c.unify(resultType, ft)
			return resultType
		}
	}

	// Otherwise, create a partial record constraint:
	// the left type must be a record containing at least this field.
	field := types.Field{Name: e.Field, Type: resultType}
	partialRec := &types.TRecord{Fields: []types.Field{field}}
	// We use a different approach: unify the field's type within the record
	// without requiring exact field-set match.  Since TRecord unification
	// requires exact match, we relax this by only checking field presence.
	_ = partialRec

	// For unknown record types, just return the fresh result type.
	// Full record type checking would require row types or similar.
	return resultType
}

func (c *Checker) inferMethodSend(e *ast.MethodSendExpr) types.Type {
	target := c.infer(e.Target)
	result := c.fresh(e.Method)
	if object, ok := types.Apply(c.sub, target).(*types.TRecord); ok {
		if method := object.Lookup(e.Method); method != nil {
			c.unify(result, method)
			return result
		}
		c.errorfAt(e.Loc, "object has no method %s", e.Method)
	}
	return result
}

// fieldAccessName returns the dotted name for a simple field-access
// expression like Console.print_line or Chan.make, or empty string.
func (c *Checker) fieldAccessName(e *ast.FieldAccessExpr) string {
	if ctor, ok := e.Left.(*ast.ConstructorExpr); ok && ctor.Arg == nil {
		return ctor.Name + "." + e.Field
	}
	if ident, ok := e.Left.(*ast.IdentExpr); ok {
		return ident.Name + "." + e.Field
	}
	return ""
}

func (c *Checker) inferTuple(e *ast.TupleExpr) types.Type {
	elems := make([]types.Type, len(e.Elems))
	for i, el := range e.Elems {
		elems[i] = c.infer(el)
	}
	return &types.TTuple{Elems: elems}
}

func (c *Checker) inferList(e *ast.ListExpr) types.Type {
	if len(e.Elems) == 0 {
		// [] has type 'a list
		return types.ListType(c.fresh("'a"))
	}
	elemType := c.infer(e.Elems[0])
	for _, el := range e.Elems[1:] {
		c.unify(elemType, c.infer(el))
	}
	return types.ListType(elemType)
}

func (c *Checker) inferGuard(e *ast.GuardExpr) types.Type {
	// guard pat1 = expr1 and pat2 = expr2 else expr
	// Desugars to nested match, so we check each binding's pattern
	// against its expression type, then check the else branch.
	for _, b := range e.Bindings {
		patType := c.infer(b.Expr)
		c.checkPattern(e.Loc, b.Pattern, patType)
	}
	elseType := c.infer(e.Else_)
	return elseType
}

func (c *Checker) inferIs(e *ast.IsExpr) types.Type {
	leftType := c.infer(e.Left)
	// Just check the pattern; result is bool
	c.checkPattern(e.Loc, e.Pattern, leftType)
	return types.Bool
}

func (c *Checker) inferAsMatch(e *ast.AsMatchExpr) types.Type {
	leftType := c.infer(e.Left)
	saved := c.env
	c.env = NewEnv(c.env)
	c.checkPattern(e.Loc, e.Pattern, leftType)
	bodyType := c.infer(e.Body)
	c.env = saved
	elseType := c.infer(e.ElseBody)
	c.unify(bodyType, elseType)
	return bodyType
}

func (c *Checker) inferGo(e *ast.GoExpr) types.Type {
	c.checkPerformInGo(e)
	seen := make(map[string]bool)
	for _, name := range e.Moved {
		if seen[name] {
			c.errorfAt(e.Loc, "duplicate name %q in go move list", name)
			continue
		}
		seen[name] = true
		if c.env.Lookup(name) == nil {
			c.errorfAt(e.Loc, "unknown variable %q in go move list", name)
		}
	}
	exprType := c.infer(e.Expr)
	expected := &types.TFun{From: types.Unit, To: types.Unit}
	c.unify(exprType, expected)
	return types.Unit
}

func (c *Checker) inferSelect(e *ast.SelectExpr) types.Type {
	var rType types.Type
	for i := range e.Cases {
		// Infer the channel receive expression
		chType := c.infer(e.Cases[i].Recv)
		// Bind the variable
		elemType := types.Fresh("elem")
		c.unify(chType, &types.TChan{Elem: elemType})
		c.env.Bind(e.Cases[i].Bind, types.Mono(elemType))
		bodyType := c.infer(e.Cases[i].Body)
		if rType == nil {
			rType = bodyType
		} else {
			c.unify(rType, bodyType)
		}
	}
	if e.Default != nil {
		dType := c.infer(e.Default)
		if rType == nil {
			rType = dType
		} else {
			c.unify(rType, dType)
		}
	}
	if rType == nil {
		rType = types.Unit
	}
	return rType
}

func (c *Checker) inferUsing(e *ast.UsingExpr) types.Type {
	exprType := c.infer(e.Expr)
	c.checkPattern(e.Loc, e.Pattern, exprType)
	return c.infer(e.Body)
}

func (c *Checker) inferRegion(e *ast.RegionExpr) types.Type {
	saved := c.env
	c.env = NewEnv(c.env)

	var resultType types.Type = types.Unit

	for _, op := range e.Ops {
		switch o := op.(type) {
		case *ast.LetBangOp:
			// let! pattern = expr: bind pattern to RHS type (like let)
			t := c.infer(o.Expr)
			c.checkPattern(e.Loc, o.Pattern, t)
		case *ast.LetOp:
			// let pattern = expr: bind pattern to RHS type
			t := c.infer(o.Expr)
			c.checkPattern(e.Loc, o.Pattern, t)
		case *ast.DoBangOp:
			// do! expr: expr should have unit type
			t := c.infer(o.Expr)
			c.unify(t, types.Unit)
		case *ast.ReturnOp:
			// return expr: determines the region's result type
			resultType = c.infer(o.Expr)
		case *ast.ReturnBangOp:
			// return! expr: passes through
			resultType = c.infer(o.Expr)
		case *ast.BodyOp:
			// body expression (used without explicit return)
			resultType = c.infer(o.Expr)
		}
	}

	c.env = saved
	return resultType
}

// ---------------------------------------------------------------------------
// Pattern checking
// ---------------------------------------------------------------------------

func (c *Checker) checkPattern(loc token.SourceLoc, p ast.Pattern, scrutType types.Type) {
	switch p := p.(type) {
	case *ast.WildcardPattern:
		// Nothing to check
	case *ast.IdentPattern:
		// Bind the variable to the scrutinee type
		c.env.Bind(p.Name, types.Mono(scrutType))
	case *ast.LitPattern:
		// Check that the literal type matches
		litType := c.inferLit(&ast.LitExpr{Value: p.Value, Kind: p.Kind})
		c.unify(scrutType, litType)
	case *ast.ConstructorPattern:
		// Check if this is an active pattern
		if ap := active.GlobalRegistry.Lookup(p.Name); ap != nil {
			// Active pattern: InputType -> option<OutputType>
			// Scrutinee must match InputType
			c.unifyAt(loc, scrutType, ap.InputType)
			// Inner pattern binds to OutputType
			if p.Arg != nil {
				c.checkPattern(loc, p.Arg, ap.OutputType)
			}
			return
		}

		// Find the constructor type and match
		if p.TypePrefix != "" && !c.adtHasConstructor(p.TypePrefix, p.Name) {
			c.errorfAt(loc, "constructor %s.%s is not defined", p.TypePrefix, p.Name)
			return
		}
		s := c.env.Lookup(p.Name)
		if s == nil {
			c.errorfAt(loc, "undefined constructor pattern: %s", p.Name)
			return
		}
		ctorType := s.Instantiate()
		// Constructor type is either TAdt (no arg) or TFun(Arg, TAdt)
		if p.Arg != nil {
			if fn, ok := ctorType.(*types.TFun); ok {
				c.unifyAt(loc, fn.To, scrutType)
				c.checkPattern(loc, p.Arg, fn.From)
			} else {
				c.errorfAt(loc, "constructor %s takes no argument", p.Name)
			}
		} else {
			c.unifyAt(loc, ctorType, scrutType)
		}
	case *ast.PolyvarPattern:
		payload := c.fresh("polyvar_payload")
		if p.Arg == nil {
			payload = nil
		}
		row := &types.PolyVariant{Variants: []types.Variant{{Name: p.Tag, Arg: payload}}, Open: true}
		c.unifyAt(loc, scrutType, row)
		if p.Arg != nil {
			c.checkPattern(loc, p.Arg, payload)
		}
	case *ast.OrPattern:
		c.checkOrPattern(loc, p, scrutType)
	case *ast.RecordPattern:
		// Scrutinee must be a record; each field pattern checked
		rt := c.unpackRecord(loc, scrutType)
		for _, f := range p.Fields {
			fieldType := rt.Lookup(f.Name)
			if fieldType == nil {
				c.errorfAt(loc, "record has no field %q", f.Name)
				continue
			}
			if f.Pattern != nil {
				c.checkPattern(loc, f.Pattern, fieldType)
			} else {
				// Punning: bind field name to field type
				c.env.Bind(f.Name, types.Mono(fieldType))
			}
		}
	case *ast.TuplePattern:
		// Must be a tuple type of same arity
		if tt, ok := scrutType.(*types.TTuple); ok {
			if len(p.Elems) != len(tt.Elems) {
				c.errorfAt(loc, "tuple pattern arity mismatch: %d vs %d", len(p.Elems), len(tt.Elems))
				return
			}
			for i, ep := range p.Elems {
				c.checkPattern(loc, ep, tt.Elems[i])
			}
		} else {
			c.errorfAt(loc, "expected tuple type for tuple pattern")
		}
	case *ast.ListPattern:
		// Must be a list type
		elemType := c.unpackList(loc, scrutType)
		for _, ep := range p.Elems {
			c.checkPattern(loc, ep, elemType)
		}
	case *ast.ConsPattern:
		// head :: tail — both must match the list element type
		elemType := c.unpackList(loc, scrutType)
		c.checkPattern(loc, p.Head, elemType)
		c.checkPattern(loc, p.Tail, types.ListType(elemType))
	case *ast.AliasPattern:
		c.checkPattern(loc, p.Pattern, scrutType)
		c.env.Bind(p.Name, types.Mono(scrutType))
	case *ast.ExceptionPattern:
		// exception P matches raised values of type exn
		c.unifyAt(loc, scrutType, &types.Prim{Name: "exn"})
		c.checkPattern(loc, p.Pattern, &types.Prim{Name: "exn"})
	case *ast.LazyPattern:
		elem := c.fresh("lazy")
		c.unifyAt(loc, scrutType, types.LazyType(elem))
		c.checkPattern(loc, p.Pattern, elem)
	}
}

// unpackList extracts the element type from a list type, creating a fresh
// variable if the type is not yet known.
func (c *Checker) unpackList(loc token.SourceLoc, t types.Type) types.Type {
	if tc, ok := t.(*types.TCon); ok && tc.Name == "list" && len(tc.Args) > 0 {
		return tc.Args[0]
	}
	// Create a fresh variable and force the type to be a list
	elem := c.fresh("elem")
	c.unifyAt(loc, t, types.ListType(elem))
	return elem
}

// unpackRecord extracts the record type, or creates a fresh one.
func (c *Checker) unpackRecord(loc token.SourceLoc, t types.Type) *types.TRecord {
	if rec, ok := t.(*types.TRecord); ok {
		return rec
	}
	// Create a fresh record and unify
	rec := &types.TRecord{}
	c.unifyAt(loc, t, rec)
	return rec
}

// ---------------------------------------------------------------------------
// Unification helper
// ---------------------------------------------------------------------------

func (c *Checker) unify(t1, t2 types.Type) {
	c.unifyAt(token.SourceLoc{}, t1, t2)
}

// unifyAt is like unify but attaches a source location to any error.
func (c *Checker) unifyAt(loc token.SourceLoc, t1, t2 types.Type) {
	// Apply current substitution first
	t1 = types.Apply(c.sub, t1)
	t2 = types.Apply(c.sub, t2)

	newSub, err := types.Unify(t1, t2)
	if err != nil {
		c.errorfAt(loc, "%v", err)
		return
	}
	// Compose the new substitution into the current one
	c.sub = types.Compose(newSub, c.sub)
}

// ---------------------------------------------------------------------------
// Extern type refinement via go/types (optional gosig fallback)
// ---------------------------------------------------------------------------

// coerceTupleErrorToResult rewrites a final return of (T, error) into
// result<T, error>. Curried arrows are preserved. This is the H6 default for
// import go bindings; import go raw skips it.
func coerceTupleErrorToResult(t types.Type) types.Type {
	switch t := t.(type) {
	case *types.TFun:
		return &types.TFun{
			From:     t.From,
			To:       coerceTupleErrorToResult(t.To),
			Effects:  t.Effects,
			Label:    t.Label,
			Optional: t.Optional,
		}
	case *types.TTuple:
		if len(t.Elems) == 2 {
			if _, ok := t.Elems[1].(*types.TError); ok {
				return types.ResultType(t.Elems[0], t.Elems[1])
			}
		}
		return t
	default:
		return t
	}
}

// declaredUsesGoSliceParam reports whether a curried extern type mentions a
// Goop go_slice parameter (including `...T` decls, which convert to go_slice).
func declaredUsesGoSliceParam(t types.Type) bool {
	for {
		fn, ok := t.(*types.TFun)
		if !ok {
			_, isSlice := t.(*types.TGoSlice)
			return isSlice
		}
		if _, ok := fn.From.(*types.TGoSlice); ok {
			return true
		}
		t = fn.To
	}
}

// refineExternType attempts to look up the real Go function signature for an
// extern binding and convert it to a more precise Goop type. If the lookup
// fails or the conversion produces an unsatisfactory type, it returns nil
// and the caller keeps the declared Goop type.
func (c *Checker) refineExternType(importPath, funcName string, declared types.Type) types.Type {
	if importPath == "" {
		return nil // same-package externs have no Go package to load
	}

	// Package-level vars/consts (Stdout, LevelInfo) keep their Goop-declared
	// FFI types; do not override with gosig (*File vs Writer, etc.).
	if _, isFun := declared.(*types.TFun); !isFun {
		return nil
	}

	// Variadic Goop decls (`...any` → any go_slice) must win over gosig:
	// go/types exposes fmt.Sprintf's tail as []interface{}, which we map to
	// list<'a>. Overwriting the declared go_slice breaks spread/go_slice_of_list.
	if declaredUsesGoSliceParam(declared) {
		return nil
	}

	sig, err := gosig.LookupFunc(importPath, funcName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goop: gosig fallback for %s.%s: %v\n", importPath, funcName, err)
		return nil
	}

	// Build a curried Goop function type from the Go parameters and results.
	// Multi-value Go results become a Goop tuple (F0, F1, …); codegen wraps
	// the call in a multi-value assignment into that struct.
	var resultType types.Type
	switch len(sig.Results) {
	case 0:
		resultType = types.Unit
	case 1:
		rt := goTypeToC0TypeInPkg(sig.Results[0].Type, importPath)
		if rt == nil {
			fmt.Fprintf(os.Stderr, "goop: gosig fallback for %s.%s: cannot map Go result type %q to Goop\n",
				importPath, funcName, sig.Results[0].Type)
			return nil
		}
		resultType = rt
	default:
		elems := make([]types.Type, len(sig.Results))
		for i, r := range sig.Results {
			rt := goTypeToC0TypeInPkg(r.Type, importPath)
			if rt == nil {
				fmt.Fprintf(os.Stderr, "goop: gosig fallback for %s.%s: cannot map Go result type %q to Goop\n",
					importPath, funcName, r.Type)
				return nil
			}
			elems[i] = rt
		}
		resultType = &types.TTuple{Elems: elems}
	}

	// Extract the declared return type (the rightmost leaf of the function
	// type) to preserve it if the Go sig is less specific.
	declaredResult := extractResultType(declared)
	if declaredResult != nil && resultType != nil {
		// Prefer the Goop-declared FFI type when it names the same Go type
		// (gosig may emit unqualified ".Time" vs declared "time.Time" / Level).
		if isMoreSpecific(declaredResult, resultType) || sameGoNamed(declaredResult, resultType) {
			resultType = declaredResult
		}
	}

	// Build curried param types → result.
	result := resultType
	for i := len(sig.Params) - 1; i >= 0; i-- {
		c0ParamType := goTypeToC0TypeInPkg(sig.Params[i].Type, importPath)
		if c0ParamType == nil {
			fmt.Fprintf(os.Stderr, "goop: gosig fallback for %s.%s: cannot map Go type %q to Goop\n",
				importPath, funcName, sig.Params[i].Type)
			return nil
		}
		result = &types.TFun{From: c0ParamType, To: result}
	}

	// Zero-arg Go funcs are declared in Goop as `unit -> T` (OCaml style).
	// Preserve that arrow when the declared type uses unit.
	if len(sig.Params) == 0 {
		if fn, ok := declared.(*types.TFun); ok {
			if p, ok := fn.From.(*types.Prim); ok && p.Name == "unit" {
				result = &types.TFun{From: types.Unit, To: result}
			}
		}
	}

	return result
}

// extractResultType walks a curried function type and returns the final
// result type (the rightmost non-function leaf).
func extractResultType(t types.Type) types.Type {
	for {
		fn, ok := t.(*types.TFun)
		if !ok {
			return t
		}
		t = fn.To
	}
}

// isMoreSpecific returns true if a is a more specific (concrete) type than b.
// A concrete type like Prim or TCon is more specific than a TVar.
func isMoreSpecific(a, b types.Type) bool {
	_, aTVar := a.(*types.TVar)
	_, bTVar := b.(*types.TVar)
	// If b is a TVar and a is concrete, a is more specific.
	if bTVar && !aTVar {
		return true
	}
	// If b is bytes ([]byte) and a is concrete, prefer a.
	if bp, ok := b.(*types.Prim); ok && bp.Name == "bytes" {
		if !aTVar {
			return true
		}
	}
	return false
}

// sameGoNamed reports whether a and b refer to the same Go named type
// (possibly with different/empty package qualifiers from gosig).
func sameGoNamed(a, b types.Type) bool {
	an, aok := a.(*types.TGoNamed)
	bn, bok := b.(*types.TGoNamed)
	if aok && bok {
		return an.Name == bn.Name
	}
	ap, aok := a.(*types.TPtr)
	bp, bok := b.(*types.TPtr)
	if aok && bok {
		return sameGoNamed(ap.Elem, bp.Elem)
	}
	return false
}

// goTypeToC0Type converts a Go type string (as returned by go/types) to a
// Goop internal type. Returns nil if the type cannot be mapped.
func goTypeToC0Type(goType string) types.Type {
	return goTypeToC0TypeInPkg(goType, "")
}

// goTypeToC0TypeInPkg is like goTypeToC0Type but fills in the package name for
// unqualified named types using importPath (e.g. "Time" from "time" → time.Time).
func goTypeToC0TypeInPkg(goType, importPath string) types.Type {
	goType = strings.TrimSpace(goType)

	// Pointer types: *T → T ptr (preserve pointer structure for FFI).
	if strings.HasPrefix(goType, "*") {
		elem := goTypeToC0TypeInPkg(strings.TrimPrefix(goType, "*"), importPath)
		if elem == nil {
			return nil
		}
		return &types.TPtr{Elem: elem}
	}

	// Primitive types
	switch goType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"uintptr":
		return types.Int
	case "float64":
		return types.Float
	case "float32":
		return types.Float
	case "bool":
		return types.Bool
	case "string":
		return types.String
	case "rune":
		return types.Rune
	case "byte":
		return types.Int // byte is uint8
	case "[]byte":
		return types.Bytes
	}

	// error type → Goop error (FFI)
	if goType == "error" {
		return &types.TError{}
	}

	// Slice type: []T → Goop list (historical mapping; go_slice is declared explicitly)
	if strings.HasPrefix(goType, "[]") {
		elem := goTypeToC0TypeInPkg(strings.TrimPrefix(goType, "[]"), importPath)
		if elem == nil {
			return nil
		}
		return types.ListType(elem)
	}

	// Channel type: chan T
	if strings.HasPrefix(goType, "chan ") {
		elem := goTypeToC0TypeInPkg(strings.TrimPrefix(goType, "chan "), importPath)
		if elem == nil {
			return nil
		}
		return &types.TChan{Elem: elem}
	}

	// Function type: func(A, B) C  →  A -> B -> C
	if strings.HasPrefix(goType, "func(") {
		return parseGoFuncType(goType)
	}

	// interface{} → fresh type variable (anything can be passed)
	if goType == "interface{}" || goType == "any" {
		return types.Fresh("'a")
	}

	// Named types: Time, time.Time, slog.Level, bytes.Buffer, etc.
	if named := goNamedFromString(goType); named != nil {
		if named.Pkg == "" && importPath != "" {
			named.Pkg = pkgFromPath(importPath)
		}
		return named
	}

	return nil
}

// goNamedFromString maps a Go named-type string to TGoNamed.
func goNamedFromString(s string) *types.TGoNamed {
	if s == "" || strings.ContainsAny(s, " [](){},") {
		return nil
	}
	if i := strings.LastIndex(s, "."); i >= 0 {
		pkg := s[:i]
		name := s[i+1:]
		if pkg == "" || name == "" {
			return nil
		}
		// Only accept simple pkg.Name (no nested path separators in the short name).
		if strings.Contains(pkg, "/") {
			pkg = pkgFromPath(pkg)
		}
		return &types.TGoNamed{Pkg: pkg, Name: name, Interface: true}
	}
	// Unqualified name from relativeQualifier (same package as import).
	if s[0] >= 'A' && s[0] <= 'Z' {
		return &types.TGoNamed{Pkg: "", Name: s, Interface: true}
	}
	return nil
}

// parseGoFuncType parses a Go func type string like "func(int, string) bool"
// and returns a curried Goop function type: int -> string -> bool.
func parseGoFuncType(s string) types.Type {
	// Expect: "func(...) result"
	s = strings.TrimPrefix(s, "func")
	s = strings.TrimSpace(s)

	// Find the opening paren
	if !strings.HasPrefix(s, "(") {
		return nil
	}

	// Find matching closing paren: track nesting depth
	depth := 0
	i := 0
	for i < len(s) {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				break
			}
		}
		i++
	}
	if i >= len(s) {
		return nil
	}

	paramsStr := s[1:i] // content between outer parens
	rest := strings.TrimSpace(s[i+1:])

	// Parse result type
	var resultType types.Type
	if rest != "" {
		resultType = goTypeToC0Type(rest)
	} else {
		// No return value → unit in Goop
		resultType = types.Unit
	}
	if resultType == nil {
		return nil
	}

	// Parse params: split by top-level commas
	paramTypes := splitGoParams(paramsStr)
	// Build curried: p0 -> p1 -> ... -> result
	for i := len(paramTypes) - 1; i >= 0; i-- {
		pt := goTypeToC0Type(paramTypes[i])
		if pt == nil {
			return nil
		}
		resultType = &types.TFun{From: pt, To: resultType}
	}

	return resultType
}

// splitGoParams splits a comma-separated list of Go type strings, respecting
// nested angle brackets and parentheses.
func splitGoParams(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var params []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				params = append(params, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	last := strings.TrimSpace(s[start:])
	if last != "" {
		params = append(params, last)
	}
	return params
}

func (c *Checker) markPrivate(name string) {
	c.privateNames[name] = true
}

func (c *Checker) checkPrivateName(name string) {
	if name == "" {
		return
	}
	r, _ := utf8.DecodeRuneInString(name)
	if unicode.IsUpper(r) {
		c.errorf("private binding %q must use mixedCaps (lower initial)", name)
	}
}

func collectPrivateNames(mod *ast.Module) map[string]bool {
	priv := make(map[string]bool)
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			if d.Private {
				for _, b := range d.Bindings {
					priv[b.Name] = true
				}
			}
		case *ast.TypeDecl:
			if d.Private {
				priv[d.Name] = true
			}
		}
	}
	return priv
}

func (c *Checker) bindImportSpecs(imports []ast.ImportSpec, deps map[string]*ast.Module, resolver *modresolve.Resolver) {
	seenPaths := make(map[string]bool)
	for _, spec := range imports {
		if spec.Path != "" && seenPaths[spec.Path] {
			c.errorf("duplicate import %q", spec.Path)
		}
		seenPaths[spec.Path] = true

		switch spec.Kind {
		case ast.ImportGo:
			if len(spec.Types) > 0 {
				c.bindExternTypes(spec.Path, spec.Types)
			}
			if len(spec.Vals) > 0 {
				c.bindExternValsOpts(spec.Path, spec.Vals, spec.Raw)
			}
		case ast.ImportGoop:
			if resolver == nil {
				c.errorf("cannot resolve c0 import %q without source file context", spec.Path)
				continue
			}
			resolved, err := resolver.ResolveGoopPath(spec.Path)
			if err != nil {
				c.errorf("%v", err)
				continue
			}
			dep := deps[resolved.GoImportPath]
			if dep == nil {
				c.errorf("module %q not found", spec.Path)
				continue
			}
			alias := modresolve.ImportAlias(spec, resolved)
			if alias == "." {
				c.bindModuleExports(dep, "", true, deps, resolver)
			} else {
				c.bindModuleExports(dep, alias, false, deps, resolver)
			}
		}
	}
}

func (c *Checker) bindDependencyExports(deps map[string]*ast.Module) {
	if len(deps) == 0 {
		return
	}
	for _, dep := range deps {
		c.bindModuleExports(dep, "", true, deps, nil)
	}
}

func (c *Checker) bindModuleExports(dep *ast.Module, prefix string, unqualified bool, deps map[string]*ast.Module, resolver *modresolve.Resolver) {
	priv := collectPrivateNames(dep)
	for name := range priv {
		c.blockedNames[name] = dep.Name
	}
	depChecker := &Checker{
		env:           NewEnv(nil),
		sub:           types.EmptySubst(),
		types:         make(typeinfo.TypeMap),
		privateNames:  priv,
		blockedNames:  make(map[string]string),
		goFields:      make(map[string]map[string]types.Type),
		goStructs:     make(map[string]*goStructSchema),
		goImportPaths: make(map[string]string),
	}
	depChecker.initBuiltins()
	if len(dep.Imports) > 0 && resolver != nil {
		depChecker.bindImportSpecs(dep.Imports, deps, resolver)
	}
	depChecker.checkModule(dep)
	for _, d := range dep.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			if d.Private {
				continue
			}
			for _, b := range d.Bindings {
				if s := depChecker.env.Lookup(b.Name); s != nil {
					if unqualified {
						if existing := c.env.Lookup(b.Name); existing != nil {
							c.errorf("import binds %q which conflicts with existing name", b.Name)
						} else {
							c.env.Bind(b.Name, s)
						}
					} else if prefix != "" {
						c.env.Bind(prefix+"."+b.Name, s)
					}
				}
			}
		case *ast.TypeDecl:
			if d.Private {
				continue
			}
			if s := depChecker.env.Lookup(d.Name); s != nil {
				if unqualified {
					c.env.Bind(d.Name, s)
				} else if prefix != "" {
					c.env.Bind(prefix+"."+d.Name, s)
				}
			}
		}
	}
}
