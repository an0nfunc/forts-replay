// SPDX-License-Identifier: AGPL-3.0-or-later

package stream

import (
	"bytes"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
)

func FuzzScanner(f *testing.F) {
	for _, b := range fixture.AllBlocks(f, "..") {
		f.Add(b)
	}
	f.Add(bytes.Join([][]byte{fwrtest.Record([]byte{0xe1, 0}, true), fwrtest.Record([]byte{0x95}, false)}, nil))
	f.Add([]byte{0xab, 0xab, 0xab, 0xab, 0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		s := New(data)
		end := 0
		for s.Next() {
			r := s.Record()
			if r.Offset < end || len(r.Payload) == 0 || r.Payload[0] != r.Type {
				t.Fatalf("bad record %+v after offset %d", r, end)
			}
			end = r.Offset + 1
		}
	})
}
