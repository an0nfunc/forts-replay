// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repocheck finds SteamIDs in the files git tracks. Its test fails
// on any that steamids.allow does not list.
package repocheck

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// steamIDHigh is the upper half of an individual SteamID64: universe 1, an
// individual account, the desktop instance.
const steamIDHigh = 0x01100001

// SteamIDs returns the individual SteamID64s in data, sorted, each once. It
// reads data as raw little-endian bytes, 17-digit decimals, hex (byte
// tokens, digit runs, xxd and hexdump -C lines; little- and big-endian), 0x
// literals of 15 or 16 digits, and \x escapes. A SteamID stored masked, as
// the header's player entries hold it, is not found.
func SteamIDs(data []byte) []uint64 {
	found := map[uint64]bool{}
	add := func(v uint64) {
		if v>>32 == steamIDHigh {
			found[v] = true
		}
	}
	windows(data, binary.LittleEndian, add)
	windows(unescape(data), binary.LittleEndian, add)
	decimals(data, add)
	literals(data, add)
	for _, s := range hexStreams(dumpLines(data)) {
		windows(s, binary.LittleEndian, add)
		windows(s, binary.BigEndian, add)
	}
	return slices.Sorted(maps.Keys(found))
}

// windows passes the value of every 8-byte window of p to add.
func windows(p []byte, order binary.ByteOrder, add func(uint64)) {
	for i := 0; i+8 <= len(p); i++ {
		add(order.Uint64(p[i:]))
	}
}

// decimals passes every run of exactly 17 digits to add.
func decimals(p []byte, add func(uint64)) {
	for i := range p {
		if !isDigit(p[i]) || i > 0 && isDigit(p[i-1]) {
			continue
		}
		if digits := p[i:run(p, i, isDigit)]; len(digits) == 17 {
			v, _ := strconv.ParseUint(string(digits), 10, 64) // 17 digits fit
			add(v)
		}
	}
}

// literals passes every 0x literal of 15 or 16 hex digits to add, so that a
// dropped leading zero is caught too.
func literals(p []byte, add func(uint64)) {
	for i := range p {
		if p[i] != '0' || !isPrefix(p, i) {
			continue
		}
		if digits := p[i+2 : run(p, i+2, isHex)]; len(digits) == 15 || len(digits) == 16 {
			v, _ := strconv.ParseUint(string(digits), 16, 64) // at most 16 digits
			add(v)
		}
	}
}

// hexStreams decodes the hex in text. Each run of hex digits is read two
// digits to a byte, and runs that only spaces, tabs, commas, line breaks and
// 0x or \x prefixes separate join into one stream. A run of odd length ends
// the stream and is read once without its first digit and once without its
// last.
func hexStreams(text []byte) [][]byte {
	var streams [][]byte
	var cur []byte
	end := func() {
		if len(cur) > 0 {
			streams = append(streams, cur)
		}
		cur = nil
	}
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case isPrefix(text, i):
			i += 2
		case isHex(c):
			j := run(text, i, isHex)
			if r := text[i:j]; len(r)%2 == 0 {
				cur = append(cur, decodeHex(r)...)
			} else {
				end()
				streams = append(streams, decodeHex(r[1:]), decodeHex(r[:len(r)-1]))
			}
			i = j
		case c == ' ' || c == '\t' || c == ',' || c == '\n' || c == '\r':
			i++
		default:
			end()
			i++
		}
	}
	end()
	return streams
}

// dumpLines cuts each xxd or hexdump -C line of text to its hex bytes, so
// that consecutive lines join into one stream with no offset between them.
func dumpLines(text []byte) []byte {
	out := make([]byte, 0, len(text))
	for line := range bytes.Lines(text) {
		if hexBytes, ok := dumpBytes(line); ok {
			out = append(append(out, hexBytes...), '\n')
		} else {
			out = append(out, line...)
		}
	}
	return out
}

// dumpBytes returns the hex bytes of a line of xxd (offset, ": ", the bytes,
// two spaces, the text) or of hexdump -C (offset, two spaces, the bytes, the
// text between bars).
func dumpBytes(line []byte) ([]byte, bool) {
	if len(line) < 10 || run(line, 0, isHex) != 8 {
		return nil, false
	}
	rest := line[8:]
	switch {
	case bytes.HasPrefix(rest, []byte(": ")):
		rest, _, _ = bytes.Cut(rest[2:], []byte("  "))
	case bytes.HasPrefix(rest, []byte("  ")):
		rest, _, _ = bytes.Cut(rest[2:], []byte("|"))
	default:
		return nil, false
	}
	return bytes.TrimRight(rest, " \r\n"), true
}

// unescape returns p with \xHH and the one-letter escapes of Go and C
// strings decoded and every other byte kept, so that the bytes of a quoted
// string that mixes escapes with printable characters line up again. It
// returns nil when p holds no \x.
func unescape(p []byte) []byte {
	if !bytes.Contains(p, []byte(`\x`)) {
		return nil
	}
	const letters, values = "abfnrtv\\\"'", "\a\b\f\n\r\t\v\\\"'"
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '\\' && i+1 < len(p) {
			if p[i+1] == 'x' && i+3 < len(p) && isHex(p[i+2]) && isHex(p[i+3]) {
				out = append(out, decodeHex(p[i+2:i+4])...)
				i += 3
				continue
			}
			if k := strings.IndexByte(letters, p[i+1]); k >= 0 {
				out = append(out, values[k])
				i++
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// isPrefix reports whether text holds a 0x or \x prefix at i, followed by a
// hex digit.
func isPrefix(text []byte, i int) bool {
	if i+2 >= len(text) {
		return false
	}
	lead, x := text[i], text[i+1]
	return (lead == '0' || lead == '\\') && (x == 'x' || x == 'X') && isHex(text[i+2])
}

// run returns the end of the run of bytes from i on that ok accepts.
func run(p []byte, i int, ok func(byte) bool) int {
	for i < len(p) && ok(p[i]) {
		i++
	}
	return i
}

func decodeHex(r []byte) []byte {
	out := make([]byte, len(r)/2)
	_, _ = hex.Decode(out, r) // r holds hex digits only, an even number
	return out
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func isHex(c byte) bool { return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' }
