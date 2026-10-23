// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/stream"
)

func decoderNamed(name string) DecodeFunc {
	return func([]byte) (Message, error) { return &Raw{Payload: []byte(name)}, nil }
}

func lookupName(r *Registry, t byte, v uint32) string {
	fn := r.Lookup(t, v)
	if fn == nil {
		return ""
	}
	m, _ := fn(nil)
	return string(m.(*Raw).Payload)
}

func TestVersionGating(t *testing.T) {
	r := NewRegistry()
	r.Register(0x95, Versions{Min: 1300, Max: 1382}, decoderNamed("old"))
	r.Register(0x95, Versions{Min: 1383}, decoderNamed("new"))
	cases := map[uint32]string{1299: "", 1300: "old", 1382: "old", 1383: "new", 9999: "new"}
	for v, want := range cases {
		if got := lookupName(r, 0x95, v); got != want {
			t.Errorf("version %d: %q, want %q", v, got, want)
		}
	}
	if r.Lookup(0x96, 1383) != nil {
		t.Error("unregistered type found")
	}
}

func TestRegisterOverlapPanics(t *testing.T) {
	r := NewRegistry()
	r.Register(0x95, Versions{Min: 1383}, decoderNamed("a"))
	defer func() {
		if recover() == nil {
			t.Error("overlapping registration did not panic")
		}
	}()
	r.Register(0x95, Versions{Min: 1000, Max: 1400}, decoderNamed("b"))
}

func TestDefaultRegistryIsFresh(t *testing.T) {
	a := DefaultRegistry()
	a.Register(0x01, Versions{}, decoderNamed("x"))
	if DefaultRegistry().Lookup(0x01, 1383) != nil {
		t.Error("DefaultRegistry shares state between calls")
	}
}

// TestRecordType pins the record types of version 1390 and checks that
// other versions keep those of 1383.
func TestRecordType(t *testing.T) {
	want := map[byte]byte{
		TypeHover: 0x95, 0x93: 0x96, TypeAim: 0x97, TypePointer: 0x98, TypeAPM: 0x9a, 0x9a: 0x9d,
		TypeTick: 0xc2, TypeSync: 0xc4, TypeFireStart: 0xc9, TypeFireEnd: 0xca, TypeFireBlocked: 0xcb, 0xc8: 0xcc,
		TypeFire: 0xce, TypeStrut: 0xcf, TypeDeleteNode: 0xd0, TypeLink: 0xd1, TypeDeleteLink: 0xd2,
		TypeRepairLink: 0xd3, TypeRepair: 0xd4, TypeDoor: 0xd5, TypePlace: 0xd6, TypeRepairDevice: 0xd8,
		TypeUpgrade: 0xd9, TypeScrap: 0xda, TypePortalLink: 0xdb, TypePortalReverse: 0xdc, TypeAbility: 0xde,
		TypeFireAt: 0xdf, TypeAmmo: 0xe0, TypeRepairArea: 0xe4, TypeSnapshot: 0xe5, TypeScreenshot: 0xe6,
		TypeScriptCall: 0xea,
	}
	for typ, w := range want {
		if rt, ok := RecordType(typ, 1390); !ok || rt != w {
			t.Errorf("%#02x: %#02x, %v; want %#02x", typ, rt, ok, w)
		}
	}
	for _, typ := range []byte{0, 0x8d, 0x91, 0xb8, 0xba, TypeResult, 0xe7, 0xff} {
		if rt, ok := RecordType(typ, 1390); ok {
			t.Errorf("%#02x: %#02x in version 1390", typ, rt)
		}
	}
	for _, v := range []uint32{0, 1331, 1383, 1389, 1391} {
		for i := range 256 {
			if rt, ok := RecordType(byte(i), v); !ok || rt != byte(i) {
				t.Errorf("version %d: %#02x is %#02x, %v", v, i, rt, ok)
			}
		}
	}
}

// TestRegistry1390: the 1383 types that version 1390 moved or left
// unplaced have no decoder there.
func TestRegistry1390(t *testing.T) {
	r := DefaultRegistry()
	for _, typ := range []byte{TypeTick, 0xbf, TypeSync, TypeResult, TypeSnapshot, 0xe2} {
		if r.Lookup(typ, 1390) != nil {
			t.Errorf("%#02x: decoder in version 1390", typ)
		}
	}
}

// TestFixtures1390: every v1.39.0 replay decodes without an invalid record,
// with a snapshot per block, a sync every 2 frames up to the header's frame
// count, and raw records only of the types not decoded in 1383 either.
func TestFixtures1390(t *testing.T) {
	for _, name := range fixture.V1390 {
		hdr, blocks := fixture.Read1390(t, "..", name)
		raw := map[byte]int{}
		var syncs []uint32
		snaps := 0
		var d *Decoder
		for i, b := range blocks {
			if d == nil {
				d = NewDecoder(stream.New(b), DefaultRegistry(), hdr.Version, Options{})
			} else {
				d.Reset(stream.New(b))
			}
			for d.Next() {
				switch m := d.Message().(type) {
				case *Invalid:
					t.Errorf("%s block %d: %v", name, i, m.Err)
				case *Raw:
					raw[m.Type]++
				case *Sync:
					syncs = append(syncs, m.Frame)
				case *Snapshot:
					snaps++
				}
			}
			if err := d.Err(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if snaps != len(blocks) {
			t.Errorf("%s: %d snapshots in %d blocks", name, snaps, len(blocks))
		}
		delete(raw, 0x96) // the 1383 0x93
		delete(raw, 0xcc) // the 1383 0xc8
		if len(raw) > 0 {
			t.Errorf("%s: raw records %v", name, raw)
		}
		for i, f := range syncs {
			if f != uint32(2*i+2) {
				t.Errorf("%s: sync %d is frame %d", name, i, f)
				break
			}
		}
		if len(syncs) == 0 || syncs[len(syncs)-1] != hdr.Frames {
			t.Errorf("%s: %d syncs for %d frames", name, len(syncs), hdr.Frames)
		}
	}
}
