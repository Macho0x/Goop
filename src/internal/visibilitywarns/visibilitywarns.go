// Package visibilitywarns reports private types leaking into public APIs (VIS002).
package visibilitywarns

import (
	"fmt"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
)

const CodePrivateInPublic = "VIS002"

// Error is a private-in-public diagnostic.
type Error struct {
	Msg string
	Loc token.SourceLoc
}

func (e *Error) Error() string {
	prefix := CodePrivateInPublic + ": "
	if e.Loc.File != "" && e.Loc.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s%s", e.Loc.File, e.Loc.Line, e.Loc.Column, prefix, e.Msg)
	}
	return prefix + e.Msg
}

func (e *Error) GetLoc() token.SourceLoc { return e.Loc }

// CheckWithConfig warns when a non-private let/type mentions a private type from this module.
func CheckWithConfig(mod *ast.Module, cfg *config.Config) (errors, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Check.PrivateInPublic == config.SeverityOff {
		return nil, nil
	}
	privateTypes := map[string]bool{}
	privateCtors := map[string]string{} // ctor → type name
	for _, d := range mod.Decls {
		td, ok := d.(*ast.TypeDecl)
		if !ok || !td.Private {
			continue
		}
		privateTypes[td.Name] = true
		switch k := td.Kind.(type) {
		case *ast.ADTTypeKind:
			for _, c := range k.Cases {
				privateCtors[c.Name] = td.Name
			}
		case *ast.GADTTypeKind:
			for _, c := range k.Cases {
				privateCtors[c.Name] = td.Name
			}
		}
	}
	if len(privateTypes) == 0 {
		return nil, nil
	}

	c := &checker{
		cfg:          cfg,
		privateTypes: privateTypes,
		privateCtors: privateCtors,
	}
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			if d.Private {
				continue
			}
			for i := range d.Bindings {
				b := &d.Bindings[i]
				for _, p := range b.Params {
					c.walkType(p.Type, b.Name)
				}
				c.walkType(b.RetType, b.Name)
			}
		case *ast.TypeDecl:
			if d.Private {
				continue
			}
			c.walkTypeKind(d.Kind, d.Name)
		}
	}
	return c.errors, c.warnings
}

type checker struct {
	cfg          *config.Config
	privateTypes map[string]bool
	privateCtors map[string]string
	seen         map[string]bool
	errors       []error
	warnings     []error
}

func (c *checker) emit(msg string) {
	key := msg
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	e := &Error{Msg: msg, Loc: token.SourceLoc{}}
	if c.cfg.Check.PrivateInPublic == config.SeverityError {
		c.errors = append(c.errors, e)
	} else {
		c.warnings = append(c.warnings, e)
	}
}

func (c *checker) noteType(name, publicItem string) {
	if name == "" || !c.privateTypes[name] {
		return
	}
	c.emit(fmt.Sprintf("public %q exposes private type %q", publicItem, name))
}

func (c *checker) noteCtor(name, publicItem string) {
	if tn, ok := c.privateCtors[name]; ok {
		c.emit(fmt.Sprintf("public %q exposes private constructor %q of type %q", publicItem, name, tn))
	}
}

func (c *checker) walkTypeKind(k ast.TypeKind, publicItem string) {
	if k == nil {
		return
	}
	switch k := k.(type) {
	case *ast.AliasTypeKind:
		c.walkType(k.Alias, publicItem)
	case *ast.NewtypeTypeKind:
		c.walkType(k.Rep, publicItem)
	case *ast.RecordTypeKind:
		for _, f := range k.Fields {
			c.walkType(f.Type, publicItem)
		}
	case *ast.ADTTypeKind:
		for _, cas := range k.Cases {
			c.walkType(cas.Arg, publicItem)
		}
	case *ast.GADTTypeKind:
		for _, cas := range k.Cases {
			c.walkType(cas.Arg, publicItem)
			c.walkType(cas.Result, publicItem)
		}
	}
}

func (c *checker) walkType(t ast.Type, publicItem string) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *ast.TIdent:
		c.noteType(t.Name, publicItem)
		c.noteCtor(t.Name, publicItem)
	case *ast.TApp:
		c.walkType(t.Func, publicItem)
		c.walkType(t.Arg, publicItem)
	case *ast.TFun:
		c.walkType(t.From, publicItem)
		c.walkType(t.To, publicItem)
	case *ast.TTuple:
		for _, e := range t.Elems {
			c.walkType(e, publicItem)
		}
	case *ast.TRecord:
		for _, f := range t.Fields {
			c.walkType(f.Type, publicItem)
		}
	case *ast.TObject:
		for _, m := range t.Methods {
			c.walkType(m.Type, publicItem)
		}
	case *ast.TPtr:
		c.walkType(t.Elem, publicItem)
	case *ast.TGoSlice:
		c.walkType(t.Elem, publicItem)
	case *ast.TVariadic:
		c.walkType(t.Elem, publicItem)
	case *ast.TChan:
		c.walkType(t.Elem, publicItem)
	case *ast.TMap:
		c.walkType(t.Key, publicItem)
		c.walkType(t.Val, publicItem)
	case *ast.RefinementType:
		c.walkType(t.Inner, publicItem)
	case *ast.TPolyVariant:
		for _, cas := range t.Cases {
			c.walkType(cas.Arg, publicItem)
		}
	}
}
