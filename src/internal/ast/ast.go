// Package ast defines the Abstract Syntax Tree types for Goop.
package ast

import (
	"fmt"
	"strings"

	"goop.dev/compiler/internal/token"
)

// ---------------------------------------------------------------------------
// Top-level nodes
// ---------------------------------------------------------------------------

// Module represents a complete Goop source file.
type Module struct {
	Name       string       // module path, e.g. "Trading.OrderBook"
	Imports    []ImportSpec // import directives (go and goop)
	Decls      []TopDecl    // top-level declarations
	Attributes []Attribute  // ignored OCaml attributes/extensions stripped at parse time
}

// ImportKind distinguishes Go package imports from Goop module imports.
type ImportKind int

const (
	ImportGo   ImportKind = iota // import go "path"
	ImportGoop                   // import goop "path"
)

// ImportSpec is one arm of an import declaration.
// Alias is empty for default qualification, "." for dot import, or a local name.
type ImportSpec struct {
	Kind  ImportKind
	Path  string // import path string literal (logical or canonical)
	Alias string
	Raw   bool         // import go raw "path" — keep (T, error) as a tuple (H6 opt-out)
	Vals  []ExternVal  // go imports only: optional FFI signatures
	Types []ExternType // go imports only: opaque Go named types
	Loc   token.SourceLoc
}

// OpenStmt is a legacy `open Path` statement (parser no longer produces these).
type OpenStmt struct {
	Path string
}

// ---------------------------------------------------------------------------
// Top-level declarations
// ---------------------------------------------------------------------------

// TopDecl is the interface for module-level declarations.
type TopDecl interface {
	topDeclNode()
}

// LetDecl is a value binding at the top level.
type LetDecl struct {
	Rec           bool
	Mutable       bool
	Private       bool
	ActivePattern bool         // true for `let (|Name|_|) …`
	Bindings      []LetBinding // multiple when `and` is used
}

func (*LetDecl) topDeclNode() {}

// TypeDecl is a type declaration at the top level.
type TypeDecl struct {
	Name       string
	TypeParams []string // e.g. ["'a"]
	Kind       TypeKind
	Quantity   int // 0 = unrestricted (default), 1 = linear
	Private    bool
}

func (*TypeDecl) topDeclNode() {}

// ExternDecl is an `extern` block (parsed but not elaborated).
type ExternDecl struct {
	Lang     string // e.g. "go"
	Path     string // e.g. "github.com/example/lib"
	Vals     []ExternVal
	GoBlocks []string // raw Go source code from go { ... } blocks (inline Go extern)
}

func (*ExternDecl) topDeclNode() {}

// LangEmbedDecl is a top-level `@[go]` / `@[c]` block with optional `val` signatures.
type LangEmbedDecl struct {
	Lang string // "go" or "c"
	Body string // raw embedded source
	Vals []ExternVal
}

func (*LangEmbedDecl) topDeclNode() {}

// ExternVal is a single `val` inside an extern block.
type ExternValKind int

const (
	ExternFunc ExternValKind = iota
	ExternMethod
	ExternField
)

type ExternVal struct {
	Name     string
	Type     Type
	Kind     ExternValKind
	RecvName string // empty for functions
	RecvType Type   // nil for functions
}

// ExternType is an opaque Go named type imported from an FFI package.
type ExternType struct {
	Name string
}

// ImplementsDecl defines Go methods that make a Goop type satisfy an imported
// Go interface.
type ImplementsDecl struct {
	Interface string
	ForType   string
	Methods   []LetBinding
}

func (*ImplementsDecl) topDeclNode() {}

// ---------------------------------------------------------------------------
// Bindings & parameters
// ---------------------------------------------------------------------------

// LetBinding represents one arm of a (possibly mutually-recursive) let.
type LetBinding struct {
	Name       string
	Params     []Param
	RetType    Type           // nil if omitted
	RetEffects *EffectRowType // nil if no `with` clause
	Body       Expr
}

// Param is a function parameter.
type Param struct {
	Name     string
	Type     Type   // nil if unannotated (e.g. `fun x ->`)
	Label    string // labelled arg `~x` / `~label:x` (empty = positional)
	Optional bool   // `?x` optional labelled arg
	Default  Expr   // default expression for optional labelled arguments
	Variadic bool
}

// ---------------------------------------------------------------------------
// TypeKind: record vs ADT vs alias
// ---------------------------------------------------------------------------

type TypeKind interface {
	typeKindNode()
}

// RecordTypeKind is `type T = { field: Type; ... }`.
type RecordTypeKind struct {
	Fields []FieldType
}

func (*RecordTypeKind) typeKindNode() {}

// ADTTypeKind is `type T = | Case1 of ... | Case2 | ...`.
type ADTTypeKind struct {
	Cases []ADTCase
}

func (*ADTTypeKind) typeKindNode() {}

// AliasTypeKind is `type T = SomeType` (a type alias).
type AliasTypeKind struct {
	Alias Type
}

func (*AliasTypeKind) typeKindNode() {}

// NewtypeTypeKind is `type T = newtype SomeType` (a nominal wrapper).
type NewtypeTypeKind struct {
	Rep Type
}

func (*NewtypeTypeKind) typeKindNode() {}

// OpaqueTypeKind is `type T : 1` — an opaque linear type with no body.
type OpaqueTypeKind struct{}

func (*OpaqueTypeKind) typeKindNode() {}

// GADTTypeKind is `type _ t = | C : int -> int t` (approximate GADT support).
type GADTTypeKind struct {
	Cases []GADTCase
}

func (*GADTTypeKind) typeKindNode() {}

// ExtensibleTypeKind is `type t = ..`. Its constructors are added later by
// ExtensibleVariantDecl declarations.
type ExtensibleTypeKind struct{}

func (*ExtensibleTypeKind) typeKindNode() {}

