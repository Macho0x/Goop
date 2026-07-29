package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLSPDocumentFormatting(t *testing.T) {
	bin := filepath.Join(t.TempDir(), exeName("goop-test"))
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "lsp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	send := func(obj map[string]interface{}) {
		body, _ := json.Marshal(obj)
		fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	readMsg := func() map[string]interface{} {
		br := bufio.NewReader(stdout)
		deadline := time.After(5 * time.Second)
		for {
			select {
			case <-deadline:
				t.Fatal("timeout waiting for LSP response")
			default:
			}
			headers := ""
			for {
				line, err := br.ReadString('\n')
				if err != nil {
					t.Fatalf("read header: %v", err)
				}
				if line == "\r\n" || line == "\n" {
					break
				}
				headers += line
			}
			var n int
			for _, line := range strings.Split(headers, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Content-Length:") {
					fmt.Sscanf(line, "Content-Length: %d", &n)
				}
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(br, body); err != nil {
				t.Fatalf("read body: %v", err)
			}
			var msg map[string]interface{}
			if err := json.Unmarshal(body, &msg); err != nil {
				t.Fatalf("json: %v", err)
			}
			// Skip server notifications (no id).
			if _, hasID := msg["id"]; hasID {
				return msg
			}
		}
	}

	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{"capabilities": map[string]interface{}{}},
	})
	init := readMsg()
	caps := init["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	if caps["documentFormattingProvider"] != true {
		t.Fatalf("expected documentFormattingProvider, got %#v", caps)
	}

	src := "module main\nlet main () =println \"hi\"\n"
	send(map[string]interface{}{
		"jsonrpc": "2.0", "method": "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": "file:///tmp/fmt.goop", "languageId": "goop", "version": 1, "text": src,
			},
		},
	})
	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "textDocument/formatting",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": "file:///tmp/fmt.goop"},
			"options":      map[string]interface{}{"tabSize": 2, "insertSpaces": true},
		},
	})
	fmtResp := readMsg()
	result, ok := fmtResp["result"].([]interface{})
	if !ok || len(result) < 1 {
		t.Fatalf("expected TextEdit list, got %#v", fmtResp)
	}
	edit := result[0].(map[string]interface{})
	newText, _ := edit["newText"].(string)
	if !strings.Contains(newText, "let main") {
		t.Fatalf("unexpected formatted text: %q", newText)
	}
}
