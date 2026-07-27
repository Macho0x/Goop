package fmt

import (
	"goop.dev/compiler/internal/ast"
)

// FormatType renders a type for display (signatures, docs, formatting).
func FormatType(t ast.Type) string {
	return formatType(t)
}

func formatType(t ast.Type) string {
	if t == nil {
		return ""
	}
	switch t := t.(type) {
	case *ast.TIdent:
		return t.Name
	case *ast.TApp:
		return formatType(t.Func) + " " + formatType(t.Arg)
	case *ast.TFun:
		s := formatType(t.From) + " -> " + formatType(t.To)
		if t.Effects != nil {
			s += formatEffects(t.Effects)
		}
		return s
	case *ast.TTuple:
		var elems []string
		for _, e := range t.Elems {
			elems = append(elems, formatType(e))
		}
		return "(" + stringsJoin(elems, " * ") + ")"
	case *ast.TRecord:
		var fields []string
		for _, f := range t.Fields {
			fields = append(fields, f.Name+": "+formatType(f.Type))
		}
		s := "{ " + stringsJoin(fields, "; ") + " "
		if t.Open {
			s += "| .. "
		}
		return s + "}"
	case *ast.TVar:
		return t.Name
	case *ast.RefinementType:
		return formatType(t.Inner) + " where " + formatExprInline(t.Pred, precLowest)
	case *ast.TChan:
		return formatType(t.Elem) + " chan"
	case *ast.TMap:
		return "map[" + formatType(t.Key) + "] " + formatType(t.Val)
	default:
		return "<type>"
	}
}

func formatEffects(e *ast.EffectRowType) string {
	if e == nil {
		return ""
	}
	effs := stringsJoin(e.Effects, "; ")
	if e.Open {
		if e.Rest != "" {
			effs += " | " + e.Rest
		} else {
			effs += " | .."
		}
	}
	if effs == "" {
		return ""
	}
	return " with { " + effs + " }"
}
