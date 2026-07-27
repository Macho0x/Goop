// Package parser implements a recursive-descent parser for Goop.
//
// It handles:
//   - Module declarations with qualified names (dots)
//   - Import declarations (go / goop) and @[go]/@[c] embeds
//   - Let / type / exception / effect declarations
//   - OCaml-aligned expression language (match, try, while, ref, …)
//   - Pattern matching (wildcard, identifier, literal, constructor, record, tuple, list, cons, alias)
//   - Migration errors (PARSE-MIG*) for removed pre-1.0 syntax
package parser

import (
	"fmt"
	"strings"

	"goop.dev/compiler/internal/ast"
	lc0 "goop.dev/compiler/internal/lexer"
	"goop.dev/compiler/internal/token"
)

// ParseError records a parse failure with source location.
type ParseError struct {
	Msg string
	Loc token.SourceLoc
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Loc, e.Msg)
}

// Parser holds the state of the recursive-descent parser.
type Parser struct {
	file      string
	src       []byte // raw source bytes for reading inline Go blocks
	tokens    []token.Token
	pos       int // index into tokens slice
	errs      []error
	wherePred bool // true when parsing a where-clause predicate (stops at =)
}

// Parse reads a complete Goop source file and returns the AST module.
func Parse(file string, src []byte) (*ast.Module, error) {
	// Lex a masked copy so @[go]/@[c] bodies are opaque to the Goop lexer
	// (e.g. Go's func(*T) must not start a (* *) comment). Parser.src stays
	// the original bytes so readRawGoBlock recovers the real embed text.
	lexSrc := maskLangEmbedBodies(src)
	toks, err := lc0.Lex(file, lexSrc)
	if err != nil {
		return nil, err
	}
	toks, attrs := stripAttributes(toks)
	p := &Parser{file: file, src: src, tokens: toks}
	mod := p.parseModule()
	mod.Attributes = attrs
	if len(p.errs) > 0 {
		// Return the module along with the first error so callers can inspect
		// partial results.
		return mod, p.errs[0]
	}
	return mod, nil
}

// Errors returns all accumulated parse errors.
func (p *Parser) Errors() []error {
	return p.errs
}

// ---------------------------------------------------------------------------
// Low‑level helpers
// ---------------------------------------------------------------------------

