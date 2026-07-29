// Interactive Read-Eval-Print loop for Goop.
//
// MVP strategy: accumulate declarations in a session buffer, and for each
// expression compile a small temp module then `go run` it. Goop has no
// interpreter — this is a pragmatic compile-to-Go REPL.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/effects"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/report"
	"goop.dev/compiler/internal/typecheck"
	"goop.dev/compiler/internal/types"
)

const replFileName = "repl.goop"

// runREPL starts an interactive session on stdin/stdout.
func runREPL() int {
	fmt.Fprintf(os.Stdout, "Goop REPL — compile-to-Go session (type :help for commands)\n")
	if err := runREPLWith(os.Stdin, os.Stdout, os.Stderr); err != nil {
		if err == io.EOF {
			fmt.Fprintln(os.Stdout)
			return 0
		}
		fmt.Fprintf(os.Stderr, "repl error: %v\n", err)
		return 1
	}
	return 0
}

func runREPLWith(in io.Reader, out, errOut io.Writer) error {
	s := &replSession{
		cfg:    loadProjectConfig("."),
		out:    out,
		errOut: errOut,
	}
	sc := bufio.NewScanner(in)
	// Allow long pasted snippets.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	var pending strings.Builder
	prompt := "goop> "
	for {
		fmt.Fprint(out, prompt)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return err
			}
			return io.EOF
		}
		line := sc.Text()

		// Line continuation with trailing '\'.
		if strings.HasSuffix(line, "\\") {
			pending.WriteString(strings.TrimSuffix(line, "\\"))
			pending.WriteByte('\n')
			prompt = "...> "
			continue
		}
		if pending.Len() > 0 {
			pending.WriteString(line)
			line = pending.String()
			pending.Reset()
		}
		prompt = "goop> "

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if err := s.handleLine(line); err != nil {
			if err == errREPLQuit {
				return nil
			}
			fmt.Fprintf(errOut, "%v\n", err)
		}
	}
}

var errREPLQuit = fmt.Errorf("quit")

type replSession struct {
	imports []string
	decls   []string
	cfg     *config.Config
	out     io.Writer
	errOut  io.Writer
}

func (s *replSession) handleLine(line string) error {
	if strings.HasPrefix(line, ":") {
		return s.handleCommand(line)
	}
	if isREPLDecl(line) {
		return s.handleDecl(line)
	}
	return s.handleExpr(line)
}

func (s *replSession) handleCommand(line string) error {
	fields := strings.Fields(line)
	cmd := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(line, cmd))

	switch cmd {
	case ":q", ":quit", ":exit":
		fmt.Fprintln(s.out, "bye")
		return errREPLQuit
	case ":h", ":help":
		fmt.Fprint(s.out, replHelp)
		return nil
	case ":reset":
		s.imports = nil
		s.decls = nil
		fmt.Fprintln(s.out, "session cleared")
		return nil
	case ":type", ":t":
		if rest == "" {
			return fmt.Errorf("usage: :type <expr>")
		}
		typ, err := s.inferExprType(rest)
		if err != nil {
			return err
		}
		fmt.Fprintf(s.out, "- : %s\n", typ)
		return nil
	default:
		return fmt.Errorf("unknown command %q (try :help)", cmd)
	}
}

const replHelp = `Commands:
  :help, :h          show this help
  :quit, :q, :exit   leave the REPL
  :type <expr>, :t   print the inferred type of an expression
  :reset             clear session declarations

Enter declarations (let, type, import, …) or expressions.
Expressions are compiled to a temp Go program and run with go run.
Continuation: end a line with \ to keep typing.
`

func (s *replSession) handleDecl(line string) error {
	// Imports must precede declarations in Goop source.
	if isREPLImport(line) {
		src := s.buildModule(line, true)
		if err := s.checkOnly(src); err != nil {
			return err
		}
		s.imports = append(s.imports, line)
		fmt.Fprintln(s.out, "ok")
		return nil
	}

	// let main — execute as a program; do not keep as a permanent decl
	// (would clash with expression wrappers).
	if isREPLMain(line) {
		src := s.buildModule(line, false)
		out, err := s.compileAndRun(src)
		if err != nil {
			return err
		}
		if out != "" {
			fmt.Fprint(s.out, out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Fprintln(s.out)
			}
		} else {
			fmt.Fprintln(s.out, "ok")
		}
		return nil
	}

	src := s.buildModule(line, false)
	if err := s.checkOnly(src); err != nil {
		return err
	}
	s.decls = append(s.decls, line)
	fmt.Fprintln(s.out, "ok")
	return nil
}

func (s *replSession) handleExpr(expr string) error {
	typ, err := s.inferExprType(expr)
	if err != nil {
		return err
	}

	wrapper, printable := wrapExprForPrint(expr, typ)
	if !printable {
		fmt.Fprintf(s.out, "- : %s  (value not printable in REPL)\n", typ)
		return nil
	}

	src := s.buildModule(wrapper, false)
	out, err := s.compileAndRun(src)
	if err != nil {
		return err
	}
	if typ == "unit" {
		if out == "" {
			fmt.Fprintln(s.out, "()")
		} else {
			fmt.Fprint(s.out, out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Fprintln(s.out)
			}
		}
		return nil
	}
	fmt.Fprint(s.out, out)
	if out != "" && !strings.HasSuffix(out, "\n") {
		fmt.Fprintln(s.out)
	}
	return nil
}

