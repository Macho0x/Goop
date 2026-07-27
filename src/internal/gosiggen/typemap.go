package gosiggen

import (
	"fmt"
	"strings"

	gotypes "go/types"
)

// MapResult is the outcome of mapping a Go type to a Goop .gosig type string.
type MapResult struct {
	Goop    string // Goop type text when OK
	OK      bool
	Reason  string // why mapping failed or a soft warning
	TODOH6  bool   // true when (T, error) product; typecheck coerces call sites to result
	Skipped bool   // true when intentionally omitted (map, etc.)
}

// MapType converts a go/types.Type into a Goop type string suitable for a
// .gosig val/type signature.
//
// Mapping policy (H5 foundation):
//   - primitives → Goop primitives (int/float/bool/string/rune/bytes/error)
//   - any / interface{} → obj
//   - *T → T ptr
//   - []T → T go_slice (best-effort; []byte → bytes)
//   - chan T → T chan
//   - maps → skipped (not yet representable)
//   - (T, error) → "T * error" with TODOH6=true (typecheck/codegen coerce to result)
//   - other multi-results → skipped
//   - named types in pkgPath → short name; others → pkg.Name
func MapType(t gotypes.Type, pkgPath string) MapResult {
	if t == nil {
		return MapResult{Reason: "nil type"}
	}
	return mapType(t, pkgPath, 0)
}

const maxTypeDepth = 16

func mapType(t gotypes.Type, pkgPath string, depth int) MapResult {
	if depth > maxTypeDepth {
		return MapResult{Reason: "type nesting too deep"}
	}
	// Preserve rune/byte alias spellings before Unalias peels them to int32/uint8.
	if a, ok := t.(*gotypes.Alias); ok {
		switch a.Obj().Name() {
		case "rune":
			return MapResult{Goop: "rune", OK: true}
		case "byte":
			return MapResult{Goop: "int", OK: true} // Goop has no byte; matches typecheck
		case "any":
			return MapResult{Goop: "obj", OK: true}
		}
	}
	t = gotypes.Unalias(t)

	switch u := t.(type) {
	case *gotypes.Basic:
		return mapBasic(u)

	case *gotypes.Pointer:
		elem := mapType(u.Elem(), pkgPath, depth+1)
		if !elem.OK {
			return MapResult{Reason: "pointer elem: " + elem.Reason, Skipped: elem.Skipped}
		}
		return MapResult{Goop: elem.Goop + " ptr", OK: true}

	case *gotypes.Slice:
		if basic, ok := u.Elem().(*gotypes.Basic); ok && basic.Kind() == gotypes.Byte {
			return MapResult{Goop: "bytes", OK: true}
		}
		elem := mapType(u.Elem(), pkgPath, depth+1)
		if !elem.OK {
			return MapResult{
				Reason:  "slice elem: " + elem.Reason,
				Skipped: true,
			}
		}
		// Best-effort: Go slices → Goop go_slice (see design doc).
		return MapResult{Goop: elem.Goop + " go_slice", OK: true}

	case *gotypes.Array:
		elem := mapType(u.Elem(), pkgPath, depth+1)
		if !elem.OK {
			return MapResult{Reason: "array elem: " + elem.Reason, Skipped: true}
		}
		// Fixed arrays: treat like go_slice for MVP surface.
		return MapResult{Goop: elem.Goop + " go_slice", OK: true}

	case *gotypes.Chan:
		elem := mapType(u.Elem(), pkgPath, depth+1)
		if !elem.OK {
			return MapResult{Reason: "chan elem: " + elem.Reason, Skipped: true}
		}
		return MapResult{Goop: elem.Goop + " chan", OK: true}

	case *gotypes.Map:
		key := mapType(u.Key(), pkgPath, depth+1)
		if !key.OK {
			return MapResult{Reason: "map key: " + key.Reason, Skipped: true}
		}
		val := mapType(u.Elem(), pkgPath, depth+1)
		if !val.OK {
			return MapResult{Reason: "map val: " + val.Reason, Skipped: true}
		}
		return MapResult{Goop: "map[" + key.Goop + "] " + val.Goop, OK: true}

	case *gotypes.Signature:
		return mapSignature(u, pkgPath, depth)

	case *gotypes.Named:
		return mapNamed(u, pkgPath)

	case *gotypes.Alias:
		return mapType(u.Underlying(), pkgPath, depth+1)

	case *gotypes.Interface:
		if u.Empty() {
			// interface{} / any
			return MapResult{Goop: "obj", OK: true}
		}
		// Non-empty anonymous interface — not representable as a named import.
		return MapResult{Reason: "anonymous interface not representable", Skipped: true}

	case *gotypes.Struct:
		return MapResult{Reason: "anonymous struct not representable", Skipped: true}

	case *gotypes.Tuple:
		// Bare tuples only appear as multi-results; handled in mapResults.
		return MapResult{Reason: "bare tuple not representable", Skipped: true}

	default:
		return MapResult{Reason: fmt.Sprintf("unsupported Go type %T", t), Skipped: true}
	}
}

