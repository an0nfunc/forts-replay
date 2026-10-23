// SPDX-License-Identifier: AGPL-3.0-or-later

// Package snapshot reads the Lua sections of a state snapshot (message
// 0xe1, 0xe5 in header version 1390). Package state decodes the binary data
// around them.
package snapshot

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"iter"

	"github.com/an0nfunc/forts-replay/lua"
)

// Every section the game writes starts with this text and is preceded by
// its u32 length. Sections are found by this signature, without reading
// the binary data between them.
const signature = "data = "

const maxNameLen = 255

// maxSections bounds the sections of one snapshot; v1.38.2 writes 7.
const maxSections = 256

// Section is one Lua section of a snapshot.
type Section struct {
	Offset int    // offset of Text in the payload
	Name   string // script name stored before the section, "" if none
	Text   []byte
	Table  *lua.Table // nil when Err is set
	Err    error
}

// Snapshot is a snapshot payload and its Lua sections.
type Snapshot struct {
	Payload  []byte
	Sections []Section
}

// Parse finds and parses the Lua sections of a snapshot payload. A section
// that does not parse keeps its error and the rest are still parsed.
// lim.MaxSize bounds the Lua text of all sections together, since the
// parsed trees of one snapshot are held at once; sections past it get an
// ErrLimit error, as does one more after maxSections, where the scan stops.
func Parse(payload []byte, lim lua.Limits) *Snapshot {
	lim = lim.WithDefaults()
	budget := lim.MaxSize
	s := &Snapshot{Payload: payload}
	for p, n := range sections(payload) {
		sec := Section{Offset: p, Name: nameBefore(payload, p), Text: payload[p : p+n : p+n]}
		switch {
		case len(s.Sections) == maxSections:
			sec.Err = fmt.Errorf("%w: more than %d sections", lua.ErrLimit, maxSections)
			s.Sections = append(s.Sections, sec)
			return s
		case n > budget:
			sec.Err = fmt.Errorf("%w: snapshot Lua text above %d bytes", lua.ErrLimit, lim.MaxSize)
		default:
			budget -= n
			sec.Table, sec.Err = lua.Parse(sec.Text, lim)
		}
		s.Sections = append(s.Sections, sec)
	}
	return s
}

// End returns the end of the last Lua section, where the binary world
// state starts, without parsing any Lua. It reports false when the payload
// has no section.
func End(payload []byte) (int, bool) {
	end, ok := 0, false
	for p, n := range sections(payload) {
		end, ok = p+n, true
	}
	return end, ok
}

// sections yields the offset and length of each section's text, at most
// maxSections+1 of them.
func sections(payload []byte) iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		found := 0
		for i := 0; i < len(payload); {
			j := bytes.Index(payload[i:], []byte(signature))
			if j < 0 {
				return
			}
			p := i + j
			n, ok := textLen(payload, p)
			if !ok {
				i = p + 1
				continue
			}
			if !yield(p, n) || found == maxSections {
				return
			}
			found++
			i = p + n
		}
	}
}

func textLen(b []byte, p int) (int, bool) {
	if p < 4 {
		return 0, false
	}
	n := binary.LittleEndian.Uint32(b[p-4:])
	if n < uint32(len(signature)) || int64(n) > int64(len(b)-p) {
		return 0, false
	}
	return int(n), true
}

// nameBefore returns the length-prefixed name that ends right before the
// section's length field, or "".
func nameBefore(b []byte, p int) string {
	end := p - 4
	for l := 1; l <= maxNameLen && end-l-4 >= 0; l++ {
		if binary.LittleEndian.Uint32(b[end-l-4:]) == uint32(l) && isName(b[end-l:end]) {
			return string(b[end-l : end])
		}
	}
	return ""
}

func isName(b []byte) bool {
	for _, c := range b {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '/' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
