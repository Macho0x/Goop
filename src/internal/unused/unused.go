// Package unused warns about unused local bindings and imports.
package unused

import (
	"fmt"
	"path"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

const (
	CodeLocal  = "UNUSED001"
	CodeImport = "UNUSED002"
)

// Error is an unused-binding / unused-import diagnostic.
type Error struct {
	Code string
	Msg  string
	Loc  token.SourceLoc
}

func (e *Error) Error() string {
	prefix := e.Code + ": "
	if e.Loc.File != "" && e.Loc.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s%s", e.Loc.File, e.Loc.Line, e.Loc.Column, prefix, e.Msg)
	}
	return prefix + e.Msg
}

func (e *Error) GetLoc() token.SourceLoc { return e.Loc }

func isUnit(t types.Type) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(*types.Prim); ok {
		return p.Name == "unit"
	}
	return t == types.Unit
}

// CheckWithConfig reports unused locals and imports (default warn).
// Unused unit-typed value bindings are ignored (common sequencing: let u = assert …).
func CheckWithConfig(mod *ast.Module, tm typeinfo.TypeMap, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Check.Unused == config.SeverityOff {
		return nil, nil
	}
	c := &checker{cfg: cfg, tm: tm, used: map[string]bool{}, importUsed: map[string]bool{}}
	for _, d := range mod.Decls {
		ld, ok := d.(*ast.LetDecl)
		if !ok {
			continue
		}
		for i := range ld.Bindings {
			b := &ld.Bindings[i]
			loc := ast.ExprLoc(b.Body)
			for _, p := range b.Params {
				if p.Name != "_" && p.Name != "" && !strings.HasPrefix(p.Name, "_") {
					c.locals = append(c.locals, localBind{name: p.Name, loc: loc, isParam: true})
				}
				c.walkType(p.Type)
			}
			c.walkType(b.RetType)
			c.walkExpr(b.Body)
		}
	}
	for _, b := range c.locals {
		if b.name == "" || b.name == "_" || strings.HasPrefix(b.name, "_") {
			continue
		}
		if c.used[b.name] {
			continue
		}
		// Sequencing bindings of type unit: let u = close ch in …
		if !b.isParam && b.rhs != nil && c.tm != nil {
			if t, ok := c.tm[b.rhs]; ok && isUnit(t) {
				continue
			}
		}
		c.emit(CodeLocal, fmt.Sprintf("unused binding %q", b.name), b.loc)
	}
	for _, spec := range mod.Imports {
		if spec.Kind == ast.ImportGo {
			continue
		}
		qual := importQualifier(spec)
		if qual == "" || qual == "." {
			continue
		}
		if !c.importUsed[qual] {
			c.emit(CodeImport, fmt.Sprintf("unused import %q", spec.Path), token.SourceLoc{})
		}
	}
	return c.errors, c.warnings
}

type localBind struct {
	name    string
	loc     token.SourceLoc
	rhs     ast.Expr
	isParam bool
}

type checker struct {
	cfg        *config.Config
	tm         typeinfo.TypeMap
	used       map[string]bool
	importUsed map[string]bool
	locals     []localBind
	errors     []error
	warnings   []error
}

func (c *checker) emit(code, msg string, loc token.SourceLoc) {
	e := &Error{Code: code, Msg: msg, Loc: loc}
	if c.cfg.Check.Unused == config.SeverityError {
		c.errors = append(c.errors, e)
	} else {
		c.warnings = append(c.warnings, e)
	}
}

func importQualifier(spec ast.ImportSpec) string {
	if spec.Alias != "" {
		return spec.Alias
	}
	base := path.Base(spec.Path)
	if base == "." || base == "/" || base == "" {
		return ""
	}
	return base
}

func (c *checker) markIdent(name string) {
	if name == "" {
		return
	}
	c.used[name] = true
	if i := strings.IndexByte(name, '.'); i > 0 {
		c.importUsed[name[:i]] = true
	}
}

func (c *checker) walkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.IdentExpr:
		c.markIdent(e.Name)
	case *ast.ConstructorExpr:
		c.markIdent(e.Name)
		if e.TypePrefix != "" {
			c.markIdent(e.TypePrefix)
		}
		c.walkExpr(e.Arg)
	case *ast.LetInExpr:
		for i := range e.Bindings {
			b := &e.Bindings[i]
			loc := ast.ExprLoc(b.Body)
			for _, p := range b.Params {
				if p.Name != "_" && p.Name != "" && !strings.HasPrefix(p.Name, "_") {
					c.locals = append(c.locals, localBind{name: p.Name, loc: loc, isParam: true})
				}
				c.walkType(p.Type)
			}
			if b.Name != "_" && b.Name != "" && !strings.HasPrefix(b.Name, "_") {
				c.locals = append(c.locals, localBind{name: b.Name, loc: loc, rhs: b.Body, isParam: false})
			}
			c.walkType(b.RetType)
		}
		for i := range e.Bindings {
			c.walkExpr(e.Bindings[i].Body)
		}
		c.walkExpr(e.Body)
	case *ast.FunExpr:
		loc := ast.ExprLoc(e.Body)
		for _, p := range e.Params {
			if p.Name != "_" && p.Name != "" && !strings.HasPrefix(p.Name, "_") {
				c.locals = append(c.locals, localBind{name: p.Name, loc: loc, isParam: true})
			}
			c.walkType(p.Type)
		}
		c.walkExpr(e.Body)
	case *ast.AppExpr:
		c.walkExpr(e.Func)
		c.walkExpr(e.Arg)
	case *ast.IfExpr:
		c.walkExpr(e.Cond)
		c.walkExpr(e.ThenBranch)
		c.walkExpr(e.ElseBranch)
	case *ast.MatchExpr:
		c.walkExpr(e.Scrutinee)
		for _, arm := range e.Arms {
			c.walkPat(arm.Pattern)
			c.walkExpr(arm.Guard)
			c.walkExpr(arm.Body)
		}
	case *ast.BeginExpr:
		for _, s := range e.Stmts {
			c.walkExpr(s)
		}
	case *ast.BinaryExpr:
		c.walkExpr(e.Left)
		c.walkExpr(e.Right)
	case *ast.ForExpr:
		if e.Var != "_" && e.Var != "" {
			c.locals = append(c.locals, localBind{name: e.Var, loc: e.Loc})
		}
		c.walkExpr(e.From)
		c.walkExpr(e.To)
		c.walkExpr(e.Body)
	case *ast.WhileExpr:
		c.walkExpr(e.Cond)
		c.walkExpr(e.Body)
	case *ast.GoExpr:
		for _, m := range e.Moved {
			c.markIdent(m)
		}
		c.walkExpr(e.Expr)
	case *ast.TryExpr:
		c.walkExpr(e.Body)
		for _, arm := range e.Arms {
			c.walkPat(arm.Pattern)
			c.walkExpr(arm.Body)
		}
		c.walkExpr(e.Finally)
	case *ast.RecordExpr:
		for _, f := range e.Fields {
			c.walkExpr(f.Value)
		}
	case *ast.RecordUpdateExpr:
		c.walkExpr(e.Base)
		for _, f := range e.Fields {
			c.walkExpr(f.Value)
		}
	case *ast.TupleExpr:
		for _, el := range e.Elems {
			c.walkExpr(el)
		}
	case *ast.ListExpr:
		for _, el := range e.Elems {
			c.walkExpr(el)
		}
	case *ast.ArrayLitExpr:
		for _, el := range e.Elems {
			c.walkExpr(el)
		}
	case *ast.FieldAccessExpr:
		c.walkExpr(e.Left)
	case *ast.MethodSendExpr:
		c.walkExpr(e.Target)
	case *ast.IndexExpr:
		c.walkExpr(e.Target)
		c.walkExpr(e.Index)
	case *ast.AssignExpr:
		c.walkExpr(e.Target)
		c.walkExpr(e.Value)
	case *ast.ParenExpr:
		c.walkExpr(e.Inner)
	case *ast.PipeExpr:
		c.walkExpr(e.Left)
		c.walkExpr(e.Right)
	case *ast.RefExpr:
		c.walkExpr(e.Value)
	case *ast.DerefExpr:
		c.walkExpr(e.Target)
	case *ast.GuardExpr:
		for _, b := range e.Bindings {
			c.walkPat(b.Pattern)
			c.walkExpr(b.Expr)
		}
		c.walkExpr(e.Else_)
	case *ast.IsExpr:
		c.walkExpr(e.Left)
		c.walkPat(e.Pattern)
	case *ast.AsMatchExpr:
		c.walkExpr(e.Left)
		c.walkPat(e.Pattern)
		c.walkExpr(e.Body)
		c.walkExpr(e.ElseBody)
	case *ast.QuestionExpr:
		c.walkExpr(e.Left)
		c.walkExpr(e.Arg)
	case *ast.AssertExpr:
		c.walkExpr(e.Cond)
	case *ast.RaiseExpr:
		c.walkExpr(e.Exn)
	case *ast.LazyExpr:
		c.walkExpr(e.Value)
	case *ast.PerformExpr:
		c.walkExpr(e.Op)
	case *ast.PolyvarExpr:
		c.walkExpr(e.Arg)
	case *ast.ObjectExpr:
		for _, m := range e.Methods {
			c.walkExpr(m.Body)
		}
		for _, init := range e.Initializers {
			c.walkExpr(init)
		}
	case *ast.NewExpr:
		// class name only
	case *ast.LabelledArgExpr:
		c.walkExpr(e.Value)
	case *ast.FunctionExpr:
		for _, arm := range e.Arms {
			c.walkPat(arm.Pattern)
			c.walkExpr(arm.Guard)
			c.walkExpr(arm.Body)
		}
	case *ast.PtrOfExpr:
		c.walkExpr(e.Inner)
	case *ast.IsNullExpr:
		c.walkExpr(e.Inner)
	case *ast.SpreadExpr:
		c.walkExpr(e.Inner)
	case *ast.SelectExpr:
		for i := range e.Cases {
			c.walkExpr(e.Cases[i].Recv)
			c.walkExpr(e.Cases[i].Body)
		}
		c.walkExpr(e.Default)
	case *ast.UsingExpr:
		c.walkExpr(e.Expr)
		c.walkPat(e.Pattern)
		c.walkExpr(e.Body)
	case *ast.CompExpr:
		for _, op := range e.Ops {
			c.walkCompOp(op)
		}
	case *ast.RegionExpr:
		for _, op := range e.Ops {
			c.walkCompOp(op)
		}
	}
}

