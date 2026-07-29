package refine

import (
	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typeinfo"
)

// CheckRefinements walks a module's function bodies and checks refinement
// contracts at each call site. Returns:
//   - provenSites: call-site AST nodes whose refinements have been proven.
//   - funcAllProven: functions whose every refinement call site was proven.
//   - warnings: unproven refinements (runtime check will be emitted).
//   - errors: disproven refinements (compile error).
func CheckRefinements(mod *ast.Module, tm typeinfo.TypeMap, cfg *config.Config) (ProvenSites, map[string]bool, []error, []error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	provenSites, funcAllProven, warnings, errs := checkRefinements(mod, tm, cfg.Check.SMT)
	switch cfg.Check.RefinementUnproven {
	case config.SeverityOff:
		warnings = nil
	case config.SeverityError:
		errs = append(errs, warnings...)
		warnings = nil
	}
	return provenSites, funcAllProven, warnings, errs
}

func checkRefinements(mod *ast.Module, tm typeinfo.TypeMap, useSMT bool) (ProvenSites, map[string]bool, []error, []error) {
	provenSites := make(ProvenSites)
	funcAllProven := make(map[string]bool)
	funcCallTotal := make(map[string]int)
	funcCallProven := make(map[string]int)
	var warnings []error
	var errs []error

	// First pass: collect all function definitions and their refinement-annotated
	// parameters, keyed by function name.
	funcInfo := make(map[string]*funcDef)
	for _, d := range mod.Decls {
		ld, ok := d.(*ast.LetDecl)
		if !ok {
			continue
		}
		for i := range ld.Bindings {
			b := &ld.Bindings[i]
			fd := &funcDef{
				name:       b.Name,
				params:     b.Params,
				refinements: make(map[int]*ast.RefinementType),
			}
			hasRefinement := false
			for j, p := range b.Params {
				if rt, ok := p.Type.(*ast.RefinementType); ok {
					fd.refinements[j] = rt
					hasRefinement = true
				}
			}
			if hasRefinement {
				funcInfo[b.Name] = fd
				funcAllProven[b.Name] = true // no calls yet; entry guard may be skipped
			}
		}
	}

	// Second pass: walk each function body, collecting call sites.
	for _, d := range mod.Decls {
		ld, ok := d.(*ast.LetDecl)
		if !ok {
			continue
		}
		for i := range ld.Bindings {
			b := &ld.Bindings[i]
			// Skip functions without bodies (e.g., extern)
			if b.Body == nil {
				continue
			}
			pctx := &pathCtx{
				constraints:    nil,
				funcInfo:       funcInfo,
				proven:         provenSites,
				tm:             tm,
				funcCallTotal:  funcCallTotal,
				funcCallProven: funcCallProven,
				useSMT:         useSMT,
			}
			walkExpr(b.Body, pctx, &warnings, &errs)
		}
	}

	for name := range funcInfo {
		total := funcCallTotal[name]
		proven := funcCallProven[name]
		funcAllProven[name] = total == 0 || proven == total
	}

	return provenSites, funcAllProven, warnings, errs
}

// funcDef stores information about a function with refinement-annotated params.
type funcDef struct {
	name        string
	params      []ast.Param
	refinements map[int]*ast.RefinementType // param index → refinement type
}

// pathCtx tracks the current path condition during AST traversal.
type pathCtx struct {
	constraints    []ast.Expr
	funcInfo       map[string]*funcDef
	proven         ProvenSites
	tm             typeinfo.TypeMap
	funcCallTotal  map[string]int
	funcCallProven map[string]int
	useSMT         bool
}

