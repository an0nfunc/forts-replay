// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"fmt"
	"math"
)

// DecodeFunc decodes a payload, type byte included. It must not keep
// references to payload beyond the returned message.
type DecodeFunc func(payload []byte) (Message, error)

// Versions is an inclusive range of header versions. Max 0 means no upper bound.
type Versions struct {
	Min, Max uint32
}

func (v Versions) upper() uint32 {
	if v.Max == 0 {
		return math.MaxUint32
	}
	return v.Max
}

func (v Versions) contains(x uint32) bool { return v.Min <= x && x <= v.upper() }

func (v Versions) overlaps(o Versions) bool { return v.Min <= o.upper() && o.Min <= v.upper() }

type entry struct {
	v  Versions
	fn DecodeFunc
}

// Registry maps message types to decoders.
type Registry struct {
	byType map[byte][]entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{byType: map[byte][]entry{}} }

// Register adds a decoder. Overlapping ranges for one type are a
// programming error and panic.
func (r *Registry) Register(t byte, v Versions, fn DecodeFunc) {
	for _, e := range r.byType[t] {
		if e.v.overlaps(v) {
			panic(fmt.Sprintf("msg: type %#02x: versions %v overlap %v", t, v, e.v))
		}
	}
	r.byType[t] = append(r.byType[t], entry{v, fn})
}

// Lookup returns the decoder for t at version, or nil.
func (r *Registry) Lookup(t byte, version uint32) DecodeFunc {
	for _, e := range r.byType[t] {
		if e.v.contains(version) {
			return e.fn
		}
	}
	return nil
}

// RecordType returns the record type that messages of type t have in
// replays of header version v. The Type constants and MsgType name message
// types by their record types in version 1383. Version 1390 moves the input
// records and chat (0x92 to 0x9a) by 3, and the ranked result and the types
// from the tick (0xbe) to the script call (0xe6) by 4; it reports false for
// any other type there. Every other version keeps the 1383 types.
func RecordType(t byte, v uint32) (byte, bool) {
	if v != 1390 {
		return t, true
	}
	switch {
	case t >= TypeHover && t <= 0x9a:
		return t + 3, true
	case t == TypeResult, t >= TypeTick && t <= TypeScriptCall:
		return t + 4, true
	}
	return 0, false
}

// register adds fn for messages of type t in version v, under the record
// type t has there; a type v does not place stays undecoded.
func register(r *Registry, v uint32, t byte, fn DecodeFunc) {
	if rt, ok := RecordType(t, v); ok {
		r.Register(rt, Versions{Min: v, Max: v}, fn)
	}
}

// DefaultRegistry returns a new registry holding the built-in decoders.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	// Snapshots decode in every version, under the 1383 type except in 1390.
	r.Register(TypeSnapshot, Versions{Max: 1389}, decodeSnapshot)
	register(r, 1390, TypeSnapshot, decodeSnapshot)
	r.Register(TypeSnapshot, Versions{Min: 1391}, decodeSnapshot)
	for _, v := range []uint32{1383, 1390} {
		register(r, v, TypeTick, decodeTick)
		register(r, v, TypeSync, decodeSync)
		registerInputs(r, v)
		registerCommands(r, v)
		registerScripts(r, v)
		registerGame(r, v)
	}
	return r
}
