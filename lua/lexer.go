// SPDX-License-Identifier: AGPL-3.0-or-later

package lua

import (
	"bytes"
	"fmt"
	"regexp"
)

type tokKind uint8

const (
	tokEOF tokKind = iota
	tokErr
	tokIdent
	tokString
	tokNumber
	tokLBrace
	tokRBrace
	tokLBrack
	tokRBrack
	tokEq
	tokSep
)

type token struct {
	kind tokKind
	text string // identifier, string contents, number text, or the error message
	line int
}

func (t token) String() string {
	switch t.kind {
	case tokEOF:
		return "end of input"
	case tokErr:
		return t.text
	case tokString:
		return fmt.Sprintf("string %q", t.text)
	default:
		return fmt.Sprintf("%q", t.text)
	}
}

// numberText is what Lua's %.14g output and the Windows runtime's inf and
// nan spellings look like.
var numberText = regexp.MustCompile(`^-?(?:[0-9]+(?:\.[0-9]+)?(?:[eE][-+]?[0-9]+)?|inf|nan(?:\(ind\))?)$`)

func single(c byte) (tokKind, bool) {
	switch c {
	case '{':
		return tokLBrace, true
	case '}':
		return tokRBrace, true
	case '[':
		return tokLBrack, true
	case ']':
		return tokRBrack, true
	case '=':
		return tokEq, true
	case ',', ';':
		return tokSep, true
	}
	return 0, false
}

type lexer struct {
	src  []byte
	pos  int
	line int
}

func (lx *lexer) next() token {
	lx.skipSpace()
	line := lx.line
	if lx.pos >= len(lx.src) {
		return token{kind: tokEOF, line: line}
	}
	c := lx.src[lx.pos]
	switch k, ok := single(c); {
	case ok:
		lx.pos++
		return token{kind: k, text: string(c), line: line}
	case c == '"':
		return lx.str(lx.pos+1, line)
	case c == 'L' && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] == '"':
		return lx.str(lx.pos+2, line)
	case c == '-' || isDigit(c):
		return lx.number(line)
	case isIdentStart(c):
		start := lx.pos
		for lx.pos < len(lx.src) && isIdentPart(lx.src[lx.pos]) {
			lx.pos++
		}
		return token{kind: tokIdent, text: string(lx.src[start:lx.pos]), line: line}
	}
	return token{kind: tokErr, text: fmt.Sprintf("unexpected byte %#02x", c), line: line}
}

func (lx *lexer) skipSpace() {
	for lx.pos < len(lx.src) {
		switch c := lx.src[lx.pos]; {
		case c == '\n':
			lx.line++
			lx.pos++
		case c == ' ' || c == '\t' || c == '\r':
			lx.pos++
		case c == '-' && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] == '-':
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.pos++
			}
		default:
			return
		}
	}
}

// str reads a string up to the next quote. The game writes strings without
// escaping, so backslashes are kept as they are.
func (lx *lexer) str(start, line int) token {
	end := bytes.IndexByte(lx.src[start:], '"')
	if end < 0 {
		lx.pos = len(lx.src)
		return token{kind: tokErr, text: "unterminated string", line: line}
	}
	text := lx.src[start : start+end]
	lx.line += bytes.Count(text, []byte{'\n'})
	lx.pos = start + end + 1
	return token{kind: tokString, text: string(text), line: line}
}

// number reads the longest run that could be part of a number and then
// checks it against numberText.
func (lx *lexer) number(line int) token {
	start := lx.pos
	lx.pos++
	for lx.pos < len(lx.src) && numberPart(lx.src[lx.pos-1], lx.src[lx.pos]) {
		lx.pos++
	}
	text := string(lx.src[start:lx.pos])
	if !numberText.MatchString(text) {
		return token{kind: tokErr, text: fmt.Sprintf("bad number %q", text), line: line}
	}
	return token{kind: tokNumber, text: text, line: line}
}

// numberPart reports whether c continues a number after prev. A sign only
// continues one as an exponent sign.
func numberPart(prev, c byte) bool {
	switch c {
	case '.', '(', ')':
		return true
	case '+', '-':
		return prev == 'e' || prev == 'E'
	}
	return isIdentPart(c)
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }
