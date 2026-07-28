// Package moneyfloat warns when float is used for money-like names (DECIMAL001).
package moneyfloat

import (
	"fmt"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
)

const Code = "DECIMAL001"

// Error is a money-as-float diagnostic.
type Error struct {
	Msg string
	Loc token.SourceLoc
}

func (e *Error) Error() string {
	prefix := Code + ": "
	if e.Loc.File != "" && e.Loc.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s%s", e.Loc.File, e.Loc.Line, e.Loc.Column, prefix, e.Msg)
	}
	return prefix + e.Msg
}

func (e *Error) GetLoc() token.SourceLoc { return e.Loc }

var moneyNames = map[string]bool{
	"price": true, "px": true, "bid": true, "ask": true, "mid": true, "midprice": true,
	"spread": true, "notional": true, "amount": true, "balance": true,
	"fee": true, "cost": true, "pnl": true, "equity": true, "margin": true,
	"fill": true, "proceeds": true,
}

var moneySuffixes = []string{"_price", "_amount", "_usd", "_cents", "_notional"}

var excludeSuffixes = []string{
	"_mult", "_multiplier", "_ratio", "_bps", "_pct", "_offset", "_zscore", "_score", "_cap",
}

// CheckWithConfig reports float annotations used for money-ish names.
func CheckWithConfig(mod *ast.Module, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Check.MoneyFloat == config.SeverityOff {
		return nil, nil
	}
	c := &checker{cfg: cfg}
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			c.walkTypeKind(d.Kind)
		case *ast.LetDecl:
			for i := range d.Bindings {
				b := &d.Bindings[i]
				for _, p := range b.Params {
					c.checkNamed(p.Name, p.Type)
				}
				c.walkType(b.RetType, b.Name)
				c.walkExpr(b.Body)
			}
		}
	}
	return c.errors, c.warnings
}

type checker struct {
	cfg      *config.Config
	errors   []error
	warnings []error
	seen     map[string]bool
}

func (c *checker) emit(msg string, loc token.SourceLoc) {
	key := msg
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	e := &Error{Msg: msg, Loc: loc}
	if c.cfg.Check.MoneyFloat == config.SeverityError {
		c.errors = append(c.errors, e)
	} else {
		c.warnings = append(c.warnings, e)
	}
}

func isFloatType(t ast.Type) bool {
	for t != nil {
		switch tt := t.(type) {
		case *ast.TIdent:
			return tt.Name == "float"
		case *ast.RefinementType:
			t = tt.Inner
		default:
			return false
		}
	}
	return false
}

func moneyish(name string) bool {
	n := strings.TrimPrefix(strings.ToLower(name), "_")
	if n == "" {
		return false
	}
	for _, suf := range excludeSuffixes {
		if strings.HasSuffix(n, suf) {
			return false
		}
	}
	if moneyNames[n] {
		return true
	}
	for _, suf := range moneySuffixes {
		if strings.HasSuffix(n, suf) {
			return true
		}
	}
	return false
}

func (c *checker) checkNamed(name string, t ast.Type) {
	if name == "" || !moneyish(name) || !isFloatType(t) {
		return
	}
	c.emit(fmt.Sprintf("%q uses float for money; use std.decimal.Decimal instead", name), token.SourceLoc{})
}

func (c *checker) walkTypeKind(k ast.TypeKind) {
	if k == nil {
		return
	}
	switch k := k.(type) {
	case *ast.RecordTypeKind:
		for _, f := range k.Fields {
			c.checkNamed(f.Name, f.Type)
		}
	case *ast.AliasTypeKind:
		c.walkType(k.Alias, "")
	case *ast.NewtypeTypeKind:
		c.walkType(k.Rep, "")
	case *ast.ADTTypeKind:
		for _, cas := range k.Cases {
			c.walkType(cas.Arg, "")
		}
	case *ast.GADTTypeKind:
		for _, cas := range k.Cases {
			c.walkType(cas.Arg, "")
			c.walkType(cas.Result, "")
		}
	}
}

func (c *checker) walkType(t ast.Type, bindingHint string) {
	if t == nil {
		return
	}
	if bindingHint != "" {
		c.checkNamed(bindingHint, t)
	}
	switch t := t.(type) {
	case *ast.TApp:
		c.walkType(t.Func, "")
		c.walkType(t.Arg, "")
	case *ast.TFun:
		c.walkType(t.From, "")
		c.walkType(t.To, "")
	case *ast.TTuple:
		for _, e := range t.Elems {
			c.walkType(e, "")
		}
	case *ast.TRecord:
		for _, f := range t.Fields {
			c.checkNamed(f.Name, f.Type)
		}
	case *ast.RefinementType:
		c.walkType(t.Inner, bindingHint)
	}
}

func (c *checker) walkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.LetInExpr:
		for i := range e.Bindings {
			b := &e.Bindings[i]
			for _, p := range b.Params {
				c.checkNamed(p.Name, p.Type)
			}
			c.walkType(b.RetType, b.Name)
			c.walkExpr(b.Body)
		}
		c.walkExpr(e.Body)
	case *ast.FunExpr:
		for _, p := range e.Params {
			c.checkNamed(p.Name, p.Type)
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
			c.walkExpr(arm.Body)
		}
	case *ast.BeginExpr:
		for _, s := range e.Stmts {
			c.walkExpr(s)
		}
	case *ast.BinaryExpr:
		c.walkExpr(e.Left)
		c.walkExpr(e.Right)
	}
}
