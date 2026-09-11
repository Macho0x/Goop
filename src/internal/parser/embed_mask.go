package parser

import "bytes"

// maskLangEmbedBodies replaces the interior of @[go]/@[c] { ... } blocks with
// spaces (newlines preserved) so the Goop lexer does not interpret Go/C syntax.
// Offsets stay stable for Loc mapping. The parser still reads embed bodies from
// the original source via readRawGoBlock.
func maskLangEmbedBodies(src []byte) []byte {
	out := bytes.Clone(src)
	i := 0
	for i < len(src) {
		start, langEnd := findLangEmbedOpen(src, i)
		if start < 0 {
			break
		}
		// Find '{' after @[lang]
		j := langEnd
		for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
			j++
		}
		if j >= len(src) || src[j] != '{' {
			i = langEnd
			continue
		}
		closeAt := matchRawBrace(src, j)
		if closeAt < 0 {
			break
		}
		// Mask interior (j+1 .. closeAt-1), keep braces and newlines.
		for k := j + 1; k < closeAt; k++ {
			if src[k] != '\n' && src[k] != '\r' {
				out[k] = ' '
			}
		}
		i = closeAt + 1
	}
	return out
}

// findLangEmbedOpen finds @[go] or @[c] starting at or after from.
// Returns start index of '@' and index just past ']'.
func findLangEmbedOpen(src []byte, from int) (start, afterBracket int) {
	for i := from; i < len(src); i++ {
		if src[i] != '@' {
			continue
		}
		if i+1 >= len(src) || src[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
		lang := ""
		if j+2 <= len(src) && string(src[j:j+2]) == "go" && (j+2 == len(src) || !isIdentByte(src[j+2])) {
			lang = "go"
			j += 2
		} else if j+1 <= len(src) && src[j] == 'c' && (j+1 == len(src) || !isIdentByte(src[j+1])) {
			lang = "c"
			j++
		} else {
			continue
		}
		_ = lang
		for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
		if j < len(src) && src[j] == ']' {
			return i, j + 1
		}
	}
	return -1, -1
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// matchRawBrace finds the closing '}' matching src[open] == '{', skipping
// Go strings and // /* */ comments (same rules as readRawGoBlock).
func matchRawBrace(src []byte, open int) int {
	if open >= len(src) || src[open] != '{' {
		return -1
	}
	pos := open + 1
	depth := 1
	for pos < len(src) && depth > 0 {
		switch src[pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return pos
			}
		case '/':
			if pos+1 < len(src) && src[pos+1] == '/' {
				pos += 2
				for pos < len(src) && src[pos] != '\n' {
					pos++
				}
				continue
			}
			if pos+1 < len(src) && src[pos+1] == '*' {
				pos += 2
				for pos+1 < len(src) && !(src[pos] == '*' && src[pos+1] == '/') {
					pos++
				}
				pos += 2
				continue
			}
		case '"':
			pos++
			for pos < len(src) && src[pos] != '"' {
				if src[pos] == '\\' {
					pos++
				}
				pos++
			}
		case '\'':
			pos++
			for pos < len(src) && src[pos] != '\'' {
				if src[pos] == '\\' {
					pos++
				}
				pos++
			}
		case '`':
			pos++
			for pos < len(src) && src[pos] != '`' {
				pos++
			}
		}
		pos++
	}
	return -1
}
