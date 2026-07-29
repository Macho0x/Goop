package typecheck

import (
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/types"
)

// moduleExports tracks bindings introduced by nested modules for open/include.
type moduleExports struct {
	vals map[string]*types.Scheme
}

type moduleSignature struct {
	vals map[string]ast.Type
}

type functorDef struct {
	arg   string
	sig   string
	decls []ast.TopDecl
}

type classInfo struct {
	members []types.Field
	virtual map[string]types.Type
}

func (c *Checker) ensureModules() {
	if c.modules == nil {
		c.modules = make(map[string]*moduleExports)
	}
	if c.moduleTypes == nil {
		c.moduleTypes = make(map[string]*moduleSignature)
	}
	if c.functors == nil {
		c.functors = make(map[string]*functorDef)
	}
	if c.classes == nil {
		c.classes = make(map[string]*classInfo)
	}
}

// checkTopDeclExtra handles Phase 3–5 declarations.
func (c *Checker) checkTopDeclExtra(d ast.TopDecl) {
	switch d := d.(type) {
	case *ast.NestedModuleDecl:
		c.checkNestedModule(d)
	case *ast.ModuleTypeDecl:
		c.checkModuleType(d)
	case *ast.OpenModuleDecl:
		c.checkOpen(d.Path)
	case *ast.IncludeDecl:
		c.checkInclude(d.Path, nil)
	case *ast.ClassDecl:
		c.checkClassDecl(d)
	case *ast.EffectDecl:
		// Register effect name as a constructor-like value for perform.
		from := c.convertASTType(d.From)
		to := c.convertASTType(d.To)
		c.env.Bind(d.Name, types.Mono(&types.TFun{From: from, To: to}))
	}
}

func (c *Checker) checkNestedModule(d *ast.NestedModuleDecl) {
	c.ensureModules()
	if d.Rec {
		// Register recursive peers before their bodies, then check each body.
		for _, peer := range append([]*ast.NestedModuleDecl{d}, d.RecDecls...) {
			c.modules[peer.Name] = &moduleExports{vals: make(map[string]*types.Scheme)}
			c.env.Bind(peer.Name, types.Mono(types.Unit))
		}
		d.Rec = false
		c.checkNestedModule(d)
		for _, peer := range d.RecDecls {
			peer.Rec = false
			c.checkNestedModule(peer)
		}
		return
	}
	if d.FunctorArg != "" && !d.IsApp {
		c.functors[d.Name] = &functorDef{arg: d.FunctorArg, sig: d.FunctorSig, decls: d.Decls}
		// Validate the functor body in an environment containing its argument.
		arg := c.signatureExports(d.FunctorSig)
		c.checkModuleBody(d.Decls, d.FunctorArg, arg)
		c.modules[d.Name] = &moduleExports{vals: make(map[string]*types.Scheme)}
		c.env.Bind(d.Name, types.Mono(types.Unit))
		return
	}
	saved := c.env
	var ex *moduleExports

	if d.IsApp {
		if f := c.functors[d.AppFunc]; f != nil {
			arg := c.moduleFor(d.AppArg)
			c.checkSignatureExports(d.AppArg, arg, f.sig)
			ex = c.checkModuleBody(f.decls, f.arg, arg)
		} else {
			// Module alias: copy exports from AppArg.
			ex = c.moduleFor(d.AppArg)
			if ex == nil {
				ex = &moduleExports{vals: make(map[string]*types.Scheme)}
			}
		}
	} else {
		ex = c.checkModuleBody(d.Decls, "", nil)
	}

	if d.SealSig != "" {
		c.checkSignatureExports(d.Name, ex, d.SealSig)
		ex = c.sealExports(ex, d.SealSig)
	}
	c.env = saved
	c.modules[d.Name] = ex
	// Bind module name as a unit placeholder so references don't fail hard.
	c.env.Bind(d.Name, types.Mono(types.Unit))
}