func (s *replSession) inferExprType(expr string) (string, error) {
	probe := "let __repl_probe () = (" + expr + ")"
	src := s.buildModule(probe, false)
	mod, srcBytes, err := s.parseDesugar(src)
	if err != nil {
		return "", err
	}
	tm, vtm, typeErrs := typecheck.CheckWithTypesForFile(mod, replFileName, s.cfg, nil)
	if len(typeErrs) > 0 {
		return "", formatREPLErrors(typeErrs, srcBytes)
	}

	// Prefer var scheme: __repl_probe : unit -> T
	if t, ok := vtm["__repl_probe"]; ok {
		if f, ok := t.(*types.TFun); ok {
			return f.To.String(), nil
		}
		return t.String(), nil
	}
	for _, d := range mod.Decls {
		ld, ok := d.(*ast.LetDecl)
		if !ok {
			continue
		}
		for _, b := range ld.Bindings {
			if b.Name != "__repl_probe" || b.Body == nil {
				continue
			}
			if t, ok := tm[b.Body]; ok {
				return t.String(), nil
			}
		}
	}
	_ = tm
	return "?", fmt.Errorf("could not infer type")
}

func wrapExprForPrint(expr, typ string) (wrapper string, printable bool) {
	switch typ {
	case "int":
		return "let main () =\n  println (int_to_string (" + expr + "))", true
	case "float":
		return "let main () =\n  println (float_to_string (" + expr + "))", true
	case "string":
		return "let main () =\n  println (" + expr + ")", true
	case "bool":
		return "let main () =\n  println (if (" + expr + ") then \"true\" else \"false\")", true
	case "unit":
		return "let main () =\n  (" + expr + ")", true
	default:
		return "", false
	}
}

func (s *replSession) buildModule(extra string, extraIsImport bool) string {
	var b strings.Builder
	b.WriteString("module main\n\n")
	for _, im := range s.imports {
		b.WriteString(im)
		b.WriteByte('\n')
	}
	if extraIsImport && extra != "" {
		b.WriteString(extra)
		b.WriteByte('\n')
	}
	if len(s.imports) > 0 || extraIsImport {
		b.WriteByte('\n')
	}
	for _, d := range s.decls {
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	if !extraIsImport && extra != "" {
		b.WriteString(extra)
		b.WriteByte('\n')
	}
	return b.String()
}

func (s *replSession) parseDesugar(src string) (*ast.Module, []byte, error) {
	srcBytes := []byte(src)
	mod, err := parser.Parse(replFileName, srcBytes)
	if err != nil {
		return nil, srcBytes, err
	}
	return desugar.DesugarModule(mod), srcBytes, nil
}

func (s *replSession) checkOnly(src string) error {
	mod, srcBytes, err := s.parseDesugar(src)
	if err != nil {
		return err
	}
	_, _, typeErrs := typecheck.CheckWithTypesForFile(mod, replFileName, s.cfg, nil)
	if len(typeErrs) > 0 {
		return formatREPLErrors(typeErrs, srcBytes)
	}
	return nil
}

func (s *replSession) compileAndRun(src string) (string, error) {
	mod, srcBytes, err := s.parseDesugar(src)
	if err != nil {
		return "", err
	}
	tm, vtm, typeErrs := typecheck.CheckWithTypesForFile(mod, replFileName, s.cfg, nil)
	if len(typeErrs) > 0 {
		return "", formatREPLErrors(typeErrs, srcBytes)
	}

	mod = effects.TransformCPS(mod)
	gen := codegen.NewGenerator(replFileName, s.cfg)
	gen.SetTypeMap(tm, vtm)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		return "", fmt.Errorf("codegen: %w", err)
	}

	dir, err := newBuildDir("repl-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	goMod := "module gooprepl\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		return "", err
	}
	goFile := gen.GoFileName()
	if err := os.WriteFile(filepath.Join(dir, goFile), []byte(goSrc), 0644); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "run", goFile)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("go run failed:\n%s", msg)
	}
	return string(out), nil
}

func formatREPLErrors(errs []error, src []byte) error {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(report.Render(e, src), "\n"))
	}
	return fmt.Errorf("%s", b.String())
}

func isREPLDecl(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "@[") {
		return true
	}
	kw := firstIdent(trimmed)
	switch kw {
	case "let", "type", "private", "exception", "effect", "import",
		"open", "include", "implements", "module", "val", "external":
		return true
	default:
		return false
	}
}

func isREPLImport(line string) bool {
	return firstIdent(strings.TrimSpace(line)) == "import"
}

func isREPLMain(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "let ") && !strings.HasPrefix(trimmed, "let\t") {
		return false
	}
	rest := strings.TrimSpace(trimmed[len("let"):])
	if strings.HasPrefix(rest, "rec ") {
		rest = strings.TrimSpace(rest[len("rec"):])
	}
	return strings.HasPrefix(rest, "main ") || strings.HasPrefix(rest, "main(") || rest == "main"
}

func firstIdent(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			break
		}
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		return ""
	}
	j := i
	for j < len(s) {
		c := s[j]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			j++
			continue
		}
		break
	}
	return s[i:j]
}
