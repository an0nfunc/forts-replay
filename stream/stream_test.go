// SPDX-License-Identifier: AGPL-3.0-or-later

package stream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
)

func TestScan(t *testing.T) {
	data := bytes.Join([][]byte{
		fwrtest.Record([]byte{0xe1, 1, 2}, true),
		fwrtest.Record([]byte{0xbe}, false),
		fwrtest.Record([]byte{0x95, 9}, false),
	}, nil)
	s := New(data)
	var got []Record
	for s.Next() {
		got = append(got, s.Record())
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	want := []Record{
		{Type: 0xe1, Keyframe: true, Offset: 0, Payload: []byte{0xe1, 1, 2}},
		{Type: 0xbe, Offset: 15, Payload: []byte{0xbe}},
		{Type: 0x95, Offset: 24, Payload: []byte{0x95, 9}},
	}
	if len(got) != len(want) {
		t.Fatalf("%d records", len(got))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Type != w.Type || g.Keyframe != w.Keyframe || g.Offset != w.Offset || !bytes.Equal(g.Payload, w.Payload) {
			t.Errorf("record %d: %+v, want %+v", i, g, w)
		}
		if cap(g.Payload) != len(g.Payload) {
			t.Errorf("record %d: payload capacity extends past the record", i)
		}
	}
}

func TestEmpty(t *testing.T) {
	s := New(nil)
	if s.Next() || s.Err() != nil {
		t.Fatal("empty block must yield nothing and no error")
	}
}

func TestCorrupt(t *testing.T) {
	ok := fwrtest.Record([]byte{0x95}, false)
	le := binary.LittleEndian
	cases := map[string][]byte{
		"bad marker":     append(le.AppendUint32(nil, 0x12345678), 1, 0, 0, 0, 0x95),
		"zero length":    fwrtest.Record(nil, false),
		"length too big": append(le.AppendUint32(nil, 0xabababab), 0xff, 0, 0, 0, 0x95),
		"trailing bytes": append(append([]byte{}, ok...), 1, 2, 3),
		"prefix only":    le.AppendUint32(nil, 0xc0c0c0c0),
	}
	for name, data := range cases {
		s := New(data)
		for s.Next() {
		}
		if !errors.Is(s.Err(), ErrCorrupt) {
			t.Errorf("%s: err %v", name, s.Err())
		}
	}
}

func TestFixtures(t *testing.T) {
	cases := []struct {
		file     string
		perBlock []int
		types    map[byte]int
	}{
		{"abyss-bot-278f.fwr", []int{184, 185, 23}, map[byte]int{0x92: 10, 0x93: 2, 0x95: 94, 0x97: 4, 0xbe: 139, 0xc0: 139, 0xe1: 3, 0xe6: 1}},
		{"abyss-bot-974f.fwr", []int{201, 182, 169, 170, 162, 169, 132, 98}, map[byte]int{0x92: 72, 0x93: 2, 0x95: 178, 0x97: 34, 0xbe: 487, 0xc0: 487, 0xcc: 4, 0xce: 10, 0xe1: 8, 0xe6: 1}},
	}
	for _, c := range cases {
		types := map[byte]int{}
		keyframes := 0
		var perBlock []int
		for i, data := range fixture.Blocks(t, "..", c.file) {
			s := New(data)
			n := 0
			for s.Next() {
				rec := s.Record()
				if n == 0 && rec.Type != 0xe1 {
					t.Errorf("%s block %d starts with %#x", c.file, i, rec.Type)
				}
				if rec.Keyframe {
					keyframes++
				}
				types[rec.Type]++
				n++
			}
			if err := s.Err(); err != nil {
				t.Fatalf("%s block %d: %v", c.file, i, err)
			}
			perBlock = append(perBlock, n)
		}
		if !slices.Equal(perBlock, c.perBlock) || !maps.Equal(types, c.types) || keyframes != 1 {
			t.Errorf("%s: records per block %v, types %v, keyframes %d", c.file, perBlock, types, keyframes)
		}
	}
}