// GADTCase is one GADT constructor: `C : arg -> ret` or `C : ret`.
type GADTCase struct {
	Name   string
	Arg    Type // nil if nullary (`C : int t`)
	Result Type // return type annotation (e.g. `int t`)
}

// ADTCase is one constructor alternative in an ADT.
type ADTCase struct {
	Name string
	Arg  Type // nil if no payload (e.g. `| Point`)
}

// ExtensibleVariantDecl is `type t += C of payload`.
type ExtensibleVariantDecl struct {
	TypeName string
	Cases    []ADTCase
}

func (*ExtensibleVariantDecl) topDeclNode() {}

// FieldType is a record field declaration: `name : Type` or `mutable name : Type`.
type FieldType struct {
	Name    string
	Type    Type
	Mutable bool
}

// ---------------------------------------------------------------------------
// Type expressions
// ---------------------------------------------------------------------------

// Type is an interface for type expressions.
type Type interface {
	typeNode()
}

// TIdent is a named type: `int`, `string`, `MyModule.t`.
type TIdent struct {
	Name string // may contain dots for qualified names
}

func (*TIdent) typeNode() {}

type TPtr struct{ Elem Type }

func (*TPtr) typeNode() {}

type TGoSlice struct{ Elem Type }

func (*TGoSlice) typeNode() {}

type TVariadic struct{ Elem Type }

func (*TVariadic) typeNode() {}

// TApp is type application: `order list`, `(int, string) result`.
// Func is the "outer" type, Arg is the parameter.
type TApp struct {
	Func Type
	Arg  Type
}

func (*TApp) typeNode() {}

// TFun is a function type: `A -> B`.
type TFun struct {
	From    Type
	To      Type
	Effects *EffectRowType // nil if no `with` clause
}

func (*TFun) typeNode() {}

// EffectRowType is an effect row annotation: `with { io; log }` or `with { e | .. }`.
type EffectRowType struct {
	Effects []string // effect names, e.g. "io", "log"
	Open    bool     // `| ..` present
	Rest    string   // row variable name (before `| ..`), or ""
}

func (*EffectRowType) typeNode() {}

// TTuple is a tuple type: `(A, B, C)`.
type TTuple struct {
	Elems []Type
}

func (*TTuple) typeNode() {}

// TRecord is a record type: `{ id: int; name: string }`.
// If Open is true, it's a row type: `{ name: string | .. }`.
type TRecord struct {
	Fields []FieldType
	Open   bool // true for row-polymorphic `{ ... | .. }`
}

func (*TRecord) typeNode() {}

// TObject is a structural object type: `< method : type; ... >`.
type TObject struct {
	Methods []FieldType
	Open    bool
}

func (*TObject) typeNode() {}

// TPolyVariant is a polymorphic-variant row. Open accepts extra tags, while
// Closed is the ordinary exact row form. UpperBound records `[< ...]`.
type TPolyVariant struct {
	Cases      []ADTCase
	Open       bool
	UpperBound bool
}

func (*TPolyVariant) typeNode() {}

// TVar is a type variable: `'a`.
type TVar struct {
	Name string // includes the leading apostrophe, e.g. "'a"
}

func (*TVar) typeNode() {}

// TChan is a channel type: `int chan`.
type TChan struct {
	Elem Type
}

func (*TChan) typeNode() {}

// TMap is a map type: `map[K] V`.
type TMap struct {
	Key Type
	Val Type
}

func (*TMap) typeNode() {}

// RefinementType wraps a type with a where clause.
// `int where x > 0` → RefinementType{Inner: int, Pred: x > 0}
type RefinementType struct {
	Inner Type // the base type
	Pred  Expr // the predicate expression
}

