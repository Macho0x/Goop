# Analysis: Compile-Time Safety Checks in Goop

This document analyzes four compile-time safety features requested for Goop: SMT-based refinement checking, data race detection, deadlock detection, and channel misuse detection. Each section covers feasibility, cost, Go-lowering impact, comparison to other languages, and a verdict. The analysis assumes Goop's hard constraint: everything must lower to idiomatic Go.

---

## 1. Refinement Contracts at Compile Time (SMT-Based)

### 1.1 Current state

Goop already has runtime refinement contracts (`where` clauses on types) that lower to `panic` guards. The entire infrastructure — AST nodes (`RefinementType`), parser support, codegen lowering — is implemented and tested (`contracts.goop`, `codegen.go` lines 2766-2821). There is **no SMT solver** and no compile-time proof obligation. All checks happen at runtime.

### 1.2 What SMT-based compile-time checking would add

The LiquidHaskell model works in three phases:

1. **Run standard HM inference** — get base types (`int`, `float`, etc.). Goop already does this.
2. **Generate verification conditions (VCs)** — for each function body containing refinement-annotated parameters/returns, produce logical formulas encoding that the refinements hold at each call site.
3. **Discharge VCs with an SMT solver** — send formulas to Z3/CVC5. If all VCs discharge, the function is safe. If not, report a counterexample.

The key insight: this is a **separate pass after type inference**. It does not affect HM unification. Goop's architecture (`typecheck` → `codegen`) supports adding a post-inference analysis pass cleanly.

### 1.3 How it works concretely

```goop
let safeDiv (a: int) (b: int where b <> 0) : int = a / b

let compute (x: int) (y: int) : int =
  if y <> 0 then
    safeDiv x y    (* typechecker proves y <> 0 from if-condition → VC discharged *)
  else
    0
```

The VC generation pass would:
1. Walk the AST, identifying calls to functions with refinement-annotated parameters.
2. At each call site, synthesize a formula: `call-site-predicate → refinement-predicate`.
3. For `safeDiv x y` in the `then` branch, the call-site context includes `y <> 0`, which implies `b <> 0` — the VC discharges trivially.
4. If the call were unguarded (`safeDiv x y` without the if-check), the VC would fail — the compiler reports `could not prove y <> 0 at line 42`.

### 1.4 Go-lowering impact

**Zero.** Refinements are already erased in Go output (see `codegen.go` line 569-571: `case *ast.RefinementType: return g.typeToGo(t.Inner)`). The only change: if a VC is **proven** at compile time, the codegen can **skip emitting a runtime panic guard**. Currently, every `where` clause emits a check unconditionally. With SMT, proven-safe call sites would omit the check, reducing runtime overhead.

However, the compiler must still emit runtime guards at the Goop→Go boundary (for Go functions calling Goop functions). A refinement like `where b <> 0` is not enforced by Go's type system, so a Go caller passing `-1` would bypass the compile-time proof. The compiler should emit runtime guards on all **exported** functions regardless of internal proofs — or accept this unsoundness at the FFI boundary as a documented limitation.

### 1.5 Cost analysis

| Concern | Impact |
|---|---|
| **Dependency** | Z3 binary (~15 MB). Could embed Z3 via WASM or shell out. Go bindings (`go-z3`) exist but are maintenance-heavy. |
| **VC generation** | ~500-800 lines of new code. A new pass walking the typed AST, synthesizing SMT-LIB formulas for function call sites. |
| **SMT solver interface** | ~200-300 lines. Translate internal VC representation to SMT-LIB text, invoke Z3, parse response. |
| **Error messages** | **This is the hard part.** Z3 returns a counterexample model (`b = -1` at line 42). Mapping that back to a user-friendly error with source locations is non-trivial. LiquidHaskell and Dafny have invested years in error message quality. |
| **Compile-time overhead** | Proportional to the number of refinement-annotated functions. In most codebases, <10% of functions would carry refinements. Each annotated function generates 1-5 VCs depending on call count. |
| **Annotation burden** | Functions without refinement annotations get plain HM types — zero overhead. Users annotate only the functions where they want compile-time proofs. This matches Goop's incremental adoption philosophy. |

