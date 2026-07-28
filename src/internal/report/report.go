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
	"EXHAUST003":  "Handle every constructor, or add a wildcard `| _ -> …` arm.",
	"EXHAUST001":  "Remove the unreachable pattern or reorder arms.",
	"EXHAUST002":  "Remove the unreachable pattern or reorder arms.",
	"NIL001":      "Initialize with Chan.make () (or OwnedChan.make ()) before send/recv/close.",
	"RESULT001":   "Handle the result with match, or discard explicitly: let _ = …",
	"LINEAR006":   "Do not share mutable refs across goroutines; use channels or go (move …).",
	"LINEAR007":   "Move or synchronize the captured mutable value before spawning.",
	"LINEAR008":   "Avoid racing channel-mediated mutable state across goroutines.",
	"DEADLOCK001": "Reorder sends/recvs or buffer the channel to break the cycle.",
	"PARSE-MIG010": "Use ref / := / ! instead of let mutable.",
	"PARSE-MIG011": "Use ref / := for rebinding; keep arr.(i) <- for array cells.",
	"PARSE-MIG015": "Use a single-constructor ADT: type id = Id of string (optionally private).",
	"PARSE-MIG016": "Drop with { io } on arrows; use effect / perform / handlers.",
	"PARSE-MIG017": "Use failwith or raise instead of panic.",
	"REFINE001":   "Strengthen the precondition or fix the call site so the VC holds.",
	"REFINE002":   "Prove the call site, or accept a runtime guard (severity via goop.toml).",
	"UNIFY":       "Check annotated types and argument order; see the mismatch details above.",
	"TYPE":        "Check the expression type against its annotation or expected context.",
}

func tipForMessage(rest string) string {
	upper := strings.ToUpper(rest)
	for code, tip := range helpTips {
		if strings.Contains(upper, code) {
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
		return msg // fallback for CLI errors without location
	}
	file := strings.TrimSpace(parts[0])
	line, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	col, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
	rest := strings.TrimSpace(parts[3])

	lines := strings.Split(string(src), "\n")
	if line < 1 || line > len(lines) {
		return msg
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
