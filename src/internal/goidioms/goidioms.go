// Package goidioms warns about Go-stdlib usage that compiles but is the
// wrong idiom (float equality, regexp-in-loop, WaitGroup.Add inside go, …).
package goidioms

import (
	"fmt"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

const (
	CodeFloat  = "FLOAT001"
	CodeStr    = "STR001"
	CodeRegexp = "REGEXP001"
	CodeWG     = "WG001"
	CodeURL    = "URL001"
	CodeExit   = "EXIT001"
	CodeCtx    = "CTX001"
)

// Error is a Go-idiom diagnostic.
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

// CheckWithConfig reports Go-idiom issues (default warn).
func CheckWithConfig(mod *ast.Module, tm typeinfo.TypeMap, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Check.GoIdioms == config.SeverityOff {
		return nil, nil
	}
	c := &checker{tm: tm, cfg: cfg}
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			for i := range d.Bindings {
				c.walk(d.Bindings[i].Body)
			}
		case *ast.ImplementsDecl:
			for i := range d.Methods {
				c.walk(d.Methods[i].Body)
			}
		}
	}
	return c.errors, c.warnings
}

type checker struct {
	tm         typeinfo.TypeMap
	cfg        *config.Config
	inLoop     int
	inGo       int
	inFinally  int
	errors     []error
	warnings   []error
}

func (c *checker) emit(code, msg string, loc token.SourceLoc) {
	if c.cfg.Check.GoIdioms == config.SeverityOff {
		return
	}
	e := &Error{Code: code, Msg: msg, Loc: loc}
	if c.cfg.Check.GoIdioms == config.SeverityError {
		c.errors = append(c.errors, e)
	} else {
		c.warnings = append(c.warnings, e)
	}
}

func (c *checker) walk(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.BinaryExpr:
		c.checkBinary(e)
		c.walk(e.Left)
		c.walk(e.Right)
	case *ast.AppExpr:
		c.checkApp(e)
		c.walk(e.Func)
		c.walk(e.Arg)
	case *ast.BeginExpr:
		for i, s := range e.Stmts {
			if i < len(e.Stmts)-1 {
				c.checkDiscarded(s)
			}
			c.walk(s)
		}
	case *ast.LetInExpr:
		for i := range e.Bindings {
			c.checkCtxBinding(&e.Bindings[i])
			c.walk(e.Bindings[i].Body)
		}
		c.walk(e.Body)
	case *ast.FunExpr:
		c.walk(e.Body)
	case *ast.IfExpr:
		c.walk(e.Cond)
		c.walk(e.ThenBranch)
		c.walk(e.ElseBranch)
	case *ast.MatchExpr:
		c.walk(e.Scrutinee)
		for _, arm := range e.Arms {
			c.walk(arm.Guard)
			c.walk(arm.Body)
		}
	case *ast.ForExpr:
		c.inLoop++
		c.walk(e.From)
		c.walk(e.To)
		c.walk(e.Body)
		c.inLoop--
	case *ast.WhileExpr:
		c.inLoop++
		c.walk(e.Cond)
		c.walk(e.Body)
		c.inLoop--
	case *ast.GoExpr:
		c.inGo++
		c.walk(e.Expr)
		c.inGo--
	case *ast.TryExpr:
		if e.Finally != nil {
			c.inFinally++
		}
		c.walk(e.Body)
		if e.Finally != nil {
			c.inFinally--
		}
		for _, arm := range e.Arms {
			c.walk(arm.Body)
		}
		c.walk(e.Finally)
	case *ast.FieldAccessExpr:
		c.walk(e.Left)
	case *ast.TupleExpr:
		for _, el := range e.Elems {
			c.walk(el)
		}
	case *ast.ListExpr:
		for _, el := range e.Elems {
			c.walk(el)
		}
	case *ast.ArrayLitExpr:
		for _, el := range e.Elems {
			c.walk(el)
		}
	case *ast.RecordExpr:
		for _, f := range e.Fields {
			c.walk(f.Value)
		}
	case *ast.RecordUpdateExpr:
		c.walk(e.Base)
		for _, f := range e.Fields {
			c.walk(f.Value)
		}
	case *ast.ParenExpr:
		c.walk(e.Inner)
	case *ast.PipeExpr:
		c.walk(e.Left)
		c.walk(e.Right)
	case *ast.IndexExpr:
		c.walk(e.Target)
		c.walk(e.Index)
	case *ast.AssignExpr:
		c.walk(e.Target)
		c.walk(e.Value)
	case *ast.MethodSendExpr:
		c.walk(e.Target)
	case *ast.ConstructorExpr:
		c.walk(e.Arg)
	case *ast.RefExpr:
		c.walk(e.Value)
	case *ast.DerefExpr:
		c.walk(e.Target)
	case *ast.AssertExpr:
		c.walk(e.Cond)
	case *ast.RaiseExpr:
		c.walk(e.Exn)
	case *ast.SelectExpr:
		for i := range e.Cases {
			c.walk(e.Cases[i].Recv)
			c.walk(e.Cases[i].Body)
		}
		c.walk(e.Default)
	case *ast.FunctionExpr:
		for _, arm := range e.Arms {
			c.walk(arm.Guard)
			c.walk(arm.Body)
		}
	case *ast.PtrOfExpr:
		c.walk(e.Inner)
	case *ast.LabelledArgExpr:
		c.walk(e.Value)
	}
}

