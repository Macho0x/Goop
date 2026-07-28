// Package linear implements compile-time linear resource checking for Goop.
//
// Linear types are declared with `: 1` syntax (e.g. `type handle : 1`).
// Values of linear type must be used exactly once on every control-flow
// path — either handed-off to another function or explicitly discharged.
//
// V1 rule (conservative): the first use of a linear variable discharges it
// ("hand-off"). Any subsequent use is an error. This is sound but strict;
// a future version will distinguish borrowing from consuming calls.
//
// The checker is flow-sensitive: at branch points (if/else, match arms),
// both branches must discharge all live linear variables. At function exit,
// all live linear variables must be discharged.
//
// Linear types are erased in Go output — this is purely compile-time checking.
//
// Goroutine sharing analysis: extends the linear checker to detect potential
// data races when `ref` cells (and legacy mutable bindings) are captured by
// `go` closures while still accessible in the spawning scope. Flow-sensitive
// liveness suppresses false positives when the spawning scope does not access
// the variable after `go`. Linear variables captured by `go` are handed off
// to the goroutine.
package linear

import (
	"fmt"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
)

// Error represents a linear discharge error.
type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

// Check runs linear discharge checking on a module, using linearTypes to
// identify which type names are linear. Returns a list of errors.
func Check(mod *ast.Module, linearTypes map[string]bool) []error {
	errs, _ := CheckWithConfig(mod, linearTypes, config.DefaultConfig())
	return errs
}

// CheckWithConfig runs linear checking and splits concurrent race diagnostics
// into errors or warnings per cfg.Check.Concurrent.
func CheckWithConfig(mod *ast.Module, linearTypes map[string]bool, cfg *config.Config) (errors []error, warnings []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	c := &checker{
		linearTypes: linearTypes,
	}
	c.checkModule(mod)
	for _, e := range c.errors {
		if isConcurrentRace(e.Message) && cfg.Check.Concurrent != config.SeverityError {
			if cfg.Check.Concurrent == config.SeverityWarn {
				warnings = append(warnings, e)
			}
			continue
		}
		errors = append(errors, e)
	}
	return errors, warnings
}

func isConcurrentRace(msg string) bool {
	return strings.Contains(msg, "LINEAR006:") || strings.Contains(msg, "LINEAR007:")
}

// ---------------------------------------------------------------------------
// checker state
// ---------------------------------------------------------------------------

type checker struct {
	linearTypes       map[string]bool // set of linear type names
	errors            []*Error        // accumulated errors
	live              map[string]bool // live linear variables
	allLinear         map[string]bool // all linear variables ever bound (for double-use detection)
	mutableVars       map[string]bool // ref/mutable cells currently in scope
	goroutineCaptured map[string]bool // ref/mutable vars already captured by a go (per-binding)
	pendingGoRace     map[string]bool // ref/mutable captures pending liveness check at scope end
	accessedAfterGo   map[string]bool // ref/mutable vars accessed after a go in current scope
	goSeenInSeq       bool            // sequential go seen in current binding body
	borrowVarName     string          // when set, useVar treats this variable as a borrow (no discharge)
	inGoHandoff       bool            // checking closure body handed off to goroutine
	goLinearCaptures  map[string]bool // linear vars handed to current go closure
	movedVars         map[string]bool // vars explicitly moved into current go closure
}

func (c *checker) errf(format string, args ...any) {
	c.errors = append(c.errors, &Error{
		Message: fmt.Sprintf(format, args...),
	})
}

// copyLive returns a shallow copy of the live set.
func copyLive(live map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k, v := range live {
		out[k] = v
	}
	return out
}