func (c *checker) walkCompOp(op ast.CompOp) {
	switch o := op.(type) {
	case *ast.LetBangOp:
		c.walkExpr(o.Expr)
	case *ast.DoBangOp:
		c.walkExpr(o.Expr)
	case *ast.LetOp:
		c.walkExpr(o.Expr)
	case *ast.ReturnOp:
		c.walkExpr(o.Expr)
	case *ast.ReturnBangOp:
		c.walkExpr(o.Expr)
	case *ast.BodyOp:
		c.walkExpr(o.Expr)
	}
}

func (c *checker) walkType(t ast.Type) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *ast.TIdent:
		c.markIdent(t.Name)
	case *ast.TApp:
		c.walkType(t.Func)
		c.walkType(t.Arg)
	case *ast.TFun:
		c.walkType(t.From)
		c.walkType(t.To)
	case *ast.TTuple:
		for _, e := range t.Elems {
			c.walkType(e)
		}
	case *ast.TRecord:
		for _, f := range t.Fields {
			c.walkType(f.Type)
		}
	case *ast.TObject:
		for _, m := range t.Methods {
			c.walkType(m.Type)
		}
	case *ast.TPtr:
		c.walkType(t.Elem)
	case *ast.TGoSlice:
		c.walkType(t.Elem)
	case *ast.TVariadic:
		c.walkType(t.Elem)
	case *ast.TChan:
		c.walkType(t.Elem)
	case *ast.TMap:
		c.walkType(t.Key)
		c.walkType(t.Val)
	case *ast.RefinementType:
		c.walkType(t.Inner)
	case *ast.TPolyVariant:
		for _, cas := range t.Cases {
			c.walkType(cas.Arg)
		}
	}
}

func (c *checker) walkPat(p ast.Pattern) {
	if p == nil {
		return
	}
	switch p := p.(type) {
	case *ast.ConstructorPattern:
		c.markIdent(p.Name)
		if p.TypePrefix != "" {
			c.markIdent(p.TypePrefix)
		}
		c.walkPat(p.Arg)
	case *ast.PolyvarPattern:
		c.walkPat(p.Arg)
	case *ast.TuplePattern:
		for _, el := range p.Elems {
			c.walkPat(el)
		}
	case *ast.RecordPattern:
		for _, f := range p.Fields {
			c.walkPat(f.Pattern)
		}
	case *ast.ListPattern:
		for _, el := range p.Elems {
			c.walkPat(el)
		}
	case *ast.ConsPattern:
		c.walkPat(p.Head)
		c.walkPat(p.Tail)
	case *ast.OrPattern:
		c.walkPat(p.Left)
		c.walkPat(p.Right)
	case *ast.AliasPattern:
		c.walkPat(p.Pattern)
	}
}
