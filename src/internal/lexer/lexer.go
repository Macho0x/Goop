// Package lexer implements a hand-written lexer for Goop.
//
// It handles:
//   - Line comments //
//   - (* *) is a lexer error; use //
//   - Double-quoted strings with escapes (\n \t \r \\ \" \e \xHH \ooo)
//   - Integers, floats, chars, polymorphic variants (`Tag)
//   - All keywords and operators defined in the token package
package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"goop.dev/compiler/internal/token"
)

// Lexer holds the state of the scanner.
type Lexer struct {
	src    []byte        // full source text (UTF‑8)
	file   string        // source filename
	pos    int           // current byte offset (0‑based)
	line   int           // current line (1‑based)
	col    int           // current column (1‑based)
	tokens []token.Token // accumulated token stream
}

// Lex runs the complete lexer over src and returns the token stream.
func Lex(file string, src []byte) ([]token.Token, error) {
	l := &Lexer{
		src:  src,
		file: file,
		line: 1,
		col:  1,
	}
	l.run()
	// Filter out ERROR tokens and propagate as error, or include them.
	// We include them in the stream and let the caller decide.
	return l.tokens, nil
}

func (l *Lexer) emit(t token.TokenType, lexeme string, lit any) {
	l.tokens = append(l.tokens, token.Token{
		Type:    t,
		Lexeme:  lexeme,
		Literal: lit,
		Loc: token.SourceLoc{
			File:   l.file,
			Line:   l.line,
			Column: l.col - len(lexeme), // start column
			Offset: l.pos - len(lexeme),
		},
	})
}

func (l *Lexer) errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.tokens = append(l.tokens, token.Token{
		Type:   token.ERROR,
		Lexeme: msg,
		Loc: token.SourceLoc{
			File:   l.file,
			Line:   l.line,
			Column: l.col,
			Offset: l.pos,
		},
	})
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