func (*RefinementType) typeNode() {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// Expr is the interface for all expression nodes.
type Expr interface {
	exprNode()
}

// LitExpr is a literal: `42`, `3.14`, `"hello"`, `true`, `false`, `()`.
type LitExpr struct {
	Value any             // int64, float64, string, bool, or nil for unit
	Kind  token.TokenType // INT, FLOAT, STRING, TRUE, FALSE, UNIT
	Loc   token.SourceLoc // source location
}

func (*LitExpr) exprNode() {}

// NullExpr is the Go FFI null pointer literal.
type NullExpr struct{}

func (*NullExpr) exprNode() {}

// PtrOfExpr takes the address of an expression for Go FFI.
type PtrOfExpr struct{ Inner Expr }

func (*PtrOfExpr) exprNode() {}

// IsNullExpr tests a Go FFI pointer against nil.
type IsNullExpr struct{ Inner Expr }

func (*IsNullExpr) exprNode() {}

// SpreadExpr expands a Go slice in a variadic Go call.
type SpreadExpr struct{ Inner Expr }

func (*SpreadExpr) exprNode() {}

// IdentExpr is an identifier reference: `x`, `Console.println`.
type IdentExpr struct {
	Name string
	Loc  token.SourceLoc // source location
}

func (*IdentExpr) exprNode() {}

// ConstructorExpr is a constructor (capitalised) used in expression position.
// e.g. `None`, `Some 42`.
type ConstructorExpr struct {
	Name       string
	TypePrefix string          // optional ADT qualifier: Type.Ctor
	Arg        Expr            // nil when no argument
	Loc        token.SourceLoc // source location
}

func (*ConstructorExpr) exprNode() {}

// AppExpr is function application: `f x y`.
type AppExpr struct {
	Func Expr
	Arg  Expr
	Loc  token.SourceLoc // source location
}

func (*AppExpr) exprNode() {}

// IfExpr is `if cond then trueBranch else falseBranch`.
type IfExpr struct {
	Cond       Expr
	ThenBranch Expr
	ElseBranch Expr
	Loc        token.SourceLoc // source location
}

func (*IfExpr) exprNode() {}

// MatchExpr is `match scrutinee with | pat -> body | ...`.
type MatchExpr struct {
	Scrutinee Expr
	Arms      []MatchArm
	Loc       token.SourceLoc // source location
}

func (*MatchExpr) exprNode() {}

// MatchArm is one branch of a match expression.
type MatchArm struct {
	Pattern       Pattern
	Guard         Expr // nil if no `when`
	Body          Expr
	EffectHandler bool   // `effect (E x) k ->` arm
	ContName      string // continuation name `k` for effect handlers
}

// LetInExpr is `let binding in body`.
type LetInExpr struct {
	Mutable  bool // true for `let mutable … in …`
	Bindings []LetBinding
	Body     Expr
	Loc      token.SourceLoc // source location
}

func (*LetInExpr) exprNode() {}

// FunExpr is `fun params -> body`.
type FunExpr struct {
	Params []Param
	Body   Expr
	Loc    token.SourceLoc // source location
}

func (*FunExpr) exprNode() {}

// GuardExpr is `guard pat = expr else expr` (single binding)
// or `guard pat1 = expr1 and pat2 = expr2 else expr` (multiple bindings).
type GuardExpr struct {
	Bindings []GuardBinding // at least one
	Else_    Expr
	Loc      token.SourceLoc // source location
}

// GuardBinding is one arm of a guard expression: `pattern = expr`.
type GuardBinding struct {
	Pattern Pattern
	Expr    Expr
}

func (*GuardExpr) exprNode() {}

// IsExpr is `expr is pattern` (match macro).
type IsExpr struct {
	Left    Expr
	Pattern Pattern
	Loc     token.SourceLoc // source location
}

func (*IsExpr) exprNode() {}

// AsMatchExpr is `expr as pat -> body else elseBody` (match macro).
type AsMatchExpr struct {
	Left     Expr
	Pattern  Pattern
	Body     Expr
	ElseBody Expr
	Loc      token.SourceLoc // source location
}

func (*AsMatchExpr) exprNode() {}

// BinaryExpr is a binary operator expression: `a + b`, `x :: xs`, etc.
type BinaryExpr struct {
	Left  Expr
	Op    token.TokenType
	Right Expr
	Loc   token.SourceLoc // source location
}

func (*BinaryExpr) exprNode() {}

// PipeExpr is a pipeline: `left |> right`.
type PipeExpr struct {
	Left  Expr
	Right Expr
	Loc   token.SourceLoc // source location
}

func (*PipeExpr) exprNode() {}

// QuestionExpr is the error-propagation operator: `expr ?` or `expr ? arg`.
type QuestionExpr struct {
	Left Expr
	Arg  Expr            // nil for bare `?`
	Loc  token.SourceLoc // source location
}

func (*QuestionExpr) exprNode() {}

// RecordExpr is a record literal: `{ x = 1; y = 2 }` or `{ x; y }` (punning).
type RecordExpr struct {
	Fields []RecordField
	Loc    token.SourceLoc // source location
}

func (*RecordExpr) exprNode() {}

// RecordField is one field in a record literal.
type RecordField struct {
	Name  string
	Value Expr // nil for punning
}

// RecordUpdateExpr is `{ expr with field = value; ... }`.
type RecordUpdateExpr struct {
	Base   Expr
	Fields []RecordField   // all have Values set
	Loc    token.SourceLoc // source location
}

func (*RecordUpdateExpr) exprNode() {}

// FieldAccessExpr is `expr.field`.
type FieldAccessExpr struct {
	Left  Expr
	Field string
	Loc   token.SourceLoc // source location
}

func (*FieldAccessExpr) exprNode() {}

// MethodSendExpr is `expr#method`.
type MethodSendExpr struct {
	Target Expr
	Method string
	Loc    token.SourceLoc
}

func (*MethodSendExpr) exprNode() {}

// TupleExpr is a tuple literal: `(a, b, c)`.
type TupleExpr struct {
	Elems []Expr
	Loc   token.SourceLoc // source location
}

func (*TupleExpr) exprNode() {}

// ListExpr is a list literal: `[a; b; c]` or `[]`.
type ListExpr struct {
	Elems []Expr          // nil for `[]`
	Loc   token.SourceLoc // source location
}

func (*ListExpr) exprNode() {}

// ParenExpr is a parenthesised expression: `(expr)`.
type ParenExpr struct {
	Inner Expr
	Loc   token.SourceLoc // source location
}

func (*ParenExpr) exprNode() {}

// IndexExpr is array (or slice) indexing: `arr.(i)`.
type IndexExpr struct {
	Target Expr
	Index  Expr
	Loc    token.SourceLoc
}

func (*IndexExpr) exprNode() {}

// AssignExpr is in-place mutation: `target <- value` (array/field) or `target := value` (ref).
type AssignExpr struct {
	Target  Expr
	Value   Expr
	Coloneq bool // true for :=, false for <-
	Loc     token.SourceLoc
}

func (*AssignExpr) exprNode() {}

// ForExpr is `for var = from to/downto to do body done`.
type ForExpr struct {
	Var  string
	From Expr
	To   Expr
	Down bool // true for downto
	Body Expr
	Loc  token.SourceLoc
}

func (*ForExpr) exprNode() {}

// BeginExpr is `begin e1; e2; ... end`.
type BeginExpr struct {
	Stmts []Expr
	Loc   token.SourceLoc
}

func (*BeginExpr) exprNode() {}

// WhileExpr is `while cond do body done`.
type WhileExpr struct {
	Cond Expr
	Body Expr
	Loc  token.SourceLoc
}

func (*WhileExpr) exprNode() {}

// FunctionExpr is `function | p -> e | ...` (desugars to fun x -> match x with ...).
type FunctionExpr struct {
	Arms []MatchArm
	Loc  token.SourceLoc
}

func (*FunctionExpr) exprNode() {}

// RefExpr is `ref e` — allocate a reference cell.
type RefExpr struct {
	Value Expr
	Loc   token.SourceLoc
}

func (*RefExpr) exprNode() {}

// DerefExpr is `!e` — dereference a ref cell.
type DerefExpr struct {
	Target Expr
	Loc    token.SourceLoc
}

func (*DerefExpr) exprNode() {}

// TryExpr is `try e with | ...` or `try e finally e`.
type TryExpr struct {
	Body    Expr
	Arms    []MatchArm // nil if finally-only
	Finally Expr       // nil if with-only
	Loc     token.SourceLoc
}

func (*TryExpr) exprNode() {}

// RaiseExpr is `raise e`.
type RaiseExpr struct {
	Exn Expr
	Loc token.SourceLoc
}

func (*RaiseExpr) exprNode() {}

// AssertExpr is `assert e`.
type AssertExpr struct {
	Cond Expr
	Loc  token.SourceLoc
}

func (*AssertExpr) exprNode() {}

// LazyExpr is `lazy e`.
type LazyExpr struct {
	Value Expr
	Loc   token.SourceLoc
}

func (*LazyExpr) exprNode() {}

// PerformExpr is `perform e` (OCaml 5 effects).
type PerformExpr struct {
	Op  Expr
	Loc token.SourceLoc
}

func (*PerformExpr) exprNode() {}

// ArrayLitExpr is `[| e1; e2; ... |]`.
type ArrayLitExpr struct {
	Elems []Expr
	Loc   token.SourceLoc
}

func (*ArrayLitExpr) exprNode() {}

// PolyvarExpr is “ `Tag “ or “ `Tag e “.
type PolyvarExpr struct {
	Tag string
	Arg Expr // nil if no payload
	Loc token.SourceLoc
}

func (*PolyvarExpr) exprNode() {}

// ObjectExpr is `object (self) ... end`.
type ObjectExpr struct {
	Self         string
	Fields       []ClassField
	Methods      []ClassMethod
	Inherits     []string
	Initializers []Expr
	Constraints  []ClassConstraint
	Loc          token.SourceLoc
}

func (*ObjectExpr) exprNode() {}

// NewExpr is `new C`.
type NewExpr struct {
	Class string
	Loc   token.SourceLoc
}

func (*NewExpr) exprNode() {}

// LetModuleExpr is `let module M = struct ... end in expr`.
type LetModuleExpr struct {
	Name  string
	Decls []TopDecl
	Body  Expr
	Loc   token.SourceLoc
}

func (*LetModuleExpr) exprNode() {}

// ModuleAppExpr is functor application `F(M)` used as a module RHS.
type ModuleAppExpr struct {
	Func string
	Arg  string
	Loc  token.SourceLoc
}

func (*ModuleAppExpr) exprNode() {}

// LabelledArgExpr is `~x` or `~x:e` used as a function argument.
type LabelledArgExpr struct {
	Label    string
	Value    Expr // nil means punning `~x` ≡ `~x:x`
	Optional bool
	Loc      token.SourceLoc
}

func (*LabelledArgExpr) exprNode() {}

// ExceptionDecl is `exception E` or `exception E of t`.
type ExceptionDecl struct {
	Name string
	Arg  Type // nil if no payload
}

func (*ExceptionDecl) topDeclNode() {}

// EffectDecl is `effect E : a -> b`.
type EffectDecl struct {
	Name string
	From Type
	To   Type
}

func (*EffectDecl) topDeclNode() {}

// NestedModuleDecl is `module M = struct ... end` or a functor
// `module F (X : S) = struct ... end`.
type NestedModuleDecl struct {
	Name       string
	FunctorArg string              // empty if not a functor; param name e.g. "X"
	FunctorSig string              // signature name e.g. "S"
	SealSig    string              // optional result signature in `module M : S = ...`
	Rec        bool                // true for `module rec M = ...`
	RecDecls   []*NestedModuleDecl // peers following `and` in a recursive group
	Decls      []TopDecl
	IsApp      bool   // true for `module M = F(N)` application
	AppFunc    string // functor name when IsApp
	AppArg     string // argument module when IsApp
}

func (*NestedModuleDecl) topDeclNode() {}

// ModuleTypeDecl is `module type S = sig ... end`.
type ModuleTypeDecl struct {
	Name            string
	Items           []SigItem
	OfModule        string          // `module type S = module type of M`
	WithConstraints []SigConstraint // `S with type ...` / `with module ...`
}

func (*ModuleTypeDecl) topDeclNode() {}

// SigItem is one item inside a `sig ... end` (minimal: val / type).
type SigItem struct {
	Kind string // "val" or "type"
	Name string
	Type Type // for val; nil for type
}

// SigConstraint preserves `with type/module` refinements on module signatures.
// The checker currently uses them as transparent compatibility constraints.
type SigConstraint struct {
	Kind        string // "type" or "module"
	Name        string
	Manifest    string
	Destructive bool // `:=` rather than `=`
}

// OpenModuleDecl is `open M` or `open! M` (module path).
type OpenModuleDecl struct {
	Path  string
	Force bool // open!
}

func (*OpenModuleDecl) topDeclNode() {}

// IncludeDecl is `include M`.
type IncludeDecl struct {
	Path string
}

func (*IncludeDecl) topDeclNode() {}

// LetOpenExpr is `let open M in body` (or `let open! M in body`).
type LetOpenExpr struct {
	Path  string
	Force bool
	Body  Expr
	Loc   token.SourceLoc
}

func (*LetOpenExpr) exprNode() {}

// LocalOpenExpr is `M.( body )` — expression-scoped open of module M.
type LocalOpenExpr struct {
	Path string
	Body Expr
	Loc  token.SourceLoc
}

func (*LocalOpenExpr) exprNode() {}

// ContinueExpr is `continue k x` (effect handler resume sugar).
type ContinueExpr struct {
	Cont Expr
	Arg  Expr
	Loc  token.SourceLoc
}

func (*ContinueExpr) exprNode() {}

// DiscontinueExpr is `discontinue k exn` (effect handler abort sugar).
type DiscontinueExpr struct {
	Cont Expr
	Exn  Expr
	Loc  token.SourceLoc
}

func (*DiscontinueExpr) exprNode() {}

// PackModuleExpr is `(module M : S)` — pack a module as a first-class value.
type PackModuleExpr struct {
	Module string
	Sig    string // optional signature name
	Loc    token.SourceLoc
}

func (*PackModuleExpr) exprNode() {}

// UnpackModuleExpr is `(val e : S)` — unpack a first-class module.
type UnpackModuleExpr struct {
	Value Expr
	Sig   string
	Loc   token.SourceLoc
}

func (*UnpackModuleExpr) exprNode() {}

// ExceptionPattern is `exception P` in match/try arms.
type ExceptionPattern struct {
	Pattern Pattern
}

func (*ExceptionPattern) patternNode() {}

// LazyPattern is `lazy P`.
type LazyPattern struct {
	Pattern Pattern
}

func (*LazyPattern) patternNode() {}

// Attribute is a parsed but stripped OCaml-style attribute/extension.
type Attribute struct {
	Name     string
	Payload  string // raw payload text
	Attached string // "item" | "expr" | "type" | "ext"
}

// AttrStrip holds attributes removed before typecheck (parse+strip).
type AttrStrip struct {
	Attrs []Attribute
}

// ClassDecl is `class [virtual] c = object (self) ... end` or `class type`.
type ClassDecl struct {
	Name         string
	Self         string // optional self name
	Fields       []ClassField
	Methods      []ClassMethod
	Inherits     []string
	Initializers []Expr
	Constraints  []ClassConstraint
	Virtual      bool
	TypeOnly     bool
}

func (*ClassDecl) topDeclNode() {}

// ClassField is `val [mutable] x = e` inside an object.
type ClassField struct {
	Name    string
	Mutable bool
	Value   Expr
}

// ClassMethod is `method [private] [virtual] name ...`.
type ClassMethod struct {
	Name    string
	Params  []Param
	Body    Expr
	Type    Type // declared type for virtual methods
	Private bool
	Virtual bool
}

// ClassConstraint is `constraint left = right`.
type ClassConstraint struct {
	Left  Type
	Right Type
}

// CompExpr is a computation expression: `builder { ops }` (removed in 1.0; kept for AST compat during migration errors).
type CompExpr struct {
	Builder string // e.g. "result", "async"
	Ops     []CompOp
	Loc     token.SourceLoc // source location
}

func (*CompExpr) exprNode() {}

// CompOp is one operation inside a computation expression.
type CompOp interface {
	compOpNode()
}

// LetBangOp is `let! pattern = expr` inside a CE.
type LetBangOp struct {
	Pattern Pattern
	Expr    Expr
}

func (*LetBangOp) compOpNode() {}

// DoBangOp is `do! expr` inside a CE.
type DoBangOp struct {
	Expr Expr
}

func (*DoBangOp) compOpNode() {}

// LetOp is `let pattern = expr` inside a CE.
type LetOp struct {
	Pattern Pattern
	Expr    Expr
}

func (*LetOp) compOpNode() {}

// ReturnOp is `return expr` inside a CE.
type ReturnOp struct {
	Expr Expr
}

func (*ReturnOp) compOpNode() {}

// ReturnBangOp is `return! expr` inside a CE.
type ReturnBangOp struct {
	Expr Expr
}

func (*ReturnBangOp) compOpNode() {}

// BodyOp is the final expression in a CE (no keyword).
type BodyOp struct {
	Expr Expr
}

func (*BodyOp) compOpNode() {}

// GoExpr is a `go expr` or `go (move x, ...) expr` expression.
type GoExpr struct {
	Moved []string // optional move list
	Expr  Expr
	Loc   token.SourceLoc // source location
}

func (*GoExpr) exprNode() {}

// SelectExpr is a `select { case x = expr -> body ... }` expression.
type SelectExpr struct {
	Cases   []SelectCase
	Default Expr            // nil if no default
	Loc     token.SourceLoc // source location
}

func (*SelectExpr) exprNode() {}

// UsingExpr is a `using pat = expr in body` expression.
type UsingExpr struct {
	Pattern Pattern
	Expr    Expr
	Body    Expr
	Loc     token.SourceLoc // source location
}

func (*UsingExpr) exprNode() {}

// RegionExpr is a desugared `region { ops }` computation expression for
// scoped linear resource management. Each let! binding emits a defer
// Close(varName) in codegen, and the linear checker auto-discharges
// any live linear variables at the region exit.
type RegionExpr struct {
	Ops []CompOp
	Loc token.SourceLoc // source location
}

func (*RegionExpr) exprNode() {}

// SelectCase is one case in a select expression.
type SelectCase struct {
	Bind string // variable binding for receive case
	Recv Expr   // the expression to receive from (of channel type)
	Body Expr
}

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

// Pattern is the interface for match patterns.
type Pattern interface {
	patternNode()
}

// WildcardPattern is `_`.
type WildcardPattern struct{}

func (*WildcardPattern) patternNode() {}

// IdentPattern is a variable pattern: `x`, captures a value.
type IdentPattern struct {
	Name string
}

func (*IdentPattern) patternNode() {}

// LitPattern matches a literal: `42`, `"hello"`, `true`.
type LitPattern struct {
	Value any
	Kind  token.TokenType
}

func (*LitPattern) patternNode() {}

// ConstructorPattern matches a constructor: `None`, `Some x`, `Circle { radius }`.
type ConstructorPattern struct {
	Name       string
	TypePrefix string  // optional ADT qualifier: Type.Ctor
	Arg        Pattern // nil when no payload
}

func (*ConstructorPattern) patternNode() {}

// PolyvarPattern matches “ `Tag “ or “ `Tag p “.
type PolyvarPattern struct {
	Tag string
	Arg Pattern
}

func (*PolyvarPattern) patternNode() {}

// OrPattern is `| A | B` (or-pattern).
type OrPattern struct {
	Left  Pattern
	Right Pattern
}

func (*OrPattern) patternNode() {}

// RecordPattern matches a record: `{ x; y }` or `{ x = pat }`.
type RecordPattern struct {
	Fields []RecordPatField
}

func (*RecordPattern) patternNode() {}

// RecordPatField is a field within a record pattern.
type RecordPatField struct {
	Name    string
	Pattern Pattern // nil for punning (binds to same name)
}

// TuplePattern matches a tuple: `(a, b)`.
type TuplePattern struct {
	Elems []Pattern
}

func (*TuplePattern) patternNode() {}

// ListPattern matches a list: `[]` or `[a; b]`.
type ListPattern struct {
	Elems []Pattern // nil for `[]`
}

func (*ListPattern) patternNode() {}

// ConsPattern matches `head :: tail`.
type ConsPattern struct {
	Head Pattern
	Tail Pattern
}

func (*ConsPattern) patternNode() {}

// AliasPattern is `pat as name`.
type AliasPattern struct {
	Pattern Pattern
	Name    string
}

func (*AliasPattern) patternNode() {}

// ---------------------------------------------------------------------------
// Location container (optional — used by nodes that carry source info)
// ---------------------------------------------------------------------------

// Located wraps a node with source location.
type Located[T any] struct {
	Node T
	Loc  token.SourceLoc
}

// ---------------------------------------------------------------------------
// Pretty-printing helpers (for debugging / parser tests)
// ---------------------------------------------------------------------------

// typeString converts a Type to a human-readable string.
func typeString(t Type) string {
	switch t := t.(type) {
	case *TIdent:
		return t.Name
	case *TApp:
		return fmt.Sprintf("%s(%s)", typeString(t.Func), typeString(t.Arg))
	case *TFun:
		s := typeString(t.From) + " -> " + typeString(t.To)
		if t.Effects != nil {
			effs := strings.Join(t.Effects.Effects, "; ")
			if t.Effects.Open {
				if t.Effects.Rest != "" {
					effs += " | " + t.Effects.Rest
				} else {
					effs += " | .."
				}
			}
			if effs != "" {
				s += " with {" + effs + "}"
			}
		}
		return s
	case *TTuple:
		parts := make([]string, len(t.Elems))
		for i, e := range t.Elems {
			parts[i] = typeString(e)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *TRecord:
		parts := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			parts[i] = f.Name + ": " + typeString(f.Type)
		}
		s := "{ " + strings.Join(parts, "; ") + " "
		if t.Open {
			s += "| .. "
		}
		return s + "}"
	case *TObject:
		parts := make([]string, len(t.Methods))
		for i, m := range t.Methods {
			parts[i] = m.Name + ": " + typeString(m.Type)
		}
		s := "< " + strings.Join(parts, "; ")
		if t.Open {
			s += " | .."
		}
		return s + " >"
	case *TChan:
		return typeString(t.Elem) + " chan"
	case *TMap:
		return "map[" + typeString(t.Key) + "] " + typeString(t.Val)
	case *RefinementType:
		return typeString(t.Inner) + " where " + ExprString(t.Pred)
	case *TVar:
		return t.Name
	default:
		return "<unknown type>"
	}
}

// ExprString converts an Expr to a human-readable string.
func ExprString(e Expr) string {
	switch e := e.(type) {
	case *LitExpr:
		return fmt.Sprintf("%v", e.Value)
	case *IdentExpr:
		return e.Name
	case *ConstructorExpr:
		name := e.Name
		if e.TypePrefix != "" {
			name = e.TypePrefix + "." + name
		}
		if e.Arg != nil {
			return name + " " + ExprString(e.Arg)
		}
		return name
	case *AppExpr:
		return "App(" + ExprString(e.Func) + ", " + ExprString(e.Arg) + ")"
	case *IfExpr:
		return fmt.Sprintf("If(%s, %s, %s)", ExprString(e.Cond), ExprString(e.ThenBranch), ExprString(e.ElseBranch))
	case *MatchExpr:
		arms := make([]string, len(e.Arms))
		for i, a := range e.Arms {
			arms[i] = patternString(a.Pattern) + " -> " + ExprString(a.Body)
		}
		return "Match(" + ExprString(e.Scrutinee) + ", [" + strings.Join(arms, " | ") + "])"
	case *LetInExpr:
		bs := make([]string, len(e.Bindings))
		for i, b := range e.Bindings {
			bs[i] = b.Name + " = " + ExprString(b.Body)
		}
		return "Let([" + strings.Join(bs, "; ") + "], " + ExprString(e.Body) + ")"
	case *FunExpr:
		ps := make([]string, len(e.Params))
		for i, p := range e.Params {
			ps[i] = p.Name
		}
		return "Fun([" + strings.Join(ps, ", ") + "], " + ExprString(e.Body) + ")"
	case *GuardExpr:
		bs := make([]string, len(e.Bindings))
		for i, b := range e.Bindings {
			bs[i] = patternString(b.Pattern) + " = " + ExprString(b.Expr)
		}
		return "Guard(" + strings.Join(bs, " and ") + " else " + ExprString(e.Else_) + ")"
	case *IsExpr:
		return "Is(" + ExprString(e.Left) + ", " + patternString(e.Pattern) + ")"
	case *AsMatchExpr:
		return "AsMatch(" + ExprString(e.Left) + ", " + patternString(e.Pattern) + " -> " + ExprString(e.Body) + " else " + ExprString(e.ElseBody) + ")"
	case *BinaryExpr:
		return fmt.Sprintf("Bin(%s, %s, %s)", ExprString(e.Left), e.Op, ExprString(e.Right))
	case *PipeExpr:
		return "Pipe(" + ExprString(e.Left) + " |> " + ExprString(e.Right) + ")"
	case *QuestionExpr:
		if e.Arg != nil {
			return "Q(" + ExprString(e.Left) + " ? " + ExprString(e.Arg) + ")"
		}
		return "Q(" + ExprString(e.Left) + " ?)"
	case *RecordExpr:
		parts := make([]string, len(e.Fields))
		for i, f := range e.Fields {
			if f.Value != nil {
				parts[i] = f.Name + " = " + ExprString(f.Value)
			} else {
				parts[i] = f.Name
			}
		}
		return "{" + strings.Join(parts, "; ") + "}"
	case *RecordUpdateExpr:
		parts := make([]string, len(e.Fields))
		for i, f := range e.Fields {
			parts[i] = f.Name + " = " + ExprString(f.Value)
		}
		return "{" + ExprString(e.Base) + " with " + strings.Join(parts, "; ") + "}"
	case *FieldAccessExpr:
		return ExprString(e.Left) + "." + e.Field
	case *TupleExpr:
		parts := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			parts[i] = ExprString(el)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *ListExpr:
		parts := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			parts[i] = ExprString(el)
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case *ParenExpr:
		return "(" + ExprString(e.Inner) + ")"
	case *IndexExpr:
		return ExprString(e.Target) + ".(" + ExprString(e.Index) + ")"
	case *AssignExpr:
		op := " <- "
		if e.Coloneq {
			op = " := "
		}
		return ExprString(e.Target) + op + ExprString(e.Value)
	case *ForExpr:
		kw := "to"
		if e.Down {
			kw = "downto"
		}
		return fmt.Sprintf("for %s = %s %s %s do %s done", e.Var, ExprString(e.From), kw, ExprString(e.To), ExprString(e.Body))
	case *LetOpenExpr:
		bang := ""
		if e.Force {
			bang = "!"
		}
		return fmt.Sprintf("let open%s %s in %s", bang, e.Path, ExprString(e.Body))
	case *LocalOpenExpr:
		return e.Path + ".(" + ExprString(e.Body) + ")"
	case *ContinueExpr:
		return "continue " + ExprString(e.Cont) + " " + ExprString(e.Arg)
	case *DiscontinueExpr:
		return "discontinue " + ExprString(e.Cont) + " " + ExprString(e.Exn)
	case *BeginExpr:
		parts := make([]string, len(e.Stmts))
		for i, s := range e.Stmts {
			parts[i] = ExprString(s)
		}
		return "begin " + strings.Join(parts, "; ") + " end"
	case *CompExpr:
		ops := make([]string, len(e.Ops))
		for i, op := range e.Ops {
			switch o := op.(type) {
			case *LetBangOp:
				ops[i] = fmt.Sprintf("let! %s = %s", patternString(o.Pattern), ExprString(o.Expr))
			case *DoBangOp:
				ops[i] = fmt.Sprintf("do! %s", ExprString(o.Expr))
			case *LetOp:
				ops[i] = fmt.Sprintf("let %s = %s", patternString(o.Pattern), ExprString(o.Expr))
			case *ReturnOp:
				ops[i] = fmt.Sprintf("return %s", ExprString(o.Expr))
			case *ReturnBangOp:
				ops[i] = fmt.Sprintf("return! %s", ExprString(o.Expr))
			case *BodyOp:
				ops[i] = ExprString(o.Expr)
			}
		}
		return e.Builder + " { " + strings.Join(ops, "; ") + " }"
	case *GoExpr:
		if len(e.Moved) > 0 {
			return "go (move " + strings.Join(e.Moved, ", ") + ") " + ExprString(e.Expr)
		}
		return "go " + ExprString(e.Expr)
	case *SelectExpr:
		cases := make([]string, len(e.Cases))
		for i, c := range e.Cases {
			cases[i] = "case " + c.Bind + " = " + ExprString(c.Recv) + " -> " + ExprString(c.Body)
		}
		s := "select { " + strings.Join(cases, "; ")
		if e.Default != nil {
			s += "; default -> " + ExprString(e.Default)
		}
		return s + " }"
	case *UsingExpr:
		return "using " + patternString(e.Pattern) + " = " + ExprString(e.Expr) + " in " + ExprString(e.Body)
	case *RegionExpr:
		ops := make([]string, len(e.Ops))
		for i, op := range e.Ops {
			switch o := op.(type) {
			case *LetBangOp:
				ops[i] = fmt.Sprintf("let! %s = %s", patternString(o.Pattern), ExprString(o.Expr))
			case *DoBangOp:
				ops[i] = fmt.Sprintf("do! %s", ExprString(o.Expr))
			case *LetOp:
				ops[i] = fmt.Sprintf("let %s = %s", patternString(o.Pattern), ExprString(o.Expr))
			case *ReturnOp:
				ops[i] = fmt.Sprintf("return %s", ExprString(o.Expr))
			case *ReturnBangOp:
				ops[i] = fmt.Sprintf("return! %s", ExprString(o.Expr))
			case *BodyOp:
				ops[i] = ExprString(o.Expr)
			}
		}
		return "region { " + strings.Join(ops, "; ") + " }"
	case *WhileExpr:
		return fmt.Sprintf("while %s do %s done", ExprString(e.Cond), ExprString(e.Body))
	case *FunctionExpr:
		arms := make([]string, len(e.Arms))
		for i, a := range e.Arms {
			arms[i] = patternString(a.Pattern) + " -> " + ExprString(a.Body)
		}
		return "function [" + strings.Join(arms, " | ") + "]"
	case *RefExpr:
		return "ref " + ExprString(e.Value)
	case *DerefExpr:
		return "!" + ExprString(e.Target)
	case *TryExpr:
		s := "try " + ExprString(e.Body)
		if len(e.Arms) > 0 {
			arms := make([]string, len(e.Arms))
			for i, a := range e.Arms {
				arms[i] = patternString(a.Pattern) + " -> " + ExprString(a.Body)
			}
			s += " with [" + strings.Join(arms, " | ") + "]"
		}
		if e.Finally != nil {
			s += " finally " + ExprString(e.Finally)
		}
		return s
	case *RaiseExpr:
		return "raise " + ExprString(e.Exn)
	case *AssertExpr:
		return "assert " + ExprString(e.Cond)
	case *LazyExpr:
		return "lazy " + ExprString(e.Value)
	case *PerformExpr:
		return "perform " + ExprString(e.Op)
	case *ArrayLitExpr:
		parts := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			parts[i] = ExprString(el)
		}
		return "[|" + strings.Join(parts, "; ") + "|]"
	case *PolyvarExpr:
		if e.Arg != nil {
			return "`" + e.Tag + " " + ExprString(e.Arg)
		}
		return "`" + e.Tag
	case *ObjectExpr:
		return "object ... end"
	case *MethodSendExpr:
		return ExprString(e.Target) + "#" + e.Method
	case *NewExpr:
		return "new " + e.Class
	case *LetModuleExpr:
		return "let module " + e.Name + " = struct ... end in " + ExprString(e.Body)
	case *ModuleAppExpr:
		return e.Func + "(" + e.Arg + ")"
	case *LabelledArgExpr:
		if e.Value != nil {
			return "~" + e.Label + ":" + ExprString(e.Value)
		}
		return "~" + e.Label
	default:
		return "<unknown expr>"
	}
}

