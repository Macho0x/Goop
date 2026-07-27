package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionNonEmpty(t *testing.T) {
	v := strings.TrimSpace(Version)
	if v == "" {
		t.Fatal("embedded Version is empty")
	}
}

func TestSyncWithRootVERSION(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	rootVERSION := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "VERSION")
	data, err := os.ReadFile(rootVERSION)
	if err != nil {
		t.Fatalf("read root VERSION: %v", err)
	}
	want := strings.TrimSpace(string(data))
	got := strings.TrimSpace(Version)
	if got != want {
		t.Fatalf("embedded Version %q != root VERSION %q — keep src/internal/version/VERSION in sync", got, want)
	}
}
