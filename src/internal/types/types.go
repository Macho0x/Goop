// Package types defines the internal type representation for Goop type checking.
//
// This is separate from the AST types (ast.Type) which represent parsed type
// expressions. The type checker converts ast.Type nodes into these internal
// types during inference.
package types

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Type interface
// ---------------------------------------------------------------------------

// Type is the interface for all internal type representations.
type Type interface {
	// String returns a human-readable representation.
	String() string
}

// ---------------------------------------------------------------------------
// Primitive types
// ---------------------------------------------------------------------------

// Prim is a named primitive type: int, float, bool, string, unit, etc.
type Prim struct {
	Name string
}

func (p *Prim) String() string { return p.Name }

// TGoNamed is an opaque named type imported from a Go package.
type TGoNamed struct {
	Pkg, Name string
	Interface bool
}

func (t *TGoNamed) String() string { return t.Pkg + "." + t.Name }

type TPtr struct{ Elem Type }

func (t *TPtr) String() string { return t.Elem.String() + " ptr" }

type TGoSlice struct{ Elem Type }

func (t *TGoSlice) String() string { return t.Elem.String() + " go_slice" }

type TError struct{}

func (*TError) String() string { return "error" }

// Well-known primitives.
var (
	Int    = &Prim{"int"}
	Float  = &Prim{"float"}
	Bool   = &Prim{"bool"}
	String = &Prim{"string"}
	Unit   = &Prim{"unit"}
	Bytes  = &Prim{"bytes"}
	Rune   = &Prim{"rune"}
)

// ---------------------------------------------------------------------------
// Type variables
// ---------------------------------------------------------------------------

var freshCounter int64

// Fresh creates a new unique type variable.
func Fresh(name string) *TVar {
	id := atomic.AddInt64(&freshCounter, 1)
	if name == "" {
		name = fmt.Sprintf("'t%d", id)
	}
	return &TVar{ID: id, Name: name}
}

// TVar is a type variable used during unification.
type TVar struct {
	ID   int64
	Name string
}

func (v *TVar) String() string {
	if strings.HasPrefix(v.Name, "'") {
		return v.Name
	}
	return "'" + v.Name
}

// ---------------------------------------------------------------------------
// Function types
// ---------------------------------------------------------------------------

// EffectRow represents a set of effects on a function.
// When Open is true, it's row-polymorphic (accepts any superset).
// When nil, it means "unknown effects" — an implicit open row variable
// used for backward compat (existing code without effect annotations).
type EffectRow struct {
	Effects []string // effect names, e.g. "io", "log"
	Open    bool     // true for row-polymorphic `{ e | .. }`
	Rest    *TVar    // the row variable for open rows (may be nil)
}

func (e *EffectRow) String() string {
	if e == nil {
		return ""
	}
	if len(e.Effects) == 0 && !e.Open {
		return ""
	}
	parts := make([]string, len(e.Effects))
	for i, eff := range e.Effects {
		parts[i] = eff
	}
	s := strings.Join(parts, "; ")
	if e.Open {
		if e.Rest != nil {
			s += " | " + e.Rest.String()
		} else {
			s += " | .."
		}
	}
	return s
}

// TFun is a function type: From -> To.
// Effects is nil for "unknown" (backward-compat implicit open row),
// non-nil for explicit effect annotations.
type TFun struct {
	From     Type
	To       Type
	Effects  *EffectRow // nil means unknown (permissive for backward compat)
	Label    string
	Optional bool
}

func (f *TFun) String() string {
	base := fmt.Sprintf("(%s -> %s)", f.From, f.To)
	if f.Effects != nil {
		s := f.Effects.String()
		if s != "" {
			base += " with {" + s + "}"
		}
	}
	return base
}

// PolyVariant is a structural polymorphic-variant row.
type PolyVariant struct {
	Variants   []Variant
	Open       bool
	UpperBound bool
}

func (p *PolyVariant) String() string {
	parts := make([]string, len(p.Variants))
	for i, v := range p.Variants {
		parts[i] = "`" + v.Name
		if v.Arg != nil {
			parts[i] += " of " + v.Arg.String()
		}
	}
	prefix := "["
	if p.Open {
		prefix += ">"
	} else if p.UpperBound {
		prefix += "<"
	}
	return prefix + strings.Join(parts, " | ") + "]"
}

// ---------------------------------------------------------------------------
// Tuple types
// ---------------------------------------------------------------------------

// TTuple is a fixed-arity tuple type.
type TTuple struct {
	Elems []Type
}

