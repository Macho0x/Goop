package lexer

import (
	"strings"
	"testing"

	"goop.dev/compiler/internal/token"
)

func stringLiteralValue(t *testing.T, src string) string {
	t.Helper()
	toks, err := Lex("test.goop", []byte(src))
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	for _, tok := range toks {
		if tok.Type == token.ERROR {
			t.Fatalf("unexpected lexer error: %s", tok.Lexeme)
		}
		if tok.Type == token.STRING {
			s, _ := tok.Literal.(string)
			return s
		}
	}
	t.Fatalf("no STRING token in %q", src)
	return ""
}

func TestStringEscapes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"hello"`, "hello"},
		{`"a\nb"`, "a\nb"},
		{`"a\tb"`, "a\tb"},
		{`"a\rb"`, "a\rb"},
		{`"a\\b"`, "a\\b"},
		{`"a\"b"`, "a\"b"},
		{`"\x1b[31m"`, "\x1b[31m"},
		{`"\033[0m"`, "\033[0m"},
		{`"\e[0m"`, "\x1b[0m"},
		{`"\x00"`, "\x00"},
		{`"\377"`, "\377"},
		{`"\0"`, "\x00"},
		{`"\7"`, "\x07"},
	}
	for _, tc := range cases {
		got := stringLiteralValue(t, "let s = "+tc.src)
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.src, got, tc.want)
		}
	}
}

func TestStringEscapeErrors(t *testing.T) {
	cases := []struct {
		src     string
		wantSub string
	}{
		{`"\x1"`, "incomplete hex"},
		{`"\x"`, "incomplete hex"},
		{`"\xg0"`, "invalid hex"},
		{`"\q"`, "invalid string escape"},
		{`"\400"`, "octal escape out of range"},
	}
	for _, tc := range cases {
		toks, err := Lex("test.goop", []byte("let s = "+tc.src))
		if err != nil {
			t.Fatalf("Lex: %v", err)
		}
		found := false
		for _, tok := range toks {
			if tok.Type == token.ERROR && strings.Contains(tok.Lexeme, tc.wantSub) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: expected ERROR containing %q; toks=%v", tc.src, tc.wantSub, toks)
		}
	}
}

func TestCharEscapes(t *testing.T) {
	toks, err := Lex("test.goop", []byte(`let c = '\e'`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range toks {
		if tok.Type == token.CHAR {
			if ch, ok := tok.Literal.(rune); !ok || ch != 0x1b {
				t.Fatalf("got %#v want ESC", tok.Literal)
			}
			return
		}
		if tok.Type == token.ERROR {
			t.Fatalf("unexpected error: %s", tok.Lexeme)
		}
	}
	t.Fatal("no CHAR token")
}

func TestLineCommentsSkipped(t *testing.T) {
	toks, err := Lex("test.goop", []byte("module M\n// hi (* not a block *)\nlet x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range toks {
		if tok.Type == token.ERROR {
			t.Fatalf("unexpected ERROR %s", tok.Lexeme)
		}
	}
}

func TestBlockCommentsRejected(t *testing.T) {
	toks, err := Lex("test.goop", []byte("module M\n(* nope *)\nlet x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tok := range toks {
		if tok.Type == token.ERROR && strings.Contains(tok.Lexeme, "LEX002") && strings.Contains(tok.Lexeme, "block comments") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected LEX002 block-comment ERROR; toks=%v", toks)
	}
}