// intersectLive returns the intersection of two live sets (variables live in both).
func intersectLive(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// mergeInto adds all entries from src into dst.
func mergeInto(dst, src map[string]bool) {
	for k := range src {
		dst[k] = true
	}
}

// checkDischarged checks that all live variables were discharged at exit.
func (c *checker) checkDischarged(context string) {
	for name := range c.live {
		c.errf("LINEAR001: linear variable %q not discharged on all paths (%s)", name, context)
	}
	c.live = make(map[string]bool)
	c.allLinear = make(map[string]bool)
}

// isLinearType checks whether a type annotation refers to a linear type.
func (c *checker) isLinearType(t ast.Type) bool {
	if t == nil {
		return false
	}
	if ident, ok := t.(*ast.TIdent); ok {
		return c.linearTypes[ident.Name]
	}
	// Support generic linear types: e.g., 'a owned_chan
	if tapp, ok := t.(*ast.TApp); ok {
		if ident, ok := tapp.Func.(*ast.TIdent); ok {
			return c.linearTypes[ident.Name]
		}
	}
	return false
}

// isBorrowCall checks whether this AppExpr is a call to OwnedChan.send or
// OwnedChan.recv, which borrow rather than consume the channel argument.
// Returns (true, varName) if the argument is a simple variable borrow.
func (c *checker) isBorrowCall(app *ast.AppExpr) (bool, string) {
	// Check if the immediate function is a borrow call.
	// In the curried chain App(Func: OwnedChan.send, Arg: ch), the function
	// is directly OwnedChan.send and the argument is the channel.
	name := funcName(app.Func)
	if name != "OwnedChan.send" && name != "OwnedChan.recv" {
		return false, ""
	}
	if ident, ok := app.Arg.(*ast.IdentExpr); ok {
		return true, ident.Name
	}
	return false, ""
}

// funcName extracts the full qualified name from a function expression.
func funcName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.FieldAccessExpr:
		if ctor, ok := e.Left.(*ast.ConstructorExpr); ok && ctor.Arg == nil {
			return ctor.Name + "." + e.Field
		}
		if ident, ok := e.Left.(*ast.IdentExpr); ok {
			return ident.Name + "." + e.Field
		}
	case *ast.IdentExpr:
		return e.Name
	}
	return ""
}

// bindLinear marks a variable as linear (live) and records it for double-use detection.
func (c *checker) bindLinear(name string) {
	c.live[name] = true
	c.allLinear[name] = true
}

// useVar handles a variable occurrence. Returns true if the variable was
// successfully discharged (or was unrestricted), false if an error was
// detected.
func (c *checker) useVar(name string) {
	if !c.allLinear[name] {
		// Not a linear variable — ignore
		return
	}
	if !c.live[name] {
		// Already discharged — double-use error
		c.errf("LINEAR002: linear variable %q used after being discharged", name)
		return
	}
	// Borrow: don't discharge — OwnedChan.send/recv borrow the channel
	if name == c.borrowVarName {
		return
	}
	// Discharge: first use = hand-off
	delete(c.live, name)
}

// ---------------------------------------------------------------------------
// Module-level dispatch
// ---------------------------------------------------------------------------

// isRefAlloc reports whether e is a `ref …` allocation (parens unwrapped).
func isRefAlloc(e ast.Expr) bool {
	for {
		switch v := e.(type) {
		case *ast.ParenExpr:
			e = v.Inner
		case *ast.RefExpr:
			return true
		default:
			return false
		}
	}
}

func (c *checker) checkModule(mod *ast.Module) {
	// Initialize module-level mutable/ref tracking
	c.mutableVars = make(map[string]bool)
	c.pendingGoRace = make(map[string]bool)
	c.accessedAfterGo = make(map[string]bool)

	// First pass: collect module-level mutable/ref variable declarations.
	// These variables are visible to all subsequent bindings.
	for _, d := range mod.Decls {
		if d, ok := d.(*ast.LetDecl); ok {
			for _, b := range d.Bindings {
				if d.Mutable || (len(b.Params) == 0 && isRefAlloc(b.Body)) {
					c.mutableVars[b.Name] = true
				}
			}
		}
	}

	// Second pass: check each binding independently.
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.LetDecl:
			for i := range d.Bindings {
				c.live = make(map[string]bool)
				c.allLinear = make(map[string]bool)
				c.goroutineCaptured = make(map[string]bool)
				c.checkBinding(d.Bindings[i])
			}
		case *ast.TypeDecl, *ast.ExternDecl, *ast.LangEmbedDecl:
			// no expressions
		}
	}
}

