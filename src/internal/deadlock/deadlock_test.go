package deadlock

import (
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/parser"
)

func TestDeadlockCycle(t *testing.T) {
	src := `module DeadlockTest

let run () : unit =
  let ch1 : int chan = Chan.make () in
  let ch2 : int chan = Chan.make () in
  let g1 = go (fun () ->
    let s1 = Chan.send ch1 42 in
    let v = Chan.recv ch2 in
    println (int_to_string v)) in
  let g2 = go (fun () ->
    let s2 = Chan.send ch2 99 in
    let v = Chan.recv ch1 in
    println (int_to_string v)) in
  ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	_, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(warns) == 0 {
		t.Fatal("expected DEADLOCK001 warning")
	}
}

func TestDeadlockSelectSilent(t *testing.T) {
	src := `module DeadlockSelect

let main () : unit =
  let ch : int chan = Chan.make () in
  let g = go (fun () ->
    select {
      case x = ch -> println (int_to_string x)
      default -> ()
    }) in
  ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	_, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(warns) != 0 {
		t.Fatalf("select should not trigger deadlock lint, got %v", warns)
	}
}

func TestCyclicPair(t *testing.T) {
	a := []commEvent{{Kind: evSend, Channel: "ch1"}, {Kind: evRecv, Channel: "ch2"}}
	b := []commEvent{{Kind: evSend, Channel: "ch2"}, {Kind: evRecv, Channel: "ch1"}}
	if !cyclicPair(a, b) {
		t.Fatal("expected cyclic pair")
	}
}

func TestParseChanSendRecv(t *testing.T) {
	src := `module T
let f (ch1: int chan) (ch2: int chan) : unit =
  let s1 = Chan.send ch1 42 in
  let v = Chan.recv ch2 in
  ()
`
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	ld := mod.Decls[0].(*ast.LetDecl)
	letIn := ld.Bindings[0].Body.(*ast.LetInExpr)
	sendApp := letIn.Bindings[0].Body.(*ast.AppExpr)
	ok, ch, _ := parseChanSend(sendApp)
	if !ok || ch != "ch1" {
		t.Fatalf("parseChanSend failed ok=%v ch=%q", ok, ch)
	}
	events := straightLineEvents(ld.Bindings[0].Body)
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
}
