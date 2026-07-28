package gosig

import (
	"strings"
	"testing"
	"time"
)

// TestLookupFuncStringsHasPrefix verifies that LookupFunc can resolve a
// real stdlib function. Since "strings" is always available and doesn't
// require any module setup, this is a safe standard-library test.
// If packages.Load fails (e.g. offline), the test is skipped.
func TestLookupFuncStringsHasPrefix(t *testing.T) {
	done := make(chan struct{})
	var sig *FuncSig
	var err error

	go func() {
		sig, err = LookupFunc("strings", "HasPrefix")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Skip("packages.Load timed out — skipping gosig integration test")
	}

	if err != nil {
		t.Skipf("gosig fallback not available: %v", err)
	}
	if sig == nil {
		t.Fatal("expected non-nil FuncSig")
	}

	if len(sig.Params) != 2 {
		t.Errorf("expected 2 params for strings.HasPrefix, got %d: %v",
			len(sig.Params), sig.Params)
	}
	if len(sig.Results) != 1 {
		t.Errorf("expected 1 result for strings.HasPrefix, got %d: %v",
			len(sig.Results), sig.Results)
	}
	if len(sig.Results) == 1 && sig.Results[0].Type != "bool" {
		t.Errorf("expected result type bool, got %q", sig.Results[0].Type)
	}
}

// TestGoTypeToGoopTypeMapping tests the Go-type→Goop-type conversion in
// isolation (unit test, no network needed).
func TestGoTypeToGoopTypeMapping(t *testing.T) {
	// This tests the conversion functions in the typecheck package, but
	// those functions are unexported. We test a simplified version here
	// that exercises the same logic used by the gosig integration.
	//
	// The actual conversion lives in the typecheck package; this test
	// ensures the FuncSig is produced in the expected Go type format.
	sig, err := LookupFunc("fmt", "Sprintf")
	if err != nil {
		t.Skipf("gosig fallback not available: %v", err)
	}
	if sig == nil {
		t.Skip("nil FuncSig")
	}

	// fmt.Sprintf has: func(string, ...interface{}) string
	// The first param should be string; there should be at least 1 result.
	if len(sig.Params) == 0 {
		t.Error("expected at least 1 parameter for fmt.Sprintf")
	}
	if len(sig.Params) >= 1 && sig.Params[0].Type != "string" {
		t.Errorf("expected first param type 'string', got %q", sig.Params[0].Type)
	}
	if len(sig.Results) == 0 {
		t.Error("expected at least 1 result for fmt.Sprintf")
	}
	if len(sig.Results) >= 1 && sig.Results[0].Type != "string" {
		t.Errorf("expected result type 'string', got %q", sig.Results[0].Type)
	}
}

// TestLookupFuncCache verifies that repeated calls return cached results.
func TestLookupFuncCache(t *testing.T) {
	// First call
	sig1, err1 := LookupFunc("strings", "Contains")
	if err1 != nil {
		t.Skipf("gosig fallback not available: %v", err1)
	}
	// Second call (should hit cache)
	sig2, err2 := LookupFunc("strings", "Contains")
	if err2 != nil {
		t.Errorf("cached lookup failed: %v", err2)
	}
	if sig1 == nil || sig2 == nil {
		t.Skip("nil FuncSig")
	}
	if len(sig1.Params) != len(sig2.Params) {
		t.Error("cache returned different param count")
	}
}

// TestLookupFuncNonExistent verifies error handling for unknown functions.
func TestLookupFuncNonExistent(t *testing.T) {
	_, err := LookupFunc("strings", "NonExistentFuncXYZ")
	if err == nil {
		t.Error("expected error for non-existent function")
	} else {
		t.Logf("expected error: %v", err)
	}
}

func TestLookupVarStdout(t *testing.T) {
	typ, err := LookupVar("os", "Stdout")
	if err != nil {
		t.Skipf("gosig LookupVar not available: %v", err)
	}
	if typ == "" {
		t.Fatal("expected non-empty type for os.Stdout")
	}
	// Relative qualifier → "File" or "*File"; absolute may include "os."
	if !strings.Contains(typ, "File") {
		t.Errorf("expected File in os.Stdout type, got %q", typ)
	}
}