func (c *Checker) checkModuleBody(decls []ast.TopDecl, argName string, arg *moduleExports) *moduleExports {
	saved := c.env
	c.env = NewEnv(c.env)
	ex := &moduleExports{vals: make(map[string]*types.Scheme)}
	if argName != "" && arg != nil {
		c.modules[argName] = arg
		for name, scheme := range arg.vals {
			c.env.Bind(argName+"."+name, scheme)
		}
	}
	for _, decl := range decls {
		if td, ok := decl.(*ast.TypeDecl); ok {
			c.env.Bind(td.Name, c.convertTypeDecl(td))
		}
	}
	for _, decl := range decls {
		switch decl := decl.(type) {
		case *ast.LetDecl:
			c.checkLetDecl(decl)
			for _, b := range decl.Bindings {
				if s := c.env.Lookup(b.Name); s != nil {
					ex.vals[b.Name] = s
				}
			}
		case *ast.TypeDecl:
		case *ast.ExceptionDecl:
			c.checkExceptionDecl(decl)
		case *ast.NestedModuleDecl, *ast.ModuleTypeDecl, *ast.OpenModuleDecl, *ast.ClassDecl, *ast.EffectDecl:
			c.checkTopDeclExtra(decl)
		case *ast.IncludeDecl:
			c.checkInclude(decl.Path, ex)
		}
	}
	c.env = saved
	return ex
}

func (c *Checker) checkModuleType(d *ast.ModuleTypeDecl) {
	c.ensureModules()
	sig := &moduleSignature{vals: make(map[string]ast.Type)}
	if d.OfModule != "" {
		if ex := c.moduleFor(d.OfModule); ex != nil {
			for name, scheme := range ex.vals {
				sig.vals[name] = nil
				_ = scheme
			}
		}
	} else {
		for _, item := range d.Items {
			if item.Kind == "val" {
				sig.vals[item.Name] = item.Type
			}
		}
	}
	c.moduleTypes[d.Name] = sig
}

func (c *Checker) signatureExports(name string) *moduleExports {
	sig := c.moduleTypes[name]
	if sig == nil {
		return nil
	}
	ex := &moduleExports{vals: make(map[string]*types.Scheme)}
	for n, typ := range sig.vals {
		ex.vals[n] = types.Mono(c.convertASTType(typ))
	}
	return ex
}

func (c *Checker) sealExports(ex *moduleExports, sigName string) *moduleExports {
	sig := c.moduleTypes[sigName]
	if sig == nil {
		return ex
	}
	sealed := &moduleExports{vals: make(map[string]*types.Scheme)}
	for name, typ := range sig.vals {
		if actual := ex.vals[name]; actual != nil {
			sealed.vals[name] = types.Mono(c.convertASTType(typ))
		}
	}
	return sealed
}

func (c *Checker) checkSignatureExports(module string, ex *moduleExports, sigName string) {
	sig := c.moduleTypes[sigName]
	if sig == nil {
		c.errorf("unknown module type %s", sigName)
		return
	}
	if ex == nil {
		c.errorf("unknown module %s", module)
		return
	}
	for name, typ := range sig.vals {
		actual := ex.vals[name]
		if actual == nil {
			c.errorf("module %s does not provide val %s required by %s", module, name, sigName)
			continue
		}
		c.unify(actual.Instantiate(), c.convertASTType(typ))
	}
}

func (c *Checker) moduleFor(path string) *moduleExports {
	if m, ok := c.modules[path]; ok {
		return m
	}
	if i := strings.LastIndex(path, "."); i >= 0 {
		return c.modules[path[i+1:]]
	}
	return nil
}

func (c *Checker) checkInclude(path string, exports *moduleExports) {
	m := c.moduleFor(path)
	if m == nil {
		return
	}
	for name, scheme := range m.vals {
		c.env.Bind(name, scheme)
		if exports != nil {
			exports.vals[name] = scheme
		}
	}
}

