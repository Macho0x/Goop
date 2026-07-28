// Package discardedresult warns when a result value is discarded in a sequence.
package discardedresult

import (
	"fmt"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

const Code = "RESULT001"

// Error is a discarded-result diagnostic.
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

// CheckWithConfig finds discarded result values in begin sequences.
func CheckWithConfig(mod *ast.Module, tm typeinfo.TypeMap, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Check.DiscardedResult == config.SeverityOff {
		return nil, nil
	}
	c := &checker{tm: tm, cfg: cfg}
	for _, d := range mod.Decls {
		if ld, ok := d.(*ast.LetDecl); ok {
			for i := range ld.Bindings {
				c.walk(ld.Bindings[i].Body)
			}
		}
	}
	return c.errors, c.warnings
}

type checker struct {
	tm       typeinfo.TypeMap
	cfg      *config.Config
	errors   []error
	warnings []error
}

func (c *checker) emit(loc token.SourceLoc) {
	e := &Error{
		Code: Code,
		Msg:  "result value is discarded; handle with match or bind with let _ = …",
		Loc:  loc,
	}
	switch c.cfg.Check.DiscardedResult {
	case config.SeverityError:
		c.errors = append(c.errors, e)
	default:
		c.warnings = append(c.warnings, e)
	}
}

func isResultType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t := t.(type) {
	case *types.TCon:
		return t.Name == "result"
	case *types.TAdt:
		return t.Name == "result"
	default:
		return false
	}
}

func (c *checker) checkDiscarded(e ast.Expr) {
	if e == nil || c.tm == nil {
		return
	}
	t, ok := c.tm[e]
	if ok && isResultType(t) {
		c.emit(ast.ExprLoc(e))
	}
}

func (c *checker) walk(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.BeginExpr:
		for i, s := range e.Stmts {
			if i < len(e.Stmts)-1 {
				c.checkDiscarded(s)
			}
			c.walk(s)
		}
	case *ast.LetInExpr:
		for i := range e.Bindings {
			c.walk(e.Bindings[i].Body)
		}
		c.walk(e.Body)
	case *ast.IfExpr:
		c.walk(e.ThenBranch)
		c.walk(e.ElseBranch)
	case *ast.MatchExpr:
		c.walk(e.Scrutinee)
		for _, arm := range e.Arms {
			c.walk(arm.Body)
		}
	case *ast.AppExpr:
		c.walk(e.Func)
		c.walk(e.Arg)
	case *ast.FunExpr:
		c.walk(e.Body)
	case *ast.ForExpr:
		c.walk(e.Body)
	case *ast.WhileExpr:
		c.walk(e.Cond)
		c.walk(e.Body)
	case *ast.GoExpr:
		c.walk(e.Expr)
	case *ast.TryExpr:
		c.walk(e.Body)
		for _, arm := range e.Arms {
			c.walk(arm.Body)
		}
		c.walk(e.Finally)
	}
}
