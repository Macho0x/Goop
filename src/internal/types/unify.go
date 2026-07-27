// Package types — unification algorithm with occurs check.
package types

import (
	"fmt"
)

// UnifyError represents a type mismatch during unification.
type UnifyError struct {
	Msg   string
	Left  Type
	Right Type
}

func (e *UnifyError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("type mismatch: got %s, expected %s (%s)", e.Right, e.Left, e.Msg)
	}
	return fmt.Sprintf("type mismatch: got %s, expected %s", e.Right, e.Left)
}

// Unify attempts to unify two types, returning the resulting substitution
// or an error if unification fails.
func Unify(t1, t2 Type) (Subst, error) {
	sub := EmptySubst()
	if err := unify(sub, t1, t2); err != nil {
		return nil, err
	}
	return sub, nil
}

// unify is the core unification algorithm. It mutates `sub` in place.
func unify(sub Subst, t1, t2 Type) error {
	// Apply current substitution to both sides
	t1 = Apply(sub, t1)
	t2 = Apply(sub, t2)

	// Handle type variable cases first
	if v1, ok := t1.(*TVar); ok {
		if v2, ok2 := t2.(*TVar); ok2 && v1.ID == v2.ID {
			return nil // same variable
		}
		if occurs(v1.ID, t2) {
			return &UnifyError{
				Msg:   fmt.Sprintf("occurs check: %s occurs in %s", v1, t2),
				Left:  v1,
				Right: t2,
			}
		}
		sub[v1.ID] = t2
		return nil
	}
	if v2, ok := t2.(*TVar); ok {
		if occurs(v2.ID, t1) {
			return &UnifyError{
				Msg:   fmt.Sprintf("occurs check: %s occurs in %s", v2, t1),
				Left:  t1,
				Right: v2,
			}
		}
		sub[v2.ID] = t1
		return nil
	}

	// Structural unification
	switch l := t1.(type) {
	case *Prim:
		r, ok := t2.(*Prim)
		if !ok {
			return mismatch(t1, t2, "expected primitive type")
		}
		if l.Name != r.Name {
			return mismatch(t1, t2, "different primitive types")
		}
		return nil

	case *TFun:
		r, ok := t2.(*TFun)
		if !ok {
			return mismatch(t1, t2, "expected a function")
		}
		if err := unify(sub, l.From, r.From); err != nil {
			return err
		}
		if err := unify(sub, l.To, r.To); err != nil {
			return err
		}
		// Effect row unification
		return unifyEffects(sub, l, r)

	case *TTuple:
		r, ok := t2.(*TTuple)
		if !ok {
			return mismatch(t1, t2, "expected a tuple")
		}
		if len(l.Elems) != len(r.Elems) {
			return mismatch(t1, t2, fmt.Sprintf("tuple arity mismatch: %d vs %d", len(l.Elems), len(r.Elems)))
		}
		for i := range l.Elems {
			if err := unify(sub, l.Elems[i], r.Elems[i]); err != nil {
				return err
			}
		}
		return nil

	case *TRecord:
		// Record implementors may be used where a Go interface is expected.
		if r, ok := t2.(*TGoNamed); ok && r.Interface {
			return nil
		}
		r, ok := t2.(*TRecord)
		if !ok {
			return mismatch(t1, t2, "expected a record")
		}
		// Row polymorphism: if either side is open, only check the
		// required fields (the side without `| ..` may have extra fields).
		if l.Open || r.Open {
			lFields := fieldMap(l)
			rFields := fieldMap(r)
			// l is open: r must have all of l's fields
			if l.Open {
				for name, lt := range lFields {
					rt, ok := rFields[name]
					if !ok {
						return mismatch(t1, t2, fmt.Sprintf("record has no field %q", name))
					}
					if err := unify(sub, lt, rt); err != nil {
						return err
					}
				}
			}
			// r is open: l must have all of r's fields
			if r.Open {
				for name, rt := range rFields {
					lt, ok := lFields[name]
					if !ok {
						return mismatch(t1, t2, fmt.Sprintf("record has no field %q", name))
					}
					if err := unify(sub, lt, rt); err != nil {
						return err
					}
				}
			}
			return nil
		}
		// Both closed: both records must have the same field names
		lFields := fieldMap(l)
		rFields := fieldMap(r)
		for name, lt := range lFields {
			rt, ok := rFields[name]
			if !ok {
				return mismatch(t1, t2, fmt.Sprintf("record has no field %q", name))
			}
			if err := unify(sub, lt, rt); err != nil {
				return err
			}
		}
		for name := range rFields {
			if _, ok := lFields[name]; !ok {
				return mismatch(t1, t2, fmt.Sprintf("record has unexpected field %q", name))
			}
		}
		return nil

	case *TAdt:
		r, ok := t2.(*TAdt)
		if !ok {
			return mismatch(t1, t2, "expected an ADT")
		}
		if l.Name != r.Name {
			return mismatch(t1, t2, "different ADT names")
		}
		if len(l.Params) != len(r.Params) {
			return mismatch(t1, t2, "ADT arity mismatch")
		}
		for i := range l.Params {
			if err := unify(sub, l.Params[i], r.Params[i]); err != nil {
				return err
			}
		}
		return nil

	case *PolyVariant:
		r, ok := t2.(*PolyVariant)
		if !ok {
			return mismatch(t1, t2, "expected a polymorphic variant row")
		}
		lm, rm := variantMap(l.Variants), variantMap(r.Variants)
		for name, lv := range lm {
			rv, ok := rm[name]
			if !ok {
				if r.Open || r.UpperBound {
					continue
				}
				return mismatch(t1, t2, "polymorphic variant tag missing: `"+name)
			}
			if lv.Arg != nil && rv.Arg != nil {
				if err := unify(sub, lv.Arg, rv.Arg); err != nil {
					return err
				}
			} else if lv.Arg != nil || rv.Arg != nil {
				return mismatch(t1, t2, "polymorphic variant payload mismatch: `"+name)
			}
		}
		for name := range rm {
			if _, ok := lm[name]; !ok && !l.Open && !l.UpperBound {
				return mismatch(t1, t2, "unexpected polymorphic variant tag: `"+name)
			}
		}
		return nil

	case *TNewtype:
		r, ok := t2.(*TNewtype)
		if !ok {
			return mismatch(t1, t2, "expected newtype "+l.Name)
		}
		if l.Name != r.Name {
			return mismatch(t1, t2, "different newtype names")
		}
		return nil

	case *TCon:
		r, ok := t2.(*TCon)
		if !ok {
			return mismatch(t1, t2, "expected a type constructor")
		}
		if l.Name != r.Name {
			return mismatch(t1, t2, "different type constructors")
		}
		if len(l.Args) != len(r.Args) {
			return mismatch(t1, t2, "type constructor arity mismatch")
		}
		for i := range l.Args {
			if err := unify(sub, l.Args[i], r.Args[i]); err != nil {
				return err
			}
		}
		return nil

	case *TChan:
		r, ok := t2.(*TChan)
		if !ok {
			return mismatch(t1, t2, "expected a channel type")
		}
		return unify(sub, l.Elem, r.Elem)

	case *TMap:
		r, ok := t2.(*TMap)
		if !ok {
			return mismatch(t1, t2, "expected a map type")
		}
		if err := unify(sub, l.Key, r.Key); err != nil {
			return err
		}
		return unify(sub, l.Val, r.Val)

	case *TError:
		switch t2.(type) {
		case *TError:
			return nil
		case *TPtr:
			// null (nil) is a valid error value
			return nil
		default:
			return mismatch(t1, t2, "expected error")
		}

	case *TPtr:
		switch r := t2.(type) {
		case *TPtr:
			return unify(sub, l.Elem, r.Elem)
		case *TError:
			return nil
		case *TGoNamed:
			// *Implementor used where a Go interface is expected (slog.New, etc.).
			if r.Interface {
				return nil
			}
			return mismatch(t1, t2, "expected a pointer")
		default:
			return mismatch(t1, t2, "expected a pointer")
		}

	case *TGoSlice:
		r, ok := t2.(*TGoSlice)
		if !ok {
			return mismatch(t1, t2, "expected a go_slice")
		}
		return unify(sub, l.Elem, r.Elem)

	case *TGoNamed:
		switch r := t2.(type) {
		case *TGoNamed:
			if l.Pkg == r.Pkg && l.Name == r.Name {
				return nil
			}
			return mismatch(t1, t2, "different Go named types")
		case *TRecord, *TAdt:
			// Implementor value used where a Go interface is expected.
			if l.Interface {
				return nil
			}
			return mismatch(t1, t2, "expected Go named type "+l.String())
		case *TPtr:
			if l.Interface {
				return nil
			}
			return mismatch(t1, t2, "expected Go named type "+l.String())
		default:
			return mismatch(t1, t2, "expected Go named type")
		}
	}

	return mismatch(t1, t2, "unknown type")
}