// ExprLoc returns the source location of an expression, or zero if unknown.
func ExprLoc(e Expr) token.SourceLoc {
	if e == nil {
		return token.SourceLoc{}
	}
	switch e := e.(type) {
	case *LitExpr:
		return e.Loc
	case *IdentExpr:
		return e.Loc
	case *ConstructorExpr:
		return e.Loc
	case *AppExpr:
		return e.Loc
	case *IfExpr:
		return e.Loc
	case *MatchExpr:
		return e.Loc
	case *LetInExpr:
		return e.Loc
	case *FunExpr:
		return e.Loc
	case *GuardExpr:
		return e.Loc
	case *IsExpr:
		return e.Loc
	case *AsMatchExpr:
		return e.Loc
	case *BinaryExpr:
		return e.Loc
	case *PipeExpr:
		return e.Loc
	case *QuestionExpr:
		return e.Loc
	case *RecordExpr:
		return e.Loc
	case *RecordUpdateExpr:
		return e.Loc
	case *FieldAccessExpr:
		return e.Loc
	case *MethodSendExpr:
		return e.Loc
	case *TupleExpr:
		return e.Loc
	case *ListExpr:
		return e.Loc
	case *ParenExpr:
		return e.Loc
	case *IndexExpr:
		return e.Loc
	case *AssignExpr:
		return e.Loc
	case *ForExpr:
		return e.Loc
	case *LetOpenExpr:
		return e.Loc
	case *LocalOpenExpr:
		return e.Loc
	case *ContinueExpr:
		return e.Loc
	case *DiscontinueExpr:
		return e.Loc
	case *PackModuleExpr:
		return e.Loc
	case *UnpackModuleExpr:
		return e.Loc
	case *BeginExpr:
		return e.Loc
	case *WhileExpr:
		return e.Loc
	case *FunctionExpr:
		return e.Loc
	case *RefExpr:
		return e.Loc
	case *DerefExpr:
		return e.Loc
	case *TryExpr:
		return e.Loc
	case *RaiseExpr:
		return e.Loc
	case *AssertExpr:
		return e.Loc
	case *LazyExpr:
		return e.Loc
	case *PerformExpr:
		return e.Loc
	case *ArrayLitExpr:
		return e.Loc
	case *PolyvarExpr:
		return e.Loc
	case *ObjectExpr:
		return e.Loc
	case *NewExpr:
		return e.Loc
	case *LetModuleExpr:
		return e.Loc
	case *ModuleAppExpr:
		return e.Loc
	case *LabelledArgExpr:
		return e.Loc
	case *GoExpr:
		return e.Loc
	case *SelectExpr:
		return e.Loc
	case *UsingExpr:
		return e.Loc
	case *RegionExpr:
		return e.Loc
	case *CompExpr:
		return e.Loc
	default:
		return token.SourceLoc{}
	}
}

