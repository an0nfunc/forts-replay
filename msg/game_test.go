// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// result returns a ranked result record.
func result(sender, winner uint32, team int32, id, id2 uint64) []byte {
	p := make([]byte, 85)
	p[0], p[17] = TypeResult, TypeResult
	binary.LittleEndian.PutUint32(p[20:], sender)
	binary.LittleEndian.PutUint32(p[24:], winner)
	binary.LittleEndian.PutUint32(p[28:], uint32(team))
	binary.LittleEndian.PutUint64(p[32:], id)
	binary.LittleEndian.PutUint64(p[40:], id2)
	return p
}

// screenshot returns a screenshot record with w × h × depth bytes of
// pixels after its 45-byte head.
func screenshot(w, h, depth uint32, pixels int) []byte {
	p := make([]byte, 45+pixels)
	p[0], p[17], p[44] = TypeScreenshot, TypeScreenshot, 1
	binary.LittleEndian.PutUint32(p[24:], w)
	binary.LittleEndian.PutUint32(p[28:], h)
	binary.LittleEndian.PutUint32(p[32:], depth)
	binary.LittleEndian.PutUint32(p[36:], 0x1908)
	binary.LittleEndian.PutUint32(p[40:], 101)
	return p
}

func TestResult(t *testing.T) {
	const id = 1234567890123456789
	dec := DefaultRegistry().Lookup(TypeResult, 1383)
	m, err := dec(result(0, 2, 102, id, id))
	if err != nil || !reflect.DeepEqual(m, &Result{Winner: 2, Team: 102, SteamID: id}) {
		t.Errorf("%#v, %v", m, err)
	}
	m, err = dec(result(2, 2, 102, id, id))
	if err != nil || !reflect.DeepEqual(m, &Result{Player: 2, Winner: 2, Team: 102, SteamID: id}) {
		t.Errorf("another client's record: %#v, %v", m, err)
	}
	bad := map[string][]byte{
		"SteamIDs differ": result(0, 2, 102, id, id+1),
		"84 bytes":        result(0, 2, 102, id, id)[:84],
		"byte 17":         func() []byte { p := result(0, 2, 102, id, id); p[17] = 0; return p }(),
	}
	for name, p := range bad {
		if m, err := dec(p); err == nil {
			t.Errorf("%s: decodes to %#v", name, m)
		}
	}
	if DefaultRegistry().Lookup(TypeResult, 1384) != nil {
		t.Error("decoder at version 1384")
	}
	// The 1390 type is not known: no candidate decodes the 1383 layout.
	for _, typ := range []byte{TypeResult, 0xbe, 0xbf} {
		if fn := DefaultRegistry().Lookup(typ, 1390); fn != nil {
			p := result(0, 2, 102, id, id)
			p[0], p[17] = typ, typ
			m, err := fn(p)
			t.Errorf("version 1390, type %#02x: %#v, %v", typ, m, err)
		}
	}
}

func TestScreenshot(t *testing.T) {
	dec := DefaultRegistry().Lookup(TypeScreenshot, 1383)
	m, err := dec(screenshot(4, 2, 4, 32))
	if err != nil || !reflect.DeepEqual(m, &Screenshot{Width: 4, Height: 2, Depth: 4, Format: 0x1908, Frame: 101, Flag: true}) {
		t.Errorf("%#v, %v", m, err)
	}
	if _, err := dec(screenshot(4, 2, 3, 24)); err != nil {
		t.Errorf("depth 3: %v", err)
	}
	clear44 := screenshot(4, 2, 4, 32)
	clear44[44] = 0
	if m, err := dec(clear44); err != nil || m.(*Screenshot).Flag {
		t.Errorf("byte 44 is 0: %#v, %v", m, err)
	}
	if _, err := dec(screenshot(maxScreenshotSide, 1, 3, 3*maxScreenshotSide)); err != nil {
		t.Errorf("width %d: %v", maxScreenshotSide, err)
	}
	bad := map[string][]byte{
		"pixels short":     screenshot(4, 2, 4, 31),
		"pixels long":      screenshot(4, 2, 4, 33),
		"depth 2":          screenshot(4, 2, 2, 16),
		"width above max":  screenshot(maxScreenshotSide+1, 1, 3, 3*(maxScreenshotSide+1)),
		"overflowing size": screenshot(0xffffffff, 0xffffffff, 4, 0),
		// w × h × 4 is 2^64 + 32: only the side limit stops the product
		// from wrapping to the 32 pixel bytes present.
		"wraps to 32 bytes": screenshot(1824726041, 2527330632, 4, 32),
		"head only":         screenshot(4, 2, 4, 32)[:44],
		"byte 44 is 2":      func() []byte { p := screenshot(4, 2, 4, 32); p[44] = 2; return p }(),
	}
	for name, p := range bad {
		if m, err := dec(p); err == nil {
			t.Errorf("%s: decodes to %#v", name, m)
		}
	}
}

// TestScreenshot1390: type 0xe6 in version 1390, with the BGRA pixel
// format the game writes there.
func TestScreenshot1390(t *testing.T) {
	dec := DefaultRegistry().Lookup(0xe6, 1390)
	if dec == nil {
		t.Fatal("no decoder")
	}
	p := screenshot(4, 2, 4, 32)
	p[0], p[17] = 0xe6, 0xe6
	binary.LittleEndian.PutUint32(p[36:], 0x80e1)
	m, err := dec(p)
	if err != nil || !reflect.DeepEqual(m, &Screenshot{Width: 4, Height: 2, Depth: 4, Format: 0x80e1, Frame: 101, Flag: true}) || m.MsgType() != TypeScreenshot {
		t.Errorf("%#v, %v", m, err)
	}
	p[17] = TypeScreenshot
	if m, err := dec(p); err == nil {
		t.Errorf("byte 17 %#02x: decodes to %#v", p[17], m)
	}
}
