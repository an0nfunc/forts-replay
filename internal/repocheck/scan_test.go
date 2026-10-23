// SPDX-License-Identifier: AGPL-3.0-or-later

package repocheck

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestSteamIDs: each form SteamIDs reads is found, and near misses are
// not. The cases build every SteamID at run time, so that this file holds
// none.
func TestSteamIDs(t *testing.T) {
	bad := uint64(steamIDHigh)<<32 | 0xfffffffd
	printable := uint64(steamIDHigh)<<32 | 0x44434241 // little-endian "ABCD"
	wrong := uint64(steamIDHigh+1)<<32 | 0xfffffffd
	le := binary.LittleEndian.AppendUint64(nil, bad)
	be := binary.BigEndian.AppendUint64(nil, bad)
	dec := strconv.FormatUint(bad, 10)
	tokens := func(p []byte, format, sep string) string {
		var s []string
		for _, c := range p {
			s = append(s, fmt.Sprintf(format, c))
		}
		return strings.Join(s, sep)
	}
	split := append(make([]byte, 12), le...) // across a 16-byte line
	for _, c := range []struct {
		name, data string
		want       uint64
	}{
		{"raw bytes", "\x7fELF" + string(le) + "\xff", bad},
		{"decimal", "id " + dec + ".", bad},
		{"decimal between letters", "x" + dec + "y", bad},
		{"decimal in a list", "1," + dec + ",2", bad},
		{"spaced hex", tokens(le, "%02x", " "), bad},
		{"hex digits", "payload:" + tokens(le, "%02x", "") + ";", bad},
		{"upper-case hex", tokens(le, "%02X", ""), bad},
		{"big-endian hex", tokens(be, "%02x", ""), bad},
		{"0x literal", fmt.Sprintf("v := 0x%016x", bad), bad},
		{"0x literal without its leading zero", fmt.Sprintf("v := 0x%x", bad), bad},
		{"Go byte literal", "[]byte{" + tokens(le, "0x%02x", ", ") + "}", bad},
		{"Go byte literal over lines", "[]byte{\n\t" + tokens(le[:4], "0x%02x", ", ") + ",\n\t" + tokens(le[4:], "0x%02x", ", ") + ",\n}", bad},
		{"\\x escapes", `"` + tokens(le, `\x%02x`, "") + `"`, bad},
		{"fuzz corpus file", "go test fuzz v1\n[]byte(" + strconv.Quote(string(binary.LittleEndian.AppendUint64(nil, printable))) + ")\n", printable},
		{"xxd", xxd(split), bad},
		{"hexdump -C", hexdumpC(split), bad},

		{"16 digits", "id " + dec[:16] + ".", 0},
		{"18 digits", "id " + dec + "0.", 0},
		{"wrong upper half", strconv.FormatUint(wrong, 10) + " " + tokens(binary.LittleEndian.AppendUint64(nil, wrong), "%02x", " "), 0},
		{"high half alone", "[]byte{0x01, 0x00, 0x10, 0x01}", 0},
		{"text", "SteamIDs live in the header; see docs/format.md.\n", 0},
	} {
		var want []uint64
		if c.want != 0 {
			want = []uint64{c.want}
		}
		data := []byte(c.data)
		if got := SteamIDs(data); !slices.Equal(got, want) {
			t.Errorf("%s: found %v, want %v, in %q", c.name, got, want, c.data)
		}
		if string(data) != c.data {
			t.Errorf("%s: SteamIDs changed its argument to %q", c.name, data)
		}
	}
}

// TestAllowlist: the allowlisted IDs are found, and allowed.
func TestAllowlist(t *testing.T) {
	allowed := allowlist(t)
	if len(allowed) != 2 {
		t.Fatalf("%d allowlisted IDs, want 2", len(allowed))
	}
	for id := range allowed {
		if got := SteamIDs([]byte(strconv.FormatUint(id, 10))); !slices.Equal(got, []uint64{id}) {
			t.Errorf("%d: found %v", id, got)
		}
	}
}

// xxd lays p out as xxd does: offset, eight groups of two bytes, the text.
func xxd(p []byte) string {
	var b strings.Builder
	for off := 0; off < len(p); off += 16 {
		line := p[off:min(off+16, len(p))]
		fmt.Fprintf(&b, "%08x:", off)
		for i := 0; i < 16; i += 2 {
			switch {
			case i+1 < len(line):
				fmt.Fprintf(&b, " %02x%02x", line[i], line[i+1])
			case i < len(line):
				fmt.Fprintf(&b, " %02x  ", line[i])
			default:
				b.WriteString("     ")
			}
		}
		fmt.Fprintf(&b, "  %s\n", text(line))
	}
	return b.String()
}

// hexdumpC lays p out as hexdump -C does: the lines of hex.Dump, then the
// length.
func hexdumpC(p []byte) string {
	return hex.Dump(p) + fmt.Sprintf("%08x\n", len(p))
}

// text is the text column of a dump: printable ASCII, a dot for the rest.
func text(p []byte) string {
	out := make([]byte, len(p))
	for i, c := range p {
		if c < ' ' || c > '~' {
			c = '.'
		}
		out[i] = c
	}
	return string(out)
}
