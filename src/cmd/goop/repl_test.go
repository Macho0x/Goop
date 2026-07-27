package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestREPLScripted(t *testing.T) {
	withGOOPHome(t)

	input := strings.Join([]string{
		":help",
		"1 + 1",
		`:type 1 + 1`,
		`let double (x: int) : int = x + x`,
		"double 21",
		`"hi"`,
		"true",
		":reset",
		`:type 1 + 1`,
		":quit",
		"",
	}, "\n")

	in := strings.NewReader(input)
	var out, errOut bytes.Buffer
	err := runREPLWith(in, &out, &errOut)
	if err != nil {
		t.Fatalf("runREPLWith: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}

	got := out.String()
	checks := []string{
		":help",
		"2",
		"- : int",
		"ok",
		"42",
		"hi",
		"true",
		"session cleared",
		"bye",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q\nstderr:\n%s\nstdout:\n%s", want, errOut.String(), got)
		}
	}
	// :type after :reset should still work (prelude / builtins only).
	if strings.Count(got, "- : int") < 2 {
		t.Errorf("expected :type to work after :reset; stdout:\n%s", got)
	}
}

func TestREPLQuitAliases(t *testing.T) {
	withGOOPHome(t)
	for _, q := range []string{":q", ":quit", ":exit"} {
		var out, errOut bytes.Buffer
		err := runREPLWith(strings.NewReader(q+"\n"), &out, &errOut)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if !strings.Contains(out.String(), "bye") {
			t.Errorf("%s: want bye, got %q", q, out.String())
		}
	}
}

func TestIsREPLDecl(t *testing.T) {
	decls := []string{
		"let x = 1",
		"type color = Red | Blue",
		"import go \"fmt\"",
		"private let x = 1",
		"@[go] { func f() {} }",
	}
	for _, d := range decls {
		if !isREPLDecl(d) {
			t.Errorf("expected decl: %q", d)
		}
	}
	exprs := []string{"1 + 1", "double 21", `"hi"`, "true", "(1, 2)"}
	for _, e := range exprs {
		if isREPLDecl(e) {
			t.Errorf("expected expr, got decl: %q", e)
		}
	}
}

func TestWrapExprForPrint(t *testing.T) {
	w, ok := wrapExprForPrint("1 + 1", "int")
	if !ok || !strings.Contains(w, "int_to_string") {
		t.Fatalf("int wrap: %q ok=%v", w, ok)
	}
	_, ok = wrapExprForPrint("xs", "int list")
	if ok {
		t.Fatal("list should not be printable")
	}
}