func (l *Lexer) run() {
	for l.pos < len(l.src) {
		r, size := l.peekRune()
		switch {
		case r == utf8.RuneError && size == 0:
			return
		case isWhitespace(r):
			l.skipWhitespace()
		case r == '(' && l.peekByte(1) == '*':
			l.errorf("LEX002: block comments (* *) were removed; use //")
			l.skipRejectedBlockComment()
		case r == '/' && l.peekByte(1) == '/':
			l.skipLineComment()
		case r == '"':
			l.lexString()
		case r == '`':
			l.lexPolyvar()
		case r == '\'':
			l.lexCharOrTyvar()
		case r == '-' && l.peekByte(1) == '>':
			l.consumeN(2)
			l.emit(token.ARROW, "->", nil)
		case r == '-' && l.peekByte(1) == '.':
			l.consumeN(2)
			l.emit(token.MINUSDOT, "-.", nil)
		case r == '-':
			l.consumeN(1)
			l.emit(token.MINUS, "-", nil)
		case r == '|':
			switch {
			case l.peekByte(1) == '>':
				l.consumeN(2)
				l.emit(token.PIPEOP, "|>", nil)
			case l.peekByte(1) == '|':
				l.consumeN(2)
				l.emit(token.PIPEPIPE, "||", nil)
			case l.peekByte(1) == ']':
				l.consumeN(2)
				l.emit(token.PIPERBRACKET, "|]", nil)
			default:
				l.consumeN(1)
				l.emit(token.PIPE, "|", nil)
			}
		case r == ':':
			switch {
			case l.peekByte(1) == ':':
				l.consumeN(2)
				l.emit(token.CONS, "::", nil)
			case l.peekByte(1) == '=':
				l.consumeN(2)
				l.emit(token.COLONEQ, ":=", nil)
			default:
				l.consumeN(1)
				l.emit(token.COLON, ":", nil)
			}
		case r == '=':
			if l.peekByte(1) == '=' {
				l.consumeN(2)
				l.emit(token.EQEQ, "==", nil)
			} else {
				l.consumeN(1)
				l.emit(token.EQUALS, "=", nil)
			}
		case r == '<':
			switch {
			case l.peekByte(1) == '=':
				l.consumeN(2)
				l.emit(token.LEQ, "<=", nil)
			case l.peekByte(1) == '>':
				l.consumeN(2)
				l.emit(token.DIAMOND, "<>", nil)
			case l.peekByte(1) == '-':
				l.consumeN(2)
				l.emit(token.LARROW, "<-", nil)
			default:
				l.consumeN(1)
				l.emit(token.LT, "<", nil)
			}
		case r == '>':
			if l.peekByte(1) == '=' {
				l.consumeN(2)
				l.emit(token.GEQ, ">=", nil)
			} else {
				l.consumeN(1)
				l.emit(token.GT, ">", nil)
			}
		case r == '!':
			if l.peekByte(1) == '=' {
				l.consumeN(2)
				l.emit(token.NEQ, "!=", nil)
			} else {
				l.consumeN(1)
				l.emit(token.BANG, "!", nil)
			}
		case r == '&':
			if l.peekByte(1) == '&' {
				l.consumeN(2)
				l.emit(token.AMPAMP, "&&", nil)
			} else {
				l.consumeN(1)
				l.emit(token.ERROR, "unexpected '&' without '&'", nil)
			}
		case r == '.' && l.peekByte(1) == '.' && l.peekByte(2) == '.':
			l.consumeN(3)
			l.emit(token.ELLIPSIS, "...", nil)
		case r == '.':
			l.consumeN(1)
			l.emit(token.DOT, ".", nil)
		case r == ',':
			l.consumeN(1)
			l.emit(token.COMMA, ",", nil)
		case r == ';':
			l.consumeN(1)
			l.emit(token.SEMI, ";", nil)
		case r == '+':
			if l.peekByte(1) == '.' {
				l.consumeN(2)
				l.emit(token.PLUSDOT, "+.", nil)
			} else {
				l.consumeN(1)
				l.emit(token.PLUS, "+", nil)
			}
		case r == '*':
			if l.peekByte(1) == '.' {
				l.consumeN(2)
				l.emit(token.STARDOT, "*.", nil)
			} else {
				l.consumeN(1)
				l.emit(token.STAR, "*", nil)
			}
		case r == '/':
			// // already handled above; /. is float div
			if l.peekByte(1) == '.' {
				l.consumeN(2)
				l.emit(token.SLASHDOT, "/.", nil)
			} else {
				l.consumeN(1)
				l.emit(token.SLASH, "/", nil)
			}
		case r == '^':
			l.consumeN(1)
			l.emit(token.CARET, "^", nil)
		case r == '@':
			l.consumeN(1)
			l.emit(token.AT, "@", nil)
		case r == '#':
			l.consumeN(1)
			l.emit(token.HASH, "#", nil)
		case r == '%':
			l.consumeN(1)
			l.emit(token.PERCENT, "%", nil)
		case r == '?':
			l.consumeN(1)
			l.emit(token.QUESTION, "?", nil)
		case r == '~':
			l.consumeN(1)
			l.emit(token.TILDE, "~", nil)
		case r == '(':
			l.consumeN(1)
			l.emit(token.LPAREN, "(", nil)
		case r == ')':
			l.consumeN(1)
			l.emit(token.RPAREN, ")", nil)
		case r == '{':
			l.consumeN(1)
			l.emit(token.LBRACE, "{", nil)
		case r == '}':
			l.consumeN(1)
			l.emit(token.RBRACE, "}", nil)
		case r == '[':
			if l.peekByte(1) == '|' {
				l.consumeN(2)
				l.emit(token.LBRACKETPIPE, "[|", nil)
			} else {
				l.consumeN(1)
				l.emit(token.LBRACKET, "[", nil)
			}
		case r == ']':
			l.consumeN(1)
			l.emit(token.RBRACKET, "]", nil)
		case r == '_':
			l.consumeN(1)
			if isIdentContinue(l.peekRuneRaw()) {
				// It's an identifier starting with underscore, not the wildcard keyword.
				ident := "_" + l.readIdentTail()
				l.emit(token.IDENT, ident, nil)
			} else {
				l.emit(token.UNDERSCORE, "_", nil)
			}
		case unicode.IsDigit(r):
			l.lexNumber()
		case isIdentStart(r):
			l.lexIdentOrKeyword()
		default:
			l.consumeN(1)
			l.errorf("unexpected character %q", r)
		}
	}
	// Emit EOF
	l.tokens = append(l.tokens, token.Token{
		Type:   token.EOF,
		Lexeme: "",
		Loc: token.SourceLoc{
			File:   l.file,
			Line:   l.line,
			Column: l.col,
			Offset: l.pos,
		},
	})
}

// ---------------------------------------------------------------------------
// Character helpers
// ---------------------------------------------------------------------------

func (l *Lexer) peekByte(ahead int) byte {
	off := l.pos + ahead
	if off < len(l.src) {
		return l.src[off]
	}
	return 0
}