func (c *Checker) checkOpen(path string) {
	if m := c.moduleFor(path); m != nil {
		for k, v := range m.vals {
			c.env.Bind(k, v)
		}
	}
}

func (c *Checker) checkClassDecl(d *ast.ClassDecl) {
	c.ensureModules()
	fields := make([]types.Field, 0, len(d.Fields)+len(d.Methods))
	required := make(map[string]types.Type)
	for _, parent := range d.Inherits {
		info := c.classes[parent]
		if info == nil {
			c.errorf("cannot inherit unknown class %s", parent)
			continue
		}
		fields = append(fields, info.members...)
		for name, typ := range info.virtual {
			required[name] = typ
		}
	}
	for _, f := range d.Fields {
		var ft types.Type = types.Unit
		if f.Value != nil {
			ft = c.infer(f.Value)
		}
		fields = append(fields, types.Field{Name: f.Name, Type: ft})
	}
	saved := c.env
	c.env = NewEnv(c.env)
	if d.Self != "" {
		c.env.Bind(d.Self, types.Mono(&types.TRecord{Fields: fields, Open: true}))
	}
	for _, m := range d.Methods {
		var mt types.Type
		if m.Virtual {
			if m.Type == nil {
				c.errorf("virtual method %s requires a type", m.Name)
				mt = c.fresh(m.Name)
			} else {
				mt = c.convertASTType(m.Type)
			}
			required[m.Name] = mt
		} else if d.TypeOnly {
			if m.Type == nil {
				c.errorf("class type method %s requires a type", m.Name)
				mt = c.fresh(m.Name)
			} else {
				mt = c.convertASTType(m.Type)
			}
		} else {
			mt = c.infer(&ast.FunExpr{Params: m.Params, Body: m.Body})
			if m.Type != nil {
				c.unify(mt, c.convertASTType(m.Type))
			}
			if wanted := required[m.Name]; wanted != nil {
				c.unify(mt, wanted)
				delete(required, m.Name)
			}
		}
		fields = replaceObjectMember(fields, types.Field{Name: m.Name, Type: mt})
	}
	c.env = saved
	for _, constraint := range d.Constraints {
		c.unify(c.convertASTType(constraint.Left), c.convertASTType(constraint.Right))
	}
	rec := &types.TRecord{Fields: fields}
	c.classes[d.Name] = &classInfo{members: fields, virtual: required}
	c.env.Bind(d.Name, types.Mono(&types.TCon{Name: "class", Args: []types.Type{rec}}))
	if !d.TypeOnly && len(required) == 0 {
		c.env.Bind("new_"+d.Name, types.Mono(&types.TFun{From: types.Unit, To: rec}))
	} else if !d.TypeOnly && !d.Virtual {
		for name := range required {
			c.errorf("class %s must implement virtual method %s", d.Name, name)
		}
	}
}

func replaceObjectMember(fields []types.Field, member types.Field) []types.Field {
	for i := range fields {
		if fields[i].Name == member.Name {
			fields[i] = member
			return fields
		}
	}
	return append(fields, member)
}

func (c *Checker) convertGADT(td *ast.TypeDecl, k *ast.GADTTypeKind) *types.Scheme {
	variants := make([]types.Variant, len(k.Cases))
	params := make([]types.Type, len(td.TypeParams))
	for i, name := range td.TypeParams {
		params[i] = c.fresh(name)
	}
	adt := &types.TAdt{Name: td.Name, Params: params, Variants: variants, Linear: td.Quantity == 1}
	for i, cs := range k.Cases {
		v := types.Variant{Name: cs.Name}
		if cs.Arg != nil {
			v.Arg = c.convertASTType(cs.Arg)
		}
		variants[i] = v
		result := c.gadtResultType(td, cs.Result, adt)
		var ctorType types.Type = result
		if cs.Arg != nil {
			ctorType = &types.TFun{From: c.convertASTType(cs.Arg), To: result}
		}
		c.env.Bind(cs.Name, types.Mono(ctorType))
	}
	adt.Variants = variants
	return types.Mono(adt)
}

