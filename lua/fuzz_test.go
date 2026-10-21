// SPDX-License-Identifier: AGPL-3.0-or-later

package lua_test

import (
	"encoding/json"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/lua"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/stream"
)

func FuzzParse(f *testing.F) {
	for _, b := range fixture.AllBlocks(f, "..") {
		s := stream.New(b)
		for s.Next() {
			if r := s.Record(); r.Type == 0xe1 {
				for _, sec := range snapshot.Parse(r.Payload, lua.Limits{}).Sections {
					f.Add(sec.Text)
				}
			}
		}
	}
	f.Add([]byte("data = { [1] = -nan(ind), L\"w\", x = { y = { } }, -- c\r\n }"))
	f.Fuzz(func(t *testing.T, src []byte) {
		tbl, err := lua.Parse(src, lua.Limits{MaxDepth: 32})
		if err != nil {
			return
		}
		out, err := json.Marshal(tbl)
		if err != nil || !json.Valid(out) {
			t.Fatalf("JSON for %q: %v %s", src, err, out)
		}
	})
}
