# Goop Grammar (1.0 — OCaml-aligned)

High-level EBNF for the Goop 1.0 surface. Not a complete LALR(1) grammar.

## Lexical elements

```
ident    := [a-zA-Z_][a-zA-Z0-9_']*
tyvar    := '\'' ident
constr   := [A-Z][a-zA-Z0-9_']*
polyvar  := '`' constr
integer  := [0-9]+
float    := [0-9]+ '.' [0-9]* | [0-9]* '.' [0-9]+
string   := '"' (str_char | escape)* '"'
char     := '\'' (char_char | escape) '\''
escape   := '\' ( 'n' | 't' | 'r' | '\\' | '"' | "'" | 'e'
                | 'x' hex hex | oct oct? oct? )
hex      := [0-9a-fA-F]
oct      := [0-7]
(* \e = ESC (0x1b); \xHH = byte; \ooo = 1–3 octal digits, value ≤ 255 *)
unit     := '()'
line_comment := '//' [^\n]*
block_comment := '(*' … '*)'   (* nestable *)
```

## Reserved words

```
and as assert begin class constraint do done else end effect exception
false for fun function go goop if in include inherit initializer
implements lazy let match method mod module move mutable new null object of open
perform private raise rec sig struct then to true try type val virtual
when while with
```

Removed as keywords (parse migration errors): `guard`, `panic`, `region`, `async` (as CE builder), `newtype`, `golang`.

## Program structure

```
program      := module_decl import_decl* top_decl*
module_decl  := 'module' constr ('.' constr)*
import_decl  := 'import' import_spec | 'import' '(' import_spec* ')'
import_spec  := ident? ('go' | 'goop') string import_vals?
import_vals  := '{' import_item* '}'
import_item  := 'val' ident ':' type
              | 'val' '(' ident ':' type ')' '.' ident ':' type
              | 'type' constr

top_decl     := val_decl | type_decl | exception_decl | effect_decl
              | nested_module | module_type_decl | class_decl
              | lang_embed_decl | implements_decl

implements_decl := 'implements' constr 'for' constr 'with' impl_method* 'end'
impl_method     := 'let' ident param+ (':' type)? '=' expr

lang_embed_decl := '@[' ('go' | 'c') ']' '{' raw_body '}' ('val' ident ':' type)*
exception_decl    := 'exception' constr ('of' type)?
effect_decl       := 'effect' constr ':' type '->' type
```

## Nested modules

```
nested_module    := 'module' constr functor_params? '=' module_expr
module_type_decl := 'module' 'type' constr '=' module_type
functor_params   := '(' constr ':' module_type ')'*
module_expr      := 'struct' top_decl* 'end'
                  | constr functor_app*
                  | 'functor' '(' constr ':' module_type ')' '->' module_expr
module_type      := 'sig' sig_item* 'end' | constr
functor_app      := '(' module_expr ')'
sig_item         := 'val' ident ':' type | 'type' type_binding | 'exception' …
                  | 'module' constr ':' module_type | 'open' constr
```

`.mli` files are **not** part of Goop 1.1 — seal modules inline with `module M : S = …`.

## Value declarations

```
val_decl     := 'let' rec? binding ('and' binding)*
binding      := ident labeled_param* (':' type)? '=' expr
labeled_param := param | '~' ident | '?' ident | '?' '(' ident '=' expr ')'
param        := ident | '(' ident ':' type ')'

expr         := if_expr | match_expr | try_expr | let_expr | fun_expr
              | function_expr | while_expr | for_expr | begin_expr
              | lazy_expr | assert_expr | raise_expr | perform_expr
              | object_expr | new_expr | let_module_expr | binary_expr

if_expr      := 'if' expr 'then' expr 'else' expr
match_expr   := 'match' expr 'with' '|'? case ('|' case)*
case         := pattern ('when' expr)? '->' expr
              | 'effect' '(' constr pattern? ')' ident '->' expr
try_expr     := 'try' expr 'with' '|'? case ('|' case)*
              | 'try' expr 'finally' expr
let_expr     := 'let' binding ('and' binding)* 'in' expr
              | 'let' '*' binding 'in' expr
              | 'let' '+' binding 'in' expr
fun_expr     := 'fun' labeled_param+ '->' expr
function_expr:= 'function' '|'? case ('|' case)*
while_expr   := 'while' expr 'do' expr 'done'
for_expr     := 'for' ident '=' expr 'to' expr 'do' expr 'done'
begin_expr   := 'begin' expr (';' expr)* 'end'
lazy_expr    := 'lazy' expr
assert_expr  := 'assert' expr
raise_expr   := 'raise' expr
perform_expr := 'perform' expr
let_module_expr := 'let' 'module' constr '=' module_expr 'in' expr
object_expr  := 'object' ('(' ident ')')? object_item* 'end'
new_expr     := 'new' constr expr*
go_expr      := 'go' ('(' 'move' ident (',' ident)* ')')? expr