// gadtResultType preserves the constructor's indexed result instead of
// collapsing every constructor to the declaration's unrefined ADT.
func (c *Checker) gadtResultType(td *ast.TypeDecl, result ast.Type, fallback *types.TAdt) types.Type {
	app, ok := result.(*ast.TApp)
	if !ok {
		return fallback
	}
	name, ok := app.Func.(*ast.TIdent)
	if !ok || name.Name != td.Name {
		return fallback
	}
	var params []types.Type
	if tuple, ok := app.Arg.(*ast.TTuple); ok {
		for _, elem := range tuple.Elems {
			params = append(params, c.convertASTType(elem))
		}
	} else {
		params = append(params, c.convertASTType(app.Arg))
	}
	return &types.TAdt{Name: td.Name, Params: params, Variants: fallback.Variants, Linear: fallback.Linear}
}

func (c *Checker) inferObject(e *ast.ObjectExpr) types.Type {
	fields := make([]types.Field, 0, len(e.Fields)+len(e.Methods))
	for _, parent := range e.Inherits {
		if info := c.classes[parent]; info != nil {
			fields = append(fields, info.members...)
		} else {
			c.errorfAt(e.Loc, "cannot inherit unknown class %s", parent)
		}
	}
	for _, f := range e.Fields {
		var ft types.Type = types.Unit
		if f.Value != nil {
			ft = c.infer(f.Value)
		}
		fields = append(fields, types.Field{Name: f.Name, Type: ft})
	}
	saved := c.env
	c.env = NewEnv(c.env)
	if e.Self != "" {
		c.env.Bind(e.Self, types.Mono(&types.TRecord{Fields: fields}))
	}
	for _, m := range e.Methods {
		if m.Virtual {
			continue
		}
		mt := c.infer(&ast.FunExpr{Params: m.Params, Body: m.Body})
		if m.Type != nil {
			c.unify(mt, c.convertASTType(m.Type))
		}
		fields = replaceObjectMember(fields, types.Field{Name: m.Name, Type: mt})
	}
	c.env = saved
	for _, constraint := range e.Constraints {
		c.unify(c.convertASTType(constraint.Left), c.convertASTType(constraint.Right))
	}
	return &types.TRecord{Fields: fields}
}

func (c *Checker) inferNew(e *ast.NewExpr) types.Type {
	s := c.env.Lookup(e.Class)
	if s == nil {
		c.errorfAt(e.Loc, "undefined class %s", e.Class)
		return c.fresh("new")
	}
	t := s.Instantiate()
	if tc, ok := t.(*types.TCon); ok && tc.Name == "class" && len(tc.Args) > 0 {
		return tc.Args[0]
	}
	return t
}

func (c *Checker) inferLetModule(e *ast.LetModuleExpr) types.Type {
	tmp := &ast.NestedModuleDecl{Name: e.Name, Decls: e.Decls}
	c.checkNestedModule(tmp)
	c.checkOpen(e.Name)
	return c.infer(e.Body)
}

func (c *Checker) inferLetOpen(e *ast.LetOpenExpr) types.Type {
	saved := c.env
	c.env = NewEnv(c.env)
	c.checkOpen(e.Path)
	t := c.infer(e.Body)
	c.env = saved
	return t
}

func (c *Checker) inferLocalOpen(e *ast.LocalOpenExpr) types.Type {
	saved := c.env
	c.env = NewEnv(c.env)
	c.checkOpen(e.Path)
	t := c.infer(e.Body)
	c.env = saved
	return t
}

func (c *Checker) inferContinue(e *ast.ContinueExpr) types.Type {
	_ = c.infer(e.Cont)
	return c.infer(e.Arg)
}

func (c *Checker) inferDiscontinue(e *ast.DiscontinueExpr) types.Type {
	_ = c.infer(e.Cont)
	c.unifyAt(e.Loc, c.infer(e.Exn), &types.Prim{Name: "exn"})
	return c.fresh("discontinue")
}