func (c *checker) checkBinary(e *ast.BinaryExpr) {
	switch e.Op {
	case token.EQUALS, token.EQEQ, token.NEQ, token.DIAMOND:
	default:
		return
	}
	if isToCaseCall(e.Left) || isToCaseCall(e.Right) {
		c.emit(CodeStr, "case-fold then compare; use strings.EqualFold", ast.ExprLoc(e))
	}
	if c.isFloat(e.Left) || c.isFloat(e.Right) {
		c.emit(CodeFloat, "float equality is inexact; compare with a tolerance or use std.decimal", ast.ExprLoc(e))
	}
}

func (c *checker) isFloat(e ast.Expr) bool {
	if e == nil || c.tm == nil {
		return false
	}
	t, ok := c.tm[e]
	if !ok || t == nil {
		return false
	}
	if p, ok := t.(*types.Prim); ok && p.Name == "float" {
		return true
	}
	return t == types.Float
}

func (c *checker) checkApp(e *ast.AppExpr) {
	pkg, name := callPkgName(e)
	if c.inLoop > 0 && pkg == "regexp" && (name == "Compile" || name == "MustCompile" || name == "MatchString") {
		c.emit(CodeRegexp, "regexp compile/match inside a loop; compile once outside", ast.ExprLoc(e))
	}
	if c.inGo > 0 && name == "Add" && isIntyArg(e.Arg, c.tm) {
		c.emit(CodeWG, "WaitGroup.Add inside go; call Add before spawning", ast.ExprLoc(e))
	}
	if c.inFinally > 0 && pkg == "os" && name == "Exit" {
		c.emit(CodeExit, "os.Exit skips try/finally (Go defer); return an error instead", ast.ExprLoc(e))
	}
}

func (c *checker) checkDiscarded(e ast.Expr) {
	if isQuerySet(e) {
		c.emit(CodeURL, "url.Values.Query().Set is discarded; assign Query, Set, then write RawQuery", ast.ExprLoc(e))
	}
	pkg, name := callPkgName(e)
	if (pkg == "context" || pkg == "") && (name == "WithCancel" || name == "WithTimeout") {
		c.emit(CodeCtx, "WithCancel/WithTimeout cancel func is discarded; keep it and call it", ast.ExprLoc(e))
	}
}

func (c *checker) checkCtxBinding(b *ast.LetBinding) {
	if b.Name != "_" {
		return
	}
	pkg, name := callPkgName(b.Body)
	if name == "WithCancel" || name == "WithTimeout" {
		c.emit(CodeCtx, "WithCancel/WithTimeout cancel func is discarded; keep it and call it", ast.ExprLoc(b.Body))
	}
	_ = pkg
}

func isToCaseCall(e ast.Expr) bool {
	_, name := callPkgName(e)
	return name == "ToLower" || name == "ToUpper"
}

func isIntyArg(e ast.Expr, tm typeinfo.TypeMap) bool {
	if lit, ok := e.(*ast.LitExpr); ok && lit.Kind == token.INT {
		return true
	}
	if tm == nil || e == nil {
		return false
	}
	t, ok := tm[e]
	if !ok || t == nil {
		return false
	}
	if p, ok := t.(*types.Prim); ok && p.Name == "int" {
		return true
	}
	return t == types.Int
}

func callPkgName(e ast.Expr) (pkg, name string) {
	cur := e
	for {
		app, ok := cur.(*ast.AppExpr)
		if !ok {
			break
		}
		cur = app.Func
	}
	fa, ok := cur.(*ast.FieldAccessExpr)
	if !ok {
		return "", ""
	}
	switch l := fa.Left.(type) {
	case *ast.IdentExpr:
		return l.Name, fa.Field
	case *ast.ConstructorExpr:
		return l.Name, fa.Field
	default:
		return "", fa.Field
	}
}

func isQuerySet(e ast.Expr) bool {
	cur := e
	for {
		app, ok := cur.(*ast.AppExpr)
		if !ok {
			break
		}
		cur = app.Func
	}
	fa, ok := cur.(*ast.FieldAccessExpr)
	if !ok || fa.Field != "Set" {
		return false
	}
	_, name := callPkgName(fa.Left)
	return name == "Query"
}