binary_expr  := pipeline_expr
pipeline_expr:= app_expr ('|>' app_expr)*
app_expr     := primary_expr primary_expr*
primary_expr := literal | ident | constr | polyvar | qualified_constr
              | '(' expr ')' | record_expr | list_expr | array_lit
              | tuple_expr | field_expr | index_expr | go_expr
              | deref_expr | ref_expr

deref_expr   := '!' expr
ref_expr     := 'ref' expr
index_expr   := expr '.(' expr ')'
assign_expr  := expr ':=' expr | expr '.(' expr ')' '<-' expr
array_lit    := '[|' (expr (';' expr)*)? '|]'
qualified_constr := constr '.' constr

literal      := integer | float | string | char | 'true' | 'false' | '()'
record_expr  := '{' field_init (';' field_init)* '}'
field_init   := ident '=' expr | ident
list_expr    := '[' (expr (';' expr)*)? ']'
tuple_expr   := '(' expr (',' expr)+ ')'
field_expr   := expr '.' ident
```

## Patterns

```
pattern      := or_pattern
or_pattern   := constr_pattern ('|' constr_pattern)*
constr_pattern := constr pattern? | polyvar pattern? | record_pattern
              | tuple_pattern | list_pattern | array_pattern
              | literal_pattern | ident_pattern | wildcard_pattern
              | alias_pattern | lazy_pattern | exception_pattern

record_pattern := '{' field_pattern (';' field_pattern)* ('|' '..')? '}'
array_pattern  := '[|' (pattern (';' pattern)*)? '|]'
lazy_pattern   := 'lazy' pattern
exception_pattern := 'exception' pattern
alias_pattern  := pattern 'as' ident
```

## Type expressions

```
type         := fn_type
fn_type      := labeled_type ('->' fn_type)?
labeled_type := ('~' ident ':' | '?' ident ':')? refined_type
refined_type := product_type ('where' expr)?
product_type := app_type ('*' app_type)*
app_type     := primary_type primary_type*
primary_type := ident | tyvar | '(' type ')' | type 'array' | type 'ref'
              | type 'ptr' | type 'go_slice' | 'error'
              | '{' field_type (';' field_type)* ('|' '..')? '}'
              | '<' method_type (';' method_type)* ('|' '..')? '>'
              | poly_variant_type | '(' type (',' type)+ ')'
              | '...' type   (* variadic FFI param type *)

field_type   := 'mutable'? ident ':' type
method_type  := ident ':' type
poly_variant_type := '[' ['>'|'<']? poly_case ('|' poly_case)* ']'
poly_case    := polyvar ('of' type)?
```

## Type declarations

```
type_decl    := 'type' type_binding ('and' type_binding)*
type_binding := 'private'? ident type_param* linear? '=' type_rhs
              | 'private'? ident type_param* linear
linear       := ':' '1'
type_param   := tyvar | '_'
type_rhs     := record_type | adt_type | gadt_type | type
record_type  := '{' field_type (';' field_type)* '}'
adt_type     := '|'? adt_case ('|' adt_case)*
adt_case     := constr ('of' type)?
gadt_type    := '|'? gadt_case ('|' gadt_case)*
gadt_case    := constr (':' type)?   (* constr : args -> ret *)
```

## Classes (OCaml OOP)

```
class_decl   := 'class' 'virtual'? constr type_param* '=' class_expr
class_expr   := 'object' ('(' ident ')')? object_item* 'end'
object_item  := 'val' 'mutable'? ident '=' expr
              | 'method' 'private'? 'virtual'? ident labeled_param* '=' expr
              | 'inherit' class_expr
              | 'initializer' expr
              | 'constraint' type '=' type
```

## Operators

```
mod land lor lxor          (* integer; % removed *)
+. -. *. /.                (* float *)
^                          (* string concat *)
= <> < > <= >=             (* compare *)
&& || not
|>                         (* pipeline *)
:=                         (* ref assign *)
<-                         (* array / mutable field only *)
```

## Migration parse errors (removed in 1.0)

| Old | Error | Use instead |
|-----|-------|-------------|
| `let mutable` | PARSE-MIG010 | `ref` / `:=` / `!` |
| `x <- e` (non-array) | TYPE011 | `x := e` on refs; keep `arr.(i) <- e` |
| `e ?` | PARSE-MIG012 | `match` on `result` |
| `result { }` / `async { }` / `region { }` | PARSE-MIG013 | `match` on `result` / `try`/`finally` |
| `e is p` / `e as p ->` / `guard` | PARSE-MIG014 | `match` |
| `type t = newtype r` | PARSE-MIG015 | single-ctor ADT + `private` |
| `… with { io }` | PARSE-MIG016 | effect handlers |
| `panic` | PARSE-MIG017 | `failwith` / `raise` |
| `n % m` | PARSE-MIG018 | `n mod m` |

Legacy `extern "go"` and bare `open` (pre-v0.3) remain parse errors. Nested `open M` inside modules is restored as OCaml `open`.