// walkExpr recursively walks an expression, tracking path conditions.
func walkExpr(e ast.Expr, ctx *pathCtx, warnings *[]error, errs *[]error) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.IfExpr:
		// If: add cond to path for then-branch, not cond for else-branch
		walkExpr(e.Cond, ctx, warnings, errs)

		thenCtx := ctx.withConstraint(e.Cond)
		walkExpr(e.ThenBranch, thenCtx, warnings, errs)

		elseCtx := ctx.withConstraint(negateExpr(e.Cond))
		walkExpr(e.ElseBranch, elseCtx, warnings, errs)

	case *ast.MatchExpr:
		walkExpr(e.Scrutinee, ctx, warnings, errs)
		for _, arm := range e.Arms {
			armCtx := ctx.withConstraints(patternConstraints(arm.Pattern, e.Scrutinee))
			if arm.Guard != nil {
				armCtx = armCtx.withConstraint(arm.Guard)
			}
			walkExpr(arm.Body, armCtx, warnings, errs)
		}

	case *ast.LetInExpr:
		for i := range e.Bindings {
			if e.Bindings[i].Body != nil {
				walkExpr(e.Bindings[i].Body, ctx, warnings, errs)
			}
		}
		walkExpr(e.Body, ctx, warnings, errs)

	case *ast.AppExpr:
		// This is a call site. Extract function name and arguments from
		// the potentially-curried call chain.
		allArgs := collectArgs(e)
		calledFunc := allArgs[0].(ast.Expr)
		args := allArgs[1:]
		funcName := getFuncName(calledFunc)
		fd := ctx.funcInfo[funcName]
		if fd == nil {
			// Not a function with refinements — walk children and continue
			walkExpr(e.Func, ctx, warnings, errs)
			walkExpr(e.Arg, ctx, warnings, errs)
			return
		}

		// Walk sub-expressions
		walkExpr(calledFunc, ctx, warnings, errs)
		for _, a := range args {
			walkExpr(a.(ast.Expr), ctx, warnings, errs)
		}

		// Check each refinement param against the actual argument
		allProven := true
		for paramIdx, rt := range fd.refinements {
			if paramIdx >= len(args) {
				continue // partial application, skip
			}
			actualArg := args[paramIdx].(ast.Expr)
			paramName := fd.params[paramIdx].Name
			pred := SubstituteIdent(rt.Pred, paramName, actualArg)

			result := CheckWithSMT(pred, ctx.constraints, ctx.useSMT)
			switch result {
			case Disproven:
				*errs = append(*errs, refineDisproven(rt, e))
				allProven = false
			case Unknown:
				*warnings = append(*warnings, refineUnproven(rt, e))
				allProven = false
			case Proven:
				// proven — no error, no warning
			}
		}

		if len(fd.refinements) > 0 {
			ctx.funcCallTotal[funcName]++
			if allProven {
				ctx.funcCallProven[funcName]++
				ctx.proven[e] = true
			}
		}

	case *ast.BinaryExpr:
		walkExpr(e.Left, ctx, warnings, errs)
		walkExpr(e.Right, ctx, warnings, errs)

	case *ast.FunExpr:
		walkExpr(e.Body, ctx, warnings, errs)

	case *ast.PipeExpr:
		walkExpr(e.Left, ctx, warnings, errs)
		walkExpr(e.Right, ctx, warnings, errs)

	case *ast.GuardExpr:
		for _, b := range e.Bindings {
			walkExpr(b.Expr, ctx, warnings, errs)
		}
		walkExpr(e.Else_, ctx, warnings, errs)

	case *ast.GoExpr:
		walkExpr(e.Expr, ctx, warnings, errs)

	case *ast.TupleExpr:
		for _, el := range e.Elems {
			walkExpr(el, ctx, warnings, errs)
		}

	case *ast.ListExpr:
		for _, el := range e.Elems {
			walkExpr(el, ctx, warnings, errs)
		}

	case *ast.ParenExpr:
		walkExpr(e.Inner, ctx, warnings, errs)

	case *ast.RecordExpr:
		for _, f := range e.Fields {
			if f.Value != nil {
				walkExpr(f.Value, ctx, warnings, errs)
			}
		}

	case *ast.RecordUpdateExpr:
		walkExpr(e.Base, ctx, warnings, errs)
		for _, f := range e.Fields {
			if f.Value != nil {
				walkExpr(f.Value, ctx, warnings, errs)
			}
		}

	case *ast.FieldAccessExpr:
		walkExpr(e.Left, ctx, warnings, errs)

	case *ast.ConstructorExpr:
		if e.Arg != nil {
			walkExpr(e.Arg, ctx, warnings, errs)
		}

	case *ast.QuestionExpr:
		walkExpr(e.Left, ctx, warnings, errs)
		if e.Arg != nil {
			walkExpr(e.Arg, ctx, warnings, errs)
		}

	case *ast.CompExpr:
		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.DoBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.LetOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.ReturnOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.ReturnBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.BodyOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			}
		}

	case *ast.RegionExpr:
		for _, op := range e.Ops {
			switch o := op.(type) {
			case *ast.LetBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.DoBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.LetOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.ReturnOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.ReturnBangOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			case *ast.BodyOp:
				walkExpr(o.Expr, ctx, warnings, errs)
			}
		}

	case *ast.SelectExpr:
		for _, c := range e.Cases {
			walkExpr(c.Recv, ctx, warnings, errs)
			walkExpr(c.Body, ctx, warnings, errs)
		}
		if e.Default != nil {
			walkExpr(e.Default, ctx, warnings, errs)
		}

	case *ast.UsingExpr:
		walkExpr(e.Expr, ctx, warnings, errs)
		walkExpr(e.Body, ctx, warnings, errs)

	case *ast.IsExpr:
		walkExpr(e.Left, ctx, warnings, errs)

	case *ast.AsMatchExpr:
		walkExpr(e.Left, ctx, warnings, errs)
		walkExpr(e.Body, ctx, warnings, errs)
		walkExpr(e.ElseBody, ctx, warnings, errs)

	// Leaf nodes — nothing to recurse into
	case *ast.LitExpr, *ast.IdentExpr:
		// no children
	}
}

// withConstraint returns a copy of ctx with an additional constraint.
func (ctx *pathCtx) withConstraint(c ast.Expr) *pathCtx {
	newConstraints := make([]ast.Expr, len(ctx.constraints)+1)
	copy(newConstraints, ctx.constraints)
	newConstraints[len(ctx.constraints)] = c
	return &pathCtx{
		constraints:    newConstraints,
		funcInfo:       ctx.funcInfo,
		proven:         ctx.proven,
		tm:             ctx.tm,
		funcCallTotal:  ctx.funcCallTotal,
		funcCallProven: ctx.funcCallProven,
	}
}

