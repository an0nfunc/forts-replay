// SPDX-License-Identifier: AGPL-3.0-or-later

// Package lua parses the Lua table text the game writes into replay
// snapshots. It only parses; nothing is evaluated.
package lua

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var (
	// ErrSyntax reports Lua text that does not parse.
	ErrSyntax = errors.New("lua: syntax error")
	// ErrLimit reports input beyond a Limits bound.
	ErrLimit = errors.New("lua: limit exceeded")
)

// Kind is the type of a Value.
type Kind uint8

// Kinds of Value.
const (
	KindTable Kind = iota + 1
	KindString
	KindNumber
	KindBool
)

// Value is a parsed Lua value. Kind says which field holds it.
type Value struct {
	Kind  Kind
	Str   string // KindString: contents; KindNumber: the number as written
	Bool  bool
	Table *Table
}

// Float parses a number, including the inf and nan spellings of the
// Windows C runtime.
func (v Value) Float() (float64, bool) {
	if v.Kind != KindNumber {
		return 0, false
	}
	s := strings.TrimSuffix(v.Str, "(ind)")
	if strings.TrimPrefix(s, "-") == "nan" {
		return math.NaN(), true
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// Key is a table key: a name, or an integer index. Positional entries get
// indexes 1, 2, ... as in Lua.
type Key struct {
	Name    string
	Index   int64
	IsIndex bool
}

func (k Key) String() string {
	if k.IsIndex {
		return "[" + strconv.FormatInt(k.Index, 10) + "]"
	}
	return k.Name
}

// Entry is one key and value of a Table.
type Entry struct {
	Key   Key
	Value Value
}

// Table keeps entries in source order.
type Table struct {
	Entries []Entry
}

// Get returns the first entry with the given name.
func (t *Table) Get(name string) (Value, bool) {
	for _, e := range t.Entries {
		if !e.Key.IsIndex && e.Key.Name == name {
			return e.Value, true
		}
	}
	return Value{}, false
}

// Limits bounds parsing. The parsed tree takes several times the input
// size in memory, so MaxSize is the effective memory bound.
type Limits struct {
	MaxDepth int // table nesting; default 64
	MaxSize  int // input bytes; default 1 MiB
}

// WithDefaults returns l with zero fields set to their defaults.
func (l Limits) WithDefaults() Limits {
	if l.MaxDepth <= 0 {
		l.MaxDepth = 64
	}
	if l.MaxSize <= 0 {
		l.MaxSize = 1 << 20
	}
	return l
}
