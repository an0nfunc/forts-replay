// SPDX-License-Identifier: AGPL-3.0-or-later

// Package header decodes the version-specific fields of the replay header,
// the bytes container.Header.Raw holds.
package header

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	// ErrVersion reports a header version whose layout is not known.
	ErrVersion = errors.New("header: version not decoded")
	// ErrCorrupt reports a header that does not follow its version's layout.
	ErrCorrupt = errors.New("header: corrupt")
)

const (
	rankedOffset  = 0x33a
	rankedLen     = 0x80 // bytes a ranked header has after the ranked flag
	optionsOffset = 0x570
	dateOffset    = 25   // version 1390
	dateLen       = 15   // YYYYMMDD_HHMMSS
	dateShift     = 0x16 // bytes the date and 3 more add before the ranked flag
	maxOptions    = 64
	maxString     = 1024
	maxPlayers    = 16
	maxAI         = 16
	maxImages     = 8
	nameUnits     = 16 // UTF-16 code units of a name field, NUL included

	playerEnd    = 0xfffffffd // i32 -3 closing every player entry
	aiMarker     = 0xa1a1a1a1
	imagesMarker = 0xb7b7b7b7
	imageMarker  = 0x3c3c3c3c
)

// Header holds the decoded fields. Apart from the date, the ranked flag and
// the commanders, the bytes before the options are not decoded yet.
type Header struct {
	Date         string   // version 1390: the recording's date and time, YYYYMMDD_HHMMSS, as in the default file name
	Ranked       bool     // a ranked match: 128 more bytes before the options
	Commanders   []string // side 1's and side 2's save-names, "" for none; nil when they do not decode
	Options      []string // game options, e.g. "repair_station", "dlc3"
	Map          string   // map file, e.g. "maps/Abyss/Abyss.fwe"
	MapSPR       string   // the map's .spr file, e.g. "maps/Abyss/Abyss.spr"
	Rest         int      // offset of the first byte after MapSPR
	Recorder     uint32   // player ID of the client that recorded the replay
	RecorderTeam int32    // its team; -3 when it spectates
	Players      []Player // spectators included
	AI           []AI
}

// Player is one entry of the player table.
type Player struct {
	Slot    uint32
	ID      uint32 // the player field (u32@20) of the player's records
	Team    int32  // side + 100 × fort index, or plain 1 and 2; -3 for a spectator
	Name    string // in a ranked replay, the opponent's entry holds a generic word for "player"
	SteamID uint64 // SteamID64; 0 when the entry holds none, as in single-player games
}

// AI is one entry of the AI fort list. Entries with Value 0 hold the place
// of a human side.
type AI struct {
	Team   int32
	Script string // the AI fort's script file; "" for some AI forts
	Value  uint32 // non-zero for a fort the AI plays; meaning unknown
}

// Parse decodes raw, the whole header including its fixed fields, for
// header version 1383 or 1390. Version 1390 adds the date at offset 25, and
// every fixed offset from the ranked flag on moves by 0x16. When only the
// fields after MapSPR are corrupt, it returns the header up to MapSPR,
// without the later fields, together with the error.
func Parse(version uint32, raw []byte) (*Header, error) {
	var h Header
	r := &reader{b: raw}
	shift := 0
	switch version {
	case 1383:
	case 1390:
		r.off = dateOffset
		if h.Date = r.str(); r.err == nil && len(h.Date) != dateLen {
			r.failf("date of %d bytes at offset %#x", len(h.Date), dateOffset)
		}
		shift = dateShift
	default:
		return nil, fmt.Errorf("%w: version %d", ErrVersion, version)
	}
	r.off = rankedOffset + shift
	ranked := r.u8()
	if ranked > 1 {
		r.failf("ranked flag %d at offset %#x", ranked, rankedOffset+shift)
	}
	h.Ranked = ranked == 1
	if h.Ranked {
		shift += rankedLen
	}
	r.off = optionsOffset + shift
	start := r.off
	n := r.u32()
	if n > maxOptions {
		r.failf("%d options at offset %#x", n, start)
	}
	for i := uint32(0); i < n && r.err == nil; i++ {
		h.Options = append(h.Options, r.str())
	}
	h.Map = r.str()
	h.MapSPR = r.str()
	if r.err != nil {
		return nil, r.err
	}
	h.Commanders = commanders(raw, shift)
	h.Rest = r.off
	full := h
	if err := r.tail(&full); err != nil {
		return &h, err
	}
	return &full, nil
}