// occurs checks if a type variable ID appears in a type.
func occurs(vid int64, t Type) bool {
	switch t := t.(type) {
	case *TVar:
		return t.ID == vid
	case *TFun:
		if occurs(vid, t.From) || occurs(vid, t.To) {
			return true
		}
		if t.Effects != nil && t.Effects.Rest != nil {
			if t.Effects.Rest.ID == vid {
				return true
			}
		}
		return false
	case *TTuple:
		for _, e := range t.Elems {
			if occurs(vid, e) {
				return true
			}
		}
	case *TRecord:
		if t == nil {
			return false
		}
		for _, f := range t.Fields {
			if occurs(vid, f.Type) {
				return true
			}
		}
	case *TAdt:
		for _, p := range t.Params {
			if occurs(vid, p) {
				return true
			}
		}
	case *PolyVariant:
		for _, v := range t.Variants {
			if v.Arg != nil && occurs(vid, v.Arg) {
				return true
			}
		}
	case *TCon:
		for _, a := range t.Args {
			if occurs(vid, a) {
				return true
			}
		}
	case *TChan:
		return occurs(vid, t.Elem)
	case *TMap:
		return occurs(vid, t.Key) || occurs(vid, t.Val)
	}
	return false
}

func fieldMap(r *TRecord) map[string]Type {
	if r == nil {
		return nil
	}
	m := make(map[string]Type)
	for _, f := range r.Fields {
		m[f.Name] = f.Type
	}
	return m
}

