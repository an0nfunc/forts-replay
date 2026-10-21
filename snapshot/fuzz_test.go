// SPDX-License-Identifier: AGPL-3.0-or-later

package snapshot

import (
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/lua"
)

func FuzzParse(f *testing.F) {
	for _, p := range snapshotsIn(fixture.AllBlocks(f, "..")) {
		f.Add(p)
	}
	f.Add(section("dlc3", "data = { x = { 1, 2 }, }"))
	f.Add([]byte("\x07\x00\x00\x00data = "))
	f.Fuzz(func(t *testing.T, payload []byte) {
		s := Parse(payload, lua.Limits{MaxDepth: 32})
		end := 0
		for _, sec := range s.Sections {
			if sec.Offset < end || sec.Offset+len(sec.Text) > len(payload) {
				t.Fatalf("section at %d overlaps or overruns", sec.Offset)
			}
			end = sec.Offset + len(sec.Text)
		}
	})
}