func (p *Parser) cur() token.Token {
	if p.pos >= len(p.tokens) {
		return token.Token{Type: token.EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peek() token.Token {
	if p.pos+1 >= len(p.tokens) {
		return token.Token{Type: token.EOF}
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) peekN(n int) token.Token {
	if p.pos+n >= len(p.tokens) {
		return token.Token{Type: token.EOF}
	}
	return p.tokens[p.pos+n]
}

func (p *Parser) advance() token.Token {
	t := p.cur()
	if t.Type != token.EOF {
		p.pos++
	}
	return t
}

func (p *Parser) match(t token.TokenType) bool {
	if p.cur().Type == t {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(t token.TokenType) (token.Token, error) {
	if p.cur().Type == t {
		return p.advance(), nil
	}
	err := p.errorf("expected %s, got %s", t, p.cur().Type)
	return p.cur(), err
}

func (p *Parser) errorf(format string, args ...any) error {
	err := &ParseError{
		Msg: fmt.Sprintf(format, args...),
		Loc: p.cur().Loc,
	}
	p.errs = append(p.errs, err)
	return err
}

func (p *Parser) parseIdentName() string {
	tok := p.cur()
	if tok.Type != token.IDENT && tok.Type != token.CONSTRUCTOR {
		p.errorf("expected identifier, got %s", tok.Type)
		return "_"
	}
	p.advance()
	return tok.Lexeme
}

// parseGoBlock parses a `go { … }` block inside an extern declaration.
// It reads the raw Go source text between braces, uses source byte offsets
// to advance the parser position past all tokens inside the block INCLUDING
// the closing '}', and returns the Go code content (without surrounding braces).
func (p *Parser) parseGoBlock() string {
	braceTok, _ := p.expect(token.LBRACE)
	if braceTok.Type != token.LBRACE {
		return ""
	}
	startOffset := braceTok.Loc.Offset

	// Read raw source bytes and get the byte offset of the closing '}'.
	code, endOffset := p.readRawGoBlock(startOffset)

	// Advance parser position past all tokens whose source offset is
	// inside the go block, consuming the closing '}' token too.
	for p.pos < len(p.tokens) {
		tok := p.cur()
		if tok.Type == token.EOF {
			break
		}
		if tok.Loc.Offset > endOffset {
			// We've consumed the closing '}'. Stop.
			break
		}
		p.advance()
	}

	return code
}

// readRawGoBlock reads raw source bytes from just after '{' until the matching '}'.
// It counts brace depth in the raw source, skipping string literals and comments.
// Returns the Go code content (trimmed) and the byte offset of the closing '}'.
func (p *Parser) readRawGoBlock(braceOffset int) (string, int) {
	if braceOffset >= len(p.src) || p.src[braceOffset] != '{' {
		p.errorf("internal: readRawGoBlock called without opening brace")
		return "", braceOffset + 1
	}
	pos := braceOffset + 1 // skip '{'
	depth := 1
	for pos < len(p.src) && depth > 0 {
		switch p.src[pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				// Don't advance past the matching '}'
				content := string(p.src[braceOffset+1 : pos])
				return strings.TrimSpace(content), pos
			}
		case '/':
			// Skip line comments and block comments
			if pos+1 < len(p.src) && p.src[pos+1] == '/' {
				pos += 2
				for pos < len(p.src) && p.src[pos] != '\n' {
					pos++
				}
				continue
			}
			if pos+1 < len(p.src) && p.src[pos+1] == '*' {
				pos += 2
				for pos+1 < len(p.src) && !(p.src[pos] == '*' && p.src[pos+1] == '/') {
					pos++
				}
				pos += 2 // skip */
				continue
			}
		case '"':
			// Skip Go string literal
			pos++
			for pos < len(p.src) && p.src[pos] != '"' {
				if p.src[pos] == '\\' {
					pos++ // skip escaped char
				}
				pos++
			}
		case '\'':
			// Skip Go rune literal
			pos++
			for pos < len(p.src) && p.src[pos] != '\'' {
				if p.src[pos] == '\\' {
					pos++
				}
				pos++
			}
		case '`':
			// Skip Go raw string literal
			pos++
			for pos < len(p.src) && p.src[pos] != '`' {
				pos++
			}
		}
		pos++
	}
	if depth != 0 {
		p.errorf("unterminated go block (missing '}')")
		return "", braceOffset + 1
	}
	content := string(p.src[braceOffset+1 : pos])
	return strings.TrimSpace(content), pos
}

// ---------------------------------------------------------------------------
// Expression / pattern / type start‑of‑input predicates
// ---------------------------------------------------------------------------

func (p *Parser) canStartExpr(t token.Token) bool {
	switch t.Type {
	// Note: token.LET is intentionally excluded from canStartExpr
	// because a `let` at expression level must always be `let … in …`,
	// which is parsed by parsePrefix().  Including LET here causes the
	// function‑application juxtaposition loop to greedily consume a
	// top‑level `let` declaration as if it were an argument.
	case token.IDENT, token.CONSTRUCTOR, token.POLYVAR, token.INT, token.FLOAT,
		token.STRING, token.CHAR, token.TRUE, token.FALSE,
		token.LPAREN, token.LBRACE, token.LBRACKET, token.LBRACKETPIPE,
		token.IF, token.MATCH, token.FUN, token.FUNCTION,
		token.FOR, token.WHILE, token.BEGIN, token.GO,
		token.TRY, token.RAISE, token.ASSERT, token.LAZY, token.PERFORM,
		token.FAILWITH, token.REF, token.BANG, token.OBJECT, token.NEW, token.TILDE,
		// Migration keywords: still start an expr so we can emit PARSE-MIG*.
		token.GUARD, token.USING, token.PANIC:
		return true
	default:
		return false
	}
}

func (p *Parser) canStartPattern(t token.Token) bool {
	switch t.Type {
	case token.UNDERSCORE, token.IDENT, token.CONSTRUCTOR,
		token.POLYVAR, token.INT, token.FLOAT, token.STRING, token.CHAR,
		token.TRUE, token.FALSE, token.EXCEPTION, token.LAZY,
		token.LPAREN, token.LBRACE, token.LBRACKET:
		return true
	default:
		return false
	}
}

func (p *Parser) canStartType(t token.Token) bool {
	switch t.Type {
	case token.IDENT, token.CONSTRUCTOR, token.TYVAR,
		token.LPAREN, token.LBRACE, token.LBRACKET:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

func (p *Parser) parseModule() *ast.Module {
	mod := &ast.Module{}

	// Optional `module Name` file header (not `module M =` / `module type` / functor)
	if p.cur().Type == token.MODULE {
		next := p.peek().Type
		if next != token.TYPE && next != token.REC && next != token.EQUALS {
			saved := p.pos
			p.advance() // module
			name := p.parseQualifiedName()
			if p.cur().Type == token.EQUALS || p.cur().Type == token.LPAREN || p.cur().Type == token.COLON {
				// Nested module / functor — rewind for top-decl parsing
				p.pos = saved
			} else {
				mod.Name = name
			}
		}
	}

	// Zero or more import declarations
	for p.cur().Type == token.IMPORT {
		specs := p.parseImportDecl()
		mod.Imports = append(mod.Imports, specs...)
	}

	// Zero or more top-level declarations
	for p.cur().Type != token.EOF {
		decl := p.parseTopDecl()
		if decl != nil {
			mod.Decls = append(mod.Decls, decl)
		}
	}

	return mod
}

// parseQualifiedName reads a dot-separated identifier chain.
// E.g. "Trading.OrderBook" or "Std.List".
func (p *Parser) parseQualifiedName() string {
	name := ""
	for {
		tok := p.cur()
		if tok.Type == token.CONSTRUCTOR || tok.Type == token.IDENT {
			p.advance()
			name += tok.Lexeme
		} else {
			break
		}
		if p.cur().Type == token.DOT {
			p.advance()
			name += "."
		} else {
			break
		}
	}
	return name
}

// ---------------------------------------------------------------------------
// Top-level declarations
// ---------------------------------------------------------------------------

func (p *Parser) parseTopDecl() ast.TopDecl {
	switch p.cur().Type {
	case token.AT:
		return p.parseLangEmbedDecl()
	case token.PRIVATE:
		p.advance()
		switch p.cur().Type {
		case token.LET:
			return p.parseLetDecl(true)
		case token.TYPE:
			return p.parseTypeDecl(true)
		default:
			p.errorf("expected let or type after private, got %s", p.cur().Type)
			p.advance()
			return nil
		}
	case token.LET:
		return p.parseLetDecl(false)
	case token.TYPE:
		return p.parseTypeDecl(false)
	case token.IMPLEMENTS:
		return p.parseImplementsDecl()
	case token.EXCEPTION:
		return p.parseExceptionDecl()
	case token.EFFECT:
		return p.parseEffectDecl()
	case token.EXTERN:
		p.errorf("'extern' is removed; use `import go \"path\"` or `import go \"path\" { val ... }`")
		return p.parseExternDeclSkip()
	case token.IMPORT:
		p.errorf("'import' must appear before any declarations")
		p.parseImportDecl()
		return nil
	case token.MODULE:
		return p.parseModuleDecl()
	case token.OPEN:
		return p.parseOpenDecl()
	case token.INCLUDE:
		return p.parseIncludeDecl()
	case token.CLASS:
		return p.parseClassDecl()
	default:
		p.errorf("unexpected token %s at top level", p.cur().Type)
		p.advance()
		return nil
	}
}

func (p *Parser) parseImplementsDecl() *ast.ImplementsDecl {
	p.expect(token.IMPLEMENTS)
	decl := &ast.ImplementsDecl{Interface: p.parseIdentName()}
	p.expect(token.FOR)
	decl.ForType = p.parseIdentName()
	p.expect(token.WITH)
	for p.cur().Type == token.LET {
		p.advance()
		decl.Methods = append(decl.Methods, p.parseBinding())
	}
	p.expect(token.END)
	return decl
}

// parseLetDecl parses a top-level `let` (no `in`), or
// an active pattern `let (|Name|_|) (...) = ...`.
func (p *Parser) parseLetDecl(isPrivate bool) *ast.LetDecl {
	p.expect(token.LET)
	decl := &ast.LetDecl{Private: isPrivate}

	// Active pattern: let (|Name|_|) ...
	if p.cur().Type == token.LPAREN && p.peek().Type == token.PIPE {
		p.advance() // consume LPAREN
		p.expect(token.PIPE)
		tok := p.cur()
		if tok.Type != token.CONSTRUCTOR && tok.Type != token.IDENT {
			p.errorf("expected active pattern name, got %s", tok.Type)
		} else {
			p.advance()
		}
		p.expect(token.PIPE)
		p.expect(token.UNDERSCORE)
		p.expect(token.PIPE)
		p.expect(token.RPAREN)

		decl.ActivePattern = true
		// parseBinding expects name first, but we already consumed it.
		// Parse params, return type, and body manually.
		b := ast.LetBinding{Name: tok.Lexeme}
		b.Params = p.parseParams()
		if p.match(token.COLON) {
			b.RetType = p.parseType()
		}
		p.expect(token.EQUALS)
		b.Body = p.parseExpr(0)
		decl.Bindings = append(decl.Bindings, b)
		return decl
	}

	if p.match(token.REC) {
		decl.Rec = true
	}
	if p.cur().Type == token.MUTABLE {
		p.errorf("PARSE-MIG010: 'let mutable' is removed; use `ref` / `:=` / `!`")
		p.advance()
	}
	decl.Bindings = append(decl.Bindings, p.parseBinding())
	for p.match(token.AND) {
		decl.Bindings = append(decl.Bindings, p.parseBinding())
	}
	return decl
}

// parseBinding parses `name params… (: type)? = expr`.
func (p *Parser) parseBinding() ast.LetBinding {
	b := ast.LetBinding{}

	tok := p.cur()
	if tok.Type != token.IDENT && tok.Type != token.CONSTRUCTOR && tok.Type != token.UNDERSCORE {
		p.errorf("expected binding name, got %s", tok.Type)
		if p.cur().Type != token.EOF {
			p.advance()
		}
		return b
	}
	p.advance()
	b.Name = tok.Lexeme

	// Parse parameters (zero or more)
	b.Params = p.parseParams()

	// Optional return type annotation
	if p.match(token.COLON) {
		b.RetType = p.parseType()
		// Effect rows on return types are removed (PARSE-MIG016).
		if p.cur().Type == token.WITH && p.peek().Type == token.LBRACE {
			p.errorf("PARSE-MIG016: effect row `with { … }` is removed; use effect handlers")
			_ = p.parseEffectRow() // parse and discard
		}
	}

	// Expect `=`
	p.expect(token.EQUALS)

	// Parse body expression
	b.Body = p.parseExpr(0)

	return b
}

// parseParams parses a sequence of function parameters.
func (p *Parser) parseParams() []ast.Param {
	var params []ast.Param
	for {
		switch p.cur().Type {
		case token.TILDE:
			p.advance()
			param := ast.Param{}
			tok := p.cur()
			if tok.Type == token.IDENT || tok.Type == token.CONSTRUCTOR {
				p.advance()
				param.Name = tok.Lexeme
				param.Label = tok.Lexeme
			}
			if p.match(token.COLON) {
				// ~label:name or ~label:(name : type) — treat next ident as name
				if p.cur().Type == token.LPAREN {
					p.advance()
					nameTok := p.cur()
					if nameTok.Type == token.IDENT {
						p.advance()
						param.Name = nameTok.Lexeme
					}
					if p.match(token.COLON) {
						param.Type = p.parseType()
					}
					p.expect(token.RPAREN)
				} else if p.cur().Type == token.IDENT {
					nameTok := p.advance()
					param.Name = nameTok.Lexeme
				}
			}
			params = append(params, param)
		case token.QUESTION:
			p.advance()
			param := ast.Param{Optional: true}
			tok := p.cur()
			if tok.Type == token.IDENT || tok.Type == token.CONSTRUCTOR {
				p.advance()
				param.Name = tok.Lexeme
				param.Label = tok.Lexeme
			}
			if p.match(token.EQUALS) {
				param.Default = p.parsePrefix()
			}
			params = append(params, param)
		case token.IDENT:
			tok := p.advance()
			params = append(params, ast.Param{Name: tok.Lexeme})
		case token.LPAREN:
			// `(name : type)`
			p.advance()
			param := ast.Param{}
			if p.match(token.ELLIPSIS) {
				param.Variadic = true
			}
			nameTok := p.cur()
			if nameTok.Type == token.IDENT || nameTok.Type == token.CONSTRUCTOR {
				p.advance()
				param.Name = nameTok.Lexeme
			}
			if p.match(token.COLON) {
				param.Type = p.parseType()
			}
			p.expect(token.RPAREN)
			params = append(params, param)
		default:
			return params
		}
	}
}

// ---------------------------------------------------------------------------
// Type declarations
// ---------------------------------------------------------------------------

func (p *Parser) parseTypeDecl(isPrivate bool) ast.TopDecl {
	p.expect(token.TYPE)
	// Extensible variant extension: `type t += C of ...`.
	if (p.cur().Type == token.IDENT || p.cur().Type == token.CONSTRUCTOR) &&
		p.peek().Type == token.PLUS && p.peekN(2).Type == token.EQUALS {
		name := p.advance().Lexeme
		p.advance() // +
		p.advance() // =
		kind := p.parseADTOrGADT()
		if adt, ok := kind.(*ast.ADTTypeKind); ok {
			return &ast.ExtensibleVariantDecl{TypeName: name, Cases: adt.Cases}
		}
		p.errorf("extensible variants must use ordinary constructors")
		return &ast.ExtensibleVariantDecl{TypeName: name}
	}

	// We handle the first binding inline; `and` produces additional decls.
	// NOTE: `and`-connected type decls are returned one at a time.
	// This matches the current module builder which calls parseTopDecl in a loop.

	var allDecls []*ast.TypeDecl

	for {
		d := p.parseTypeBinding()
		allDecls = append(allDecls, d)
		if !p.match(token.AND) {
			break
		}
	}

	// Return only the first; remaining will be returned on subsequent calls
	// via a side channel.  For the bootstrap we accept this limitation.
	// A production parser would push the extras onto a pending queue.
	if len(allDecls) > 0 {
		allDecls[0].Private = isPrivate
		return allDecls[0]
	}
	return nil
}

func (p *Parser) parseTypeBinding() *ast.TypeDecl {
	d := &ast.TypeDecl{}

	// OCaml-style prefix params: `type 'a t` or `type _ t`
	var prefixParams []string
	for p.cur().Type == token.TYVAR || p.cur().Type == token.UNDERSCORE {
		tok := p.advance()
		if tok.Type == token.UNDERSCORE {
			prefixParams = append(prefixParams, "_")
		} else {
			prefixParams = append(prefixParams, tok.Lexeme)
		}
	}

	tok := p.cur()
	if tok.Type == token.CONSTRUCTOR || tok.Type == token.IDENT {
		p.advance()
		d.Name = tok.Lexeme
	} else {
		p.errorf("expected type name, got %s", tok.Type)
		return d
	}

	// Optional postfix type parameters: 'a 'b …
	for p.cur().Type == token.TYVAR {
		tv := p.advance()
		d.TypeParams = append(d.TypeParams, tv.Lexeme)
	}
	d.TypeParams = append(prefixParams, d.TypeParams...)

	// Check for `: 1` linear quantity annotation
	// e.g. `type file_handle : 1` or `type file_handle : 1 = ...`
	if p.cur().Type == token.COLON {
		// Peek: if next token is INT, it's a quantity annotation
		if p.peek().Type == token.INT {
			p.advance() // consume COLON
			intTok := p.advance()
			val, ok := intTok.Literal.(int64)
			if ok && val == 1 {
				d.Quantity = 1
			} else {
				p.errorf("expected '1' for linear quantity, got %v", intTok.Literal)
			}
		}
		// If next token is not INT, it might be a type annotation for a
		// different syntax — bail out and let the expected '=' catch it.
	}

	// Opaque linear type: `type T : 1` (no `= ...` body)
	if d.Quantity == 1 && p.cur().Type != token.EQUALS {
		d.Kind = &ast.OpaqueTypeKind{}
		return d
	}

	p.expect(token.EQUALS)
	if p.cur().Type == token.DOT && p.peek().Type == token.DOT {
		p.advance()
		p.advance()
		d.Kind = &ast.ExtensibleTypeKind{}
		return d
	}

	// Determine the kind of RHS
	switch {
	case p.cur().Type == token.LBRACE:
		d.Kind = p.parseRecordTypeKind()
	case p.cur().Type == token.PIPE:
		d.Kind = p.parseADTOrGADT()
	default:
		// Could be ADT without leading pipe, or a type alias.
		if p.cur().Type == token.CONSTRUCTOR {
			next := p.peek().Type
			if next == token.PIPE || next == token.EOF || next == token.RBRACE ||
				next == token.LET || next == token.TYPE || next == token.MODULE || next == token.OPEN ||
				next == token.COLON || next == token.OF || next == token.END {
				d.Kind = p.parseADTOrGADT()
				return d
			}
		}
		if p.cur().Type == token.NEWTYPE {
			p.errorf("PARSE-MIG015: 'newtype' is removed; use a single-constructor ADT and `private`")
			p.advance()
			d.Kind = &ast.NewtypeTypeKind{Rep: p.parseType()}
			return d
		}
		d.Kind = &ast.AliasTypeKind{Alias: p.parseType()}
	}
	return d
}

// parseADTOrGADT parses ADT or GADT constructors after optional leading `|`.
func (p *Parser) parseADTOrGADT() ast.TypeKind {
	if p.cur().Type == token.PIPE {
		p.advance()
	}
	// Peek first constructor: if followed by `:`, it's a GADT.
	if p.cur().Type == token.CONSTRUCTOR && p.peek().Type == token.COLON {
		return p.parseGADTTypeKind()
	}
	return p.parseADTTypeKindNoPipe()
}

func (p *Parser) parseGADTTypeKind() *ast.GADTTypeKind {
	gk := &ast.GADTTypeKind{}
	for {
		c := ast.GADTCase{}
		tok := p.cur()
		if tok.Type != token.CONSTRUCTOR {
			break
		}
		p.advance()
		c.Name = tok.Lexeme
		p.expect(token.COLON)
		// Parse `arg -> result` or just `result`
		t := p.parseType()
		if p.match(token.ARROW) {
			c.Arg = t
			c.Result = p.parseType()
		} else {
			c.Result = t
		}
		gk.Cases = append(gk.Cases, c)
		if !p.match(token.PIPE) {
			break
		}
	}
	return gk
}

func (p *Parser) parseExceptionDecl() *ast.ExceptionDecl {
	p.expect(token.EXCEPTION)
	d := &ast.ExceptionDecl{}
	tok := p.cur()
	if tok.Type != token.CONSTRUCTOR && tok.Type != token.IDENT {
		p.errorf("expected exception name, got %s", tok.Type)
		return d
	}
	p.advance()
	d.Name = tok.Lexeme
	if p.match(token.OF) {
		d.Arg = p.parseType()
	}
	return d
}

func (p *Parser) parseEffectDecl() *ast.EffectDecl {
	p.expect(token.EFFECT)
	d := &ast.EffectDecl{}
	tok := p.cur()
	if tok.Type != token.CONSTRUCTOR && tok.Type != token.IDENT {
		p.errorf("expected effect name, got %s", tok.Type)
		return d
	}
	p.advance()
	d.Name = tok.Lexeme
	p.expect(token.COLON)
	from := p.parseTupleType()
	p.expect(token.ARROW)
	to := p.parseType()
	d.From = from
	d.To = to
	return d
}

func (p *Parser) parseRecordTypeKind() *ast.RecordTypeKind {
	p.expect(token.LBRACE)
	rk := &ast.RecordTypeKind{}
	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		ft := ast.FieldType{}
		if p.match(token.MUTABLE) {
			ft.Mutable = true
		}
		tok := p.cur()
		if tok.Type == token.IDENT {
			p.advance()
			ft.Name = tok.Lexeme
		}
		p.expect(token.COLON)
		ft.Type = p.parseType()
		rk.Fields = append(rk.Fields, ft)
		if !p.match(token.SEMI) {
			break
		}
	}
	p.expect(token.RBRACE)
	return rk
}

func (p *Parser) parseADTTypeKindNoPipe() *ast.ADTTypeKind {
	ak := &ast.ADTTypeKind{}
	for {
		c := ast.ADTCase{}
		tok := p.cur()
		if tok.Type != token.CONSTRUCTOR {
			break
		}
		p.advance()
		c.Name = tok.Lexeme

		if p.match(token.OF) {
			c.Arg = p.parseType()
		}
		ak.Cases = append(ak.Cases, c)

		if !p.match(token.PIPE) {
			break
		}
	}
	return ak
}

// ---------------------------------------------------------------------------
// @[go] / @[c] language embeds
// ---------------------------------------------------------------------------

func (p *Parser) parseLangEmbedDecl() *ast.LangEmbedDecl {
	p.expect(token.AT)
	// Legacy @golang → migration error
	if p.cur().Type == token.IDENT && p.cur().Lexeme == "golang" {
		p.errorf("'@golang' is removed; use `@[go] { ... }`")
		p.advance()
		if p.cur().Type == token.LBRACE {
			_ = p.parseGoBlock()
		}
		return nil
	}
	if p.cur().Type != token.LBRACKET {
		p.errorf("expected @[go] or @[c] after @, got @%s", p.cur().Lexeme)
		return nil
	}
	p.advance() // [
	nameTok := p.cur()
	lang := ""
	switch nameTok.Type {
	case token.IDENT, token.CONSTRUCTOR, token.GO:
		lang = nameTok.Lexeme
		if nameTok.Type == token.GO {
			lang = "go"
		}
		p.advance()
	default:
		p.errorf("expected lang name inside @[...], got %s", nameTok.Type)
		return nil
	}
	if lang != "go" && lang != "c" {
		p.errorf("unknown lang embed @[%s] (expected @[go] or @[c])", lang)
		// still try to consume ] { ... } for recovery
	}
	p.expect(token.RBRACKET)
	decl := &ast.LangEmbedDecl{Lang: lang}
	decl.Body = p.parseGoBlock()
	for p.cur().Type == token.VAL {
		p.advance()
		ev := ast.ExternVal{}
		nameTok := p.cur()
		if nameTok.Type == token.IDENT || nameTok.Type == token.CONSTRUCTOR {
			p.advance()
			ev.Name = nameTok.Lexeme
		} else {
			p.errorf("expected val binding name, got %s", nameTok.Type)
			continue
		}
		p.expect(token.COLON)
		ev.Type = p.parseType()
		decl.Vals = append(decl.Vals, ev)
	}
	return decl
}

// ---------------------------------------------------------------------------
// Import declarations
// ---------------------------------------------------------------------------

func (p *Parser) parseImportDecl() []ast.ImportSpec {
	p.expect(token.IMPORT)
	if p.cur().Type == token.LPAREN {
		p.advance()
		var specs []ast.ImportSpec
		for p.cur().Type != token.RPAREN && p.cur().Type != token.EOF {
			specs = append(specs, p.parseImportSpec())
		}
		p.expect(token.RPAREN)
		return specs
	}
	return []ast.ImportSpec{p.parseImportSpec()}
}

func (p *Parser) parseImportSpec() ast.ImportSpec {
	spec := ast.ImportSpec{}

	// Hard-break: bare `golang` is no longer a keyword
	if p.cur().Type == token.IDENT && p.cur().Lexeme == "golang" {
		p.errorf("'golang' is removed; use `import go \"path\"` or `import go \"path\" { val ... }`")
		p.advance()
		if p.cur().Type == token.STRING {
			p.advance()
		}
		if p.cur().Type == token.LBRACE {
			p.advance()
			for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
				p.advance()
			}
			p.expect(token.RBRACE)
		}
		return spec
	}

	// Optional local alias before kind keyword (must be followed by go/goop)
	if p.cur().Type == token.IDENT {
		next := token.EOF
		if p.pos+1 < len(p.tokens) {
			next = p.tokens[p.pos+1].Type
		}
		if next == token.GO || next == token.GOOP {
			spec.Alias = p.cur().Lexeme
			p.advance()
		}
	}

	switch p.cur().Type {
	case token.GO:
		spec.Kind = ast.ImportGo
		p.advance()
		// Optional raw opt-out: import go raw "path" { … }
		// Keeps (T, error) as a Goop tuple instead of coercing to result (H6).
		if p.cur().Type == token.IDENT && p.cur().Lexeme == "raw" {
			spec.Raw = true
			p.advance()
		}
	case token.GOOP:
		spec.Kind = ast.ImportGoop
		p.advance()
		// Dot import: goop . "path"
		if p.cur().Type == token.DOT {
			if spec.Alias != "" {
				p.errorf("dot import cannot have a local alias; use `import goop . \"path\"`")
			}
			spec.Alias = "."
			p.advance()
		}
	default:
		p.errorf("expected `go` or `goop` after import, got %s", p.cur().Type)
		p.synchronizeImportSpec()
		return spec
	}

	tok := p.cur()
	if tok.Type == token.STRING {
		p.advance()
		spec.Path = tok.Lexeme
	} else {
		p.errorf("expected string literal import path, got %s", tok.Type)
	}

	if p.cur().Type == token.LBRACE {
		if spec.Kind == ast.ImportGoop {
			p.errorf("goop imports cannot have `{ val ... }` blocks (only go imports)")
		}
		p.advance()
		for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
			if p.match(token.VAL) {
				ev := ast.ExternVal{Kind: ast.ExternFunc}
				if p.match(token.LPAREN) {
					recvTok := p.cur()
					if recvTok.Type == token.IDENT {
						ev.RecvName = recvTok.Lexeme
						p.advance()
					} else {
						p.errorf("expected method receiver name, got %s", recvTok.Type)
					}
					p.expect(token.COLON)
					ev.RecvType = p.parseType()
					p.expect(token.RPAREN)
					p.expect(token.DOT)
					nameTok := p.cur()
					if nameTok.Type == token.IDENT || nameTok.Type == token.CONSTRUCTOR {
						ev.Name = nameTok.Lexeme
						p.advance()
					} else {
						p.errorf("expected method or field name, got %s", nameTok.Type)
					}
				} else {
					nameTok := p.cur()
					if nameTok.Type == token.IDENT || nameTok.Type == token.CONSTRUCTOR {
						p.advance()
						ev.Name = nameTok.Lexeme
					} else {
						p.errorf("expected import binding name, got %s", nameTok.Type)
					}
				}
				p.expect(token.COLON)
				ev.Type = p.parseType()
				if ev.RecvType != nil {
					if _, ok := ev.Type.(*ast.TFun); ok {
						ev.Kind = ast.ExternMethod
					} else {
						ev.Kind = ast.ExternField
					}
				}
				spec.Vals = append(spec.Vals, ev)
			} else if p.match(token.TYPE) {
				et := ast.ExternType{}
				nameTok := p.cur()
				if nameTok.Type == token.IDENT || nameTok.Type == token.CONSTRUCTOR {
					p.advance()
					et.Name = nameTok.Lexeme
				} else {
					p.errorf("expected imported type name, got %s", nameTok.Type)
				}
				spec.Types = append(spec.Types, et)
			} else {
				p.errorf("expected `val` or `type` inside go import block, got %s", p.cur().Type)
				p.advance()
			}
		}
		p.expect(token.RBRACE)
	}

	return spec
}

func (p *Parser) synchronizeImportSpec() {
	for p.cur().Type != token.EOF {
		switch p.cur().Type {
		case token.RPAREN, token.GO, token.GOOP, token.IMPORT:
			return
		default:
			p.advance()
		}
	}
}

// ---------------------------------------------------------------------------
// Extern declarations (legacy skip — parser rejects with migration hint)
// ---------------------------------------------------------------------------

func (p *Parser) parseExternDeclSkip() ast.TopDecl {
	p.expect(token.EXTERN)
	if p.cur().Type == token.STRING {
		p.advance()
	}
	if p.cur().Type == token.STRING {
		p.advance()
	}
	if p.cur().Type == token.LBRACE {
		depth := 1
		p.advance()
		for depth > 0 && p.cur().Type != token.EOF {
			switch p.cur().Type {
			case token.LBRACE:
				depth++
			case token.RBRACE:
				depth--
			}
			p.advance()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Expression parsing (Pratt / precedence-climbing)
// ---------------------------------------------------------------------------

// Precedence levels (higher = binds tighter).
const (
	precLowest  = 1
	precAssign  = 1 // <- (same level as lowest; right-assoc via parseExpr)
	precPipe    = 2 // |>
	precOr      = 3 // ||
	precAnd     = 4 // &&
	precCompare = 5 // == != < > <= >= <>
	precCons    = 6 // ::  (right-associative)
	precAdd     = 7 // + - ^
	precMul     = 8 // * / *.
	precApp     = 9 // function application (juxtaposition)
	precUnary   = 10
	precPostfix = 11 // . (field), ? (propagation)
)

func (p *Parser) precedence(op token.TokenType) int {
	// In where-clause predicates, = is NOT a comparison operator — it's the
	// binding separator that follows the type annotation.
	if p.wherePred && op == token.EQUALS {
		return 0
	}
	switch op {
	case token.PIPEOP:
		return precPipe
	case token.PIPEPIPE:
		return precOr
	case token.AMPAMP:
		return precAnd
	case token.EQEQ, token.NEQ, token.EQUALS, token.LT, token.GT, token.LEQ, token.GEQ, token.DIAMOND:
		return precCompare
	case token.CONS:
		return precCons
	case token.PLUS, token.MINUS, token.CARET, token.PLUSDOT, token.MINUSDOT:
		return precAdd
	case token.STAR, token.SLASH, token.STARDOT, token.SLASHDOT,
		token.MOD, token.LAND, token.LOR, token.LXOR, token.PERCENT:
		return precMul
	default:
		return 0
	}
}

func (p *Parser) isRightAssoc(op token.TokenType) bool {
	return op == token.CONS
}

// parseExpr is the top-level expression parser, starting at the given
// minimum precedence.
func (p *Parser) parseExpr(minPrec int) ast.Expr {
	left := p.parsePrefix()
	// Track line of the leftmost expression for offside rule.
	// baseCol stays fixed so indented continuations compare to the callee.
	leftLine := p.cur().Loc.Line
	baseCol := p.cur().Loc.Column
	if p.pos > 0 {
		leftLine = p.tokens[p.pos-1].Loc.Line
		baseCol = p.tokens[p.pos-1].Loc.Column
	}

	for {
		cur := p.cur()

		// Postfix operators (bind tighter than juxtaposition).
		// This ensures `o.price` is parsed before `greater o.price`.
		switch cur.Type {
		case token.QUESTION:
			if precPostfix < minPrec {
				return left
			}
			left = p.parseQuestionExpr(left)
			continue
		case token.DOT:
			if precPostfix < minPrec {
				return left
			}
			left = p.parseFieldAccessExpr(left)
			continue
		case token.HASH:
			if precPostfix < minPrec {
				return left
			}
			left = p.parseMethodSendExpr(left)
			continue
		case token.IS:
			if precCompare < minPrec {
				return left
			}
			left = p.parseIsExpr(left)
			continue
		case token.AS:
			if precCompare < minPrec {
				return left
			}
			left = p.parseAsMatchExpr(left)
			continue
		case token.LARROW, token.COLONEQ:
			if precAssign < minPrec {
				return left
			}
			assignLoc := cur.Loc
			coloneq := cur.Type == token.COLONEQ
			p.advance()
			right := p.parseExpr(precAssign)
			left = &ast.AssignExpr{Target: left, Value: right, Coloneq: coloneq, Loc: assignLoc}
			continue
		}

		// Function application (juxtaposition) — highest precedence
		// after postfix. Same-line args always apply (offside rule).
		// Cross-line: allow parenthesized args, or any arg indented past the
		// callee (OCaml-style layout without full structural offside).
		sameLine := cur.Loc.Line == leftLine
		parenCont := !sameLine && cur.Type == token.LPAREN
		indentCont := !sameLine && cur.Loc.Column > baseCol && p.canStartExpr(cur)
		if (p.canStartExpr(cur) || cur.Type == token.QUESTION) && precApp >= minPrec && (sameLine || parenCont || indentCont) {
			appLoc := cur.Loc // location of the argument start
			arg := p.parsePrefix()
			left = &ast.AppExpr{Func: left, Arg: arg, Loc: appLoc}
			leftLine = p.tokens[p.pos-1].Loc.Line
			continue
		}

		// Binary operators
		prec := p.precedence(cur.Type)
		if prec == 0 || prec < minPrec {
			break
		}

		op := cur.Type
		p.advance()
		if op == token.PERCENT {
			p.errorf("PARSE-MIG018: '%%' is removed; use `mod`")
			op = token.MOD
		}

		nextMinPrec := prec
		if !p.isRightAssoc(op) {
			nextMinPrec = prec + 1
		}
		right := p.parseExpr(nextMinPrec)
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, Loc: cur.Loc}
	}

	return left
}

// applyPostfix applies field access (.) to an atom.  This ensures
// that `o.price` is parsed as a unit before juxtaposition.
// The `?` operator is NOT applied here because it binds to the full
// preceding expression, not just the atom.
func (p *Parser) applyPostfix(left ast.Expr) ast.Expr {
	for {
		switch p.cur().Type {
		case token.DOT:
			left = p.parseFieldAccessExpr(left)
		case token.HASH:
			left = p.parseMethodSendExpr(left)
		default:
			return left
		}
	}
}

// parsePrefix handles prefix expressions: atoms, unary ops, let-in, if,
// match, fun, function, while, try, raise, etc.
func (p *Parser) parsePrefix() ast.Expr {
	cur := p.cur()

	switch cur.Type {
	// --- literals ---
	case token.INT, token.FLOAT, token.STRING, token.CHAR:
		tok := p.advance()
		return &ast.LitExpr{Value: tok.Literal, Kind: tok.Type, Loc: tok.Loc}
	case token.TRUE:
		tok := p.advance()
		return &ast.LitExpr{Value: true, Kind: token.TRUE, Loc: tok.Loc}
	case token.FALSE:
		tok := p.advance()
		return &ast.LitExpr{Value: false, Kind: token.FALSE, Loc: tok.Loc}
	case token.NULL:
		p.advance()
		return &ast.NullExpr{}

	// --- polymorphic variants ---
	case token.POLYVAR:
		tok := p.advance()
		tag, _ := tok.Literal.(string)
		if tag == "" {
			tag = strings.TrimPrefix(tok.Lexeme, "`")
		}
		pe := &ast.PolyvarExpr{Tag: tag, Loc: tok.Loc}
		if p.canStartExpr(p.cur()) {
			pe.Arg = p.parsePrefix()
		}
		return p.applyPostfix(pe)

	// --- identifiers ---
	case token.IDENT:
		tok := p.advance()
		if (tok.Lexeme == "ptr_of" || tok.Lexeme == "is_null" || tok.Lexeme == "spread") && p.canStartExpr(p.cur()) {
			inner := p.parsePrefix()
			switch tok.Lexeme {
			case "ptr_of":
				return &ast.PtrOfExpr{Inner: inner}
			case "is_null":
				return &ast.IsNullExpr{Inner: inner}
			default:
				return &ast.SpreadExpr{Inner: inner}
			}
		}
		// Check for computation expression: builder { ... } (removed)
		if isBuilder(tok.Lexeme) && p.cur().Type == token.LBRACE {
			return p.parseCompExpr(tok.Loc, tok.Lexeme)
		}
		// Check for select { ... }
		if tok.Lexeme == "select" && p.cur().Type == token.LBRACE {
			return p.parseSelectExpr(tok.Loc)
		}
		return p.applyPostfix(&ast.IdentExpr{Name: tok.Lexeme, Loc: tok.Loc})

	// --- concurrency ---
	case token.GO:
		goTok := p.advance()
		var moved []string
		if p.cur().Type == token.LPAREN && p.peek().Type == token.MOVE {
			p.advance() // (
			p.advance() // move
			moved = append(moved, p.parseIdentName())
			for p.cur().Type == token.COMMA {
				p.advance()
				moved = append(moved, p.parseIdentName())
			}
			p.expect(token.RPAREN)
		}
		return &ast.GoExpr{Moved: moved, Expr: p.parseExpr(0), Loc: goTok.Loc}

	case token.USING:
		usingTok := p.advance()
		p.errorf("PARSE-MIG013: 'using' / region CE is removed; use `try`/`finally` or explicit cleanup")
		pat := p.parseSimplePattern()
		p.expect(token.EQUALS)
		expr := p.parseExpr(0)
		p.expect(token.IN)
		body := p.parseExpr(0)
		return &ast.UsingExpr{Pattern: pat, Expr: expr, Body: body, Loc: usingTok.Loc}

	case token.CONSTRUCTOR:
		tok := p.advance()
		typePrefix, name := p.parseQualifiedCtorName(tok)
		ce := &ast.ConstructorExpr{Name: name, TypePrefix: typePrefix, Loc: tok.Loc}
		if p.canStartExpr(p.cur()) {
			ce.Arg = p.parsePrefix()
		}
		return p.applyPostfix(ce)

	// --- imperative / OCaml surface ---
	case token.FOR:
		forTok := p.advance()
		varName := p.parseIdentName()
		p.expect(token.EQUALS)
		from := p.parseExpr(0)
		down := false
		if p.match(token.DOWNTO) {
			down = true
		} else {
			p.expect(token.TO)
		}
		to := p.parseExpr(0)
		p.expect(token.DO)
		body := p.parseExpr(0)
		p.expect(token.DONE)
		return &ast.ForExpr{Var: varName, From: from, To: to, Down: down, Body: body, Loc: forTok.Loc}

	case token.WHILE:
		whileTok := p.advance()
		cond := p.parseExpr(0)
		p.expect(token.DO)
		body := p.parseExpr(0)
		p.expect(token.DONE)
		return &ast.WhileExpr{Cond: cond, Body: body, Loc: whileTok.Loc}

	case token.BEGIN:
		beginTok := p.advance()
		stmts := []ast.Expr{p.parseExpr(0)}
		for p.match(token.SEMI) {
			if p.cur().Type == token.END {
				break
			}
			stmts = append(stmts, p.parseExpr(0))
		}
		p.expect(token.END)
		return &ast.BeginExpr{Stmts: stmts, Loc: beginTok.Loc}

	// --- grouped / tuple / list / array ---
	case token.LPAREN:
		return p.parseParenOrTupleExpr()
	case token.LBRACE:
		return p.parseRecordOrUpdateExpr()
	case token.LBRACKET:
		return p.parseListExpr()
	case token.LBRACKETPIPE:
		return p.parseArrayLitExpr()

	// --- keyword expressions ---
	case token.IF:
		return p.parseIfExpr()
	case token.MATCH:
		return p.parseMatchExpr()
	case token.TRY:
		return p.parseTryExpr()
	case token.LET:
		return p.parseLetInExpr()
	case token.FUN:
		return p.parseFunExpr()
	case token.FUNCTION:
		return p.parseFunctionExpr()
	case token.RAISE:
		raiseTok := p.advance()
		return &ast.RaiseExpr{Exn: p.parseExpr(precUnary), Loc: raiseTok.Loc}
	case token.FAILWITH:
		fwTok := p.advance()
		arg := p.parseExpr(precUnary)
		return &ast.AppExpr{
			Func: &ast.IdentExpr{Name: "failwith", Loc: fwTok.Loc},
			Arg:  arg,
			Loc:  fwTok.Loc,
		}
	case token.ASSERT:
		assertTok := p.advance()
		return &ast.AssertExpr{Cond: p.parseExpr(precUnary), Loc: assertTok.Loc}
	case token.LAZY:
		lazyTok := p.advance()
		return &ast.LazyExpr{Value: p.parseExpr(precUnary), Loc: lazyTok.Loc}
	case token.PERFORM:
		perfTok := p.advance()
		return &ast.PerformExpr{Op: p.parseExpr(precUnary), Loc: perfTok.Loc}
	case token.CONTINUE:
		contTok := p.advance()
		k := p.parseExpr(precUnary)
		arg := p.parseExpr(precUnary)
		return &ast.ContinueExpr{Cont: k, Arg: arg, Loc: contTok.Loc}
	case token.DISCONTINUE:
		discTok := p.advance()
		k := p.parseExpr(precUnary)
		exn := p.parseExpr(precUnary)
		return &ast.DiscontinueExpr{Cont: k, Exn: exn, Loc: discTok.Loc}
	case token.REF:
		refTok := p.advance()
		return &ast.RefExpr{Value: p.parseExpr(precUnary), Loc: refTok.Loc}
	case token.BANG:
		bangTok := p.advance()
		return &ast.DerefExpr{Target: p.parseExpr(precUnary), Loc: bangTok.Loc}
	case token.OBJECT:
		return p.parseObjectExpr()
	case token.NEW:
		return p.parseNewExpr()
	case token.TILDE:
		return p.parseLabelledArg()
	case token.QUESTION:
		return p.parseOptionalLabelledArg()
	case token.GUARD:
		return p.parseGuardExpr()
	case token.PANIC:
		panicTok := p.advance()
		p.errorf("PARSE-MIG017: 'panic' is removed; use `failwith` / `raise`")
		return &ast.RaiseExpr{Exn: p.parseExpr(precUnary), Loc: panicTok.Loc}

	// --- unary minus ---
	case token.MINUS:
		minusTok := p.advance()
		operand := p.parseExpr(precUnary)
		// Desugar `-x` as `0 - x`
		return &ast.BinaryExpr{
			Left:  &ast.LitExpr{Value: int64(0), Kind: token.INT},
			Op:    token.MINUS,
			Right: operand,
			Loc:   minusTok.Loc,
		}

	// --- logical not ---
	case token.NOT:
		notTok := p.advance()
		operand := p.parseExpr(precUnary)
		// Emit as `!operand` — represented as a special binary or unary.
		// For now, desugar to `if operand then false else true`
		return &ast.IfExpr{
			Cond:       operand,
			ThenBranch: &ast.LitExpr{Value: false, Kind: token.FALSE},
			ElseBranch: &ast.LitExpr{Value: true, Kind: token.TRUE},
			Loc:        notTok.Loc,
		}
	}

	p.errorf("unexpected token %s in expression", cur.Type)
	p.advance()
	return &ast.LitExpr{Value: nil, Kind: token.UNIT}
}

// ---------------------------------------------------------------------------
// Expression sub-parsers
// ---------------------------------------------------------------------------

func (p *Parser) parseParenOrTupleExpr() ast.Expr {
	lparenLoc := p.cur().Loc
	p.expect(token.LPAREN)
	// First-class modules: `(module M : S)` and `(val e : S)`.
	if p.match(token.MODULE) {
		module := p.parseQualifiedName()
		sig := ""
		if p.match(token.COLON) {
			sig = p.parseSignatureRef()
		}
		p.expect(token.RPAREN)
		return &ast.PackModuleExpr{Module: module, Sig: sig, Loc: lparenLoc}
	}
	if p.match(token.VAL) {
		value := p.parseExpr(0)
		sig := ""
		if p.match(token.COLON) {
			sig = p.parseSignatureRef()
		}
		p.expect(token.RPAREN)
		return &ast.UnpackModuleExpr{Value: value, Sig: sig, Loc: lparenLoc}
	}
	// Unit literal
	if p.match(token.RPAREN) {
		return &ast.LitExpr{Value: nil, Kind: token.UNIT, Loc: lparenLoc}
	}
	first := p.parseExpr(0)
	if p.match(token.RPAREN) {
		return &ast.ParenExpr{Inner: first, Loc: lparenLoc}
	}
	// Tuple
	elems := []ast.Expr{first}
	for p.match(token.COMMA) {
		elems = append(elems, p.parseExpr(0))
	}
	p.expect(token.RPAREN)
	return &ast.TupleExpr{Elems: elems, Loc: lparenLoc}
}

func (p *Parser) parseRecordOrUpdateExpr() ast.Expr {
	lbraceLoc := p.cur().Loc
	p.expect(token.LBRACE)

	// Save position for backtracking
	savePos := p.pos
	saveErrs := len(p.errs)

	// Try parsing an expression
	var firstExpr ast.Expr
	if p.canStartExpr(p.cur()) {
		firstExpr = p.parseExpr(0)
	}

	if p.match(token.WITH) {
		// Record update: { expr with field = value; ... }
		fields := p.parseRecordFields()
		p.expect(token.RBRACE)
		return &ast.RecordUpdateExpr{Base: firstExpr, Fields: fields, Loc: lbraceLoc}
	}

	// Not a record update.  Backtrack and parse as record literal.
	// We consumed an expression that is actually the first field name.
	// E.g. `{ id = 42; ... }` → parseExpr returned IdentExpr("id"),
	// and now the next token is `=`.
	//
	// `{ x; y }` → parseExpr returned IdentExpr("x"), next is `;` or `}`.

	p.pos = savePos
	p.errs = p.errs[:saveErrs]

	fields := p.parseRecordFields()
	p.expect(token.RBRACE)
	return &ast.RecordExpr{Fields: fields, Loc: lbraceLoc}
}

// parseRecordFields parses `name = expr` or `name` (punning), separated by `;`.
func (p *Parser) parseRecordFields() []ast.RecordField {
	var fields []ast.RecordField
	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		f := ast.RecordField{}
		tok := p.cur()
		if tok.Type != token.IDENT {
			p.errorf("expected field name, got %s", tok.Type)
			break
		}
		p.advance()
		f.Name = tok.Lexeme

		if p.match(token.EQUALS) {
			f.Value = p.parseExpr(0)
		}
		fields = append(fields, f)

		if !p.match(token.SEMI) {
			break
		}
	}
	return fields
}

func (p *Parser) parseListExpr() ast.Expr {
	lbrackLoc := p.cur().Loc
	p.expect(token.LBRACKET)
	if p.match(token.RBRACKET) {
		return &ast.ListExpr{Elems: nil, Loc: lbrackLoc}
	}
	var elems []ast.Expr
	elems = append(elems, p.parseExpr(0))
	for p.match(token.SEMI) {
		elems = append(elems, p.parseExpr(0))
	}
	p.expect(token.RBRACKET)
	return &ast.ListExpr{Elems: elems, Loc: lbrackLoc}
}

func (p *Parser) parseArrayLitExpr() ast.Expr {
	loc := p.cur().Loc
	p.expect(token.LBRACKETPIPE)
	if p.match(token.PIPERBRACKET) {
		return &ast.ArrayLitExpr{Elems: nil, Loc: loc}
	}
	var elems []ast.Expr
	elems = append(elems, p.parseExpr(0))
	for p.match(token.SEMI) {
		if p.cur().Type == token.PIPERBRACKET {
			break
		}
		elems = append(elems, p.parseExpr(0))
	}
	p.expect(token.PIPERBRACKET)
	return &ast.ArrayLitExpr{Elems: elems, Loc: loc}
}

func (p *Parser) parseIfExpr() ast.Expr {
	ifLoc := p.cur().Loc
	p.expect(token.IF)
	cond := p.parseExpr(0)
	p.expect(token.THEN)
	thenBranch := p.parseExpr(0)
	p.expect(token.ELSE)
	elseBranch := p.parseExpr(0)
	return &ast.IfExpr{Cond: cond, ThenBranch: thenBranch, ElseBranch: elseBranch, Loc: ifLoc}
}

func (p *Parser) parseMatchExpr() ast.Expr {
	matchLoc := p.cur().Loc
	p.expect(token.MATCH)
	scrutinee := p.parseExpr(0)
	p.expect(token.WITH)
	// Optional leading `|`
	p.match(token.PIPE)

	arms := p.parseMatchArms()
	return &ast.MatchExpr{Scrutinee: scrutinee, Arms: arms, Loc: matchLoc}
}

func (p *Parser) parseTryExpr() ast.Expr {
	tryLoc := p.cur().Loc
	p.expect(token.TRY)
	body := p.parseExpr(0)
	te := &ast.TryExpr{Body: body, Loc: tryLoc}
	if p.match(token.WITH) {
		p.match(token.PIPE)
		te.Arms = p.parseMatchArms()
	}
	if p.match(token.FINALLY) {
		te.Finally = p.parseExpr(0)
	}
	if te.Arms == nil && te.Finally == nil {
		p.errorf("expected 'with' or 'finally' after try body")
	}
	return te
}

func (p *Parser) parseMatchArms() []ast.MatchArm {
	var arms []ast.MatchArm
	for p.cur().Type != token.EOF && (p.canStartPattern(p.cur()) || p.cur().Type == token.EFFECT) {
		arm := ast.MatchArm{}

		// Effect handler: `effect (E x) k ->` or `effect E k ->`
		if p.cur().Type == token.EFFECT {
			p.advance()
			arm.EffectHandler = true
			if p.match(token.LPAREN) {
				arm.Pattern = p.parsePattern()
				p.expect(token.RPAREN)
			} else {
				arm.Pattern = p.parsePattern()
			}
			tok := p.cur()
			if tok.Type == token.IDENT {
				p.advance()
				arm.ContName = tok.Lexeme
			} else {
				p.errorf("expected continuation name after effect pattern, got %s", tok.Type)
			}
		} else {
			arm.Pattern = p.parseOrPattern()
		}

		if p.match(token.WHEN) {
			arm.Guard = p.parseExpr(0)
		}
		p.expect(token.ARROW)
		arm.Body = p.parseExpr(0)
		arms = append(arms, arm)

		if !p.match(token.PIPE) {
			break
		}
		// Allow trailing pipe at EOF or dedent
		if !p.canStartPattern(p.cur()) && p.cur().Type != token.PIPE && p.cur().Type != token.EFFECT {
			break
		}
	}
	return arms
}

// parseOrPattern parses `p | q | r` or-patterns.
func (p *Parser) parseOrPattern() ast.Pattern {
	left := p.parsePattern()
	for p.cur().Type == token.PIPE && p.canStartPattern(p.peek()) {
		p.advance() // |
		right := p.parsePattern()
		left = &ast.OrPattern{Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseLetInExpr() ast.Expr {
	letLoc := p.cur().Loc
	// `let module M = struct ... end in expr`
	if p.peek().Type == token.MODULE {
		return p.parseLetModuleExpr()
	}
	// `let open[!] M in expr`
	if p.peek().Type == token.OPEN {
		p.advance() // let
		p.expect(token.OPEN)
		force := p.match(token.BANG)
		path := p.parseQualifiedName()
		p.match(token.IN)
		body := p.parseExpr(0)
		return &ast.LetOpenExpr{Path: path, Force: force, Body: body, Loc: letLoc}
	}
	decl := p.parseLetDecl(false)
	// `in` is optional — when absent the offside rule treats the next
	// expression at the same or greater indentation as the body.
	p.match(token.IN)
	body := p.parseExpr(0)
	return &ast.LetInExpr{Mutable: decl.Mutable, Bindings: decl.Bindings, Body: body, Loc: letLoc}
}

func (p *Parser) parseFunExpr() ast.Expr {
	funLoc := p.cur().Loc
	p.expect(token.FUN)
	params := p.parseParams()
	p.expect(token.ARROW)
	body := p.parseExpr(0)
	return &ast.FunExpr{Params: params, Body: body, Loc: funLoc}
}

func (p *Parser) parseFunctionExpr() ast.Expr {
	funLoc := p.cur().Loc
	p.expect(token.FUNCTION)
	p.match(token.PIPE)
	arms := p.parseMatchArms()
	return &ast.FunctionExpr{Arms: arms, Loc: funLoc}
}

func (p *Parser) parseGuardExpr() ast.Expr {
	guardLoc := p.cur().Loc
	p.expect(token.GUARD)
	p.errorf("PARSE-MIG014: 'guard' / 'is' / expression 'as' macros are removed; use `match`")
	ge := &ast.GuardExpr{Loc: guardLoc}
	// Parse first binding
	pat := p.parsePattern()
	p.expect(token.EQUALS)
	rh := p.parseExpr(0)
	ge.Bindings = append(ge.Bindings, ast.GuardBinding{Pattern: pat, Expr: rh})
	// Parse additional `and` bindings
	for p.match(token.AND) {
		pat2 := p.parsePattern()
		p.expect(token.EQUALS)
		rh2 := p.parseExpr(0)
		ge.Bindings = append(ge.Bindings, ast.GuardBinding{Pattern: pat2, Expr: rh2})
	}
	p.expect(token.ELSE)
	ge.Else_ = p.parseExpr(0)
	return ge
}

// Postfix / special operators

func (p *Parser) parseQuestionExpr(left ast.Expr) ast.Expr {
	qLoc := p.cur().Loc
	qLine := qLoc.Line // line of the `?` token
	p.advance()        // consume ?
	p.errorf("PARSE-MIG012: '?' error propagation is removed; use `match` on `result`")
	qe := &ast.QuestionExpr{Left: left, Loc: qLoc}
	// Optional argument: only when on the same line (offside rule).
	// Valid forms:  expr ?              (bare)
	//               expr ? "message"    (string)
	//               expr ? fun e -> ... (transform)
	if (p.cur().Type == token.STRING || p.cur().Type == token.FUN) && p.cur().Loc.Line == qLine {
		qe.Arg = p.parseExpr(precPostfix)
	}
	return qe
}

func (p *Parser) parseFieldAccessExpr(left ast.Expr) ast.Expr {
	dotLoc := p.cur().Loc
	p.advance() // consume .
	if p.cur().Type == token.LPAREN {
		p.advance()
		// Module local open: M.( expr ) when left is a capitalized path.
		if path, ok := modulePathOf(left); ok {
			body := p.parseExpr(0)
			p.expect(token.RPAREN)
			return &ast.LocalOpenExpr{Path: path, Body: body, Loc: dotLoc}
		}
		idx := p.parseExpr(0)
		p.expect(token.RPAREN)
		return &ast.IndexExpr{Target: left, Index: idx, Loc: dotLoc}
	}
	tok := p.cur()
	if tok.Type == token.IDENT || tok.Type == token.CONSTRUCTOR {
		p.advance()
		return &ast.FieldAccessExpr{Left: left, Field: tok.Lexeme, Loc: dotLoc}
	}
	p.errorf("expected field name or index after '.', got %s", tok.Type)
	return left
}

func (p *Parser) parseMethodSendExpr(target ast.Expr) ast.Expr {
	loc := p.cur().Loc
	p.expect(token.HASH)
	tok := p.cur()
	if tok.Type != token.IDENT && tok.Type != token.CONSTRUCTOR {
		p.errorf("expected method name after #, got %s", tok.Type)
		return target
	}
	p.advance()
	return &ast.MethodSendExpr{Target: target, Method: tok.Lexeme, Loc: loc}
}

// modulePathOf returns a module path if e looks like a capitalized module reference.
func modulePathOf(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.ConstructorExpr:
		if e.Arg == nil && e.Name != "" && e.Name[0] >= 'A' && e.Name[0] <= 'Z' {
			if e.TypePrefix != "" {
				return e.TypePrefix + "." + e.Name, true
			}
			return e.Name, true
		}
	case *ast.IdentExpr:
		if e.Name != "" && e.Name[0] >= 'A' && e.Name[0] <= 'Z' {
			return e.Name, true
		}
	case *ast.FieldAccessExpr:
		if base, ok := modulePathOf(e.Left); ok {
			return base + "." + e.Field, true
		}
	}
	return "", false
}

// parseQualifiedCtorName parses Type.Ctor after the first constructor token was consumed.
func (p *Parser) parseQualifiedCtorName(first token.Token) (typePrefix, name string) {
	name = first.Lexeme
	if p.cur().Type == token.DOT && p.peek().Type == token.CONSTRUCTOR {
		p.advance() // .
		typePrefix = name
		ctorTok := p.advance()
		name = ctorTok.Lexeme
	}
	return typePrefix, name
}

func (p *Parser) parseIsExpr(left ast.Expr) ast.Expr {
	isLoc := p.cur().Loc
	p.advance() // consume is
	p.errorf("PARSE-MIG014: 'guard' / 'is' / expression 'as' macros are removed; use `match`")
	pat := p.parsePattern()
	return &ast.IsExpr{Left: left, Pattern: pat, Loc: isLoc}
}

func (p *Parser) parseAsMatchExpr(left ast.Expr) ast.Expr {
	asLoc := p.cur().Loc
	p.advance() // consume as
	p.errorf("PARSE-MIG014: 'guard' / 'is' / expression 'as' macros are removed; use `match`")
	pat := p.parsePattern()
	p.expect(token.ARROW)
	body := p.parseExpr(4)
	p.expect(token.ELSE)
	elseBody := p.parseExpr(4)
	return &ast.AsMatchExpr{Left: left, Pattern: pat, Body: body, ElseBody: elseBody, Loc: asLoc}
}

// ---------------------------------------------------------------------------
// Pattern parsing
// ---------------------------------------------------------------------------

// parsePattern parses a full pattern, including `::` (cons) and `as` (alias).
func (p *Parser) parsePattern() ast.Pattern {
	left := p.parseSimplePattern()

	// `::` cons pattern (right-associative)
	if p.cur().Type == token.CONS {
		p.advance()
		right := p.parsePattern()
		return &ast.ConsPattern{Head: left, Tail: right}
	}

	// `as` alias pattern
	if p.match(token.AS) {
		tok := p.cur()
		if tok.Type == token.IDENT {
			p.advance()
			return &ast.AliasPattern{Pattern: left, Name: tok.Lexeme}
		}
		p.errorf("expected identifier after 'as', got %s", tok.Type)
	}

	return left
}

// parseSimplePattern parses a pattern atom, including constructor application.
func (p *Parser) parseSimplePattern() ast.Pattern {
	cur := p.cur()
	switch cur.Type {
	case token.UNDERSCORE:
		p.advance()
		return &ast.WildcardPattern{}

	case token.IDENT:
		tok := p.advance()
		return &ast.IdentPattern{Name: tok.Lexeme}

	case token.CONSTRUCTOR:
		tok := p.advance()
		typePrefix, name := p.parseQualifiedCtorName(tok)
		cp := &ast.ConstructorPattern{Name: name, TypePrefix: typePrefix}
		if p.canStartPattern(p.cur()) {
			cp.Arg = p.parseSimplePattern()
		}
		return cp

	case token.POLYVAR:
		tok := p.advance()
		tag, _ := tok.Literal.(string)
		pp := &ast.PolyvarPattern{Tag: tag}
		if p.canStartPattern(p.cur()) {
			pp.Arg = p.parseSimplePattern()
		}
		return pp

	case token.EXCEPTION:
		p.advance()
		return &ast.ExceptionPattern{Pattern: p.parseSimplePattern()}

	case token.LAZY:
		p.advance()
		return &ast.LazyPattern{Pattern: p.parseSimplePattern()}

	case token.INT, token.FLOAT, token.STRING, token.CHAR, token.TRUE, token.FALSE:
		tok := p.advance()
		return &ast.LitPattern{Value: tok.Literal, Kind: tok.Type}

	case token.LPAREN:
		return p.parseParenOrTuplePattern()
	case token.LBRACE:
		return p.parseRecordPattern()
	case token.LBRACKET:
		return p.parseListPattern()

	default:
		p.errorf("unexpected token %s in pattern", cur.Type)
		p.advance()
		return &ast.WildcardPattern{}
	}
}

func (p *Parser) parseParenOrTuplePattern() ast.Pattern {
	p.expect(token.LPAREN)
	first := p.parsePattern()
	if p.match(token.RPAREN) {
		return first // parenthesized
	}
	// Tuple pattern
	elems := []ast.Pattern{first}
	for p.match(token.COMMA) {
		elems = append(elems, p.parsePattern())
	}
	p.expect(token.RPAREN)
	return &ast.TuplePattern{Elems: elems}
}

func (p *Parser) parseRecordPattern() ast.Pattern {
	p.expect(token.LBRACE)
	rp := &ast.RecordPattern{}
	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		f := ast.RecordPatField{}
		tok := p.cur()
		if tok.Type == token.IDENT {
			p.advance()
			f.Name = tok.Lexeme
		} else {
			p.errorf("expected field name in record pattern, got %s", tok.Type)
			break
		}
		if p.match(token.EQUALS) {
			f.Pattern = p.parsePattern()
		}
		rp.Fields = append(rp.Fields, f)
		if !p.match(token.SEMI) {
			break
		}
	}
	p.expect(token.RBRACE)
	return rp
}

func (p *Parser) parseListPattern() ast.Pattern {
	p.expect(token.LBRACKET)
	if p.match(token.RBRACKET) {
		return &ast.ListPattern{Elems: nil}
	}
	var elems []ast.Pattern
	elems = append(elems, p.parsePattern())
	for p.match(token.SEMI) {
		elems = append(elems, p.parsePattern())
	}
	p.expect(token.RBRACKET)
	return &ast.ListPattern{Elems: elems}
}

// ---------------------------------------------------------------------------
// Type parsing
// ---------------------------------------------------------------------------

// parseType parses a type expression.
// Arrow `->` is right-associative and lowest precedence.
// Effect rows `with { … }` after function types are rejected (PARSE-MIG016).
func (p *Parser) parseType() ast.Type {
	left := p.parseTupleType()
	if p.match(token.ARROW) {
		right := p.parseType() // right-associative
		fun := &ast.TFun{From: left, To: right}
		// Effect rows removed: emit migration error and discard.
		if p.cur().Type == token.WITH && p.peek().Type == token.LBRACE {
			p.errorf("PARSE-MIG016: effect row `with { … }` is removed; use effect handlers")
			_ = p.parseEffectRow()
		}
		return fun
	}
	return left
}

// parseEffectRow parses `with { io; log; e | .. }`.
func (p *Parser) parseEffectRow() *ast.EffectRowType {
	p.expect(token.WITH)
	p.expect(token.LBRACE)

	row := &ast.EffectRowType{}

	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		// Check for row extension: `| ..` or `e | ..`
		if p.cur().Type == token.PIPE {
			p.advance()
			if p.cur().Type == token.DOT && p.peek().Type == token.DOT {
				p.advance() // first .
				p.advance() // second .
				row.Open = true
				break
			}
			p.errorf("expected '..' after '|' in effect row")
			break
		}

		tok := p.cur()
		if tok.Type == token.IDENT || tok.Type == token.TYVAR {
			p.advance()
			// Check if this is a row variable before `|`
			if p.cur().Type == token.PIPE {
				row.Rest = tok.Lexeme
				row.Open = true
				// Expect pipe and `..`
				// Already consumed; the next iteration will handle it
				continue
			}
			row.Effects = append(row.Effects, tok.Lexeme)
		} else {
			p.errorf("expected effect name or type variable in effect row, got %s", tok.Type)
			break
		}

		if !p.match(token.SEMI) {
			break
		}
	}

	p.expect(token.RBRACE)
	return row
}

// parseTupleType parses `A * B * C`.
func (p *Parser) parseTupleType() ast.Type {
	left := p.parseAppType()
	for p.match(token.STAR) {
		right := p.parseAppType()
		if tup, ok := left.(*ast.TTuple); ok {
			tup.Elems = append(tup.Elems, right)
			left = tup
		} else {
			left = &ast.TTuple{Elems: []ast.Type{left, right}}
		}
	}
	return left
}

// parseAppType parses type application: `A B` where B is the type constructor
// and A is the argument.  E.g. `order list` means `list(order)`.
//
// In ML syntax, the LAST type name is the constructor and everything before
// is the argument (or a tuple of arguments).  We accumulate primaries and
// fold them: [A, B, C] → App(C, App(B, A)).
func (p *Parser) parseAppType() ast.Type {
	var primaries []ast.Type
	primaries = append(primaries, p.parsePrimaryType())
	for p.canStartType(p.cur()) {
		if p.cur().Type == token.IDENT && (p.cur().Lexeme == "ptr" || p.cur().Lexeme == "go_slice") {
			break
		}
		// Peek ahead to avoid consuming `=` or `*` or `->` as type atoms.
		// A type variable 'a followed by `=` starts a type binding, not
		// type application.
		next := p.peek()
		if p.cur().Type == token.TYVAR && (next.Type == token.EQUALS || next.Type == token.EOF || next.Type == token.PIPE) {
			break
		}
		// Stop if the next token is a type delimiter
		if next.Type == token.EQUALS || next.Type == token.ARROW ||
			next.Type == token.RBRACE || next.Type == token.RPAREN ||
			next.Type == token.PIPE || next.Type == token.SEMI {
			// But keep going if the current token is the last primary
			primaries = append(primaries, p.parsePrimaryType())
			break
		}
		primaries = append(primaries, p.parsePrimaryType())
	}

	// Fold: [A, B, C] → App(C, App(B, A))
	result := primaries[0]
	for i := 1; i < len(primaries); i++ {
		result = &ast.TApp{Func: primaries[i], Arg: result}
	}
	// Postfix `chan`: `int chan` → TChan(Elem=int)
	if p.cur().Type == token.CHAN {
		p.advance()
		return &ast.TChan{Elem: result}
	}
	// Postfix `ref`: `int ref` → TApp(ref, int)
	if p.cur().Type == token.REF {
		p.advance()
		return &ast.TApp{Func: &ast.TIdent{Name: "ref"}, Arg: result}
	}
	if p.cur().Type == token.IDENT && p.cur().Lexeme == "ptr" {
		p.advance()
		return &ast.TPtr{Elem: result}
	}
	if p.cur().Type == token.IDENT && p.cur().Lexeme == "go_slice" {
		p.advance()
		return &ast.TGoSlice{Elem: result}
	}
	// Postfix `where`: `int where x > 0` → RefinementType{Inner: int, Pred: x > 0}
	if p.match(token.WHERE) {
		p.wherePred = true
		pred := p.parseExpr(0)
		p.wherePred = false
		return &ast.RefinementType{Inner: result, Pred: pred}
	}
	return result
}

// parsePrimaryType parses a type atom.
func (p *Parser) parsePrimaryType() ast.Type {
	cur := p.cur()
	switch cur.Type {
	case token.ELLIPSIS:
		p.advance()
		return &ast.TVariadic{Elem: p.parsePrimaryType()}
	case token.IDENT, token.CONSTRUCTOR:
		// map[K] V — prefix form (LBRACKET alone is poly-variant).
		if cur.Lexeme == "map" && p.peek().Type == token.LBRACKET {
			p.advance() // map
			p.expect(token.LBRACKET)
			key := p.parseType()
			p.expect(token.RBRACKET)
			val := p.parseAppType()
			return &ast.TMap{Key: key, Val: val}
		}
		tok := p.advance()
		name := tok.Lexeme
		// Qualified Go names: fs.FileMode, io.Reader, time.Time
		for p.cur().Type == token.DOT {
			p.advance()
			next := p.cur()
			if next.Type == token.IDENT || next.Type == token.CONSTRUCTOR {
				p.advance()
				name += "." + next.Lexeme
			} else {
				p.errorf("expected type name after '.', got %s", next.Type)
				break
			}
		}
		return &ast.TIdent{Name: name}
	case token.TYVAR:
		tok := p.advance()
		return &ast.TVar{Name: tok.Lexeme}
	case token.LPAREN:
		p.advance()
		if p.match(token.RPAREN) {
			return &ast.TIdent{Name: "unit"}
		}
		inner := p.parseType()
		if p.match(token.RPAREN) {
			return inner // parenthesized
		}
		// Tuple type
		elems := []ast.Type{inner}
		for p.match(token.COMMA) {
			elems = append(elems, p.parseType())
		}
		p.expect(token.RPAREN)
		return &ast.TTuple{Elems: elems}
	case token.LBRACE:
		p.advance()
		rt := &ast.TRecord{}
		for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
			// Check for row extension: | ..
			if p.cur().Type == token.PIPE {
				p.advance()
				if p.cur().Type == token.DOT && p.peek().Type == token.DOT {
					p.advance() // first .
					p.advance() // second .
					rt.Open = true
					break
				}
				p.errorf("expected '..' after '|' in record type")
				break
			}
			ft := ast.FieldType{}
			if p.match(token.MUTABLE) {
				ft.Mutable = true
			}
			tok := p.cur()
			if tok.Type == token.IDENT {
				p.advance()
				ft.Name = tok.Lexeme
			}
			p.expect(token.COLON)
			ft.Type = p.parseType()
			rt.Fields = append(rt.Fields, ft)
			if !p.match(token.SEMI) {
				break
			}
		}
		// Check for row extension: | ..
		if p.cur().Type == token.PIPE {
			p.advance()
			if p.cur().Type == token.DOT && p.peek().Type == token.DOT {
				p.advance()
				p.advance()
				rt.Open = true
			} else {
				p.errorf("expected '..' after '|' in record type")
			}
		}
		p.expect(token.RBRACE)
		return rt
	case token.LT:
		p.advance()
		ot := &ast.TObject{}
		for p.cur().Type != token.GT && p.cur().Type != token.EOF {
			if p.match(token.PIPE) {
				if p.cur().Type == token.DOT && p.peek().Type == token.DOT {
					p.advance()
					p.advance()
					ot.Open = true
					break
				}
				p.errorf("expected '..' after '|' in object type")
				break
			}
			tok := p.cur()
			if tok.Type != token.IDENT && tok.Type != token.CONSTRUCTOR {
				p.errorf("expected method name in object type, got %s", tok.Type)
				break
			}
			p.advance()
			p.expect(token.COLON)
			ot.Methods = append(ot.Methods, ast.FieldType{Name: tok.Lexeme, Type: p.parseType()})
			if !p.match(token.SEMI) {
				break
			}
		}
		p.expect(token.GT)
		return ot
	case token.LBRACKET:
		p.advance()
		pt := &ast.TPolyVariant{}
		if p.match(token.GT) {
			pt.Open = true
		} else if p.match(token.LT) {
			pt.UpperBound = true
		}
		p.match(token.PIPE)
		for p.cur().Type == token.POLYVAR {
			tok := p.advance()
			tag, _ := tok.Literal.(string)
			cs := ast.ADTCase{Name: tag}
			if p.match(token.OF) {
				cs.Arg = p.parseType()
			}
			pt.Cases = append(pt.Cases, cs)
			if !p.match(token.PIPE) {
				break
			}
		}
		p.expect(token.RBRACKET)
		return pt
	default:
		p.errorf("unexpected token %s in type", cur.Type)
		p.advance()
		return &ast.TIdent{Name: "<error>"}
	}
}

// isBuilder reports whether a name is a known computation expression builder.
func isBuilder(name string) bool {
	switch name {
	case "result", "async", "region":
		return true
	}
	return false
}

// parseSelectExpr parses `select { case x = expr -> body ... default -> body }`.
func (p *Parser) parseSelectExpr(loc token.SourceLoc) ast.Expr {
	p.expect(token.LBRACE)
	se := &ast.SelectExpr{Loc: loc}
	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		cur := p.cur()
		if cur.Type == token.IDENT && cur.Lexeme == "case" {
			p.advance() // consume "case"
			c := ast.SelectCase{}
			if p.cur().Type == token.IDENT && p.peek().Type == token.EQUALS {
				c.Bind = p.advance().Lexeme
				p.advance() // =
			}
			c.Recv = p.parseExpr(0)
			p.expect(token.ARROW)
			c.Body = p.parseExpr(0)
			se.Cases = append(se.Cases, c)
		} else if cur.Type == token.IDENT && cur.Lexeme == "default" {
			p.advance() // consume "default"
			p.expect(token.ARROW)
			se.Default = p.parseExpr(0)
			break
		} else {
			break
		}
	}
	p.expect(token.RBRACE)
	return se
}

// parseCompExpr parses `builder { ops }` where builder was already consumed.
// Computation expressions are removed in 1.0 (PARSE-MIG013); still parsed for recovery.
func (p *Parser) parseCompExpr(loc token.SourceLoc, builder string) ast.Expr {
	p.errorf("PARSE-MIG013: '%s { … }' computation expressions are removed; use `match` / `try`/`finally`", builder)
	p.expect(token.LBRACE)
	ce := &ast.CompExpr{Builder: builder, Loc: loc}

	for p.cur().Type != token.RBRACE && p.cur().Type != token.EOF {
		op := p.parseCompOp()
		if op != nil {
			ce.Ops = append(ce.Ops, op)
		}
		// Optional semicolon or newline separator
		p.match(token.SEMI)
	}
	p.expect(token.RBRACE)
	return ce
}

// parseCompOp parses one operation inside a computation expression.
func (p *Parser) parseCompOp() ast.CompOp {
	switch {
	case p.cur().Type == token.LET && p.peek().Type == token.BANG:
		// let! pattern = expr
		p.advance() // let
		p.advance() // !
		pat := p.parsePattern()
		p.expect(token.EQUALS)
		expr := p.parseExpr(0)
		return &ast.LetBangOp{Pattern: pat, Expr: expr}

	case p.cur().Type == token.LET:
		// let pattern = expr
		p.advance()
		pat := p.parsePattern()
		p.expect(token.EQUALS)
		expr := p.parseExpr(0)
		return &ast.LetOp{Pattern: pat, Expr: expr}

	case (p.cur().Type == token.IDENT && p.cur().Lexeme == "do") || p.cur().Type == token.DO:
		if p.peek().Type == token.BANG {
			// do! expr
			p.advance() // do
			p.advance() // !
			expr := p.parseExpr(0)
			return &ast.DoBangOp{Expr: expr}
		}
		p.errorf("unexpected bare 'do' in computation expression; use 'do!'")
		return &ast.BodyOp{Expr: &ast.LitExpr{Value: nil, Kind: token.UNIT}}

	case p.cur().Type == token.IDENT && p.cur().Lexeme == "return" && p.peek().Type == token.BANG:
		// return! expr
		p.advance() // return
		p.advance() // !
		expr := p.parseExpr(0)
		return &ast.ReturnBangOp{Expr: expr}

	case p.cur().Type == token.IDENT && p.cur().Lexeme == "return":
		// return expr
		p.advance()
		expr := p.parseExpr(0)
		return &ast.ReturnOp{Expr: expr}

	default:
		// Body expression
		expr := p.parseExpr(0)
		return &ast.BodyOp{Expr: expr}
	}
}