func (t *TTuple) String() string {
	parts := make([]string, len(t.Elems))
	for i, e := range t.Elems {
		parts[i] = e.String()
	}
	return "(" + strings.Join(parts, " * ") + ")"
}

// ---------------------------------------------------------------------------
// Record types (closed)
// ---------------------------------------------------------------------------

// Field is a named field with its type.
type Field struct {
	Name string
	Type Type
}

// TRecord is a record type.  When Open is true it is a row type that
// accepts any record having at least the listed fields.
type TRecord struct {
	Fields []Field // ordered list; nil means "unknown record" during inference
	Open   bool    // true for row-polymorphic `{ ... | .. }`
}

func (r *TRecord) String() string {
	if r == nil {
		return "<unknown record>"
	}
	parts := make([]string, len(r.Fields))
	for i, f := range r.Fields {
		parts[i] = f.Name + ": " + f.Type.String()
	}
	s := "{ " + strings.Join(parts, "; ") + " "
	if r.Open {
		s += "| .. "
	}
	return s + "}"
}

// Lookup returns the type of a field, or nil if not present.
func (r *TRecord) Lookup(name string) Type {
	if r == nil {
		return nil
	}
	for _, f := range r.Fields {
		if f.Name == name {
			return f.Type
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ADT (algebraic data type)
// ---------------------------------------------------------------------------

// Variant represents one constructor of an ADT.
type Variant struct {
	Name string // constructor name, e.g. "Circle", "None"
	Arg  Type   // nil if no payload; the type of the payload otherwise
}

// TAdt is a user-defined or built-in algebraic data type.
// Params are the type parameters (instantiations). E.g. for
// `option int`, Params would be [int]; for `Shape`, Params would be [].
type TAdt struct {
	Name     string    // fully qualified name, e.g. "Shapes.shape" or "option"
	Params   []Type    // instantiated type arguments (nil for non-generic)
	Variants []Variant // all constructors
	Linear   bool      // true if declared with `: 1` (linear resource type)
}

func (a *TAdt) String() string {
	var suffix string
	if a.Linear {
		suffix = " : 1"
	}
	if len(a.Params) == 0 {
		return a.Name + suffix
	}
	parts := make([]string, len(a.Params))
	for i, p := range a.Params {
		parts[i] = p.String()
	}
	return a.Name + "<" + strings.Join(parts, ", ") + ">" + suffix
}

// LookupVariant returns the variant with the given constructor name, or nil.
func (a *TAdt) LookupVariant(name string) *Variant {
	for i := range a.Variants {
		if a.Variants[i].Name == name {
			return &a.Variants[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Nominal newtypes
// ---------------------------------------------------------------------------

// TNewtype is a nominal wrapper around a representation type.
// Unlike aliases, TNewtype does not unify with its Rep except via constructors.
type TNewtype struct {
	Name string
	Rep  Type
}

func (n *TNewtype) String() string {
	return n.Name
}

// ---------------------------------------------------------------------------
// Type constructors (built-in generics: list, option, result)
// ---------------------------------------------------------------------------

// TCon is a built-in type constructor applied to arguments.
// Used for: list<int>, option<float>, result<user, error>.
// Internally these are equivalent to ADTs but recognized by name.
type TCon struct {
	Name string // "list", "option", "result"
	Args []Type // instantiated type arguments
}

func (c *TCon) String() string {
	if len(c.Args) == 0 {
		return c.Name
	}
	parts := make([]string, len(c.Args))
	for i, a := range c.Args {
		parts[i] = a.String()
	}
	return c.Name + "<" + strings.Join(parts, ", ") + ">"
}

// ---------------------------------------------------------------------------
// Channel type
// ---------------------------------------------------------------------------

// TChan represents a channel type: `'a chan`.
type TChan struct {
	Elem Type
}

func (*TChan) typeNode() {}

func (t *TChan) String() string {
	return t.Elem.String() + " chan"
}

// TMap represents a map type: `map[K] V`.
type TMap struct {
	Key Type
	Val Type
}

func (t *TMap) String() string {
	return "map[" + t.Key.String() + "] " + t.Val.String()
}

// ---------------------------------------------------------------------------
// Type schemes (polytypes)
// ---------------------------------------------------------------------------

// Scheme is a type scheme with quantified type variables.
// `ForAll ['a, 'b]. 'a -> 'b -> 'a`
type Scheme struct {
	Vars []*TVar // quantified type variables
	Type Type
}

func (s *Scheme) String() string {
	if len(s.Vars) == 0 {
		return s.Type.String()
	}
	parts := make([]string, len(s.Vars))
	for i, v := range s.Vars {
		parts[i] = v.String()
	}
	return "∀[" + strings.Join(parts, ", ") + "]. " + s.Type.String()
}

// ---------------------------------------------------------------------------
// Substitution
// ---------------------------------------------------------------------------

// Subst maps type variables to types.
type Subst map[int64]Type

// EmptySubst returns an empty substitution.
func EmptySubst() Subst { return Subst{} }

// Lookup returns the type for a type variable, or nil if not bound.
func (s Subst) Lookup(id int64) Type {
	if s == nil {
		return nil
	}
	return s[id]
}

// Compose composes two substitutions: s2 ∘ s1.
// Applying `Compose(s2, s1)` to a type means: first apply s1, then apply s2.
func Compose(s2, s1 Subst) Subst {
	if len(s1) == 0 {
		return s2
	}
	if len(s2) == 0 {
		return s1
	}
	result := make(Subst)
	for k, v := range s2 {
		result[k] = v
	}
	for k, v := range s1 {
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}
	// Apply s2 to all values from s1
	for k, v := range result {
		if k == 0 { // dummy
			continue
		}
		result[k] = Apply(s2, v)
	}
	return result
}

// Apply applies a substitution to a type, replacing type variables.
func Apply(s Subst, t Type) Type {
	if s == nil || len(s) == 0 {
		return t
	}
	switch t := t.(type) {
	case *TVar:
		if repl, ok := s[t.ID]; ok {
			// Recursively resolve TVar chains (e.g. result→freshA→int)
			if _, isTVar := repl.(*TVar); isTVar {
				return Apply(s, repl)
			}
			return repl
		}
		return t
	case *Prim:
		return t
	case *TGoNamed, *TError:
		return t
	case *TPtr:
		return &TPtr{Elem: Apply(s, t.Elem)}
	case *TGoSlice:
		return &TGoSlice{Elem: Apply(s, t.Elem)}
	case *TFun:
		fn := &TFun{From: Apply(s, t.From), To: Apply(s, t.To), Label: t.Label, Optional: t.Optional}
		if t.Effects != nil {
			fn.Effects = &EffectRow{
				Effects: t.Effects.Effects,
				Open:    t.Effects.Open,
				Rest:    t.Effects.Rest,
			}
		}
		return fn
	case *TTuple:
		elems := make([]Type, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = Apply(s, e)
		}
		return &TTuple{Elems: elems}
	case *TRecord:
		if t == nil {
			return nil
		}
		fields := make([]Field, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = Field{Name: f.Name, Type: Apply(s, f.Type)}
		}
		return &TRecord{Fields: fields, Open: t.Open}
	case *TAdt:
		params := make([]Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = Apply(s, p)
		}
		variants := make([]Variant, len(t.Variants))
		for i, v := range t.Variants {
			variants[i] = Variant{Name: v.Name}
			if v.Arg != nil {
				variants[i].Arg = Apply(s, v.Arg)
			}
		}
		return &TAdt{Name: t.Name, Params: params, Variants: variants, Linear: t.Linear}
	case *PolyVariant:
		variants := make([]Variant, len(t.Variants))
		for i, v := range t.Variants {
			variants[i] = Variant{Name: v.Name}
			if v.Arg != nil {
				variants[i].Arg = Apply(s, v.Arg)
			}
		}
		return &PolyVariant{Variants: variants, Open: t.Open, UpperBound: t.UpperBound}
	case *TNewtype:
		return &TNewtype{Name: t.Name, Rep: Apply(s, t.Rep)}
	case *TCon:
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = Apply(s, a)
		}
		return &TCon{Name: t.Name, Args: args}
	case *TChan:
		return &TChan{Elem: Apply(s, t.Elem)}
	case *TMap:
		return &TMap{Key: Apply(s, t.Key), Val: Apply(s, t.Val)}
	default:
		return t
	}
}

// FreeVars returns the set of free type variable IDs in a type.
func FreeVars(t Type) map[int64]bool {
	fv := make(map[int64]bool)
	freeVars(t, fv)
	return fv
}

func freeVars(t Type, fv map[int64]bool) {
	switch t := t.(type) {
	case *TVar:
		fv[t.ID] = true
	case *TPtr:
		freeVars(t.Elem, fv)
	case *TGoSlice:
		freeVars(t.Elem, fv)
	case *TFun:
		freeVars(t.From, fv)
		freeVars(t.To, fv)
		if t.Effects != nil && t.Effects.Rest != nil {
			fv[t.Effects.Rest.ID] = true
		}
	case *TTuple:
		for _, e := range t.Elems {
			freeVars(e, fv)
		}
	case *TRecord:
		if t == nil {
			return
		}
		for _, f := range t.Fields {
			freeVars(f.Type, fv)
		}
	case *TAdt:
		for _, p := range t.Params {
			freeVars(p, fv)
		}
	case *PolyVariant:
		for _, v := range t.Variants {
			if v.Arg != nil {
				freeVars(v.Arg, fv)
			}
		}
	case *TNewtype:
		freeVars(t.Rep, fv)
	case *TCon:
		for _, a := range t.Args {
			freeVars(a, fv)
		}
	case *TChan:
		freeVars(t.Elem, fv)
	case *TMap:
		freeVars(t.Key, fv)
		freeVars(t.Val, fv)
	}
}

// Generalize creates a type scheme by quantifying all free type variables
// that are not in the given set of "in-scope" variable IDs.
func Generalize(t Type, inScope map[int64]bool) *Scheme {
	fv := FreeVars(t)
	var vars []*TVar
	for id := range fv {
		if !inScope[id] {
			vars = append(vars, &TVar{ID: id})
		}
	}
	// Sort by ID for determinism
	sortVars(vars)
	return &Scheme{Vars: vars, Type: t}
}

func sortVars(vars []*TVar) {
	for i := 0; i < len(vars); i++ {
		for j := i + 1; j < len(vars); j++ {
			if vars[i].ID > vars[j].ID {
				vars[i], vars[j] = vars[j], vars[i]
			}
		}
	}
}

// Instantiate creates a fresh copy of a scheme with fresh type variables
// replacing the quantified ones.
func (s *Scheme) Instantiate() Type {
	if len(s.Vars) == 0 {
		return s.Type
	}
	sub := make(Subst)
	for _, v := range s.Vars {
		sub[v.ID] = Fresh(v.Name)
	}
	return Apply(sub, s.Type)
}

// Mono creates a monomorphic scheme (no quantified variables).
func Mono(t Type) *Scheme {
	return &Scheme{Type: t}
}

// ---------------------------------------------------------------------------
// Built-in type definitions
// ---------------------------------------------------------------------------

// Built-in generic ADT type variables used for the definitions.
var (
	tvA   = &TVar{ID: -1, Name: "'a"}
	tvB   = &TVar{ID: -2, Name: "'b"}
	tvOk  = &TVar{ID: -3, Name: "'ok"}
	tvErr = &TVar{ID: -4, Name: "'err"}
)

// BuiltinOption returns the 'a option ADT schema.
func BuiltinOption() *Scheme {
	// type 'a option = None | Some of 'a
	return &Scheme{
		Vars: []*TVar{tvA},
		Type: &TAdt{
			Name:   "option",
			Params: []Type{tvA},
			Variants: []Variant{
				{Name: "None"},
				{Name: "Some", Arg: tvA},
			},
		},
	}
}

// BuiltinResult returns the ('ok, 'err) result ADT schema.
func BuiltinResult() *Scheme {
	return &Scheme{
		Vars: []*TVar{tvOk, tvErr},
		Type: &TAdt{
			Name:   "result",
			Params: []Type{tvOk, tvErr},
			Variants: []Variant{
				{Name: "Ok", Arg: tvOk},
				{Name: "Error", Arg: tvErr},
			},
		},
	}
}

// OptionType creates an instantiated option type: option<T>.
func OptionType(t Type) Type {
	return &TCon{Name: "option", Args: []Type{t}}
}

// ResultType creates an instantiated result type: result<ok, err>.
func ResultType(ok, err Type) Type {
	return &TCon{Name: "result", Args: []Type{ok, err}}
}

// ListType creates an instantiated list type: list<T>.
func ListType(t Type) Type {
	return &TCon{Name: "list", Args: []Type{t}}
}

// ArrayType creates an instantiated array type: array<T>.
func ArrayType(t Type) Type {
	return &TCon{Name: "array", Args: []Type{t}}
}

// RefType creates an instantiated ref type: 'a ref.
func RefType(t Type) Type {
	return &TCon{Name: "ref", Args: []Type{t}}
}

// LazyType creates an instantiated lazy type: 'a lazy.
func LazyType(t Type) Type {
	return &TCon{Name: "lazy", Args: []Type{t}}
}

// ---------------------------------------------------------------------------
// Conversion from AST types to internal types
// ---------------------------------------------------------------------------

// astTypeToInternalFunc is set by the typecheck package to avoid circular
// imports. We use a function variable.
var ConvertASTType func(astType interface{}) Type

// SetASTConverter registers the AST-to-internal type converter.
func SetASTConverter(fn func(interface{}) Type) {
	ConvertASTType = fn
}
