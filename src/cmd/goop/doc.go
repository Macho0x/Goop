package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"goop.dev/compiler/internal/ast"
	gfmt "goop.dev/compiler/internal/fmt"
	"goop.dev/compiler/internal/parser"
)

// runDoc implements `goop doc <file-or-dir>`.
// Emits Markdown documentation for .goop modules and .gosig stubs to stdout.
func runDoc(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: goop doc <file-or-dir>\n")
		return 1
	}
	target := args[0]
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	var files []string
	if info.IsDir() {
		err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "node_modules" || base == "doc" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".goop" || ext == ".gosig" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error walking %s: %v\n", target, err)
			return 1
		}
		if len(files) == 0 {
			fmt.Fprintf(os.Stderr, "no .goop or .gosig files under %s\n", target)
			return 1
		}
	} else {
		files = []string{target}
	}

	var parts []string
	for i, f := range files {
		md, err := documentFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			return 1
		}
		if i > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, md)
	}
	fmt.Print(strings.Join(parts, "\n"))
	if len(parts) > 0 && !strings.HasSuffix(parts[len(parts)-1], "\n") {
		fmt.Println()
	}
	return 0
}

func documentFile(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gosig":
		return documentGosig(path, src), nil
	default:
		return documentGoop(path, src)
	}
}

func documentGoop(path string, src []byte) (string, error) {
	mod, err := parser.Parse(path, src)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	var b strings.Builder
	name := mod.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	fmt.Fprintf(&b, "# Module `%s`\n\n", name)
	fmt.Fprintf(&b, "_Source: `%s`_\n\n", filepath.ToSlash(path))

	var types []*ast.TypeDecl
	var values []*ast.LetDecl
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			if !d.Private {
				types = append(types, d)
			}
		case *ast.LetDecl:
			if !d.Private {
				values = append(values, d)
			}
		}
	}

	if len(types) > 0 {
		b.WriteString("## Types\n\n")
		for _, td := range types {
			writeTypeDoc(&b, td)
		}
	}

	if len(values) > 0 {
		b.WriteString("## Values\n\n")
		for _, ld := range values {
			writeLetDoc(&b, ld)
		}
	}

	if len(types) == 0 && len(values) == 0 {
		b.WriteString("_(no exported types or values)_\n")
	}

	return b.String(), nil
}