func (c *checker) checkBinding(b ast.LetBinding) {
	c.pendingGoRace = make(map[string]bool)
	c.accessedAfterGo = make(map[string]bool)
	c.goSeenInSeq = false

	// Bind parameters: check type annotations for linear types
	for _, p := range b.Params {
		if c.isLinearType(p.Type) {
			c.bindLinear(p.Name)
		}
	}

	c.checkExpr(b.Body)
	c.flushGoRaceChecks()

	// At function exit: auto-discharge any remaining linear parameters.
	for _, p := range b.Params {
		if c.live[p.Name] {
			delete(c.live, p.Name)
		}
	}

	// At function exit, all remaining live linear variables (from let-bindings)
	// must be discharged.
	c.checkDischarged("function " + b.Name)
}

// ---------------------------------------------------------------------------
// Expression checking
// ---------------------------------------------------------------------------

func (c *checker) checkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.LitExpr:
		// nothing to do

	case *ast.IdentExpr:
		if c.goSeenInSeq && c.mutableVars[e.Name] {
			c.accessedAfterGo[e.Name] = true
		}
		c.useVar(e.Name)

	case *ast.ConstructorExpr:
		if e.Arg != nil {
			c.checkExpr(e.Arg)
		}

	case *ast.AppExpr:
		// Check if this is a borrow call (OwnedChan.send/recv).
		// The first argument (channel) should NOT be discharged.
		prevBorrow := c.borrowVarName
		if isBorrow, varName := c.isBorrowCall(e); isBorrow {
			c.borrowVarName = varName
		}
		c.checkExpr(e.Func)
		c.checkExpr(e.Arg)
		c.borrowVarName = prevBorrow

	case *ast.IfExpr:
		c.checkExpr(e.Cond)

		// Save live set before the branch
		saved := copyLive(c.live)
		c.checkExpr(e.ThenBranch)
		// Check: all linear vars live before the branch must be discharged in then-branch
		for name := range saved {
			if c.live[name] {
				c.errf("LINEAR003: linear variable %q not discharged in then-branch", name)
			}
		}

		c.live = saved
		c.checkExpr(e.ElseBranch)
		// Check: all linear vars live before the branch must be discharged in else-branch
		for name := range saved {
			if c.live[name] {
				c.errf("LINEAR004: linear variable %q not discharged in else-branch", name)
			}
		}

		// After if/else, no variables remain live (both branches discharged everything)
		c.live = make(map[string]bool)

	case *ast.MatchExpr:
		c.checkExpr(e.Scrutinee)

		// Save live set for each arm — each arm must independently discharge
		saved := copyLive(c.live)

		for i, arm := range e.Arms {
			c.live = copyLive(saved)
			if arm.Guard != nil {
				c.checkExpr(arm.Guard)
			}
			c.checkExpr(arm.Body)
			// Check that this arm discharged all live linear variables
			for name := range saved {
				if c.live[name] {
					c.errf("LINEAR005: linear variable %q not discharged in match arm %d", name, i+1)
				}
			}
		}

		// After match, no variables remain live (all arms discharged everything)
		c.live = make(map[string]bool)

	case *ast.LetInExpr:
		// Process bindings in order. For value bindings (no params),
		// the RHS is evaluated in the current scope and discharges
		// apply directly. For function bindings (with params), the
		// body is a separate scope.
		for _, b := range e.Bindings {
			if len(b.Params) == 0 {
				// Value binding: evaluate RHS in current scope
				c.checkExpr(b.Body)

				// If the bound name has a linear type annotation, add it
				if c.isLinearType(b.RetType) {
					c.bindLinear(b.Name)
				}

				// Track legacy mutable bindings and `ref` allocations as mutable cells
				if e.Mutable || isRefAlloc(b.Body) {
					c.mutableVars[b.Name] = true
				}
			} else {
				// Function binding: separate scope
				savedLive := copyLive(c.live)
				savedAll := copyLive(c.allLinear)
				savedMutable := copyLive(c.mutableVars)

				for _, p := range b.Params {
					if c.isLinearType(p.Type) {
						c.bindLinear(p.Name)
					}
				}

				c.checkExpr(b.Body)

				// Auto-discharge unused params
				for _, p := range b.Params {
					if c.live[p.Name] {
						delete(c.live, p.Name)
					}
				}

				c.checkDischarged("binding " + b.Name)

				// Restore outer scope
				c.live = savedLive
				c.allLinear = savedAll
				c.mutableVars = savedMutable

				// Bind the function name (usually not linear, but check)
				if c.isLinearType(b.RetType) {
					c.bindLinear(b.Name)
				}
			}
		}

		c.checkExpr(e.Body)
		c.flushGoRaceChecks()

	case *ast.FunExpr:
		savedLive := copyLive(c.live)
		savedAll := copyLive(c.allLinear)
		savedMutable := copyLive(c.mutableVars)

		for _, p := range e.Params {
			if c.isLinearType(p.Type) {
				c.bindLinear(p.Name)
			}
		}

		if c.inGoHandoff {
			for name := range c.goLinearCaptures {
				c.live[name] = true
			}
		}

		c.checkExpr(e.Body)
		c.checkDischarged("lambda")

		if !c.inGoHandoff {
			c.live = savedLive
			c.allLinear = savedAll
			c.mutableVars = savedMutable
		}

	case *ast.GuardExpr:
		// guard: just check sub-expressions
		for _, b := range e.Bindings {
			c.checkExpr(b.Expr)
		}
		c.checkExpr(e.Else_)

	case *ast.BinaryExpr:
		c.checkExpr(e.Left)
		c.checkExpr(e.Right)

	case *ast.AssignExpr:
		c.checkExpr(e.Target)
		c.checkExpr(e.Value)

	case *ast.RefExpr:
		c.checkExpr(e.Value)

	case *ast.DerefExpr:
		c.checkExpr(e.Target)

	case *ast.BeginExpr:
		for _, s := range e.Stmts {
			c.checkExpr(s)
		}

	case *ast.PipeExpr:
		c.checkExpr(e.Left)
		c.checkExpr(e.Right)

	case *ast.QuestionExpr:
		c.checkExpr(e.Left)
		if e.Arg != nil {
			c.checkExpr(e.Arg)
		}

	case *ast.RecordExpr:
		for _, f := range e.Fields {
			if f.Value != nil {
				c.checkExpr(f.Value)
			}
		}

	case *ast.RecordUpdateExpr:
		c.checkExpr(e.Base)
		for _, f := range e.Fields {
			if f.Value != nil {
				c.checkExpr(f.Value)
			}
		}

	case *ast.FieldAccessExpr:
		c.checkExpr(e.Left)

	case *ast.TupleExpr:
		for _, el := range e.Elems {
			c.checkExpr(el)
		}

	case *ast.ListExpr:
		for _, el := range e.Elems {
			c.checkExpr(el)
		}

	case *ast.ParenExpr:
		c.checkExpr(e.Inner)

	case *ast.GoExpr:
		c.goLinearCaptures = make(map[string]bool)
		c.movedVars = make(map[string]bool)
		for _, name := range e.Moved {
			c.movedVars[name] = true
		}
		c.checkGoCaptures(e.Expr)
		c.inGoHandoff = true
		c.checkExpr(e.Expr)
		c.inGoHandoff = false
		c.goLinearCaptures = nil
		c.movedVars = nil
		c.goSeenInSeq = true

	case *ast.SelectExpr:
		for i := range e.Cases {
			c.checkExpr(e.Cases[i].Recv)
			c.checkExpr(e.Cases[i].Body)
		}
		if e.Default != nil {
			c.checkExpr(e.Default)
		}

	case *ast.UsingExpr:
		c.checkExpr(e.Expr)

		// Pattern binding: check if the pattern introduces a linear variable
		if c.isPatternLinear(e.Pattern, e.Expr) {
			c.bindLinearPattern(e.Pattern)
		}

		c.checkExpr(e.Body)

	case *ast.IsExpr:
		c.checkExpr(e.Left)

	case *ast.AsMatchExpr:
		c.checkExpr(e.Left)
		c.checkExpr(e.Body)
		c.checkExpr(e.ElseBody)

	case *ast.RegionExpr:
		// region { ... } opens a scoped resource block.
		// let! bindings introduce (potentially) linear variables.
		// At region exit, all live linear variables that were bound
		// inside the region are auto-discharged (codegen emits
		// defer Close() for each let! binding).
		//
		// Linear variables from the outer scope must be properly
		// discharged within the region (just like any other block).
		outerLive := copyLive(c.live)

		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				c.checkExpr(o.Expr)
				// Conservative v1: treat all let! bindings as linear.
				c.bindLinearPattern(o.Pattern)
			case *ast.DoBangOp:
				c.checkExpr(o.Expr)
			case *ast.LetOp:
				c.checkExpr(o.Expr)
			case *ast.ReturnOp:
				c.checkExpr(o.Expr)
			case *ast.ReturnBangOp:
				c.checkExpr(o.Expr)
			case *ast.BodyOp:
				c.checkExpr(o.Expr)
			}
		}

		// Auto-discharge variables bound inside this region.
		for name := range c.live {
			if !outerLive[name] {
				delete(c.live, name)
			}
		}

	case *ast.CompExpr:
		// Computation expressions are desugared before linear checking
		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				c.checkExpr(o.Expr)
			case *ast.DoBangOp:
				c.checkExpr(o.Expr)
			case *ast.LetOp:
				c.checkExpr(o.Expr)
			case *ast.ReturnOp:
				c.checkExpr(o.Expr)
			case *ast.ReturnBangOp:
				c.checkExpr(o.Expr)
			case *ast.BodyOp:
				c.checkExpr(o.Expr)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Goroutine sharing analysis