func (l *Lexer) peekRune() (rune, int) {
	if l.pos >= len(l.src) {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRune(l.src[l.pos:])
}

func (l *Lexer) peekRuneRaw() rune {
	r, _ := l.peekRune()
	return r
}

func (l *Lexer) consumeN(n int) {
	for i := 0; i < n; i++ {
		if l.pos >= len(l.src) {
			break
		}
		r, size := utf8.DecodeRune(l.src[l.pos:])
		l.pos += size
		l.col++
		if r == '\n' {
			l.line++
			l.col = 1
		}
	}
}

func (l *Lexer) advance() rune {
	r, size := l.peekRune()
	l.pos += size
	l.col++
	if r == '\n' {
		l.line++
		l.col = 1
	}
	return r
}

// ---------------------------------------------------------------------------
// Whitespace
// ---------------------------------------------------------------------------

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if !isWhitespace(r) {
			break
		}
		l.advance()
	}
}

// skipRejectedBlockComment consumes a nested (* … *) span so a single
// diagnostic is reported instead of a parse cascade.
func (l *Lexer) skipRejectedBlockComment() {
	l.consumeN(2) // skip (*
	depth := 1
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if r == '(' && l.peekByte(1) == '*' {
			l.consumeN(2)
			depth++
		} else if r == '*' && l.peekByte(1) == ')' {
			l.consumeN(2)
			depth--
			if depth == 0 {
				return
			}
		} else {
			l.advance()
		}
	}
}

func (l *Lexer) skipLineComment() {
	l.consumeN(2) // skip //
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if r == '\n' || r == 0 {
			return
		}
		l.advance()
	}
}

func (l *Lexer) lexPolyvar() {
	l.consumeN(1) // skip `
	start := l.pos
	r, _ := l.peekRune()
	if !isIdentStart(r) && !unicode.IsUpper(r) {
		l.errorf("expected constructor name after '`'")
		return
	}
	l.advance()
	for {
		r, _ := l.peekRune()
		if !isIdentContinue(r) {
			break
		}
		l.advance()
	}
	name := string(l.src[start:l.pos])
	l.emit(token.POLYVAR, "`"+name, name)
}

// ---------------------------------------------------------------------------
// String literals
// ---------------------------------------------------------------------------

func (l *Lexer) lexString() {
	l.consumeN(1) // skip opening "
	var buf strings.Builder
	startLine := l.line
	startCol := l.col - 1
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if r == '"' {
			l.consumeN(1) // skip closing "
			l.emit(token.STRING, buf.String(), buf.String())
			return
		}
		if r == '\n' {
			l.errorf("unterminated string literal")
			return
		}
		if r == '\\' {
			l.consumeN(1)
			b, ok := l.lexEscapeByte("string")
			if !ok {
				return
			}
			buf.WriteByte(b)
		} else {
			buf.WriteRune(r)
			l.consumeN(1)
		}
	}
	l.errorf("unterminated string literal starting at %d:%d", startLine, startCol)
}

// ---------------------------------------------------------------------------
// Character literal or type variable
// ---------------------------------------------------------------------------

func (l *Lexer) lexCharOrTyvar() {
	// We already consumed the opening single quote.
	// Decision: if the quote is followed by a letter/underscore, it's a type
	// variable ('a); otherwise assume a character literal.
	l.consumeN(1) // skip '
	r, _ := l.peekRune()
	if r == 0 {
		l.errorf("unexpected end of file after single quote")
		return
	}
	if isIdentStart(r) && r != '\'' {
		// Type variable: 'a, 'key, etc.
		// Consume the leading letter, then the rest of the identifier tail.
		l.advance() // consume first letter
		tail := l.readIdentTail()
		name := "'" + string(r) + tail
		l.emit(token.TYVAR, name, nil)
	} else {
		// Character literal: consume the next character and then expect closing quote.
		var ch rune
		r0, _ := l.peekRune()
		if r0 == '\\' {
			l.consumeN(1)
			b, ok := l.lexEscapeByte("character")
			if !ok {
				return
			}
			ch = rune(b)
		} else {
			ch = l.advance()
		}
		r2, _ := l.peekRune()
		if r2 != '\'' {
			l.errorf("unterminated character literal")
			return
		}
		l.consumeN(1) // skip closing '
		l.emit(token.CHAR, string(ch), ch)
	}
}

// lexEscapeByte parses a string/char escape after the leading backslash has
// already been consumed. Supports \n \t \r \\ \" \' \e \xHH and \ooo (1–3 octal).
func (l *Lexer) lexEscapeByte(kind string) (byte, bool) {
	r, _ := l.peekRune()
	if r == 0 {
		l.errorf("unterminated %s escape", kind)
		return 0, false
	}
	switch r {
	case 'n':
		l.consumeN(1)
		return '\n', true
	case 't':
		l.consumeN(1)
		return '\t', true
	case 'r':
		l.consumeN(1)
		return '\r', true
	case '\\':
		l.consumeN(1)
		return '\\', true
	case '"':
		l.consumeN(1)
		return '"', true
	case '\'':
		l.consumeN(1)
		return '\'', true
	case 'e':
		l.consumeN(1)
		return 0x1b, true
	case 'x':
		l.consumeN(1)
		return l.lexHexEscape(kind)
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return l.lexOctalEscape(kind)
	default:
		l.errorf("invalid %s escape \\%c", kind, r)
		return 0, false
	}
}