// tail reads the fields after MapSPR, which run to the end of the header.
func (r *reader) tail(h *Header) error {
	r.skip(11) // u32 0, u32 candidate DLC mask, 3 flag bytes
	r.u32()    // candidate AI difficulty
	h.Recorder = r.u32()
	h.RecorderTeam = r.i32()
	start := r.off
	n := r.u32()
	if n > maxPlayers {
		r.failf("%d players at offset %#x", n, start)
	}
	for i := uint32(0); i < n && r.err == nil; i++ {
		p := Player{Slot: r.u32(), ID: r.u32(), Team: r.i32(), Name: r.name()}
		r.skip(13 + 8) // not decoded, then a u64 not used
		p.SteamID = steamID(r.u64(), r.u64())
		r.skip(8 + 8) // a u64 not used, then 8 bytes not decoded
		r.marker(playerEnd)
		h.Players = append(h.Players, p)
	}
	r.marker(aiMarker)
	r.skip(8) // two f32, not decoded
	start = r.off
	n = r.u32()
	if n > maxAI {
		r.failf("%d AI forts at offset %#x", n, start)
	}
	for i := uint32(0); i < n && r.err == nil; i++ {
		h.AI = append(h.AI, AI{Team: r.i32(), Script: r.str(), Value: r.u32()})
	}
	r.marker(imagesMarker)
	for i := 0; r.err == nil && r.peek32() == imageMarker; i++ {
		if i == maxImages {
			r.failf("more than %d images at offset %#x", maxImages, r.off)
			break
		}
		r.skip(4)
		r.skip(r.u32())
	}
	for range 2 { // team records
		r.skipStr(1) // name key, e.g. "Team1"
		r.skipStr(2) // custom name, UTF-16LE
		r.skipStr(1) // short name key
		r.skipStr(2) // custom short name
		r.skipStr(1) // avatar texture
		r.u32()      // -1 for stock teams; candidate series score
	}
	r.skipStr(1) // badge texture
	if r.err == nil && r.off != len(r.b) {
		r.failf("%d bytes after the badge at offset %#x", len(r.b)-r.off, r.off)
	}
	return r.err
}

// reader reads fields in order and keeps the first error; reads after an
// error return zero values.
type reader struct {
	b   []byte
	off int
	err error
}

// take returns the next n bytes, or nil after an error.
func (r *reader) take(n uint32) []byte {
	if r.err != nil {
		return nil
	}
	if int64(n) > int64(len(r.b)-r.off) {
		r.failf("header ends at %#x, before offset %#x", len(r.b), int64(r.off)+int64(n))
		return nil
	}
	b := r.b[r.off : r.off+int(n)]
	r.off += int(n)
	return b
}

func (r *reader) skip(n uint32) { r.take(n) }

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (r *reader) i32() int32 {
	return int32(r.u32()) //nolint:gosec // G115: the field is a two's-complement i32
}

// peek32 returns the next u32 without reading it, or 0 when fewer than 4
// bytes are left.
func (r *reader) peek32() uint32 {
	if r.err != nil || len(r.b)-r.off < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(r.b[r.off:])
}

// marker reads a u32 that must equal want.
func (r *reader) marker(want uint32) {
	start := r.off
	if v := r.u32(); r.err == nil && v != want {
		r.failf("%#08x at offset %#x, want %#08x", v, start, want)
	}
}

// str reads a u32 byte count and that many bytes of printable UTF-8.
func (r *reader) str() string {
	start := r.off
	n := r.u32()
	if r.err == nil && (n > maxString || int64(n) > int64(len(r.b)-r.off)) {
		r.failf("string of %d bytes at offset %#x", n, start)
	}
	s := r.take(n)
	if r.err != nil {
		return ""
	}
	if !isText(s) {
		r.failf("string at offset %#x is not printable UTF-8", start)
		return ""
	}
	return string(s)
}

// skipStr skips a u32 byte count and that many bytes, a whole number of
// units of unit bytes.
func (r *reader) skipStr(unit uint32) {
	start := r.off
	n := r.u32()
	if r.err == nil && (n > maxString || n%unit != 0) {
		r.failf("string of %d bytes at offset %#x", n, start)
	}
	r.skip(n)
}

// name reads a name field: UTF-16LE up to its first NUL. Unpaired
// surrogates become U+FFFD.
func (r *reader) name() string {
	start := r.off
	b := r.take(2 * nameUnits)
	units := make([]uint16, 0, nameUnits)
	for i := 0; i+1 < len(b); i += 2 {
		u := binary.LittleEndian.Uint16(b[i:])
		if u == 0 {
			return string(utf16.Decode(units))
		}
		units = append(units, u)
	}
	if r.err == nil {
		r.failf("name at offset %#x has no NUL", start)
	}
	return ""
}

func (r *reader) failf(format string, args ...any) {
	r.err = fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

func isText(s []byte) bool {
	if !utf8.Valid(s) {
		return false
	}
	for _, c := range string(s) {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}
