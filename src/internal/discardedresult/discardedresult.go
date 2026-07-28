// Package discardedresult warns when result/option values are discarded in sequences.
package discardedresult

import (
	"fmt"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/types"
)

const (
	CodeResult = "RESULT001"
	CodeOption = "OPTION001"
)

// Error is a discarded result/option diagnostic.
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

// CheckWithConfig finds discarded result/option values in begin sequences.
func CheckWithConfig(mod *ast.Module, tm typeinfo.TypeMap, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
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

func (c *checker) emit(code, msg string, loc token.SourceLoc, sev config.Severity) {
	if sev == config.SeverityOff {
		return
	}
	e := &Error{Code: code, Msg: msg, Loc: loc}
	if sev == config.SeverityError {
		c.errors = append(c.errors, e)
	} else {
		c.warnings = append(c.warnings, e)
	}
}

func typeName(t types.Type) string {
	switch t := t.(type) {
	case *types.TCon:
		return t.Name
	case *types.TAdt:
		return t.Name
	default:
		return ""
	}
}

func (c *checker) checkDiscarded(e ast.Expr) {
	if e == nil || c.tm == nil {
		return
	}
	t, ok := c.tm[e]
	if !ok || t == nil {
		return
	}
	switch typeName(t) {
	case "result":
		c.emit(CodeResult, "result value is discarded; handle with match or bind with let _ = …",
			ast.ExprLoc(e), c.cfg.Check.DiscardedResult)
	case "option":
		c.emit(CodeOption, "option value is discarded; handle with match or bind with let _ = …",
			ast.ExprLoc(e), c.cfg.Check.DiscardedOption)
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