// ---------------------------------------------------------------------------

// checkGoCaptures examines a closure expression passed to `go` and flags any
// captured mutable variables that are still accessible in the spawning scope.
// This is a CONSERVATIVE analysis: it flags ALL mutable captures, even if the
// spawning scope never accesses the variable after the `go`. A future version
// may add flow-sensitive liveness analysis to suppress false positives.
func (c *checker) checkGoCaptures(closure ast.Expr) {
	// Unwrap any ParenExpr to get the underlying FunExpr.
	fun := closure
	for {
		if p, ok := fun.(*ast.ParenExpr); ok {
			fun = p.Inner
		} else {
			break
		}
	}

	funExpr, ok := fun.(*ast.FunExpr)
	if !ok {
		// `go` with a non-lambda expression (unusual, but handle gracefully).
		return
	}

	// Collect free variables from the closure body, excluding closure params.
	locals := make(map[string]bool)
	for _, p := range funExpr.Params {
		locals[p.Name] = true
	}
	captures := c.collectFreeVars(funExpr.Body, locals)

	for name := range c.movedVars {
		if c.allLinear[name] && c.live[name] {
			delete(c.live, name)
			c.goLinearCaptures[name] = true
		}
	}

	// Mutable race and linear handoff for captured variables.
	for name := range captures {
		if c.allLinear[name] && c.live[name] {
			delete(c.live, name)
			c.goLinearCaptures[name] = true
		}
		if c.movedVars[name] {
			delete(c.pendingGoRace, name)
			continue
		}
		if !c.mutableVars[name] {
			continue
		}
		if c.goroutineCaptured[name] {
			c.errf("LINEAR006: potential data race: mutable variable %q shared between multiple goroutines", name)
		} else {
			c.pendingGoRace[name] = true
		}
		c.goroutineCaptured[name] = true
	}
}

