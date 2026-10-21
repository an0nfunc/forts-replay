// SPDX-License-Identifier: AGPL-3.0-or-later

package lua

import (
	"fmt"
	"strconv"
)

// Parse parses a chunk of `name = value` statements, as the game writes
// them, into a root table holding one entry per statement.
func Parse(src []byte, lim Limits) (*Table, error) {
	lim = lim.WithDefaults()
	if len(src) > lim.MaxSize {
		return nil, fmt.Errorf("%w: input of %d bytes above %d", ErrLimit, len(src), lim.MaxSize)
	}
	p := &parser{lx: lexer{src: src, line: 1}, lim: lim}
	p.ahead = p.lx.next()
	p.next()
	root := &Table{}
	for p.tok.kind != tokEOF {
		if p.tok.kind != tokIdent {
			return nil, p.errorf("expected a name, got %s", p.tok)
		}
		name := p.tok.text
		p.next()
		if p.tok.kind != tokEq {
			return nil, p.errorf("expected '=', got %s", p.tok)
		}
		p.next()
		v, err := p.value(0)
		if err != nil {
			return nil, err
		}
		root.Entries = append(root.Entries, Entry{Key: Key{Name: name}, Value: v})
		if p.tok.kind == tokSep {
			p.next()
		}
	}
	return root, nil
}

type parser struct {
	lx         lexer
	lim        Limits
	tok, ahead token
}

func (p *parser) next() {
	p.tok = p.ahead
	if p.tok.kind != tokEOF && p.tok.kind != tokErr {
		p.ahead = p.lx.next()
	}
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("%w: line %d: %s", ErrSyntax, p.tok.line, fmt.Sprintf(format, args...))
}

// value parses a value inside a table nested depth levels deep.
func (p *parser) value(depth int) (Value, error) {
	t := p.tok
	switch t.kind {
	case tokLBrace:
		tbl, err := p.table(depth + 1)
		return Value{Kind: KindTable, Table: tbl}, err
	case tokString:
		p.next()
		return Value{Kind: KindString, Str: t.text}, nil
	case tokNumber:
		p.next()
		return Value{Kind: KindNumber, Str: t.text}, nil
	case tokIdent:
		switch t.text {
		case "true", "false":
			p.next()
			return Value{Kind: KindBool, Bool: t.text == "true"}, nil
		case "inf", "nan":
			p.next()
			return Value{Kind: KindNumber, Str: t.text}, nil
		}
		fallthrough
	default:
		return Value{}, p.errorf("unexpected %s", t)
	}
}

func (p *parser) table(depth int) (*Table, error) {
	if depth > p.lim.MaxDepth {
		return nil, fmt.Errorf("%w: line %d: tables nested deeper than %d", ErrLimit, p.tok.line, p.lim.MaxDepth)
	}
	p.next() // '{'
	t := &Table{}
	var pos int64
	for p.tok.kind != tokRBrace {
		var k Key
		switch {
		case p.tok.kind == tokLBrack:
			p.next()
			key, err := p.bracketKey()
			if err != nil {
				return nil, err
			}
			k = key
		case p.tok.kind == tokIdent && p.ahead.kind == tokEq:
			k = Key{Name: p.tok.text}
			p.next()
			p.next()
		default:
			pos++
			k = Key{Index: pos, IsIndex: true}
		}
		v, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		t.Entries = append(t.Entries, Entry{Key: k, Value: v})
		switch p.tok.kind {
		case tokSep:
			p.next()
		case tokRBrace:
		default:
			return nil, p.errorf("expected ',' or '}', got %s", p.tok)
		}
	}
	p.next() // '}'
	return t, nil
}

// bracketKey parses `key] =` after '['. Integer keys become indexes; any
// other number or string becomes a name.
func (p *parser) bracketKey() (Key, error) {
	var k Key
	switch p.tok.kind {
	case tokNumber:
		if i, err := strconv.ParseInt(p.tok.text, 10, 64); err == nil {
			k = Key{Index: i, IsIndex: true}
		} else {
			k = Key{Name: p.tok.text}
		}
	case tokString:
		k = Key{Name: p.tok.text}
	default:
		return Key{}, p.errorf("unexpected %s in key", p.tok)
	}
	p.next()
	if p.tok.kind != tokRBrack {
		return Key{}, p.errorf("expected ']', got %s", p.tok)
	}
	p.next()
	if p.tok.kind != tokEq {
		return Key{}, p.errorf("expected '=', got %s", p.tok)
	}
	p.next()
	return k, nil
}