func (c *Checker) inferPackModule(e *ast.PackModuleExpr) types.Type {
	ex := c.moduleFor(e.Module)
	if ex == nil {
		c.errorfAt(e.Loc, "unknown module %s", e.Module)
	} else if e.Sig != "" {
		c.checkSignatureExports(e.Module, ex, e.Sig)
	}
	// The Go backend represents a packed module as an opaque unit value.
	return types.Unit
}

func (c *Checker) inferUnpackModule(e *ast.UnpackModuleExpr) types.Type {
	c.unifyAt(e.Loc, c.infer(e.Value), types.Unit)
	if e.Sig != "" && c.moduleTypes[e.Sig] == nil {
		c.errorfAt(e.Loc, "unknown module type %s", e.Sig)
	}
	return types.Unit
}

func (c *Checker) inferLabelledArg(e *ast.LabelledArgExpr) types.Type {
	if e.Value != nil {
		return c.infer(e.Value)
	}
	s := c.env.Lookup(e.Label)
	if s == nil {
		c.errorfAt(e.Loc, "unbound labelled argument ~%s", e.Label)
		return c.fresh("label")
	}
	return s.Instantiate()
}

func (c *Checker) checkPerformInGo(e *ast.GoExpr) {
	var walk func(ast.Expr)
	walk = func(ex ast.Expr) {
		if ex == nil {
			return
		}
		switch ex := ex.(type) {
		case *ast.PerformExpr:
			c.errorfAt(ex.Loc, "perform is not allowed inside go body")
		case *ast.GoExpr:
			walk(ex.Expr)
		case *ast.AppExpr:
			walk(ex.Func)
			walk(ex.Arg)
		case *ast.IfExpr:
			walk(ex.Cond)
			walk(ex.ThenBranch)
			walk(ex.ElseBranch)
		case *ast.MatchExpr:
			walk(ex.Scrutinee)
			for _, a := range ex.Arms {
				walk(a.Body)
				walk(a.Guard)
			}
		case *ast.LetInExpr:
			for _, b := range ex.Bindings {
				walk(b.Body)
			}
			walk(ex.Body)
		case *ast.BeginExpr:
			for _, s := range ex.Stmts {
				walk(s)
			}
		case *ast.FunExpr:
			walk(ex.Body)
		case *ast.ParenExpr:
			walk(ex.Inner)
		case *ast.BinaryExpr:
			walk(ex.Left)
			walk(ex.Right)
		}
	}
	walk(e.Expr)
}


func (c *Checker) inferMatchEffectArms(e *ast.MatchExpr, scrutType, resultType types.Type) {
	for _, arm := range e.Arms {
		saved := c.env
		c.env = NewEnv(c.env)
		if arm.EffectHandler {
			// Pattern is the effect operation; cont is a function.
			c.checkPattern(e.Loc, arm.Pattern, scrutType)
			if arm.ContName != "" {
				a := c.fresh("'a")
				r := c.fresh("'r")
				c.env.Bind(arm.ContName, types.Mono(&types.TFun{From: a, To: r}))
			}
		} else {
			c.checkPattern(e.Loc, arm.Pattern, scrutType)
		}
		if arm.Guard != nil {
			c.unifyAt(e.Loc, c.infer(arm.Guard), types.Bool)
		}
		c.unifyAt(e.Loc, c.infer(arm.Body), resultType)
		c.env = saved
	}
}

// checkOrPattern binds variables that appear in all alternatives (approx: bind from left).
func (c *Checker) checkOrPattern(loc token.SourceLoc, p *ast.OrPattern, scrutType types.Type) {
	c.checkPattern(loc, p.Left, scrutType)
	// Right side checked in a throwaway env so we don't double-bind.
	saved := c.env
	c.env = NewEnv(saved)
	c.checkPattern(loc, p.Right, scrutType)
	c.env = saved
}