// patternString converts a Pattern to a human-readable string.
func patternString(p Pattern) string {
	switch p := p.(type) {
	case *WildcardPattern:
		return "_"
	case *IdentPattern:
		return p.Name
	case *LitPattern:
		return fmt.Sprintf("%v", p.Value)
	case *ConstructorPattern:
		name := p.Name
		if p.TypePrefix != "" {
			name = p.TypePrefix + "." + name
		}
		if p.Arg != nil {
			return name + " " + patternString(p.Arg)
		}
		return name
	case *RecordPattern:
		parts := make([]string, len(p.Fields))
		for i, f := range p.Fields {
			if f.Pattern != nil {
				parts[i] = f.Name + " = " + patternString(f.Pattern)
			} else {
				parts[i] = f.Name
			}
		}
		return "{" + strings.Join(parts, "; ") + "}"
	case *TuplePattern:
		parts := make([]string, len(p.Elems))
		for i, el := range p.Elems {
			parts[i] = patternString(el)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *ListPattern:
		if len(p.Elems) == 0 {
			return "[]"
		}
		parts := make([]string, len(p.Elems))
		for i, el := range p.Elems {
			parts[i] = patternString(el)
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case *ConsPattern:
		return patternString(p.Head) + " :: " + patternString(p.Tail)
	case *AliasPattern:
		return patternString(p.Pattern) + " as " + p.Name
	case *OrPattern:
		return patternString(p.Left) + " | " + patternString(p.Right)
	default:
		return "<unknown pattern>"
	}
}
