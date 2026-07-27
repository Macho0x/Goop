package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentGoop(t *testing.T) {
	dir := t.TempDir()
	src := `module demo

private type hidden = int

type Shape =
  | Circle of float
  | Point

private let secret x = x

let area (s: Shape) : float = 0.0

let describe (s: Shape) : string = "x"
`
	path := filepath.Join(dir, "demo.goop")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := documentTo(&buf, path); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"# Module `demo`",
		"## Types",
		"### `Shape`",
		"type Shape = | Circle of float | Point",
		"## Values",
		"### `area`",
		"let area (s: Shape) : float",
		"### `describe`",
		"let describe (s: Shape) : string",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, no := range []string{"hidden", "secret"} {
		if strings.Contains(out, no) {
			t.Errorf("private decl %q should be omitted:\n%s", no, out)
		}
	}
}

func TestDocumentGosig(t *testing.T) {
	dir := t.TempDir()
	src := `module fmt

type Stringer

val Println : string -> unit
val Sprint : string -> string
`
	path := filepath.Join(dir, "fmt.gosig")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := documentTo(&buf, path); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"# Signature `fmt`",
		"## Types",
		"type Stringer",
		"## Values",
		"Println : string -> unit",
		"Sprint : string -> string",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRunDocDir(t *testing.T) {
	dir := t.TempDir()
	goop := `module a

let ping () : unit = ()
`
	if err := os.WriteFile(filepath.Join(dir, "a.goop"), []byte(goop), 0644); err != nil {
		t.Fatal(err)
	}
	code := runDoc([]string{dir})
	if code != 0 {
		t.Fatalf("runDoc exit %d", code)
	}
}

func TestRunDocUsage(t *testing.T) {
	if code := runDoc(nil); code != 1 {
		t.Fatalf("expected usage exit 1, got %d", code)
	}
}
