package channelrace

import (
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/parser"
)

func TestChannelRaceParentAccess(t *testing.T) {
	src := `module RaceTest

let main () : unit =
  let counter = ref 0 in
  let ch : int chan = Chan.make () in
  let g = go (fun () -> let s = Chan.send ch !counter in ()) in
  let dummy = println (int_to_string !counter) in ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs) == 0 && len(warns) == 0 {
		t.Fatal("expected LINEAR008")
	}
}

func TestChannelRaceRefWriteInGo(t *testing.T) {
	src := `module RaceWrite

let main () : unit =
  let counter = ref 0 in
  let ch : int chan = Chan.make () in
  let g = go (fun () ->
    let _ = counter := !counter + 1 in
    Chan.send ch !counter) in
  let dummy = println (int_to_string !counter) in ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs) == 0 && len(warns) == 0 {
		t.Fatal("expected LINEAR008 for ref write in go with parent access")
	}
}

func TestChannelRaceMoveIsSilent(t *testing.T) {
	src := `module RaceMoveOk

let main () : unit =
  let counter = ref 0 in
  let g = go (move counter) (fun () ->
    let _ = counter := !counter + 1 in
    ()) in
  ()
`
	mod, err := parser.Parse("test.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := CheckWithConfig(mod, config.DefaultConfig())
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("go (move) should be silent, got errs=%v warns=%v", errs, warns)
	}
}