### 1.6 Gradual adoption path

Goop could adopt refinement checking gradually, without committing to full SMT from day one:

1. **Simple arithmetic refinements** (`x > 0`, `x == len`, `x >= y`) — check with a built-in constraint solver operating on integer linear arithmetic. This covers 80% of practical refinements without an external SMT dependency. A simple interval analysis or abstract interpretation passes suffices.

2. **SMT solver as opt-in** — configure `goop.toml` with `[check] smt = true` to enable Z3-backed checking. Without it, all refinements are runtime assertions.

3. **Hybrid mode** — the compiler proves what it can with the built-in solver; everything else falls back to runtime checks. This is transparent to the user.

This gradual path means Goop ships compile-time refinement checking for simple cases (no external dependency) while leaving the door open for full SMT later.

### 1.7 Comparison to other languages

| Language | Approach | Goop can borrow? |
|---|---|---|
| **LiquidHaskell** | Refinement types on HM-inferred Haskell; Z3-backed VC generation; erased at runtime | The model to follow. Post-inference pass, same architecture. |
| **F\*** | Full dependent types + refinements; SMT + interactive proofs | Too heavy. F* requires tactics and a dedicated IDE. |
| **Dafny** | Imperative language with pre/post contracts; Boogie/Z3 | Error message quality lessons; Boogie is an intermediate verification language worth studying. |
| **Whiley** | Flow typing + verifier; compiles with embedded runtime checks | Interesting hybrid model (prove what you can, check the rest). |
| **Rust** | No refinement types. Const generics (`[T; N]`) are a limited form. | Const generics are a practical alternative for array length checks. |
| **Swift** | No refinement types. Result builders and `precondition`/`assert` only. | Runtime-only model — like Goop's current state. |

### 1.8 Single biggest obstacle for Goop

**Error message quality.** An SMT solver produces counterexamples in the form of variable assignments. Translating `(define-fun b () Int (- 1))` into "at line 42: cannot prove `b <> 0`; counterexample: `b = -1`" requires a source-level model of which solver variables correspond to which program variables. This is a multi-year effort to get right. Without good errors, SMT-based refinement checking is worse than runtime assertions (which at least give a stack trace pointing to the exact line).

### 1.9 Verdict: **Feasible-with-effort**

**Build a built-in constraint solver for simple arithmetic refinements (integer linear arithmetic, comparisons, boolean combinations) first — no external dependency, covers ~80% of practical refinements.** This is ~1000 lines of Go code, no new dependencies, and produces actionable error messages by construction (failures point to the unsatisfiable expression directly). Defer Z3 integration until user demand materializes and the error-message problem is solved. The existing `where` clause infrastructure (parser, AST, codegen) is already in place; this just adds a compile-time proof pass that can skip emitting `panic` guards for proven-safe call sites. Runtime assertions remain the fallback for unproven or complex refinements — gradual, zero-risk adoption.

**Status: Implemented (v0.6.0).** Refinement call-site guards, arithmetic solver, linear go handoff, `go (move ...)`, and `goop.toml` severities are implemented. Z3/SMT integration remains deferred. Channel-mediated race tracking (LINEAR008, v0.7.0) and narrow static deadlock lint (DEADLOCK001, v0.7.0) are implemented for straight-line goroutine patterns.

---

## 2. Data Race Detection (Goroutine Sharing Analysis)

### 2.1 Current state

Goop exposes Go's goroutines via `go` expressions, channels via `chan` types, and multiplexing via `select`. The prelude provides `Chan.make`, `Chan.send`, `Chan.recv`. **Compile-time concurrency safety** is implemented via the linear checker (LINEAR006/007), channel-mediated race tracking (LINEAR008, v0.7.0), and narrow static deadlock lint (DEADLOCK001, v0.7.0). Severity is configurable in `goop.toml` under `[check] concurrent` and `[check] deadlock`.

