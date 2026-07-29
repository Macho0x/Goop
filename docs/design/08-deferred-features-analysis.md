# Analysis: Resumptive Effects and Dependent/Refinement Types in Goop

> **Historical analysis (pre-1.0).** Mid-document reject/defer recommendations and old snippets (`let mutable`, `?`, `with { io }`) are superseded by the **1.0 summary verdicts** at the end of this file and by [STYLE.md](STYLE.md) / [14-ocaml-parity.md](14-ocaml-parity.md). Read the closing table first if you only need current product decisions.

This document analyzes two features that were once deferred to inform whether — and how — they could be added to Goop. Each section covers: what the feature is, feasibility under the Go-lowering constraint, concrete syntax, type-system integration, lowering strategy, cost, comparison to other languages, and a recommendation.

---

## 1. Resumptive Effect System

### 1.1 What it is

A **resumptive effect system** treats computational effects (IO, state, nondeterminism, exceptions, generators) as first-class citizens of the type system. Unlike monads (Haskell) or effect annotations (Goop's current `[@io]`), a resumptive system allows a handler to intercept an effect operation, perform some action, and then *resume* the suspended computation with a result — potentially more than once. This is the model of **Koka**, **Eff**, **Unison**, and **Multicore OCaml 5**'s effect handlers.

Minimal example from Koka:

```
effect ask() : int
fun main() { handle(handler { ask() -> resume(42) }) { println(ask() + ask()) } }
```

The computation calls `ask()` twice. The handler intercepts each `ask()` and resumes with `42`. The result is `84`. This is not achievable with monads (no resumption) or simple annotations (no type-level tracking).

### 1.2 The Go-lowering constraint: the single biggest obstacle for Goop

**Go has no continuations. There is no way to capture or resume part of the stack at runtime.** This is the absolute binding constraint. Any resumptive effect system must be compiled *entirely away* into something Go can express.

There are exactly four viable lowering strategies for resumptive effects targeting a language without runtime support:

| Strategy | Mechanism | Go feasibility | Idiomatic Go? |
|---|---|---|---|
| **CPS transform** | Every function returns a callback `func(T) R` instead of `T`. Continuations are closures. | ✓ Works, Go has closures | ✗ Produces deeply nested callback chains; unreadable |
| **Free monad** | Effects become constructors of an AST; handlers are interpreters. Composition via `Bind`. | ✓ Works, everything is data | ✗ Deeply nested closures; no syntactic support; massive allocation pressure |
| **Capability passing** | Effectful functions receive explicit handler objects. No resumption — handlers are just injected interfaces. | ✓ Works, uses Go interfaces | ✓ Handlers are structs implementing methods |
| **State-machine lowering** | Compute a control-flow graph with yield/return points. Lower each block to a case in a loop+switch. | ✓ Works, pattern used in Go iterators | ✓ Familiar pattern (e.g., `database/sql.Rows`, Go 1.23 iterators) |

Let me compare these concretely.

#### Strategy A: CPS transform

```goop
effect ask : unit -> int

let compute () : int =
  let a = ask () in
  let b = ask () in
  a + b
```

CPS-lowered Go (sketch):

```go
func compute(k func(int)) {
    // ask is a continuation-based operation:
    // It calls some global handler with a continuation.
    globalAskHandler(func(a int) {
        globalAskHandler(func(b int) {
            k(a + b)
        })
    })
}
```

**Verdict**: It works. Go closures can express this. But the output violates Goop's core constraint: *"the Go a senior engineer would write by hand."* No senior Go engineer would ever write CPS-transformed Go. It is unreadable and undebuggable.

#### Strategy B: Free monad

```go
type Comp[A] interface { /* Bind-like */ }

type Pure[A struct] { val A }
type Bind[F, A, B any] struct {
    comp Comp[A]
    f    func(A) Comp[B]
}
```

Functions return `Comp[T]`; handlers interpret `Comp[T]` via recursion. Same verdict as CPS — violates the idiomatic-Go constraint, plus massive allocation overhead for every effect operation.

#### Strategy C: Capability passing (no resumption)

This is what Rust's effect system looks like in practice — and what Goop's `extern` boundary already anticipates. An effect is a trait/interface; a handler is an implementation; you pass capabilities explicitly (or implicitly via implicit parameters).

```goop
effect Console {
  val print : string -> unit
  val read_line : unit -> string
}

let greet (implicit c: Console) (name: string) : unit =
  c.print ("Hello, " ^ name)
```

This lowers trivially to a Go interface:

```go
type Console interface {
    Print(string)
    ReadLine() string
}

func Greet(c Console, name string) {
    c.Print("Hello, " + name)
}
```

This is idiomatic Go. It is also **not resumptive**. Capability passing gives you effect *tracking* but not effect *handling with resumption*. It's fine for IO, logging, metrics — the 95% use case. But it cannot express generators, async/await, nondeterminism, or transactional memory.

#### Strategy D: State-machine lowering (limited resumption)

Go 1.23 introduced `iter.Seq` — a pull-based iterator pattern. This is essentially a single-yield resumptive effect lowered to a state machine:

```go
func Counter(n int) iter.Seq[int] {
    return func(yield func(int) bool) {
        for i := 0; i < n; i++ {
            if !yield(i) { return }
        }
    }
}
```

This is the pattern for *generators*. You can lower a single effect (`yield`) with resumption via state machines. This is how Kotlin's coroutines and C#'s async/await work — all lowered to state machines.

For Goop, this could express a `gen` effect:

```goop
effect gen (value: int) : unit

let naturals () : unit =
  handle gen = fun value resume -> resume () in
  let rec loop (n: int) : unit =
    gen n;
    loop (n + 1)
  in loop 0
```

Would lower to Go 1.23 iterator style. But: `gen` is a *single* effect. A general resumptive effect system would need to interleave multiple different effect yields — which means a composite state machine, which means a `switch` on an effect tag at every resumption point. This is essentially the free-monad approach with a state-machine optimization.

**Bottom line**: state machines work for generators (1 effect, 1 yield point pattern). They explode in complexity for arbitrary effect combinations.

### 1.3 Concrete syntax for Goop

If Goop were to adopt effect types (even without resumption), the syntax should follow the existing row-polymorphism convention. Goop already has `{ x: int; y: float | .. }` for record rows. Effect rows would use the same `| ..` mechanism:

#### Effect type declarations

```goop
(* Declare an effect *)
effect state (s: 's) where
  val get : unit -> s
  val put : s -> unit
```

#### Effect rows in function types

```goop
(* A function that uses state and may perform IO *)
let incrementCounter () : int
  with { state<int>; io }
=
  let n = state.get () in
  state.put (n + 1);
  n

(* Row-polymorphic: works with any effect set that includes state *)
let readTwice () : (int, int)
  with { state<int> | e }
=
  let a = state.get () in
  let b = state.get () in
  (a, b)

(* Pure function: no effects *)
let double (x: int) : int = x * 2
```

The effect row syntax `with { eff1; eff2 | .. }` mirrors record row syntax `{ field: type | .. }`. This is the natural extension of Goop's existing design.

#### Effect handlers (if resumption is added)

```goop
(* Handle the state effect with a mutable cell *)
let withCounter (initial: int) (f: unit -> 'a with { state<int> | e }) : 'a with { e } =
  let mutable cell = initial in
  handle state<a> =
    | get ()   resume -> resume cell
    | put (v)  resume -> resume (cell <- v)
  in f ()

(* Usage *)
let result =
  withCounter 0 (fun () ->
    let a = state.get () in    (* a = 0 *)
    state.put 5;
    let b = state.get () in    (* b = 5 *)
    a + b                       (* 5 *)
  )
(* result = 5 *)
```

Without resumption (capability-passing model):

```goop
(* State handler as a record of functions *)
effect state (s: 's) where
  val get : unit -> s
  val put : s -> unit

let withCounter (initial: int) : state<int> =
  let mutable cell = initial in
  { get = fun () -> cell
  ; put = fun v -> cell <- v
  }

let increment (s: state<int>) : unit =
  s.put (s.get () + 1)
```

This is just Go interfaces with extra syntax. Useful, but not resumptive.

### 1.4 Type-system integration

Goop's type system already has row polymorphism for records. Effect rows would be a parallel row-polymorphic system:

- **`TFun`** currently has `From` and `To` fields. It would gain an `Effects` field: `*types.EffectRow`.
- **`EffectRow`** would be very similar to `TRecord` internally — a list of effect names/parameterizations, plus an `Open` flag for row-polymorphic rows.
- **Unification** of effect rows would follow the same algorithm as record rows (already implemented in `unify.go`, lines 99-153). An open row unifies with any row that has at least those effects. Two closed rows must match exactly.
- **Inference**: Effect variables (like `'e`) would be fresh type variables constrained by row unification — exactly how `'a` is constrained today. The HM algorithm extends to effect rows with minimal changes.

The main challenge is **effect inference at the `let` boundary**. HM generalizes type variables at `let`. For effects, you must decide: do you generalize effect variables too, or do you infer the most specific effect?

Koka uses **row-polymorphic effect inference** — a function's inferred effect row is the union of all effects in its body, generalized. This works with HM and is well-understood.

```goop
(* Inferred: f : 'a -> 'b -> 'a with { io } *)
let f (x: 'a) (y: 'b) =
  println "computing";
  x
```

### 1.5 The extern boundary problem

Extern Go functions have unknown effects. Goop's `extern` declarations:

```goop
extern "go" "fmt" {
  val printf : string -> 'a list -> unit
}
```

Should this be inferred as `with { io }`? As `with { .. }` (any effects possible)? As `with {}` (pure)?

Options:
1. **Conservative default**: extern functions are `with { .. }` (top effect, can do anything). Undesirable — breaks purity tracking.
2. **User-declared**: the extern block must declare effects. Tedious but sound.
3. **Opt-in purity**: extern functions are pure unless annotated otherwise. Optimistic but unsound — `fmt.Printf` is not pure.

The practical answer is **(2)** for production, with **(1)** as the initial behavior — extern functions are assumed effectful unless the user explicitly declares a narrower effect set.

### 1.6 Lowering strategies, ranked

Assuming Goop adopts effect tracking (rows in types), NOT resumption:

| Feature | Lowering | Idiomatic? |
|---|---|---|
| Effect rows in function types | Erased entirely. No runtime representation. | ✓ Zero-cost |
| `with { io }` annotation | Compile-time check only. Emit no Go code. | ✓ N/A |
| Effect handlers without resumption | Lower to Go interface + struct implementation | ✓ |
| Extern Go effects | User-declared, trusted. No runtime check. | ✓ |
| Pure-function enforcement | Compile-time check. Emit no Go code at the Go boundary (purity is invisible to Go callers). | ✓ |

If Goop adopts **limited resumption** (generators via state machines):

| Feature | Lowering | Idiomatic? |
|---|---|---|
| `gen` effect / `yield` | Go 1.23 `iter.Seq` | ✓ |
| Single resumption (exception-like) | Go `panic`/`recover` pattern or `defer` | ✓ (hacky but works) |
| Multi-shot resumption | CPS transform or free monad | ✗ |

### 1.7 Cost analysis

**Without resumption (effect tracking only)**:
- **Type-checker changes**: Add `EffectRow` type, extend `TFun`, extend unification for effect rows (~200 lines, mirrors existing record row code).
- **Inference**: Marginal. Effect variables unify exactly like type variables. Let-generalization extends trivially.
- **Go lowering**: Zero changes. Effects are erased.
- **Breaks existing code**: No. Functions without `with` clauses are inferred (like today's annotation-free functions). The `[@io]` attributes become synthesized from effect rows or remain as documentation.
- **Test impact**: Minimal. The existing 65 tests all pass unmodified (no effect syntax required).

**With resumption (state-machine lowering)**:
- **AST**: New node types for `effect` declarations, `handle` expressions, `resume` expressions.
- **Type-checker**: Full effect row + handler typing. Significant complexity: handler typing rules are non-trivial (polymorphic handlers, effect parameterization).
- **Codegen**: Major — state-machine lowering for arbitrary effect combinations. ~2,000+ lines of new lowering logic, heavily tested.
- **Go output**: Unidiomatic for anything beyond simple generators. Violates the core design constraint.

### 1.8 Comparison to other languages

| Language | Effect model | Approach | Goop can borrow? |
|---|---|---|---|
| **OCaml 5** | Resumptive effect handlers | Runtime stack manipulation, native code | No — Go has no runtime support |
| **Koka** | Row-polymorphic effect types + handlers | CPS transform, whole-program compilation | Borrow type system (row-polymorphic effects), not lowering |
| **Unison** | Algebraic effects | Interpreter with capabilities | Row-polymorphism in types |
| **F#** | Computation expressions | Monadic desugaring (compiler rewrite) | `result { }` / `async { }` already implemented |
| **Rust** | No effect system | Traits + explicit passing | Capability-passing model maps to Go interfaces |
| **Haskell** | Monads (IO, State, ExceptT) | Library pattern, no compiler support | Already have `result`, computation expressions |
| **Zig** | Explicit allocators, no hidden effects | Conventions, not types | N/A |

### 1.9 Recommendation: **Feasible-minimal (effect tracking only), reject resumption**

**Recommendation**: Adopt effect rows (row-polymorphic effect tracking in the type system) **without resumption**. This gives 90% of the value (knowing which functions are pure vs effectful, compositional effect tracking) at 5% of the cost. It leverages Goop's existing row-polymorphic infrastructure, requires zero changes to Go lowering (effects are erased), and satisfies the idiomatic-Go constraint.

**Reject resumptive handlers**. The Go lowering for general resumption is either CPS (unidiomatic, violates core constraint) or state machines (only work for simple single-effect cases like generators). Goop already has goroutines and channels for concurrency. If generators are needed, add a `yield` keyword that lowers to Go 1.23 `iter.Seq` — a single-purpose feature, not a general effect system.

**What the minimal viable version looks like**:

```goop
(* Phase 1: Effect rows in types, erased at runtime *)
let readConfig (path: string) : result<config, string> with { io } =
  let bytes = File.readAllBytes path ? in
  let text = Encoding.utf8.getString bytes ? in
  Json.parse<config> text

(* Row-polymorphic: works in any context *)
let catchAll (f: unit -> 'a with { e }) (handler: string -> 'a) : 'a with { e } =
  ...

(* Pure by default; the compiler warns if no with clause and effects detected *)
let double (x: int) : int = x * 2  (* inferred pure *)

(* Phase 2 (optional, later): Capability-passing for extern Go *)
let withLogger (l: Logger) (f: unit -> 'a with { log | e }) : 'a with { e } =
  ...
```

---

## 2. Full Dependent/Refinement Types

### 2.1 What it is

**Dependent types** allow types to depend on values. A vector of length 3 has type `Vec int 3`, not just `Vec int`. A function that divides two integers is only defined when the divisor is nonzero: `div : (a: int) -> (b: int where b <> 0) -> int`. **Refinement types** are a subset: types augmented with predicates. `int where x > 0` is a refinement of `int`.

The full spectrum:

| Level | What | Example | Language |
|---|---|---|---|
| **Full dependent types** | Types are terms; any value can appear in a type | `Vec A n`, `Fin n`, `Eq a b` | Idris, Agda, Coq |
| **Refinement types** | Base types + logical predicates | `int where v >= 0`, `string where len s > 0` | LiquidHaskell, F*, Whiley |
| **Liquid types** | HM inference + refinement predicates checked with SMT | `div :: x:Int -> {v:Int | v /= 0} -> Int` | LiquidHaskell |
| **Contract-based** | Runtime assertions on pre/post conditions | `require(v > 0)` at function entry | Eiffel, Racket, Clojure |

### 2.2 The Go-lowering constraint: the single biggest obstacle for Goop

**Go has no dependent types, no refinement types, no SMT solver integration at runtime. Every refinement and dependency must be erased or lowered to runtime assertions.** This means:

- A `Vec A 3` type must lower to a plain Go `[]A` — the length is not in the type at runtime.
- An `int where x > 0` must lower to a plain Go `int` — the predicate is not in the type at runtime.
- To preserve safety at the Go boundary, **runtime assertions** must be inserted at boundary crossings (Goop→extern Go, extern Go→Goop). But within pure Goop code, compile-time checking can erase the assertions.

This is exactly the LiquidHaskell model: liquid types are checked at compile time via an SMT solver, and erased at runtime. The Go lowering emits no predicate code for checked refinements — it emits runtime assertions for unchecked boundaries.

### 2.3 Feasibility under HM inference

**Full dependent types are fundamentally incompatible with Hindley-Milner inference.** Idris, Agda, and Coq all require explicit type signatures for dependently-typed functions. You cannot infer `Vec A 3` from `[a; b; c]` unless the length is syntactically visible.

**Refinement types** (LiquidHaskell style) *are* compatible with HM inference — but only partially. LiquidHaskell works as follows:

1. Run standard HM type inference (get `int`, `bool`, etc.).
2. Generate verification conditions (VCs) from the program's structure and any user-provided refinement annotations.
3. Discharge the VCs using Z3/SMT solver.
4. If all VCs discharge, the program is safe. If not, report an error.

This is a **two-phase** approach: infer base types, then check refinements. Goop already does HM inference in phase 1. The refinement checker would be a separate pass after type inference.

The key constraint: **without user annotations, refinements cannot be inferred.** LiquidHaskell infers refinement types for local variables automatically (using the SMT solver to propagate constraints), but function signatures usually need annotations. This means:

```goop
(* Without annotation: base type inferred *)
let bad (x: int) : int = x / 0  (* HM infers int -> int *)

(* With annotation: refinement checked *)
let safe (x: int) (y: int where y <> 0) : int =
  x / y
```

Goop could adopt a **"annotate critical paths"** model: most code uses plain HM types. Functions that need refinement safety add `where` clauses to their parameter/return types. This is consistent with Goop's existing philosophy of incremental adoption.

### 2.4 Concrete syntax for Goop

Goop already has `where` as a reserved word (for pattern guards). Refinements would co-opt it as a type-level keyword:

#### Refinement types

```goop
(* Positive integers *)
type pos = int where it > 0

(* Non-empty strings *)
type nonempty = string where len it > 0

(* Index in bounds *)
type index (n: int) = int where it >= 0 && it < n

(* A vector of known length (length-indexed via refinement) *)
type vec ('a, n: int) = { data: 'a list where length data = n }
```

The `it` keyword refers to the value being constrained. This mirrors how F# uses `it` and LiquidHaskell uses `v`.

#### Function types with refinements

```goop
(* Division requires nonzero divisor *)
let safeDiv (a: int) (b: int where b <> 0) : int = a / b

(* Taking the nth element requires a valid index *)
let nth (xs: 'a list) (n: int where n >= 0 && n < length xs) : 'a =
  ...
```

#### Dependent function types (if full dependent types)

```goop
(* Full dependent: return type depends on input value *)
let replicate (n: int) (x: 'a) : vec<'a, n> = ...

(* Pi-type syntax: n is available in the return type *)
let replicate (n: int) (x: 'a) : vec<'a, n> = ...
```

The key question: **should Goop allow value-level identifiers in types at all?** This is what makes a type "dependent." LiquidHaskell allows it but restricts what you can write (only decidable theories: linear arithmetic, uninterpreted functions). Idris allows anything but requires totality checking.

For Goop, the minimal viable approach is:
- Allow `where` predicates on `int`, `float`, `string`, `list` (any base type with decidable theories).
- Allow `len`, `length`, arithmetic, comparisons in `where` clauses.
- Do NOT allow arbitrary value-level expressions in types (no `f x` in a type — this needs dependent type checking, not just SMT).

#### How `where` interacts with existing syntax

Goop already uses `when` for pattern guards. There's no conflict:

```goop
(* `where` in a type annotation: refinement *)
let f (x: int where x > 0) : int = x + 1

(* `when` in a pattern match: guard *)
match x with
| Some v when v > 0 -> v
| _ -> 0
```

### 2.5 Lowering to Go

Refinements are **completely erased** in the Go output — no runtime representation. At the Goop→Go boundary, the compiler inserts runtime assertions for any refinement that crosses the boundary.

#### Minimal lowering (refinements only, no dependent types)

```goop
(* Goop source *)
let safeDiv (a: int) (b: int where b <> 0) : int = a / b
```

```go
// Lowered Go (refinement erased)
func SafeDiv(a, b int) int {
    return a / b
}
```

For extern Go functions called from Goop, the compiler cannot verify the refinement. It must insert runtime assertions at the call site:

```goop
(* Goop calls extern Go *)
extern "go" "math" {
  val sqrt : float where it >= 0.0 -> float
}

let compute (x: float) : float option =
  if x >= 0.0 then
    Some (sqrt x)   (* compiler knows x >= 0.0 from the if-condition, no runtime check *)
  else None
```

The compiler uses the SMT solver to prove that the refinement holds at each call site. If it cannot prove it, it inserts a runtime assertion or rejects the program (configurable via a strictness flag).

#### For length-indexed types (vec, fixed-size arrays)

```goop
type vec ('a, n: int) = 'a list where len it = n

let zip (a: vec<'a, n>) (b: vec<'b, n>) : vec<('a * 'b), n> =
  List.zip a b
```

Lowered Go (all refinements erased, just slices):

```go
func Zip[A, B any](a []A, b []B) []struct{First A; Second B} {
    // Compiler proved `len(a) == len(b)` at compile time (or inserts runtime check).
    // Emit a runtime check at the boundary if unproven.
    if len(a) != len(b) {
        panic("zip: length mismatch")
    }
    result := make([]struct{First A; Second B}, len(a))
    for i := range a {
        result[i] = struct{First A; Second B}{a[i], b[i]}
    }
    return result
}
```

The `panic` is emitted only when the compiler cannot prove the refinement. For internal Goop code where proofs succeed, no runtime code is emitted.

### 2.6 The SMT solver dependency: the practical cost

LiquidHaskell depends on Z3 (a ~15 MB binary with a complex C++ codebase). Adding an SMT solver dependency to the Goop compiler is a significant practical cost:

| Concern | Impact |
|---|---|
| **Build complexity** | Z3 must be installed on developer machines and CI. Or Goop bundles a WASM-compiled Z3. Either is a maintenance burden. |
| **Compile-time cost** | Each refinement-annotated function generates VCs sent to Z3. For large codebases, this can be slow (LiquidHaskell users report minutes for large modules). |
| **Error messages** | SMT solver failures produce counterexamples ("could not prove `x > 0` for call at line 42"). Good error messages require source mapping back from solver output — non-trivial. |
| **Unsoundness at the boundary** | Go code can call Goop's emitted Go with arbitrary values. A Go caller can pass `-1` to a function expecting `int where it >= 0`. The refinement is not enforced at the Go level. Goop must either accept this unsoundness at the FFI boundary or emit runtime assertions on all exported functions. |

### 2.7 Alternatives to an SMT solver

| Approach | Compile-time checking | Runtime cost | Implementation complexity |
|---|---|---|---|
| **SMT solver (Z3)** | Full (for decidable theories) | Zero (erased) | High |
| **Runtime assertions only** | None | Small (assert on every refinement boundary) | Low |
| **Contract-based (pre/post in doc, asserted at test time)** | None (tests) | Zero in production; small in test | Very low |
| **Property-based testing** | Statistical (QuickCheck-style) | Test-time only | Low-medium |
| **Subset of refinements checked by dataflow analysis** | Partial (simple cases like `> 0`) | Zero | Medium |
| **Gradual: SMT for annotated functions, runtime for everything else** | Mixed | Small at boundaries | High |

### 2.8 Full dependent types: not feasible

Full dependent types (Idris/Agda style) are **reject** for Goop. The reasons are structural:

1. **HM inference dies.** Full dependent types require bidirectional type checking with explicit type signatures everywhere. This is not compatible with Goop's inference-first design.
2. **Go lowering has no target.** Dependent type erasure is possible (the "erasures" of Idris 2), but the erased output for complex dependent programs includes type-level computations that have no Go representation. You'd need a full dependent-type runtime — which is a VM, which Goop explicitly rejects.
3. **The value-add is near-zero for Goop's target domain.** Trading systems, infrastructure, and protocols (Goop's target audience) rarely need length-indexed vectors or proof-carrying code. They need: null safety (have), exhaustive matching (have), explicit error handling (have), purity tracking (proposed above).

### 2.9 Comparison to other languages

| Language | Approach | Goop can borrow? |
|---|---|---|
| **LiquidHaskell** | Refinement types on top of Haskell; SMT-based checking; erased | Yes — this is the model. Separate pass after HM inference. |
| **F\*** | Full dependent types + refinement types; SMT + interactive proofs | Too heavy. F* requires a dedicated IDE and proof tactics. |
| **Idris 2** | Full dependent types, quantitative type theory | Reject — requires bidirectional checking, QTT, and a custom runtime. |
| **OCaml** | No dependent types. GADTs give some dependent-like behavior. | GADTs could be a lighter alternative. `type _ vec = Nil : unit vec | Cons : 'a * 'n vec -> 'a ('n+1) vec` |
| **Rust** | Const generics (`[T; N]` where N is a compile-time constant) | Const generics are a practical subset of dependent types. Go doesn't have them, but Goop could monomorphize array types at compile time (like Rust does). |
| **F#** | Units of measure (`float<m>`, `float<s>`) | A lightweight form of refinement — could be added without SMT. |
| **Whiley** | Flow typing + refinement; verifier generates bytecode with runtime checks | Interesting but niche; compiler is a research artifact. |

### 2.10 Cost analysis

#### Refinement types (LiquidHaskell-lite)

- **Type-checker changes**: Add `Refinement` node to AST, extend type representations, add verification condition generation pass (~500-800 lines). Add Z3 interface (~200-300 lines).
- **Inference**: No change to HM inference. Refinements are checked after inference, not inferred.
- **Go lowering**: Add optional runtime assertion emission for unverified boundary crossings (~100 lines).
- **New dependency**: Z3 (or the Go Z3 bindings `go-z3`). This is a heavyweight dependency.
- **User annotations**: Required for function signatures with refinements. Without annotations, functions get plain types (backward compatible).
- **Compile times**: VCs are generated only for annotated functions. If 10% of functions have refinements, compile-time impact is proportional.
- **Test impact**: New test category for refinement checking. Existing 65 tests pass unmodified (no refinement syntax used).

#### Full dependent types

- **Type-checker**: Complete rewrite. HM inference replaced with bidirectional checking. ~5,000+ lines of new checking logic.
- **Go lowering**: Major new pipeline. Dependent erasure, const-generic monomorphization for arrays. ~3,000+ lines.
- **Language**: Fundamentally different language at this point. Not Goop anymore.

### 2.11 Recommendation: **Defer-indefinitely (full dependent types), feasible-minimal (lightweight refinement contracts)**

**Full dependent types: reject.** The cost-to-value ratio is astronomical for Goop's target domain. HM inference, Go lowering, and backward compatibility all break. This is a different language.

**Lightweight refinement contracts: feasible-minimal.** The middle ground is:

1. **Add `where` clauses to type annotations** that lower to *runtime assertions only* (no SMT solver). This is Eiffel-style contracts.

```goop
let safeDiv (a: int) (b: int where b <> 0) : int = a / b
```

Lowers to:

```go
func SafeDiv(a, b int) int {
    if b == 0 { panic("safeDiv: precondition violated: b <> 0") }
    return a / b
}
```

This is trivial to implement (~100 lines of codegen change), backward compatible, and idiomatic Go (Go code already has `if condition { panic("...") }` guards at function entry).

2. **Use `requires` / `ensures` for function contracts** (leveraging Goop's existing `requires` / `returns` reserved words from the grammar):

```goop
let sqrt (x: float) : float
  requires x >= 0.0
  ensures result >= 0.0
=
  extern_math.sqrt x
```

3. **Optionally, add compile-time checking** later using an SMT solver — but keep runtime assertions as the fallback. This is the **gradual refinement** approach: write contracts today, get runtime checking; those same contracts are used by the SMT solver tomorrow for compile-time checking.

4. **Const generics for array types** as a separate, lighter feature:

```goop
(* Fixed-size array, known at compile time *)
type fixed_array ('a, const n: int) = ...  (* lowered to Go [N]T *)

let zeros (const n: int) : fixed_array<float, n> = ...
```

This is Rust-style const generics, not full dependent types. It's practical, lowers well (Go arrays have compile-time-known sizes), and doesn't need SMT.

**Why not SMT from day one:**

- Dependency cost is real: Z3 is a 15MB binary that must be installed and maintained.
- User experience: SMT error messages are notoriously bad ("counterexample: x = -1 at line 42, trace: ..."). Making them usable is a multi-year effort (see Dafny, F*).
- The 95% use case for Goop's audience (trading systems, infrastructure) is `x > 0`, `index < len`, `is_some` — simple predicates that are trivially lowered to runtime assertions.
- Progressive disclosure: ship runtime contracts now, add SMT later if demand materializes. The contracts are the same syntax either way — you're not painting yourself into a corner.

---

## Summary Verdicts (updated for Goop 1.0)

| Feature | Verdict | Reasoning (one paragraph) |
|---|---|---|
| **Resumptive effect system** | **Ship via CPS / free-monad** | Go has no stack capture. Goop 1.0 accepts CPS / free-monad lowering for OCaml 5-style `effect` / `perform` / handlers. Generated Go for effectful code is **not** idiomatic; pure code stays direct-style. This is an intentional exception to the senior-Go constraint (see [01-overview.md](01-overview.md)). |
| **Effect rows `with { io }`** | **Removed** | Replaced by OCaml 5 effect handlers. Row annotations on arrow types are parse errors (PARSE-MIG016). |
| **Full dependent/refinement types** | **Ship with Z3 SMT** | Goop 1.0 integrates Z3 for compile-time VCs on `where` / `requires` / `ensures`. HM remains for non-dependent code; bidirectional checking covers dependent fragments. Unproven sites retain runtime guards. |
| **Lightweight refinement contracts (runtime asserts)** | **Kept as fallback** | `where` clauses still lower to runtime guards when SMT cannot prove the VC. |
| **Const generics (fixed-size arrays)** | **Ship** | `[| … |]` and fixed-size array types monomorphize to Go `[N]T` where size is known. |
