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

// DefaultRegistry returns a new registry holding the built-in decoders.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(TypeSnapshot, Versions{}, decodeSnapshot)
	v1383 := Versions{Min: 1383, Max: 1383}
	r.Register(TypeTick, v1383, decodeTick)
	r.Register(TypeSync, v1383, decodeSync)
	registerInputs(r, v1383)
	registerCommands(r, v1383)
	registerGame(r, v1383)
	return r
}