func writeTypeDoc(b *strings.Builder, td *ast.TypeDecl) {
	sig := typeDeclSignature(td)
	fmt.Fprintf(b, "### `%s`\n\n", td.Name)
	b.WriteString("```goop\n")
	b.WriteString(sig)
	if !strings.HasSuffix(sig, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("```\n\n")
}

func writeLetDoc(b *strings.Builder, ld *ast.LetDecl) {
	for _, binding := range ld.Bindings {
		fmt.Fprintf(b, "### `%s`\n\n", binding.Name)
		b.WriteString("```goop\n")
		b.WriteString(letBindingSignature(ld, binding))
		b.WriteString("\n```\n\n")
	}
}

func typeDeclSignature(td *ast.TypeDecl) string {
	var b strings.Builder
	if td.Private {
		b.WriteString("private ")
	}
	b.WriteString("type ")
	b.WriteString(td.Name)
	for _, tp := range td.TypeParams {
		b.WriteByte(' ')
		b.WriteString(tp)
	}
	if td.Quantity == 1 {
		b.WriteString(" : 1")
	}
	switch k := td.Kind.(type) {
	case *ast.OpaqueTypeKind:
		// name only
	case *ast.RecordTypeKind:
		b.WriteString(" = { ")
		for i, f := range k.Fields {
			if i > 0 {
				b.WriteString("; ")
			}
			if f.Mutable {
				b.WriteString("mutable ")
			}
			b.WriteString(f.Name)
			b.WriteString(": ")
			b.WriteString(gfmt.FormatType(f.Type))
		}
		b.WriteString(" }")
	case *ast.ADTTypeKind:
		b.WriteString(" =")
		for _, c := range k.Cases {
			b.WriteString(" | ")
			b.WriteString(c.Name)
			if c.Arg != nil {
				b.WriteString(" of ")
				b.WriteString(gfmt.FormatType(c.Arg))
			}
		}
	case *ast.GADTTypeKind:
		b.WriteString(" =")
		for _, c := range k.Cases {
			b.WriteString(" | ")
			b.WriteString(c.Name)
			b.WriteString(" : ")
			if c.Arg != nil {
				b.WriteString(gfmt.FormatType(c.Arg))
				b.WriteString(" -> ")
			}
			b.WriteString(gfmt.FormatType(c.Result))
		}
	case *ast.AliasTypeKind:
		b.WriteString(" = ")
		b.WriteString(gfmt.FormatType(k.Alias))
	case *ast.NewtypeTypeKind:
		b.WriteString(" = newtype ")
		b.WriteString(gfmt.FormatType(k.Rep))
	case *ast.ExtensibleTypeKind:
		b.WriteString(" = ..")
	default:
		b.WriteString(" = …")
	}
	return b.String()
}

func letBindingSignature(ld *ast.LetDecl, binding ast.LetBinding) string {
	var b strings.Builder
	b.WriteString("let ")
	if ld.Private {
		b.WriteString("private ")
	}
	if ld.Rec {
		b.WriteString("rec ")
	}
	if ld.ActivePattern {
		b.WriteString("(|")
		b.WriteString(binding.Name)
		b.WriteString("|_|)")
	} else {
		b.WriteString(binding.Name)
	}
	for _, p := range binding.Params {
		b.WriteByte(' ')
		if p.Label != "" {
			if p.Optional {
				b.WriteByte('?')
			} else {
				b.WriteByte('~')
			}
			if p.Label != p.Name {
				b.WriteString(p.Label)
				b.WriteByte(':')
			}
		}
		if p.Name == "" && p.Type == nil && p.Label == "" {
			b.WriteString("()")
			continue
		}
		if p.Type != nil {
			b.WriteByte('(')
			b.WriteString(p.Name)
			b.WriteString(": ")
			b.WriteString(gfmt.FormatType(p.Type))
			b.WriteByte(')')
		} else {
			b.WriteString(p.Name)
		}
	}
	if binding.RetType != nil {
		b.WriteString(" : ")
		b.WriteString(gfmt.FormatType(binding.RetType))
	}
	return b.String()
}

// gosigValRe matches `val Name : type…` (Go FFI signature stubs; H5 format TBD).
var (
	gosigModuleRe = regexp.MustCompile(`(?m)^\s*module\s+([A-Za-z_][\w'.]*)`)
	gosigValRe    = regexp.MustCompile(`(?m)^\s*val\s+(\S+)\s*:\s*(.+?)\s*$`)
	gosigTypeRe   = regexp.MustCompile(`(?m)^\s*(?:private\s+)?type\s+(\S+)`)
	gosigLetRe    = regexp.MustCompile(`(?m)^\s*(?:private\s+)?(?:let\s+(?:rec\s+)?)(\S+)`)
)

// documentGosig renders a best-effort Markdown summary for `.gosig` stubs.
// Full format lands with H5; MVP extracts module / type / val / let lines.
func documentGosig(path string, src []byte) string {
	text := string(src)
	var b strings.Builder

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if m := gosigModuleRe.FindStringSubmatch(text); len(m) > 1 {
		name = m[1]
	}
	fmt.Fprintf(&b, "# Signature `%s`\n\n", name)
	fmt.Fprintf(&b, "_Source: `%s` (`.gosig`)_\n\n", filepath.ToSlash(path))

	types := gosigTypeRe.FindAllStringSubmatch(text, -1)
	vals := gosigValRe.FindAllStringSubmatch(text, -1)
	lets := gosigLetRe.FindAllStringSubmatch(text, -1)

	if len(types) > 0 {
		b.WriteString("## Types\n\n")
		for _, m := range types {
			fmt.Fprintf(&b, "- `%s`\n", m[0])
		}
		b.WriteByte('\n')
	}

	if len(vals) > 0 {
		b.WriteString("## Values\n\n")
		for _, m := range vals {
			fmt.Fprintf(&b, "- `%s : %s`\n", m[1], strings.TrimSpace(m[2]))
		}
		b.WriteByte('\n')
	}

	// Avoid double-counting `let` lines that are really `val` (none), and
	// skip `let` keyword noise from type bodies by only listing distinct names
	// when no vals were found (pure Goop-style stub).
	if len(vals) == 0 && len(lets) > 0 {
		b.WriteString("## Values\n\n")
		seen := map[string]bool{}
		for _, m := range lets {
			n := m[1]
			if seen[n] || n == "=" || strings.HasPrefix(n, "|") {
				continue
			}
			seen[n] = true
			fmt.Fprintf(&b, "- `%s`\n", n)
		}
		b.WriteByte('\n')
	}

	if len(types) == 0 && len(vals) == 0 && len(lets) == 0 {
		b.WriteString("_(empty or unrecognized `.gosig` stub)_\n")
	}

	return b.String()
}

// documentTo is used by tests to capture output without hijacking os.Stdout.
func documentTo(w io.Writer, path string) error {
	md, err := documentFile(path)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, md)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(md, "\n") {
		_, err = io.WriteString(w, "\n")
	}
	return err
}