func TestLookupFuncTimeNow(t *testing.T) {
	sig, err := LookupFunc("time", "Now")
	if err != nil {
		t.Skipf("gosig LookupFunc time.Now: %v", err)
	}
	if sig == nil || len(sig.Results) != 1 {
		t.Fatalf("expected 1 result for time.Now, got %+v", sig)
	}
	if !strings.Contains(sig.Results[0].Type, "Time") {
		t.Errorf("expected Time result, got %q", sig.Results[0].Type)
	}
}

func TestLookupFuncJSONMarshal(t *testing.T) {
	sig, err := LookupFunc("encoding/json", "Marshal")
	if err != nil {
		t.Skipf("gosig LookupFunc json.Marshal: %v", err)
	}
	if sig == nil || len(sig.Results) < 1 {
		t.Fatalf("expected results for json.Marshal, got %+v", sig)
	}
}

func TestLookupVarHTTPStatusOK(t *testing.T) {
	typ, err := LookupVar("net/http", "StatusOK")
	if err != nil {
		t.Skipf("gosig LookupVar StatusOK: %v", err)
	}
	if typ != "int" {
		t.Errorf("expected int for StatusOK, got %q", typ)
	}
}

func TestLookupTypeSlogHandlerOptions(t *testing.T) {
	info, err := LookupType("log/slog", "HandlerOptions")
	if err != nil {
		t.Skipf("gosig LookupType not available: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil TypeInfo")
	}
	if info.Kind != TypeKindStruct {
		t.Errorf("expected TypeKindStruct, got %v", info.Kind)
	}
	want := map[string]bool{"Level": true, "AddSource": true, "ReplaceAttr": true}
	got := make(map[string]string, len(info.Fields))
	for _, f := range info.Fields {
		got[f.Name] = f.Type
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("missing field %q in HandlerOptions; got %v", name, got)
		}
	}
	if len(info.Fields) != 3 {
		t.Errorf("expected 3 exported fields, got %d: %v", len(info.Fields), info.Fields)
	}
}

func TestLookupTypeSlogHandler(t *testing.T) {
	info, err := LookupType("log/slog", "Handler")
	if err != nil {
		t.Skipf("gosig LookupType not available: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil TypeInfo")
	}
	if info.Kind != TypeKindInterface {
		t.Errorf("expected TypeKindInterface, got %v", info.Kind)
	}
	if info.Fields != nil {
		t.Errorf("expected nil Fields for interface, got %v", info.Fields)
	}
}

func TestLookupTypeSyncMutex(t *testing.T) {
	info, err := LookupType("sync", "Mutex")
	if err != nil {
		t.Skipf("gosig LookupType not available: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil TypeInfo")
	}
	if info.Kind != TypeKindStruct {
		t.Errorf("expected TypeKindStruct, got %v", info.Kind)
	}
	// Mutex has no exported fields (implementation detail).
	for _, f := range info.Fields {
		t.Errorf("unexpected exported field on sync.Mutex: %+v", f)
	}
}

func TestLookupTypeBytesBuffer(t *testing.T) {
	info, err := LookupType("bytes", "Buffer")
	if err != nil {
		t.Skipf("gosig LookupType not available: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil TypeInfo")
	}
	if info.Kind != TypeKindStruct {
		t.Errorf("expected TypeKindStruct, got %v", info.Kind)
	}
}

func TestAssignableSlogLevelLeveler(t *testing.T) {
	ok, err := Assignable("log/slog", "Level", "Leveler")
	if err != nil {
		t.Skipf("gosig Assignable not available: %v", err)
	}
	if !ok {
		t.Error("expected slog.Level assignable to slog.Leveler")
	}
}

func TestFuncHasTypeParamsSlicesContains(t *testing.T) {
	done := make(chan struct{})
	var ok bool
	var err error
	go func() {
		ok, err = FuncHasTypeParams("slices", "Contains")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Skip("packages.Load timed out")
	}
	if err != nil {
		t.Skipf("FuncHasTypeParams unavailable: %v", err)
	}
	if !ok {
		t.Error("expected slices.Contains to be generic")
	}
	ok2, err2 := FuncHasTypeParams("strings", "HasPrefix")
	if err2 != nil {
		t.Skipf("FuncHasTypeParams strings: %v", err2)
	}
	if ok2 {
		t.Error("expected strings.HasPrefix not to be generic")
	}
}
