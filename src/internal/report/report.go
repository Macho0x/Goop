// Package report renders Goop errors in Lisette-style graphical format.
package report

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// helpTips maps diagnostic code prefixes to short actionable help lines.
var helpTips = map[string]string{
	"EXHAUST003":   "Handle every constructor, or add a wildcard `| _ -> …` arm.",
	"EXHAUST001":   "Remove the unreachable pattern or reorder arms.",
	"EXHAUST002":   "Remove the unreachable pattern or reorder arms.",
	"NIL001":       "Initialize with Chan.make () (or OwnedChan.make ()) before send/recv/close.",
	"RESULT001":    "Handle the result with match, or discard explicitly: let _ = …",
	"OPTION001":    "Handle the option with match, or discard explicitly: let _ = …",
	"UNUSED001":    "Use the binding, rename to `_`, or remove it.",
	"UNUSED002":    "Remove the unused import or reference a binding from it.",
	"VIS001":       "Export the binding (drop private) or keep access inside its module.",
	"VIS002":       "Make the type public, or keep the API private.",
	"IMPORT001":    "Fix the import path or add a [mappings] entry in goop.toml.",
	"IMPORT002":    "Remove the duplicate import.",
	"IMPORT003":    "Run check/build with a real source file path so imports can resolve.",
	"IMPORT004":    "Rename the import binding or remove the conflicting declaration.",
	"TYPE002":      "Rename the binding so it does not collide with an existing name.",
	"TYPE003":      "Rewrite using a supported expression, or report a compiler bug.",
	"TYPE004":      "Use a supported operator (see message), or call a library function.",
	"TYPE005":      "Use a defined constructor, or add it to the ADT.",
	"TYPE006":      "Drop the payload, or give the constructor an argument type.",
	"TYPE007":      "Use a field that exists on the record, or extend the type.",
	"TYPE008":      "Match the tuple arity of the scrutinee.",
	"TYPE009":      "Pattern-match a tuple value, or change the pattern.",
	"TYPE010":      "Check annotated types and argument order; see the mismatch details above.",
	"TYPE011":      "Use := for refs or <- for array/mutable fields; locals are immutable.",
	"TYPE012":      "Assign to arr.(i), a mutable field, or a ref with :=.",
	"TYPE013":      "Use a constructor defined on that ADT, with the correct qualifier.",
	"ROW001":       "Supply every field required by the open row parameter.",
	"DECIMAL001":   "Use std.decimal.Decimal for money/prices instead of float.",
	"LINEAR001":    "Discharge the linear value on every path (call a consumer or close).",
	"LINEAR002":    "Do not use a linear value after it has been consumed.",
	"LINEAR003":    "Discharge the linear value in the then-branch.",
	"LINEAR004":    "Discharge the linear value in the else-branch.",
	"LINEAR005":    "Discharge the linear value in every match arm.",
	"LINEAR006":    "Do not share mutable refs across goroutines; use channels or go (move …).",
	"LINEAR007":    "Move or synchronize the captured mutable value before spawning.",
	"LINEAR008":    "Avoid racing channel-mediated mutable state across goroutines.",
	"DEADLOCK001":  "Reorder sends/recvs or buffer the channel to break the cycle.",
	"PARSE001":     "Fix the token at the caret; see what was expected.",
	"PARSE003":     "Move import declarations before other top-level items.",
	"PARSE004":     "Check for a missing declaration keyword or stray token.",
	"PARSE005":     "Active patterns use (|Name|_| …) naming.",
	"PARSE006":     "Provide a binding name after let.",
	"PARSE013":     "Unexpected token in expression; check parentheses and keywords.",
	"PARSE017":     "Unexpected token in pattern; check constructors and punctuation.",
	"PARSE022":     "Unexpected token in type; check arrows, records, and parentheses.",
	"PARSE-MIG002": "Use import go \"path\" { val … } instead of extern.",
	"PARSE-MIG010": "Use ref / := / ! instead of let mutable.",
	"PARSE-MIG012": "Use match on result instead of ?.",
	"PARSE-MIG013": "Use match / try/finally instead of computation expressions.",
	"PARSE-MIG014": "Use match instead of guard / is / expression as.",
	"PARSE-MIG015": "Use a single-constructor ADT: type id = Id of string (optionally private).",
	"PARSE-MIG016": "Drop with { io } on arrows; use effect / perform / handlers.",
	"PARSE-MIG017": "Use failwith or raise instead of panic.",
	"PARSE-MIG018": "Use mod instead of %.",
	"REFINE001":    "Strengthen the precondition or fix the call site so the VC holds.",
	"REFINE002":    "Prove the call site, or accept a runtime guard (severity via goop.toml).",
	"CODEGEN001":   "This construct is not lowered yet; simplify or report a bug.",
	"CODEGEN002":   "Supply every field required by the row parameter.",
	"CODEGEN003":   "Pass the Go method receiver as the first argument.",
	"GOSIG001":     "Fix goop-sigs override syntax or regenerate with goop gen-sig.",
	"GOSIG002":     "Add an explicit { val … } block or improve the stub mapping.",
	"GOSIG003":     "Align the hand { val … } signature with the Go package, or disable verify_ffi.",
	"GOSIG004":     "Omit the generic export, use a concrete @[go] wrapper, or see docs/design/32-go-generics-sigs.md.",
	"UNIFY020":     "Expected a map[K] V value; check the annotation or argument.",
	"UNIFY021":     "Expected a pointer (T ptr); check the argument type.",
	"UNIFY022":     "Expected a go_slice; use go_slice_of_list or declare the FFI type.",
	"UNIFY":        "Check annotated types and argument order; see the mismatch details above.",
	"TYPE":         "Check the expression type against its annotation or expected context.",
}

