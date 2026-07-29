package report

import "testing"

func TestRender(t *testing.T) {
	src := []byte("module T\nlet x = 1 + \"hi\"\n")
	err := &mockErr{msg: "/tmp/t.goop:2:11: type mismatch"}
	out := Render(err, src)
	if !contains(out, "╰──") || !contains(out, "type mismatch") {
		t.Errorf("bad render: %s", out)
	}
}

func TestRenderNoLoc(t *testing.T) {
	err := &mockErr{msg: "usage error"}
	if Render(err, nil) != "usage error\n" {
		t.Errorf("fallback failed: %q", Render(err, nil))
	}
}

func TestRenderHelpTip(t *testing.T) {
	src := []byte("module T\nlet x = 1\n")
	err := &mockErr{msg: "t.goop:2:1: RESULT001: result value is discarded"}
	out := Render(err, src)
	if !contains(out, "help:") || !contains(out, "let _") {
		t.Errorf("expected help tip: %s", out)
	}
}

type mockErr struct{ msg string }

func (e *mockErr) Error() string { return e.msg }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && containsHelper(s, sub)))
}
func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