func mapBasic(b *gotypes.Basic) MapResult {
	// Prefer alias names so rune/byte stay distinct from int.
	switch b.Name() {
	case "rune":
		return MapResult{Goop: "rune", OK: true}
	case "byte":
		return MapResult{Goop: "int", OK: true} // Goop has no byte; matches typecheck
	}
	switch b.Kind() {
	case gotypes.Bool, gotypes.UntypedBool:
		return MapResult{Goop: "bool", OK: true}
	case gotypes.Int, gotypes.Int8, gotypes.Int16, gotypes.Int32, gotypes.Int64,
		gotypes.Uint, gotypes.Uint8, gotypes.Uint16, gotypes.Uint32, gotypes.Uint64,
		gotypes.Uintptr, gotypes.UntypedInt:
		return MapResult{Goop: "int", OK: true}
	case gotypes.Float32, gotypes.Float64, gotypes.UntypedFloat:
		return MapResult{Goop: "float", OK: true}
	case gotypes.String, gotypes.UntypedString:
		return MapResult{Goop: "string", OK: true}
	case gotypes.UntypedRune:
		return MapResult{Goop: "rune", OK: true}
	case gotypes.Complex64, gotypes.Complex128, gotypes.UntypedComplex:
		return MapResult{Reason: "complex numbers not representable", Skipped: true}
	case gotypes.UnsafePointer:
		return MapResult{Reason: "unsafe.Pointer not representable", Skipped: true}
	default:
		// byte is Uint8; rune is Int32 — covered above via kind.
		if b.Info()&gotypes.IsBoolean != 0 {
			return MapResult{Goop: "bool", OK: true}
		}
		if b.Info()&gotypes.IsInteger != 0 {
			return MapResult{Goop: "int", OK: true}
		}
		if b.Info()&gotypes.IsFloat != 0 {
			return MapResult{Goop: "float", OK: true}
		}
		if b.Info()&gotypes.IsString != 0 {
			return MapResult{Goop: "string", OK: true}
		}
		return MapResult{Reason: "unsupported basic type " + b.Name(), Skipped: true}
	}
}

func mapNamed(n *gotypes.Named, pkgPath string) MapResult {
	obj := n.Obj()
	if obj == nil {
		return MapResult{Reason: "named type with nil object", Skipped: true}
	}
	name := obj.Name()
	if name == "error" && (obj.Pkg() == nil || obj.Pkg().Path() == "") {
		return MapResult{Goop: "error", OK: true}
	}
	// universe "error" is an interface named type in some go versions —
	// also detect by underlying empty? Prefer name check via TypeString.
	if isErrorType(n) {
		return MapResult{Goop: "error", OK: true}
	}
	pkg := obj.Pkg()
	if pkg == nil {
		// Builtin named — rare.
		return MapResult{Goop: name, OK: true}
	}
	if pkg.Path() == pkgPath {
		return MapResult{Goop: name, OK: true}
	}
	return MapResult{Goop: pkg.Name() + "." + name, OK: true}
}

