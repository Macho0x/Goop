package goidioms

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/typecheck"
)

func checkSrc(t *testing.T, src string) (errs, warns []error) {
	t.Helper()
	mod, err := parser.Parse("t.goop", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mod = desugar.DesugarModule(mod)
	tm, _, _ := typecheck.CheckWithTypes(mod)
	return CheckWithConfig(mod, tm, config.DefaultConfig())
}

func TestFloatEq(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  if 1.0 = 0.0 then println "x" else println "y"
`)
	if !hasCode(warns, CodeFloat) {
		t.Fatalf("expected FLOAT001, got %v", warns)
	}
}

func TestStrEqualFold(t *testing.T) {
	_, warns := checkSrc(t, `module main
import go "strings"
let main () =
  if strings.ToLower "A" = strings.ToLower "a" then println "y" else println "n"
`)
	if !hasCode(warns, CodeStr) {
		t.Fatalf("expected STR001, got %v", warns)
	}
}

func TestRegexpInLoop(t *testing.T) {
	_, warns := checkSrc(t, `module main
import go "regexp" {
  val MustCompile : string -> unit
}
let main () =
  for i = 0 to 3 do
    let _ = regexp.MustCompile "a+" in
    ()
  done
`)
	if !hasCode(warns, CodeRegexp) {
		t.Fatalf("expected REGEXP001, got %v", warns)
	}
}

func TestWGAddInsideGo(t *testing.T) {
	_, warns := checkSrc(t, `module main
import go "sync" {
  type WaitGroup
  val (w : WaitGroup ptr).Add : int -> unit
}
let main () =
  let wg : WaitGroup ptr = ptr_of {} in
  go (fun () -> wg.Add 1)
`)
	if !hasCode(warns, CodeWG) {
		t.Fatalf("expected WG001, got %v", warns)
	}
}

func TestURLQuerySetDiscarded(t *testing.T) {
	_, warns := checkSrc(t, `module main
let main () =
  begin
    x.Query ().Set "a" "b";
    println "ok"
  end
`)
	if !hasCode(warns, CodeURL) {
		t.Fatalf("expected URL001, got %v", warns)
	}
}

func TestExitInTryFinally(t *testing.T) {
	_, warns := checkSrc(t, `module main
import go "os" {
  val Exit : int -> unit
}
let main () =
  try os.Exit 1 finally println "nope"
`)
	if !hasCode(warns, CodeExit) {
		t.Fatalf("expected EXIT001, got %v", warns)
	}
}

func TestCtxCancelDiscarded(t *testing.T) {
	_, warns := checkSrc(t, `module main
import go "context" {
  val WithCancel : unit -> unit
}
let main () =
  begin
    context.WithCancel ();
    println "x"
  end
`)
	if !hasCode(warns, CodeCtx) {
		t.Fatalf("expected CTX001, got %v", warns)
	}
}

func hasCode(errs []error, code string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), code) {
			return true
		}
	}
	return false
}