### 2.2 What Rust does (and why Goop can't copy it)

Rust prevents data races with its borrow checker: at any point, you can have either one mutable reference OR many shared references to a value. This is enforced through ownership, lifetimes, and the `&mut`/`&` distinction. The borrow checker is a whole-program analysis deeply integrated into the type system.

Goop explicitly rejected this (see `docs/design/08-deferred-features-analysis.md` §2.12): "Borrow checker with lifetimes — hard to lower to Go, unnecessary with GC." The reasons are structural:
- Go has no ownership or lifetimes at the value level. Goop's linear types are opt-in for resource types, not a general ownership system.
- Go's GC manages memory — there's no "use after free" to prevent, which is half of what a borrow checker is for.
- Lowering lifetime-annotated types to Go interfaces/structs would require runtime lifetime tracking, which Go has no mechanism for.

### 2.3 What Goop CAN do: lighter approaches

Goop doesn't need Rust's borrow checker to catch the most common data races. Four approaches, from lightest to heaviest:

#### Approach A: Goroutine sharing analysis (effect-based)

The idea: when a `go` expression captures a closure, the compiler analyzes which **mutable** variables from the enclosing scope are captured by the goroutine. If the same mutable variable is accessible from the spawning goroutine (or multiple spawned goroutines), flag it as a potential data race.

```goop
let counter = ref 0

let launch (n: int) : unit =
  for i = 0 to n do
    go (fun () -> counter := !counter + 1)  (* ⚠️ RACE: counter shared between goroutines *)
  done
```

The analysis tracks closure captures across `go` expressions. It's a pure **flow analysis**, not a type-system change. It does not require ownership, lifetimes, or any change to the HM inference engine. It looks for:

1. **Mutable variables captured by a `go` closure** — automatically flagged if the spawning scope can also access them.
2. **Mutable variables sent through a channel to another goroutine** — harder to track, but `Chan.send` is an explicit operation. The analysis could track that sending a value through a channel "transfers" access.

**Go-lowering impact**: Zero. Pure compile-time check. The lowering emits the same Go code — but with a compile error if a race is detected.

**False positives**: The analysis is conservative. It would flag any mutable variable captured by a `go` closure, even if the spawning goroutine never touches it again (e.g., if the spawning goroutine immediately returns). This could be mitigated with:
- A `move` keyword or explicit transfer annotation: `go (move counter) (fun () -> ...)` marks `counter` as moved into the goroutine.
- Flow-sensitive liveness analysis: if the spawning goroutine doesn't access `counter` after the `go`, no race.

**Effort**: ~500-800 LoC. Extend the existing linear checker (`internal/linear/`) to track mutable variable captures across goroutine boundaries. The linear checker already does flow-sensitive analysis of variable usage — extending it to goroutines is natural.

#### Approach B: Capability-based annotations

Require explicit annotations for cross-goroutine mutable state:

```goop
let shared counter : int = 0       (* explicitly shared *)
let atomic counter : atomic<int> = 0  (* sync/atomic protected *)

(* No shared/atomic annotation → cannot capture in go closure *)
go (fun () -> counter <- counter + 1)  (* error: counter not shared *)
```

This is simpler to implement than flow analysis but adds annotation burden. It's effectively a type-level opt-in: "I promise this is safe" annotations that the compiler enforces structurally.

**Effort**: ~300-500 LoC. Add a `shared`/`atomic` qualifier to AST types, enforce that only `shared`/`atomic` variables can be captured by `go` closures.

#### Approach C: Ownership-lite via linear types

Extend Goop's existing linear type system to track "unique access" patterns. If a variable is linear (not shared), passing it to a `go` closure transfers ownership — the spawning goroutine can no longer access it. This prevents races by construction: two goroutines cannot simultaneously access a linear value.

```goop
type Data : 1  (* linear resource *)

let process (d: Data) : unit =
  go (fun () -> useData d)  (* d is handed-off to goroutine *)
  (* d is no longer accessible here — linear discharge *)
```

