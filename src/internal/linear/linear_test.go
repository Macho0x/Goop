package linear_test

import (
	"testing"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/linear"
	"goop.dev/compiler/internal/parser"
)

func mustParse(t *testing.T, src string) *ast.Module {
	t.Helper()
	mod, err := parser.Parse("<test>", []byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return desugar.DesugarModule(mod)
}

func linearTypes(mod *ast.Module) map[string]bool {
	lt := make(map[string]bool)
	for _, d := range mod.Decls {
		if td, ok := d.(*ast.TypeDecl); ok && td.Quantity == 1 {
			lt[td.Name] = true
		}
	}
	return lt
}

func assertNoErrors(t *testing.T, errs []error) {
	t.Helper()
	for _, e := range errs {
		t.Errorf("unexpected error: %v", e)
	}
}

func assertHasError(t *testing.T, errs []error, substr string) {
	t.Helper()
	for _, e := range errs {
		if contains(e.Error(), substr) {
			return
		}
	}
	t.Errorf("expected error containing %q, got %v", substr, errs)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestProperDischarge_NoError: linear type properly discharged (single use) → no error.
func TestProperDischarge_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let process (h: handle) : unit =
  println "ok"

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestHandOff_NoError: passing a linear value to another function discharges it.
func TestHandOff_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let Close (h: handle) : unit =
  println "closed"

let process (h: handle) : unit =
  Close h

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestDoubleUse_Error: linear type used twice → error.
func TestDoubleUse_Error(t *testing.T) {
	src := `module Test
type handle : 1

let Close (h: handle) : unit =
  println "closed"

let process (h: handle) : unit =
  let dummy = Close h in Close h

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertHasError(t, errs, "used after being discharged")
}

// TestUnrestrictedNoError: ordinary (unrestricted) type used multiple times → no error.
func TestUnrestrictedNoError(t *testing.T) {
	src := `module Test
type config = string

let process (x: config) : config =
  let a = x in
  let b = x in
  a ^ b

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestLinearTypeDecl_Parsed: verify that : 1 syntax is parsed correctly.
func TestLinearTypeDecl_Parsed(t *testing.T) {
	src := `module Test
type handle : 1
type normal = string

let main () = println "test"
`
	mod, err := parser.Parse("<test>", []byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	mod = desugar.DesugarModule(mod)

	// Check that handle has Quantity 1
	foundHandle := false
	for _, d := range mod.Decls {
		if td, ok := d.(*ast.TypeDecl); ok {
			if td.Name == "handle" {
				foundHandle = true
				if td.Quantity != 1 {
					t.Errorf("expected handle.Quantity=1, got %d", td.Quantity)
				}
				if _, ok := td.Kind.(*ast.OpaqueTypeKind); !ok {
					t.Errorf("expected OpaqueTypeKind for handle, got %T", td.Kind)
				}
			}
			if td.Name == "normal" {
				if td.Quantity != 0 {
					t.Errorf("expected normal.Quantity=0, got %d", td.Quantity)
				}
			}
		}
	}
	if !foundHandle {
		t.Error("handle type declaration not found")
	}
}

// TestIfBranchBothDischarge_NoError: both branches discharge → no error.
func TestIfBranchBothDischarge_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let CloseA (h: handle) : unit =
  println "closed A"

let CloseB (h: handle) : unit =
  println "closed B"

let process (h: handle) = 
  if true then CloseA h else CloseB h

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestIfBranchOnlyOneDischarge_Error: one branch doesn't discharge → error.
func TestIfBranchOnlyOneDischarge_Error(t *testing.T) {
	src := `module Test
type handle : 1

let CloseA (h: handle) : unit =
  println "closed A"

let process (h: handle) = 
  if true then CloseA h else println "no close"

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertHasError(t, errs, "not discharged in else-branch")
}

// TestLinearUseDischarges_NoError: using a linear value (hand-off) discharges
// it without an explicit Close — equivalent to the old region auto-discharge.
func TestLinearUseDischarges_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let Close (h: handle) : unit =
  println "closed"

let useIt (h: handle) : unit =
  println "using"

let process (h: handle) : unit =
  useIt h

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestUnusedLinearParam_AutoDischarge_NoError: unused linear parameters are
// auto-discharged at function exit (no error).
func TestUnusedLinearParam_AutoDischarge_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let Close (h: handle) : unit =
  println "closed"

let process (h: handle) : unit =
  ()

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestMultipleUnusedLinearParams_AutoDischarge_NoError: multiple unused linear
// parameters are auto-discharged at function exit.
func TestMultipleUnusedLinearParams_AutoDischarge_NoError(t *testing.T) {
	src := `module Test
type handle : 1

let Close (h: handle) : unit =
  println "closed"

let process (h1: handle) (h2: handle) : unit =
  ()

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// ---------------------------------------------------------------------------
// Goroutine sharing analysis tests
// ---------------------------------------------------------------------------

// TestMutableGoCapture_NoErrorWhenNotUsedAfter: ref captured by go but not
// used afterward in the spawning scope → no error (flow-sensitive liveness).
func TestMutableGoCapture_NoErrorWhenNotUsedAfter(t *testing.T) {
	src := `module Test

let race () =
  let counter = ref 0 in
  go (fun () -> !counter)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestMutableMultiGoCapture_Error: a ref cell captured by two
// goroutines → error about sharing between multiple goroutines.
func TestMutableMultiGoCapture_Error(t *testing.T) {
	src := `module Test

let race () =
  let counter = ref 0 in
  let dummy = go (fun () -> !counter) in
  go (fun () -> !counter)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertHasError(t, errs, "shared between multiple goroutines")
}

// TestImmutableGoCapture_NoError: an immutable variable captured by a
// goroutine is always safe → no error.
func TestImmutableGoCapture_NoError(t *testing.T) {
	src := `module Test

let safe () =
  let x = 42 in
  go (fun () -> x)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestChannelGoCapture_NoError: channel communication (immutable channel
// variable) captured by a goroutine → no error.
func TestChannelGoCapture_NoError(t *testing.T) {
	src := `module Test

let safe () =
  let ch : int chan = Chan.make () in
  go (fun () -> Chan.send ch 42)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestMutableModuleLevelGoCapture_Error: module-level ref used after go spawn.
func TestMutableModuleLevelGoCapture_Error(t *testing.T) {
	src := `module Test

let counter = ref 0

let race () =
  let counter = ref 0 in
  let ignored = go (fun () -> !counter) in
  println (int_to_string !counter)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertHasError(t, errs, "mutable variable \"counter\"")
}

// TestMutableCapturedNotAccessed_NoError: flow-sensitive liveness — if the
// spawning scope never accesses the ref after go, no race error.
func TestMutableCapturedNotAccessed_NoError(t *testing.T) {
	src := `module Test

let race () =
  let counter = ref 0 in
  go (fun () -> !counter)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}

// TestMutableCapturedAndUsedAfterGo_Error: ref cell used after go spawn.
func TestMutableCapturedAndUsedAfterGo_Error(t *testing.T) {
	src := `module Test

let race () =
  let counter = ref 0 in
  let ignored = go (fun () -> !counter) in
  println (int_to_string !counter)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertHasError(t, errs, "mutable variable \"counter\"")
}

// ---------------------------------------------------------------------------
// OwnedChan tests
// ---------------------------------------------------------------------------

func ownedChanLinearTypes() map[string]bool {
	return map[string]bool{"owned_chan": true}
}

// TestOwnedChanBorrowSend_NoError: OwnedChan.send borrows the channel (doesn't
// discharge it), so multiple sends before close is valid.
func TestOwnedChanBorrowSend_NoError(t *testing.T) {
	src := `module Test

let send3 (ch: int owned_chan) : unit =
  let _u1 = OwnedChan.send ch 1 in
  let _u2 = OwnedChan.send ch 2 in
  let _u3 = OwnedChan.send ch 3 in
  OwnedChan.close ch

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertNoErrors(t, errs)
}

// TestOwnedChanSendAfterClose_Error: send after close is a linear double-use
// error because close discharges the channel.
func TestOwnedChanSendAfterClose_Error(t *testing.T) {
	src := `module Test

let bad (ch: int owned_chan) : unit =
  let _u1 = OwnedChan.close ch in
  OwnedChan.send ch 1

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertHasError(t, errs, "used after being discharged")
}

// TestOwnedChanDoubleClose_Error: closing an already-closed channel is a
// linear double-use error.
func TestOwnedChanDoubleClose_Error(t *testing.T) {
	src := `module Test

let bad (ch: int owned_chan) : unit =
  let _u1 = OwnedChan.close ch in
  OwnedChan.close ch

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertHasError(t, errs, "used after being discharged")
}

// TestOwnedChanNotDischarged_Error: failing to close/discard an OwnedChan is
// a discharge error.
func TestOwnedChanNotDischarged_Error(t *testing.T) {
	src := `module Test

let bad () : unit =
  let ch : int owned_chan = OwnedChan.make () in
  OwnedChan.send ch 1
  (* no close — ch is never discharged *)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertHasError(t, errs, "not discharged")
}

// TestOwnedChanHandOff_NoError: passing an OwnedChan to another function
// (hand-off) discharges it.
func TestOwnedChanHandOff_NoError(t *testing.T) {
	src := `module Test

let closeIt (ch: int owned_chan) : unit =
  OwnedChan.close ch

let process (ch: int owned_chan) : unit =
  closeIt ch

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertNoErrors(t, errs)
}

// TestOwnedChanRecvBorrow_NoError: OwnedChan.recv borrows the channel
// (doesn't discharge), so recv followed by close is valid.
func TestOwnedChanRecvBorrow_NoError(t *testing.T) {
	src := `module Test

let recvThenClose (ch: int owned_chan) : unit =
  let v = OwnedChan.recv ch in
  OwnedChan.close ch

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertNoErrors(t, errs)
}

func TestLinearGoHandoff_NoError(t *testing.T) {
	src := `module Test

let worker (ch: int owned_chan) : unit =
  go (fun () ->
    let _u = OwnedChan.send ch 1 in
    OwnedChan.close ch
  )

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, ownedChanLinearTypes())
	assertNoErrors(t, errs)
}

func TestGoMove_SuppressesRace(t *testing.T) {
	src := `module Test

let ok () : unit =
  let counter = ref 0 in
  go (move counter) (fun () -> counter := !counter + 1)

let main () = println "test"
`
	mod := mustParse(t, src)
	errs := linear.Check(mod, linearTypes(mod))
	assertNoErrors(t, errs)
}