func isErrorType(t gotypes.Type) bool {
	return gotypes.Identical(t, gotypes.Universe.Lookup("error").Type())
}

func mapSignature(sig *gotypes.Signature, pkgPath string, depth int) MapResult {
	results := mapResults(sig.Results(), pkgPath, depth+1)
	if !results.OK {
		return results
	}

	params := sig.Params()
	variadic := sig.Variadic()
	parts := make([]string, 0, params.Len()+1)

	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		pt := p.Type()
		isVar := variadic && i == params.Len()-1
		if isVar {
			// Variadic param is []T in go/types; peel to T then prefix ...
			if sl, ok := pt.(*gotypes.Slice); ok {
				pt = sl.Elem()
			}
		}
		mr := mapType(pt, pkgPath, depth+1)
		if !mr.OK {
			return MapResult{Reason: fmt.Sprintf("param %d: %s", i, mr.Reason), Skipped: mr.Skipped}
		}
		goop := mr.Goop
		// Parenthesize function-typed params so outer curry doesn't flatten.
		if _, isSig := gotypes.Unalias(pt).(*gotypes.Signature); isSig && !isVar {
			goop = "(" + goop + ")"
		}
		if isVar {
			parts = append(parts, "..."+goop)
		} else {
			parts = append(parts, goop)
		}
	}

	if len(parts) == 0 {
		// Nullary → unit -> R
		return MapResult{Goop: "unit -> " + results.Goop, OK: true, TODOH6: results.TODOH6}
	}

	parts = append(parts, results.Goop)
	return MapResult{
		Goop:   strings.Join(parts, " -> "),
		OK:     true,
		TODOH6: results.TODOH6,
	}
}

// mapResults maps Go result tuple to a Goop return type.
func mapResults(tup *gotypes.Tuple, pkgPath string, depth int) MapResult {
	if tup == nil || tup.Len() == 0 {
		return MapResult{Goop: "unit", OK: true}
	}
	if tup.Len() == 1 {
		return mapType(tup.At(0).Type(), pkgPath, depth)
	}

	// Multi-return: special-case (T, error) for H6.
	if tup.Len() == 2 && isErrorType(tup.At(1).Type()) {
		first := mapType(tup.At(0).Type(), pkgPath, depth)
		if !first.OK {
			return MapResult{
				Reason:  "(T, error) where T unmappable: " + first.Reason,
				Skipped: true,
				TODOH6:  true,
			}
		}
		// (T, error) stays as product in .gosig; typecheck+codegen coerce
		// call sites to result<T, error> (H6). Mark ErrorTuple for tooling.
		return MapResult{
			Goop:   first.Goop + " * error",
			OK:     true,
			TODOH6: true, // historical flag: product form, coerced at check/codegen
			Reason: "(T, error) product; typecheck coerces call sites to result",
		}
	}

	// Other multi-results → Goop tuple product (e.g. int * bool for Cut).
	parts := make([]string, 0, tup.Len())
	for i := 0; i < tup.Len(); i++ {
		mr := mapType(tup.At(i).Type(), pkgPath, depth)
		if !mr.OK {
			return MapResult{
				Reason:  fmt.Sprintf("multi-result elem %d: %s", i, mr.Reason),
				Skipped: true,
			}
		}
		parts = append(parts, mr.Goop)
	}
	return MapResult{Goop: strings.Join(parts, " * "), OK: true}
}

// ReceiverTypeString maps a method receiver to a Goop type for
// `val (x : Recv).M` declarations. Pointer receivers become `T ptr`.
func ReceiverTypeString(recv *gotypes.Var, pkgPath string) MapResult {
	if recv == nil {
		return MapResult{Reason: "nil receiver"}
	}
	return MapType(recv.Type(), pkgPath)
}