func (l *Lexer) lexHexEscape(kind string) (byte, bool) {
	var val int
	for i := 0; i < 2; i++ {
		r, _ := l.peekRune()
		if r == 0 || r == '"' || r == '\'' || r == '\n' {
			l.errorf("incomplete hex escape in %s literal", kind)
			return 0, false
		}
		d, ok := hexDigit(r)
		if !ok {
			l.errorf("invalid hex digit in %s escape", kind)
			return 0, false
		}
		l.consumeN(1)
		val = val*16 + d
	}
	return byte(val), true
}

func (l *Lexer) lexOctalEscape(kind string) (byte, bool) {
	var val int
	for i := 0; i < 3; i++ {
		r, _ := l.peekRune()
		if r < '0' || r > '7' {
			break
		}
		l.consumeN(1)
		val = val*8 + int(r-'0')
		if val > 255 {
			l.errorf("octal escape out of range in %s literal", kind)
			return 0, false
		}
	}
	if val > 255 {
		l.errorf("octal escape out of range in %s literal", kind)
		return 0, false
	}
	return byte(val), true
}

func hexDigit(r rune) (int, bool) {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0'), true
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10, true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Numbers
// ---------------------------------------------------------------------------

func (l *Lexer) lexNumber() {
	start := l.pos
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if !unicode.IsDigit(r) {
			break
		}
		l.advance()
	}
	// Check for fractional part
	isFloat := false
	if l.pos < len(l.src) {
		r, _ := l.peekRune()
		if r == '.' {
			l.advance() // consume '.'
			// Check there's at least one more digit
			r2, _ := l.peekRune()
			if unicode.IsDigit(r2) {
				for l.pos < len(l.src) {
					r, _ := l.peekRune()
					if !unicode.IsDigit(r) {
						break
					}
					l.advance()
				}
				isFloat = true
			} else {
				// It's not a float, back up the '.'
				l.pos--
				l.col--
			}
		}
	}
	lexeme := string(l.src[start:l.pos])
	if isFloat {
		f, err := strconv.ParseFloat(lexeme, 64)
		if err != nil {
			l.errorf("invalid float literal %q", lexeme)
			return
		}
		l.emit(token.FLOAT, lexeme, f)
	} else {
		n, err := strconv.ParseInt(lexeme, 10, 64)
		if err != nil {
			l.errorf("invalid integer literal %q", lexeme)
			return
		}
		l.emit(token.INT, lexeme, n)
	}
}

// ---------------------------------------------------------------------------
// Identifiers & keywords
// ---------------------------------------------------------------------------

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentContinue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

func (l *Lexer) readIdentTail() string {
	start := l.pos
	for l.pos < len(l.src) {
		r, _ := l.peekRune()
		if !isIdentContinue(r) {
			break
		}
		l.advance()
	}
	return string(l.src[start:l.pos])
}

func (l *Lexer) lexIdentOrKeyword() {
	// The first character is already a valid ident start.
	// Note: we haven't consumed it yet, so consume it here.
	first := l.advance()
	tail := l.readIdentTail()
	lexeme := string(first) + tail

	// Check if it's a constructor (starts with uppercase)
	if unicode.IsUpper(first) {
		l.emit(token.CONSTRUCTOR, lexeme, nil)
		return
	}

	// Check against keywords
	kw := token.LookupKeyword(lexeme)
	if kw == token.UNIT {
		// UNIT is "()" so it can't appear as an ident. But `()` might be
		// parsed as two tokens by the lexer? No — `()` is tokenised as
		// LPAREN RPAREN, and then the parser can emit a UNIT token.
		// Here we just handle the case where the keyword lookup returns something.
		l.emit(kw, lexeme, nil)
	} else if kw != token.IDENT {
		// It's a keyword
		l.emit(kw, lexeme, nil)
	} else {
		l.emit(token.IDENT, lexeme, nil)
	}
}

// ---------------------------------------------------------------------------
// Public helper: PrintTokenStream prints a token stream for debugging.
// ---------------------------------------------------------------------------

// PrintTokenStream returns a human-readable representation of the token stream.
func PrintTokenStream(toks []token.Token) string {
	var buf strings.Builder
	for _, t := range toks {
		buf.WriteString(t.String())
		buf.WriteByte('\n')
	}
	return buf.String()
}
