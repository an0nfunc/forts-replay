// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/stream"
)

func FuzzDecoder(f *testing.F) {
	for _, b := range fixture.AllBlocks(f, "..") {
		f.Add(b)
	}
	_, blocks := fixture.Read1390(f, "..", "probe-delete.fwr")
	f.Add(blocks[0])
	f.Add(records(0xe1, 0xbe, 0x95))
	f.Add(probeRecords(f))
	f.Add(fieldRecords())
	f.Add(bytes.Join([][]byte{fwrtest.Record(result(0, 1, 101, 7, 7), false), fwrtest.Record(screenshot(4, 2, 4, 32), false)}, nil))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, v := range []uint32{1383, 1390} {
			d := NewDecoder(stream.New(data), DefaultRegistry(), v, Options{})
			for d.Next() {
				if d.Message() == nil {
					t.Fatal("nil message")
				}
				if d.Frame() < -1 {
					t.Fatalf("frame %d", d.Frame())
				}
				if inv, ok := d.Message().(*Invalid); ok && !bytes.Equal(inv.Raw.Payload, d.Record().Payload) {
					t.Fatalf("invalid payload %x, record %x", inv.Raw.Payload, d.Record().Payload)
				}
			}
		}
	})
}

// probeRecords returns a stream of the first record of each type in the
// probe replays, snapshots left out.
func probeRecords(tb testing.TB) []byte {
	tb.Helper()
	seen := map[byte]bool{TypeSnapshot: true}
	var parts [][]byte
	for _, name := range fixture.Probes {
		for _, b := range fixture.Blocks(tb, "..", name) {
			s := stream.New(b)
			for s.Next() {
				if r := s.Record(); !seen[r.Type] {
					seen[r.Type] = true
					parts = append(parts, fwrtest.Record(r.Payload, false))
				}
			}
		}
	}
	return bytes.Join(parts, nil)
}

// fieldRecords returns a stream of one synthetic record of each repair,
// portal, aimed fire, ammo, aim and script call type, with its fields
// from offset 24 set.
func fieldRecords() []byte {
	rec := func(t byte, n int, words ...uint32) []byte {
		p := inputRec(t, n, t)
		for i, w := range words {
			binary.LittleEndian.PutUint32(p[24+4*i:], w)
		}
		return fwrtest.Record(p, false)
	}
	f := math.Float32bits
	return bytes.Join([][]byte{
		rec(TypeRepairLink, 36, 1, 298, 302),
		rec(TypeRepairDevice, 32, 1, 7),
		rec(TypeRepairArea, 44, 1, f(-1835.5), f(40), 0, f(200)),
		rec(TypePortalLink, 40, 303, 317, 304, 318),
		rec(TypePortalReverse, 32, 303, 317),
		rec(TypeFireAt, 52, f(1903.1), f(-147.4), f(-1.57), 0, 1394, 1395, 0xffffffff),
		rec(TypeAmmo, 32, 0xffffffff, 9),
		rec(TypeAim, 37, 1, f(-2279.4), f(-1234.7)),
		fwrtest.Record(scriptRec("OnStreamComplete", "1, true"), false),
	}, nil)
}