func tipForMessage(rest string) string {
	upper := strings.ToUpper(rest)
	// Prefer longer / more specific codes first by scanning the map for any hit;
	// check multi-segment MIG codes before short prefixes via ordered list.
	ordered := []string{
		"PARSE-MIG018", "PARSE-MIG017", "PARSE-MIG016", "PARSE-MIG015", "PARSE-MIG014",
		"PARSE-MIG013", "PARSE-MIG012", "PARSE-MIG010", "PARSE-MIG002",
		"EXHAUST003", "EXHAUST001", "EXHAUST002",
		"LINEAR008", "LINEAR007", "LINEAR006", "LINEAR005", "LINEAR004", "LINEAR003", "LINEAR002", "LINEAR001",
		"UNUSED002", "UNUSED001", "OPTION001", "RESULT001", "DEADLOCK001", "NIL001", "DECIMAL001", "ROW001",
		"VIS002", "VIS001", "IMPORT004", "IMPORT003", "IMPORT002", "IMPORT001",
		"TYPE013", "TYPE012", "TYPE011", "TYPE010", "TYPE009", "TYPE008", "TYPE007", "TYPE006", "TYPE005", "TYPE004", "TYPE003", "TYPE002",
		"PARSE022", "PARSE017", "PARSE013", "PARSE006", "PARSE005", "PARSE004", "PARSE003", "PARSE001",
		"CODEGEN003", "CODEGEN002", "CODEGEN001", "GOSIG004", "GOSIG003", "GOSIG002", "GOSIG001",
		"UNIFY022", "UNIFY021", "UNIFY020", "REFINE002", "REFINE001",
		"UNIFY", "TYPE",
	}
	for _, code := range ordered {
		if tip, ok := helpTips[code]; ok && strings.Contains(upper, code) {
			return tip
		}
	}
	return ""
}

// Render turns a Goop error (already containing "file:line:col: msg") into
// a Lisette-style diagnostic using the provided source text.
func Render(err error, src []byte) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Parse "file:line:col: rest"
	parts := strings.SplitN(msg, ":", 4)
	if len(parts) < 4 {
		if strings.HasSuffix(msg, "\n") {
			return msg
		}
		return msg + "\n" // location-less diagnostics must not concatenate on fmt.Print
	}
	file := strings.TrimSpace(parts[0])
	line, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	col, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
	rest := strings.TrimSpace(parts[3])

	lines := strings.Split(string(src), "\n")
	if line < 1 || line > len(lines) {
		if strings.HasSuffix(msg, "\n") {
			return msg
		}
		return msg + "\n"
	}

	// Build the box (3-line window around the error)
	var b strings.Builder
	b.WriteString("✕ " + rest + "\n")
	b.WriteString(fmt.Sprintf("╭─[%s:%d:%d]\n", file, line, col))

	start := max(0, line-2)
	end := min(len(lines), line+1)
	for i := start; i < end; i++ {
		prefix := " "
		if i+1 == line {
			prefix = ">"
		}
		b.WriteString(fmt.Sprintf("%s %d │ %s\n", prefix, i+1, lines[i]))
		if i+1 == line {
			// underline
			indent := strings.Repeat(" ", col-1)
			b.WriteString("  · " + indent + "╰── " + rest + "\n")
		}
	}
	b.WriteString("╰────\n")
	if tip := tipForMessage(rest); tip != "" {
		b.WriteString("help: " + tip + "\n")
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RenderFromFile is the convenience form used by cmd/goop.
func RenderFromFile(err error, filename string) string {
	src, _ := os.ReadFile(filename)
	return Render(err, src)
}
