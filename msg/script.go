// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"bytes"
	"fmt"
)

// TypeScriptCall is a call into the game's Lua scripts.
const TypeScriptCall byte = 0xe6

// ScriptCall calls a script function, from a player or, with player 0, from
// the game.
type ScriptCall struct {
	Input

	Name string `json:"name"`
	Args string `json:"args"` // the arguments as text
}

// MsgType returns TypeScriptCall.
func (*ScriptCall) MsgType() byte { return TypeScriptCall }

// cstr reads the NUL-terminated text in p[lo:hi], which must hold a
// NUL and only printable ASCII before it. Bytes after the NUL are ignored.
func (b *body) cstr(lo, hi int) string {
	text, _, ok := bytes.Cut(b.p[lo:hi], []byte{0})
	if !ok {
		if b.err == nil {
			b.err = fmt.Errorf("no NUL in bytes %d to %d", lo, hi-1)
		}
		return ""
	}
	for i, c := range text {
		if c < 0x20 || c > 0x7e {
			if b.err == nil {
				b.err = fmt.Errorf("byte %d is %#02x, want printable ASCII", lo+i, c)
			}
			return ""
		}
	}
	return string(text)
}

// registerScripts adds the script call decoder for version v.
func registerScripts(r *Registry, v uint32) {
	register(r, v, TypeScriptCall, inputDecoder(344, func(in Input, b *body) Message {
		return &ScriptCall{Input: in, Name: b.cstr(24, 152), Args: b.cstr(152, 344)}
	}))
}