func variantMap(variants []Variant) map[string]Variant {
	out := make(map[string]Variant, len(variants))
	for _, v := range variants {
		out[v.Name] = v
	}
	return out
}

// unifyEffects unifies the effect rows of two function types.
// Nil Effects means "unknown" (an implicit open row variable) for
// backward compat. Only explicitly `with {}` means pure (empty closed).
func unifyEffects(sub Subst, l, r *TFun) error {
	// Nil on either side means unknown — create a fresh open row variable
	// and treat both sides as having that variable. This means existing
	// code without effect annotations is permissive.
	if l.Effects == nil || r.Effects == nil {
		// If both are nil, nothing to unify (both are unknown/permissive).
		// If one is nil and the other is explicit, the explicit side
		// imposes constraints: the nil side gets the explicit row via
		// a fresh TVar for the rest, or if explicit is closed, the nil
		// side must match exactly.
		if l.Effects == nil && r.Effects == nil {
			return nil
		}
		if r.Effects == nil {
			// Swap so l has the explicit one
			l, r = r, l
		}
		// Now r.Effects is nil (unknown), l.Effects is explicit.
		// Create a fresh row variable for the unknown side's rest.
		// If l.Effects is closed, r must be exactly those effects.
		if r.Effects == nil {
			// r is unknown — it can be unified with l.Effects.
			// We mark r as having l's effects.
			r.Effects = &EffectRow{
				Effects: l.Effects.Effects,
				Open:    l.Effects.Open,
				Rest:    l.Effects.Rest,
			}
		}
		return nil
	}

	// Both sides have explicit effect rows.
	le, re := l.Effects, r.Effects

	// Two closed rows must match exactly (same effect set).
	if !le.Open && !re.Open {
		if len(le.Effects) != len(re.Effects) {
			return mismatch(l, r, "effect row mismatch")
		}
		for _, eff := range le.Effects {
			if !hasEffect(re.Effects, eff) {
				return mismatch(l, r, "effect row missing effect: "+eff)
			}
		}
		return nil
	}

	// At least one side is open: the closed/lesser side must have all
	// effects of the open side (or vice versa — same as record rows).
	// Like rows, we check that the set of required effects overlaps.
	// If le is open, re must have at least le's effects.
	if le.Open {
		for _, eff := range le.Effects {
			if !hasEffect(re.Effects, eff) {
				// re might be open too — but still needs those effects
				if re.Open {
					// Add the missing effect to re's set and continue
					re.Effects = append(re.Effects, eff)
				} else {
					return mismatch(l, r, "effect row missing effect: "+eff)
				}
			}
		}
		// Unify rest variables if both are open
		if re.Open && le.Rest != nil && re.Rest != nil {
			if err := unify(sub, le.Rest, re.Rest); err != nil {
				return err
			}
		}
	}
	if re.Open {
		for _, eff := range re.Effects {
			if !hasEffect(le.Effects, eff) {
				if le.Open {
					le.Effects = append(le.Effects, eff)
				} else {
					return mismatch(l, r, "effect row missing effect: "+eff)
				}
			}
		}
		// Unify rest variables if both are open
		if le.Open && le.Rest != nil && re.Rest != nil {
			if err := unify(sub, le.Rest, re.Rest); err != nil {
				return err
			}
		}
	}

	return nil
}

func hasEffect(effects []string, name string) bool {
	for _, e := range effects {
		if e == name {
			return true
		}
	}
	return false
}

func mismatch(t1, t2 Type, reason string) *UnifyError {
	return &UnifyError{
		Msg:   reason,
		Left:  t1,
		Right: t2,
	}
}