func (c *checker) flushGoRaceChecks() {
	for name := range c.pendingGoRace {
		if c.accessedAfterGo[name] {
			c.errf("LINEAR007: potential data race: mutable variable %q captured by goroutine is still accessible in spawning scope", name)
		}
	}
	c.pendingGoRace = make(map[string]bool)
	c.accessedAfterGo = make(map[string]bool)
	c.goSeenInSeq = false
}

// collectFreeVars walks an expression and returns the set of free variable
// names (i.e., IdentExpr references whose names are NOT in locals).
// The locals map tracks names bound in inner scopes (params, let bindings,
// match pattern variables).
func (c *checker) collectFreeVars(e ast.Expr, locals map[string]bool) map[string]bool {
	result := make(map[string]bool)
	if e == nil {
		return result
	}
	switch e := e.(type) {
	case *ast.IdentExpr:
		if !locals[e.Name] {
			result[e.Name] = true
		}

	case *ast.LitExpr:
		// no variables

	case *ast.ConstructorExpr:
		if e.Arg != nil {
			mergeInto(result, c.collectFreeVars(e.Arg, locals))
		}

	case *ast.AppExpr:
		mergeInto(result, c.collectFreeVars(e.Func, locals))
		mergeInto(result, c.collectFreeVars(e.Arg, locals))

	case *ast.IfExpr:
		mergeInto(result, c.collectFreeVars(e.Cond, locals))
		mergeInto(result, c.collectFreeVars(e.ThenBranch, locals))
		mergeInto(result, c.collectFreeVars(e.ElseBranch, locals))

	case *ast.MatchExpr:
		mergeInto(result, c.collectFreeVars(e.Scrutinee, locals))
		for _, arm := range e.Arms {
			armLocals := copyLive(locals)
			c.addPatternNames(arm.Pattern, armLocals)
			if arm.Guard != nil {
				mergeInto(result, c.collectFreeVars(arm.Guard, armLocals))
			}
			mergeInto(result, c.collectFreeVars(arm.Body, armLocals))
		}

	case *ast.LetInExpr:
		// RHS of bindings is evaluated in the current scope.
		for _, b := range e.Bindings {
			if len(b.Params) == 0 {
				// Value binding: RHS uses current scope
				mergeInto(result, c.collectFreeVars(b.Body, locals))
			} else {
				// Function binding: RHS is a function expression;
				// function body creates its own scope with params.
				mergeInto(result, c.collectFreeVars(b.Body, locals))
			}
		}
		// Body: binding names are in scope (shadow outer names).
		innerLocals := copyLive(locals)
		for _, b := range e.Bindings {
			innerLocals[b.Name] = true
			if len(b.Params) > 0 {
				for _, p := range b.Params {
					innerLocals[p.Name] = true
				}
			}
		}
		mergeInto(result, c.collectFreeVars(e.Body, innerLocals))

	case *ast.FunExpr:
		innerLocals := copyLive(locals)
		for _, p := range e.Params {
			innerLocals[p.Name] = true
		}
		mergeInto(result, c.collectFreeVars(e.Body, innerLocals))

	case *ast.GuardExpr:
		for _, b := range e.Bindings {
			mergeInto(result, c.collectFreeVars(b.Expr, locals))
		}
		mergeInto(result, c.collectFreeVars(e.Else_, locals))

	case *ast.BinaryExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))
		mergeInto(result, c.collectFreeVars(e.Right, locals))

	case *ast.AssignExpr:
		mergeInto(result, c.collectFreeVars(e.Target, locals))
		mergeInto(result, c.collectFreeVars(e.Value, locals))

	case *ast.RefExpr:
		mergeInto(result, c.collectFreeVars(e.Value, locals))

	case *ast.DerefExpr:
		mergeInto(result, c.collectFreeVars(e.Target, locals))

	case *ast.BeginExpr:
		for _, s := range e.Stmts {
			mergeInto(result, c.collectFreeVars(s, locals))
		}

	case *ast.PipeExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))
		mergeInto(result, c.collectFreeVars(e.Right, locals))

	case *ast.QuestionExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))
		if e.Arg != nil {
			mergeInto(result, c.collectFreeVars(e.Arg, locals))
		}

	case *ast.RecordExpr:
		for _, f := range e.Fields {
			if f.Value != nil {
				mergeInto(result, c.collectFreeVars(f.Value, locals))
			}
		}

	case *ast.RecordUpdateExpr:
		mergeInto(result, c.collectFreeVars(e.Base, locals))
		for _, f := range e.Fields {
			if f.Value != nil {
				mergeInto(result, c.collectFreeVars(f.Value, locals))
			}
		}

	case *ast.FieldAccessExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))

	case *ast.TupleExpr:
		for _, el := range e.Elems {
			mergeInto(result, c.collectFreeVars(el, locals))
		}

	case *ast.ListExpr:
		for _, el := range e.Elems {
			mergeInto(result, c.collectFreeVars(el, locals))
		}

	case *ast.ParenExpr:
		mergeInto(result, c.collectFreeVars(e.Inner, locals))

	case *ast.GoExpr:
		// Nested goroutine: walk the closure for free vars (the inner closure
		// may capture variables from this scope).
		mergeInto(result, c.collectFreeVars(e.Expr, locals))

	case *ast.SelectExpr:
		for i := range e.Cases {
			mergeInto(result, c.collectFreeVars(e.Cases[i].Recv, locals))
			// Case bind variable is local to the case body
			caseLocals := copyLive(locals)
			if e.Cases[i].Bind != "" {
				caseLocals[e.Cases[i].Bind] = true
			}
			mergeInto(result, c.collectFreeVars(e.Cases[i].Body, caseLocals))
		}
		if e.Default != nil {
			mergeInto(result, c.collectFreeVars(e.Default, locals))
		}

	case *ast.UsingExpr:
		mergeInto(result, c.collectFreeVars(e.Expr, locals))
		// Pattern variables are local to the body
		bodyLocals := copyLive(locals)
		c.addPatternNames(e.Pattern, bodyLocals)
		mergeInto(result, c.collectFreeVars(e.Body, bodyLocals))

	case *ast.IsExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))

	case *ast.AsMatchExpr:
		mergeInto(result, c.collectFreeVars(e.Left, locals))
		// Pattern variables are local to the body
		bodyLocals := copyLive(locals)
		c.addPatternNames(e.Pattern, bodyLocals)
		mergeInto(result, c.collectFreeVars(e.Body, bodyLocals))
		mergeInto(result, c.collectFreeVars(e.ElseBody, locals))

	case *ast.RegionExpr:
		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.DoBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.LetOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.ReturnOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.ReturnBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.BodyOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			}
		}

	case *ast.CompExpr:
		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.DoBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.LetOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.ReturnOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.ReturnBangOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			case *ast.BodyOp:
				mergeInto(result, c.collectFreeVars(o.Expr, locals))
			}
		}
	}
	return result
}

