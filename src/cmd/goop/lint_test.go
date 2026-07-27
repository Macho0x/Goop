package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureLintOutput(t *testing.T, fn func() int) (out string, code int) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	code = fn()
	_ = w.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), code
}

func TestLintCleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.goop")
	src := `module main

let main () =
  print_line "ok"
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	out, code := captureLintOutput(t, func() int { return runLint(path) })
	if code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, "0 error(s), 0 warning(s)") {
		t.Fatalf("expected clean summary, got:\n%s", out)
	}
}

func TestLintTypeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.goop")
	src := `module main

let main () =
  1 + "nope"
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	out, code := captureLintOutput(t, func() int { return runLint(path) })
	if code == 0 {
		t.Fatalf("expected non-zero exit, output:\n%s", out)
	}
	if !strings.Contains(out, "error(s)") {
		t.Fatalf("expected error summary, got:\n%s", out)
	}
}

func TestLintDirectory(t *testing.T) {
	dir := t.TempDir()
	ok := `module main

let main () =
  ()
`
	bad := `module main

let main () =
  true + 1
`
	if err := os.WriteFile(filepath.Join(dir, "a.goop"), []byte(ok), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.goop"), []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	out, code := captureLintOutput(t, func() int { return runLint(dir) })
	if code == 0 {
		t.Fatalf("expected non-zero for dir with bad file, output:\n%s", out)
	}
	if !strings.Contains(out, "error(s)") {
		t.Fatalf("expected summary, got:\n%s", out)
	}
}

func TestCollectGoopFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.goop"), []byte("module main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "y.txt"), []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := collectGoopFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "x.goop") {
		t.Fatalf("files = %v", files)
	}
}
