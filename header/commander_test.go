// SPDX-License-Identifier: AGPL-3.0-or-later

package header

import (
	"encoding/binary"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestMask: the stream that draws[73] = 0 seeds. Byte 34 is the first one
// that needs the carry into the upper half.
func TestMask(t *testing.T) {
	want := []byte{
		0xcd, 0x05, 0xa5, 0xcb, 0x26, 0xa4, 0x47, 0x7a, 0x2f, 0x5f, 0xe2, 0xca, 0x59, 0xbc, 0xa6, 0x51, 0xde, 0x7c, 0x49, 0xcf,
		0xc1, 0x03, 0x8e, 0xb0, 0x64, 0xb6, 0x6f, 0x57, 0x48, 0x07, 0x52, 0xcf, 0x06, 0x5f, 0x19, 0xc6, 0x2d, 0xab, 0x67, 0x51,
	}
	m := mask{s: seedXOR}
	for i, w := range want {
		if b := m.next(); b != w {
			t.Errorf("byte %d: %#02x, want %#02x", i, b, w)
		}
	}
}

// TestScale compares scale with the float32 chain format.md documents, for
// every fifteen-bit output. The explicit conversions round each step, so
// the compiler cannot fuse them.
func TestScale(t *testing.T) {
	for out := range uint32(1 << 15) {
		raw := float32(float64(out) * (1.0 / 32767.0))
		want := byte(int(float32(float32(255*raw) + 0.5)))
		if got := scale(out); got != want {
			t.Fatalf("scale(%d) = %d, want %d", out, got, want)
		}
	}
}

// decodeTo returns raw, a non-ranked header whose side 1 holds name1, with
// byte k of side's block decoding to want.
func decodeTo(raw []byte, name1 string, side, k int, want byte) []byte {
	m := mask{s: binary.LittleEndian.Uint32(raw[drawsOffset+4*seedDraw:]) ^ seedXOR}
	n := k // stream bytes before this one
	if side == 2 {
		n = len(name1) + 1 + k
	}
	for range n {
		m.next()
	}
	b := slices.Clone(raw)
	b[namesOffset+nameBlock*(side-1)+k] = m.next() ^ want
	return b
}

func TestCommandersCorrupt(t *testing.T) {
	setup := func(name1, name2 string) []byte {
		s := lobby(false)
		s.Commanders = [2]string{name1, name2}
		return setupRaw(s)
	}
	cases := []struct {
		name string
		raw  []byte
		want []string
	}{
		{"side 1 without NUL", decodeTo(setup(longName, "b"), longName, 1, 49, 'x'), nil},
		{"side 2 without NUL", decodeTo(setup("a", longName), "a", 2, 49, 'x'), nil},
		{"unprintable", decodeTo(setup("abc", "def"), "abc", 2, 1, 0x07), nil},
		{"invalid UTF-8", decodeTo(setup("abc", "def"), "abc", 1, 1, 0xff), nil},
		{"side 1 empty", setup("", "commander-test-beta"), []string{"", "commander-test-beta"}},
		{"side 1 of 49 bytes", setup(longName, "commander-test-beta"), []string{longName, "commander-test-beta"}},
	}
	for _, c := range cases {
		h, err := Parse(1383, c.raw)
		if err != nil || len(h.Players) != 9 {
			t.Errorf("%s: %+v, %v", c.name, h, err)
			continue
		}
		if !reflect.DeepEqual(h.Commanders, c.want) {
			t.Errorf("%s: commanders %q, want %q", c.name, h.Commanders, c.want)
		}
	}
}

func TestSteamIDs(t *testing.T) {
	s := lobby(false)
	s.Players = s.Players[:2]
	raw := setupRaw(s)
	h0, err := Parse(1383, raw)
	if err != nil {
		t.Fatal(err)
	}
	entry := h0.Rest + 27 // A, B, flags, C, recorder, recorder team, n
	edit := func(f func(b []byte)) []byte {
		b := slices.Clone(raw)
		f(b)
		return b
	}
	cases := []struct {
		name string
		raw  []byte
		want uint64
	}{
		{"valid", raw, testSteamID},
		{"wrong top bits", edit(func(b []byte) { b[entry+73+4] ^= 1 }), 0},
		{"unused u64s set", edit(func(b []byte) {
			copy(b[entry+57:], strings.Repeat("\xa5", 8))
			copy(b[entry+81:], strings.Repeat("\x5a", 8))
		}), testSteamID},
	}
	for _, c := range cases {
		h, err := Parse(1383, c.raw)
		if err != nil || h.Players[0].SteamID != c.want || h.Players[1].SteamID != testSteamID-1 {
			t.Errorf("%s: %+v, %v", c.name, h, err)
		}
	}
}