// addPatternNames adds all variable names bound by a pattern to locals.
func (c *checker) addPatternNames(p ast.Pattern, locals map[string]bool) {
	switch p := p.(type) {
	case *ast.IdentPattern:
		locals[p.Name] = true
	case *ast.AliasPattern:
		locals[p.Name] = true
		c.addPatternNames(p.Pattern, locals)
	case *ast.TuplePattern:
		for _, el := range p.Elems {
			c.addPatternNames(el, locals)
		}
	case *ast.ConstructorPattern:
		if p.Arg != nil {
			c.addPatternNames(p.Arg, locals)
		}
	case *ast.RecordPattern:
		for _, f := range p.Fields {
			if f.Pattern != nil {
				c.addPatternNames(f.Pattern, locals)
			} else {
				locals[f.Name] = true
			}
		}
	case *ast.ListPattern:
		for _, el := range p.Elems {
			c.addPatternNames(el, locals)
		}
	case *ast.ConsPattern:
		c.addPatternNames(p.Head, locals)
		c.addPatternNames(p.Tail, locals)
	}
	// WildcardPattern, LitPattern: no names bound
}

// ---------------------------------------------------------------------------
// Pattern helpers
// ---------------------------------------------------------------------------

// isPatternLinear checks whether a pattern introduces a linear variable.
func (c *checker) isPatternLinear(p ast.Pattern, rhs ast.Expr) bool {
	return false
}

// bindLinearPattern adds all identifier bindings in a pattern to the linear set.
func (c *checker) bindLinearPattern(p ast.Pattern) {
	switch p := p.(type) {
	case *ast.IdentPattern:
		c.bindLinear(p.Name)
	case *ast.AliasPattern:
		c.bindLinearPattern(p.Pattern)
	case *ast.TuplePattern:
		for _, el := range p.Elems {
			c.bindLinearPattern(el)
		}
	case *ast.ConstructorPattern:
		if p.Arg != nil {
			c.bindLinearPattern(p.Arg)
		}
	case *ast.RecordPattern:
		for _, f := range p.Fields {
			if f.Pattern != nil {
				c.bindLinearPattern(f.Pattern)
			} else {
				c.bindLinear(f.Name)
			}
		}
	}
}
