package fmt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goop.dev/compiler/internal/parser"
)

func TestFormatGoMove(t *testing.T) {
	src := `module Main

let main () : unit =
  let x = ref 1 in
  let dummy = go (move x) (fun () -> println (int_to_string (!x))) in
  println "done"
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := FormatModule(mod)
	if !strings.Contains(out, "go (move x)") {
		t.Fatalf("missing go (move x) in:\n%s", out)
	}
	if !strings.Contains(out, "ref ") {
		t.Fatalf("missing ref in:\n%s", out)
	}
}

func TestFormatArrayAndFor(t *testing.T) {
	src := `module Main
let main () =
  begin
    let arr = Array.make 2 0 in
    for i = 0 to 1 do arr.(i) <- i done;
    arr.(0)
  end
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := FormatModule(mod)
	if !strings.Contains(out, "arr.(i) <- i") {
		t.Fatalf("missing array assign in:\n%s", out)
	}
	if !strings.Contains(out, "for i = 0 to 1 do") || !strings.Contains(out, "done") {
		t.Fatalf("missing for/done in:\n%s", out)
	}
}

func TestFormatBeginEnd(t *testing.T) {
	src := `module Main
let main () = begin println "a"; 1 end
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := FormatModule(mod)
	if !strings.Contains(out, "begin") || !strings.Contains(out, "end") {
		t.Fatalf("missing begin/end in:\n%s", out)
	}
}

func TestFormatWhileRef(t *testing.T) {
	src := `module Main
let main () =
  let r = ref 0 in
  while !r < 3 do r := !r + 1 done
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := FormatModule(mod)
	if !strings.Contains(out, "while") || !strings.Contains(out, ":=") {
		t.Fatalf("missing while/:= in:\n%s", out)
	}
}

func TestRoundTripSelected(t *testing.T) {
	names := []string{
		"go_move_test.goop",
		"if_test.goop",
		"active_pattern_test.goop",
		"bool_test.goop",
		"array_test.goop",
		"array_mutation_test.goop",
		"for_loop_test.goop",
		"begin_end_test.goop",
		"qualified_ctor_test.goop",
	}
	for _, name := range names {
		path := filepath.Join("..", "..", "..", "tests", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mod1, err := parser.Parse(path, data)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		out := FormatModule(mod1)
		mod2, err := parser.Parse(path, []byte(out))
		if err != nil {
			t.Fatalf("%s: re-parse: %v\n---\n%s", name, err, out)
		}
		if mod1.Name != mod2.Name || len(mod1.Decls) != len(mod2.Decls) {
			t.Fatalf("%s: module shape mismatch after round-trip", name)
		}
	}
}