// withConstraints returns a copy of ctx with additional constraints.
func (ctx *pathCtx) withConstraints(cs []ast.Expr) *pathCtx {
	if len(cs) == 0 {
		return ctx
	}
	newConstraints := make([]ast.Expr, len(ctx.constraints)+len(cs))
	copy(newConstraints, ctx.constraints)
	copy(newConstraints[len(ctx.constraints):], cs)
	return &pathCtx{
		constraints:    newConstraints,
		funcInfo:       ctx.funcInfo,
		proven:         ctx.proven,
		tm:             ctx.tm,
		funcCallTotal:  ctx.funcCallTotal,
		funcCallProven: ctx.funcCallProven,
	}
}

// getFuncName extracts the function name from an expression.
func getFuncName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.IdentExpr:
		return e.Name
	case *ast.FieldAccessExpr:
		// Qualified name like Console.println
		return e.Field
	}
	return ""
}

// collectArgs flattens a curried application chain into a slice.
// AppExpr(Fun=AppExpr(Fun=f, Arg=a), Arg=b) → [f, a, b]
func collectArgs(app *ast.AppExpr) []ast.Expr {
	var result []ast.Expr
	current := app
	for {
		result = append([]ast.Expr{current.Arg}, result...)
		if inner, ok := current.Func.(*ast.AppExpr); ok {
			current = inner
		} else {
			result = append([]ast.Expr{current.Func}, result...)
			break
		}
	}
	return result
}

// negateExpr creates a negated version of a boolean expression.
// For now handles simple cases; Unknown/unhandled returns nil (which means "don't add a constraint").
func negateExpr(e ast.Expr) ast.Expr {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQEQ, token.EQUALS:
			return &ast.BinaryExpr{Left: e.Left, Op: token.DIAMOND, Right: e.Right}
		case token.DIAMOND:
			return &ast.BinaryExpr{Left: e.Left, Op: token.EQEQ, Right: e.Right}
		case token.NEQ:
			return &ast.BinaryExpr{Left: e.Left, Op: token.EQEQ, Right: e.Right}
		case token.LT:
			return &ast.BinaryExpr{Left: e.Left, Op: token.GEQ, Right: e.Right}
		case token.GT:
			return &ast.BinaryExpr{Left: e.Left, Op: token.LEQ, Right: e.Right}
		case token.LEQ:
			return &ast.BinaryExpr{Left: e.Left, Op: token.GT, Right: e.Right}
		case token.GEQ:
			return &ast.BinaryExpr{Left: e.Left, Op: token.LT, Right: e.Right}
		case token.AMPAMP:
			// De Morgan: !(a && b) => !a || !b
			nl := negateExpr(e.Left)
			nr := negateExpr(e.Right)
			if nl == nil || nr == nil {
				return nil
			}
			return &ast.BinaryExpr{Left: nl, Op: token.PIPEPIPE, Right: nr}
		case token.PIPEPIPE:
			// De Morgan: !(a || b) => !a && !b
			nl := negateExpr(e.Left)
			nr := negateExpr(e.Right)
			if nl == nil || nr == nil {
				return nil
			}
			return &ast.BinaryExpr{Left: nl, Op: token.AMPAMP, Right: nr}
		}
	case *ast.ParenExpr:
		if inner := negateExpr(e.Inner); inner != nil {
			return &ast.ParenExpr{Inner: inner}
		}
	case *ast.IdentExpr:
		// not x → x == false? For simplicity, "not x" → x == 0
		return &ast.BinaryExpr{Left: e, Op: token.EQEQ, Right: &ast.LitExpr{Value: int64(0), Kind: token.INT}}
	case *ast.LitExpr:
		if b, ok := e.Value.(bool); ok {
			if b {
				return &ast.LitExpr{Value: false, Kind: token.FALSE}
			}
			return &ast.LitExpr{Value: true, Kind: token.TRUE}
		}
	}
	return nil
}

// patternConstraints extracts path constraints from match patterns.
// For example: `head :: _` pattern adds the constraint `length scrutinee > 0`.
// For now, we extract simple constraints.
func patternConstraints(p ast.Pattern, scrutinee ast.Expr) []ast.Expr {
	switch p := p.(type) {
	case *ast.ConsPattern:
		// cons pattern: the scrutinee is non-empty → it's not equal to []
		// This is informational; for the solver it means the list is non-empty.
		return nil // Skip for now — lists aren't in scope for the integer solver
	case *ast.ConstructorPattern:
		// For constructors with arguments, the scrutinee has that variant
		return nil
	case *ast.LitPattern:
		// Literal pattern: scrutinee == value
		return []ast.Expr{
			&ast.BinaryExpr{
				Left:  scrutinee,
				Op:    token.EQUALS,
				Right: &ast.LitExpr{Value: p.Value, Kind: p.Kind},
			},
		}
	case *ast.IdentPattern:
		// Variable pattern: no constraint, just binding
		return nil
	}
	return nil
}