This is sound but limited: it works well for linear resources (files, handles) but not for general mutable state like counters. You can't make a counter linear in a useful way because multiple goroutines genuinely need to access it.

**Effort**: ~200-300 LoC. The linear checker already handles flow-sensitive discharge. Adding `go` as a hand-off point is straightforward: a linear variable captured by a `go` closure is discharged from the spawning scope.

#### Approach D: Separate static analysis pass

A dedicated concurrency analysis pass (like Go's `go vet -race` but at the Goop level) that builds a happens-before graph from the Goop AST. This is the most sophisticated approach but also the most effort.

**Effort**: ~2000+ LoC. Building a sound static race detector is a research-grade effort.

### 2.4 Which approach is minimal viable?

**Approach A (goroutine sharing analysis) is the sweet spot.** It catches the most common data race pattern (mutable variable shared between goroutines) without any annotation burden, without any type-system changes, and with zero runtime cost. It's a pure flow analysis that extends the existing linear checker infrastructure.

The false-positive risk is manageable because:
- Immutable bindings (the default in Goop) are never flagged — only explicit `mutable` variables.
- The analysis can be conservative (flag potential races) and let the user annotate with `move` to suppress false positives.
- It can be made configurable: `[check] concurrent = "warn"` in `goop.toml`.

### 2.5 Comparison to other languages

| Language | Approach | Goop can borrow? |
|---|---|---|
| **Rust** | Borrow checker (ownership + lifetimes). Prevents races at type level. | Rejected for Goop (see 08-deferred-features-analysis). |
| **Go** | Runtime race detector (`go test -race`). Instruments memory accesses. | Useful as a backend check (lowered Go code can be tested with `-race`), but not compile-time. |
| **Swift** | Actor model + `@Sendable` annotations. Compile-time actor isolation checking. | Swift's actor isolation is interesting — but Goop targets Go, not Swift's concurrency runtime. Cannot borrow directly. |
| **Pony** | Reference capabilities (iso, val, ref, box, tag). Type-level data-race freedom. | Elegant but requires a fundamentally different type system. Too late for Goop. |
| **Kotlin** | Coroutines + structured concurrency. Races still possible but patterns discourage sharing. | No static race detection. |
| **Java** | `synchronized`, `volatile`, `java.util.concurrent`. No static race detection. | N/A. |

### 2.6 Single biggest obstacle for Goop

**Channel-mediated sharing.** Tracking closure captures in `go` expressions is straightforward. Tracking data that flows through channels (`Chan.send ch v` in goroutine 1, `Chan.recv ch` in goroutine 2) requires modeling channel communication, which is significantly harder. The initial analysis should focus on direct closure captures and flag channel-mediated sharing as "unchecked" — document it as a limitation.

### 2.7 Verdict: **Feasible-minimal**

**Implement goroutine sharing analysis as an extension of the existing linear discharge checker.** Track which mutable variables from the enclosing scope are captured by `go` closure expressions. If a mutable variable is captured by a goroutine AND still accessible in the spawning scope (or captured by another goroutine), report a potential data race. No new dependencies. No type-system changes. No runtime cost. Zero Go-lowering impact. Catches the #1 source of data races (shared mutable state across goroutines). The false-positive rate is manageable because Goop defaults to immutable bindings — only explicit `mutable` variables trigger the analysis. For linear types, make `go` a hand-off point (capturing a linear value in a goroutine discharges it from the spawning scope), preventing double-use.

**Effort estimate**: ~500 LoC extending `internal/linear/` with goroutine capture analysis. No new dependencies.

---

## 3. Deadlock Detection

### 3.1 Current state

Goop has no deadlock detection. Programs that deadlock on channels produce the same behavior as Go: all goroutines block, and Go's runtime detector eventually panics with `fatal error: all goroutines are asleep - deadlock!`. This is a runtime check, not a compile-time one.

### 3.2 What's detectable at compile time

There are two classes of deadlocks that static analysis can detect:

#### A. Lock-ordering deadlocks (mutex-based)

Thread A acquires lock L1 then L2. Thread B acquires L2 then L1. If both interleave, they deadlock. This is detectable with lock-set analysis: build a partial order of lock acquisitions and check for cycles.

**Goop has no locks in the language.** Mutexes come from Go's `sync` package via `extern`. The compiler has no visibility into lock ordering. This class is **not detectable** without modeling extern Go behavior.

#### B. Channel-based communication deadlocks

Goroutine 1 sends on `ch1` then receives on `ch2`. Goroutine 2 sends on `ch2` then receives on `ch1`. If both goroutines execute their sends before either executes its receive (and channels are unbuffered), they deadlock.

```goop
let g1 (ch1: int chan) (ch2: int chan) : unit =
  go (fun () ->
    Chan.send ch1 42;       (* blocks until someone receives *)
    let v = Chan.recv ch2;  (* waits for g2 to send *)
    println (int_to_string v))

let g2 (ch1: int chan) (ch2: int chan) : unit =
  go (fun () ->
    Chan.send ch2 99;       (* blocks until someone receives *)
    let v = Chan.recv ch1;  (* waits for g1 to send *)
    println (int_to_string v))
```

This is detectable via **channel dependency graph analysis**: build a graph where nodes are goroutines and edges represent "goroutine A sends/receives on channel C before goroutine B can proceed." A cycle in this graph indicates a potential deadlock.

### 3.3 The undecidability problem

Deadlock detection for concurrent programs is undecidable in general. The problem reduces to the halting problem — you cannot statically determine whether a goroutine will execute a given send/receive statement. The best you can do is a **conservative approximation**: over-approximate potential deadlocks (false positives are acceptable, false negatives are not).

In practice, static deadlock detectors for channel-based programs work by:
1. **Abstracting goroutine bodies into sequences of communication events** (send, receive, select).
2. **Building a communication graph** connecting matching send/receive pairs.
3. **Checking for cycles** in the graph.

This works for simple patterns (two goroutines, two channels, sequential send-then-receive) but breaks down rapidly with conditionals, loops, `select`, and dynamic channel creation.

### 3.4 Conservative approximations and their limits

| Pattern | Detectable? | False positive risk |
|---|---|---|
| Two goroutines, two channels, sequential ops | Yes, trivially | Low |
| `select` with multiple cases | Partially — only if all branches lead to a cycle | High (branches introduce many paths) |
| Channels in loops | No — loop unrolling is unbounded | Very high |
| Dynamically created channels | No — channels created at runtime have unknown identities | Impossible to soundly analyze |
| Buffered channels | No — buffered channels don't block on send until full | Analysis is unsound for buffered channels |
| Channels passed as function arguments | Partially — requires interprocedural analysis | Medium |

The fundamental issue: **the analysis is only precise for straight-line channel operations between statically known goroutines.** This covers almost no real-world concurrent code. Real Go programs use loops, `select`, dynamic channel creation, buffered channels, and channel-passing patterns extensively.

### 3.5 Go's runtime deadlock detection

Go's runtime includes a built-in deadlock detector. When **all** goroutines are asleep (blocked on channels, mutexes, or I/O), the runtime panics with `fatal error: all goroutines are asleep - deadlock!`. This catch-all check is:
- **Sound**: it never false-positives (all goroutines genuinely can't make progress).
- **Complete for total deadlocks**: if the entire program is deadlocked, it's caught.
- **Incomplete for partial deadlocks**: if some goroutines are alive but a subset is deadlocked (e.g., a worker pool with 3 workers, 2 are deadlocked but 1 is still running), the runtime won't detect it.

Since Goop lowers to Go, every Goop program automatically gets Go's runtime deadlock detection. This is already implemented and costs zero effort.

### 3.6 Comparison to other languages

| Language | Approach | Goop can borrow? |
|---|---|---|
| **Go** | Runtime detection (all goroutines blocked). No static detection. | Already inherited via Go lowering. |
| **Rust** | No deadlock detection in the compiler. The borrow checker prevents some deadlocks (mutex poisoning) but not channel deadlocks. | N/A |
| **Pony** | Reference capabilities prevent deadlocks by construction (no blocking operations). | Elegant but fundamentally different concurrency model. |
| **Erlang** | Actor model — each process has a mailbox, no shared state. Deadlocks still possible but rare. The OTP framework provides supervision to detect hung processes. | Goop's goroutines aren't actors. |
| **Clojure core.async** | CSP model like Go. Runtime deadlock detection via `alt!` timeouts. No static detection. | Same model as Goop/Go. |
| **Akka (Scala)** | Actor model. Deadlocks possible but framework encourages message-passing patterns that avoid them. | Different model. |

### 3.7 Single biggest obstacle for Goop

**The analysis doesn't scale beyond toy examples.** A sound static deadlock detector for channel-based programs that handles loops, `select`, buffered channels, and dynamic channel creation is a research problem, not an engineering one. A conservative detector that only handles straight-line code would either have so many false positives it's unusable, or so many false negatives it's not worth having. Meanwhile, Go's runtime deadlock detection already catches total deadlocks for free because Goop lowers to Go.

### 3.8 Verdict: **Defer-indefinitely**

**Go's runtime deadlock detector (inherited for free via Goop→Go lowering) is the practical solution.** Building a static deadlock detector for channel-based concurrency that handles real-world patterns (loops, `select`, buffered channels) is a research-grade effort with no guarantee of success. The subset that IS statically detectable (straight-line channel ops between two goroutines) is so narrow that it would catch essentially zero real-world bugs while introducing ongoing maintenance burden. Invest instead in runtime debugging tools: structured concurrency patterns, goroutine leak detection, and channel operation tracing — pragmatic and lower-cost than static deadlock detection.

**Effort estimate**: N/A (not recommended). If pursued, a toy detector for straight-line patterns would be ~1000 LoC with 95%+ false negative rate on real code.

---

## 4. Channel Misuse Detection

### 4.1 Current state

Goop exposes channels via a typed prelude:
- `Chan.make ()` → `make(chan T)` (type inferred from annotation)
- `Chan.send ch v` → `ch <- v`
- `Chan.recv ch` → `<-ch`

There is **no `close` operation and no misuse detection.** The four misuse patterns are:

| Misuse | Go behavior | Severity |
|---|---|---|
| **Send on closed channel** | `panic: send on closed channel` | High — crashes the program |
| **Receive on closed channel** | Returns zero value (ok=false with comma-ok) | Low — not a panic, but might cause logic errors |
| **Close a nil channel** | `panic: close of nil channel` | Medium — crashes the program |
| **Close a closed channel** | `panic: close of closed channel` | High — crashes the program |
| **Send/receive on nil channel** | Blocks forever (not a panic) | Medium — silent hang |
| **Receive from empty channel with no sender** | Blocks forever | Low — expected behavior for unbuffered channels |

### 4.2 Linear channels: the most promising approach

Goop's existing **linear type system** (`type handle : 1` → flow-sensitive discharge checking) is a natural fit for channel close safety. The idea:

```goop
type 'a chan : 1  (* channels are linear resources *)

let producer (ch: int chan) : unit =
  Chan.send ch 42;
  Chan.send ch 99;
  Chan.close ch     (* close consumes (discharges) the channel *)
  (* ch is no longer accessible — linear discharge *)

let consumer (ch: int chan) : unit =
  let v1 = Chan.recv ch;
  let v2 = Chan.recv ch;
  let v3 = Chan.recv ch;  (* receives zero value on closed channel — not a panic *)
```

**What this catches:**
- **Send after close**: The channel is discharged by `close` → any further use is a linear double-use error.
- **Double close**: Same mechanism — second close is a double-use.
- **Close of nil channel**: Never created → never live → `close` on nil caught by flow analysis.

**What this doesn't catch:**
- **Receive after close**: Receiving from a closed channel is valid Go (returns zero value). Making it a linear violation would break the consumer pattern.
- **Send/receive on nil**: Requires flow-sensitive nil tracking, not linearity.

### 4.3 The multi-producer problem

Go channels support multiple senders and multiple receivers. Making channels linear breaks this pattern:

```go
// Go: multiple senders sharing a channel
for _, w := range workers {
    go func(w Worker) {
        ch <- w.Result()  // multiple goroutines send on the same channel
    }(w)
}
```

If `chan` is linear, each sender must receive its own reference to the channel, and the channel must be "handed off" between senders. This is unnatural for shared-memory concurrency.

**Solutions:**
1. **Drop the linear constraint** — accept that channels are shared references and close safety is checked at runtime (like Go). This is the pragmatic choice.
2. **Split `chan` into `Sender` and `Receiver`** (Rust's `mpsc` model) — the `Sender` is linear and closeable; the `Receiver` is unlimited. This is elegant but a departure from Go's unified channel model.
3. **Dual-mode channels** — `type 'a chan : 1` for single-producer patterns; `type 'a chan : shared` for multi-producer. The linear version gets close-safety; the shared version gets runtime checks.

### 4.4 Practical approach: hybrid

**Option 1 (minimal, boring, works):** Don't make channels linear. Add `Chan.close` as a regular (non-linear) operation. Detect send-after-close via **runtime tracking** — the lowered Go emits a wrapper around channels that tracks the closed state:

```goop
let ch = Chan.make ()   (* lowered to: ch := NewChannel() — wraps make(chan int) with a closed flag *)
Chan.close ch            (* ch.closed = true; close(ch.raw) *)
Chan.send ch 42          (* if ch.closed { panic("send on closed channel") }; ch.raw <- 42 *)
```

This adds a small runtime cost (one extra boolean check per send) but catches the most dangerous misuse (send-on-closed panics with a clear Goop-level error message). The cost is minimal — one `if` per `Chan.send`.

**Option 2 (linear-sender model):** Make channels linear, but provide a **borrow** mechanism: `Chan.borrow` creates a non-linear view of the channel that can be shared but cannot close it. Only the linear owner can close.

```goop
let ch : int chan = Chan.make ()    (* linear owner *)
let view = Chan.borrow ch           (* non-linear reference, can send/recv but not close *)
go (fun () -> Chan.send view 42)    (* OK: view is non-linear *)
Chan.send ch 99                     (* OK: owner can send *)
Chan.close ch                       (* OK: owner closes *)
(* after close: view is dead but compiler can't prove this — runtime check *)
```

This is elegant but complex: `Chan.borrow` requires tracking that borrows don't outlive the owner, which is drifting into Rust-style lifetime territory.

**Option 3 (runtime only):** Don't solve this at compile time. Goop's `Chan.close` emits Go's `close`. If the user closes and then sends, Go panics — the behavior is identical to if the user wrote the Go by hand. This is the "lower honestly" philosophy: Goop should not paper over Go's semantics.

### 4.5 Nil channel detection

Go's nil channel blocks forever on send/receive. This is easy to catch with flow-sensitive nil checking:

```goop
let ch : int chan    (* declared but never assigned — nil *)
Chan.send ch 42      (* compile error: ch is nil *)
```

Goop already forbids null — `option` is explicit. Extending this to channels: `Chan.make` always returns a non-nil channel. If a channel variable is not initialized before use, the compiler can flag it with flow-sensitive analysis. This is ~200 LoC and reuses the existing variable-liveness infrastructure.

### 4.6 Comparison to other languages

| Language | Approach | Goop can borrow? |
|---|---|---|
| **Go** | Runtime panics for send-on-closed, double-close. Nil channel blocks forever. No static detection. | Goop currently inherits all of this. |
| **Rust** | `mpsc::Sender`/`Receiver` split. `Sender::send` returns `Err` if the channel is closed (receiver dropped). `Sender` is `Send` but not `Copy` — single-owner semantics via ownership. | Split sender/receiver is elegant and maps well to linear types. But Go channels are unified — Goop targets Go. |
| **Erlang/Elixir** | Processes, not channels. Messages are sent to process IDs. If a process dies, sends to it are silently dropped. No concept of "closed channel." | Different model entirely. |
| **Clojure core.async** | `close!` on a channel. `>!` (put) returns `nil` if channel is closed. No panics. Nil channels accepted (block). | Runtime handling, not static detection. |
| **Pony** | Reference capabilities. Channels are `val` (immutable, shareable) or `tag` (opaque, message-send only). The type system prevents misuse. | Requires Pony's capability system — not portable to Goop. |

### 4.7 Single biggest obstacle for Goop

**The multi-producer/multi-consumer pattern.** Go channels are designed for shared-memory concurrency where multiple goroutines send on the same channel. Making channels linear (to get close-safety) directly conflicts with this pattern. The split-`Sender`/`Receiver` model (Rust) is the right abstraction but diverges from Go's channel semantics — Goop programs would use channels differently than Go programs, violating the "emits idiomatic Go" constraint. The most honest answer is runtime checks.

### 4.8 Verdict: **Feasible-minimal**

**Add `Chan.close` with runtime safety checks, not compile-time linear enforcement.** The lowered Go wrapper around channels tracks a `closed` flag: `Chan.send` checks it before sending (panic with a clear Goop-level message instead of Go's opaque "send on closed channel"), `Chan.close` sets it (double-close panics with clear message). This is ~100 LoC in codegen, adds a single boolean check per send, and produces Go code that a senior Go engineer would approve of. For nil channel detection: flow-sensitive initialization checking (~200 LoC, reuses existing infrastructure). **Do not make channels linear.** The fit is wrong: Go channels are shared-memory primitives, not ownership-tracked resources. If users want compile-time close safety, provide a separate `OwnedChan` as a linear wrapper around a raw channel — opt-in, not the default.

**Status: Implemented.** `OwnedChan` is now available as a linear (`: 1`) channel type with compile-time close safety. `OwnedChan.send` and `OwnedChan.recv` borrow the channel (non-consuming), while `OwnedChan.close` discharges it. Send-after-close and double-close are caught at compile time as linear double-use errors. See `docs/examples/owned_chan.goop`.

**Effort estimate**: ~300 LoC for runtime-safe `Chan.close` + nil detection. No new dependencies.

---

## Summary Verdicts

| Feature | Verdict | Reasoning (one paragraph) | Effort |
|---|---|---|---|
| **SMT-based refinement checking** | **Feasible-with-effort** | Build a built-in constraint solver for linear integer arithmetic first (no external dependency, covers 80% of practical refinements). The existing `where` clause infrastructure is complete; this adds a post-inference VC generation pass that skips runtime panic guards for proven-safe call sites. Defer Z3 integration until the error-message quality problem is solved. | ~1000-1500 LoC |
| **Data race detection** | **Implemented** | Linear checker flags mutable captures (LINEAR006/007). Channel-mediated race tracking (LINEAR008, v0.7.0) flags mutable values sent on channels while still accessible in the spawning scope. Configurable via `goop.toml` `[check] concurrent`. | ~500 LoC |
| **Deadlock detection** | **Implemented (narrow)** | Go's runtime deadlock detector (inherited for free via Goop→Go lowering) catches total deadlocks. Goop adds DEADLOCK001 (v0.7.0): a conservative static lint for the classic two-goroutine circular send/recv pattern on unbuffered channels. Full static deadlock detection for loops, `select`, and buffered channels remains deferred. | ~200 LoC |
| **Channel misuse** | **Feasible-minimal** | Add `Chan.close` with runtime safety tracking (closed flag checked on send/close). Add flow-sensitive nil channel detection. Do NOT make channels linear — the multi-producer pattern conflicts with linear ownership, and Go channels are shared-memory primitives. If compile-time close-safety is desired, provide an opt-in `OwnedChan` linear wrapper. **Status: `OwnedChan` implemented; `Chan.close` runtime safety implemented.** | ~300 LoC |
