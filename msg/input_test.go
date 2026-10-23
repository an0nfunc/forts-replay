// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/stream"
)

// inputRec returns an input record of n bytes: type t, byte 17 b17, player 1.
func inputRec(t byte, n int, b17 byte) []byte {
	p := make([]byte, n)
	p[0], p[17] = t, b17
	binary.LittleEndian.PutUint32(p[20:], 1)
	return p
}

type framed struct {
	frame int64
	m     Message
}

// fixtureMessages decodes one sample replay and returns every message with
// its frame.
func fixtureMessages(t *testing.T, name string) []framed {
	t.Helper()
	hdr, blocks := fixture.Read(t, "..", name)
	var out []framed
	var d *Decoder
	for _, b := range blocks {
		if d == nil {
			d = NewDecoder(stream.New(b), DefaultRegistry(), hdr.Version, Options{})
		} else {
			d.Reset(stream.New(b))
		}
		for d.Next() {
			out = append(out, framed{d.Frame(), d.Message()})
		}
		if err := d.Err(); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// firstAt returns the first message of type typ at frame, or nil.
func firstAt(ms []framed, frame int64, typ byte) Message {
	for _, f := range ms {
		if f.frame == frame && f.m.MsgType() == typ {
			return f.m
		}
	}
	return nil
}

// TestLayouts pins each typed record's length, byte 17 and version range,
// in version 1383 and at its version 1390 type.
func TestLayouts(t *testing.T) {
	layouts := []struct {
		t   byte
		n   int
		b17 byte
		alt byte // the other accepted byte 17, or b17 again
	}{
		{TypeHover, 53, TypeHover - 1, TypeHover},
		{TypePointer, 37, TypePointer - 1, TypePointer},
		{TypeAPM, 36, TypeAPM, TypeAPM},
		{TypeFireStart, 45, TypeFireStart, TypeFireStart},
		{TypeFireEnd, 28, TypeFireEnd, TypeFireEnd},
		{TypeFireBlocked, 33, TypeFireBlocked, TypeFireBlocked},
		{TypeFire, 51, TypeFire, TypeFire},
		{TypeStrut, 74, TypeStrut, TypeStrut},
		{TypeDeleteNode, 33, TypeDeleteNode, TypeDeleteNode},
		{TypeLink, 50, TypeLink, TypeLink},
		{TypeDeleteLink, 41, TypeDeleteLink, TypeDeleteLink},
		{TypeRepair, 32, TypeRepair, TypeRepair},
		{TypeDoor, 44, TypeDoor, TypeDoor},
		{TypePlace, 67, TypePlace, TypePlace},
		{TypeUpgrade, 36, TypeUpgrade, TypeUpgrade},
		{TypeScrap, 34, TypeScrap, TypeScrap},
		{TypeAbility, 29, TypeAbility, TypeAbility},
		{TypeResult, 85, TypeResult, TypeResult},
		{TypeRepairLink, 36, TypeRepairLink, TypeRepairLink},
		{TypeRepairDevice, 32, TypeRepairDevice, TypeRepairDevice},
		{TypeRepairArea, 44, TypeRepairArea, TypeRepairArea},
		{TypePortalLink, 40, TypePortalLink, TypePortalLink},
		{TypePortalReverse, 32, TypePortalReverse, TypePortalReverse},
		{TypeFireAt, 52, TypeFireAt, TypeFireAt},
		{TypeAmmo, 32, TypeAmmo, TypeAmmo},
		{TypeAim, 37, TypeAim, TypeAim},
		{TypeScriptCall, 344, TypeScriptCall, TypeScriptCall},
	}
	r := DefaultRegistry()
	for _, v := range []uint32{1383, 1390} {
		for _, l := range layouts {
			rt, ok := RecordType(l.t, v)
			if !ok {
				continue // the ranked result in 1390, see TestResult
			}
			checkLayout(t, r, v, rt, l.t, l.n, rt-(l.t-l.b17), rt-(l.t-l.alt))
		}
	}
}

// checkLayout checks the decoder of record type rt in version v: it decodes
// records of n bytes with byte 17 b17 or alt to a message of type typ, and
// only those.
func checkLayout(t *testing.T, r *Registry, v uint32, rt, typ byte, n int, b17, alt byte) {
	t.Helper()
	for _, other := range []uint32{v - 1, v + 1} {
		if r.Lookup(rt, other) != nil {
			t.Errorf("%d %#02x: decoder at version %d", v, rt, other)
		}
	}
	dec := r.Lookup(rt, v)
	if dec == nil {
		t.Fatalf("%d %#02x: no decoder", v, rt)
	}
	m, err := dec(inputRec(rt, n, b17))
	if err != nil || m.MsgType() != typ {
		t.Errorf("%d %#02x: %#v, %v", v, rt, m, err)
		return
	}
	if s, ok := m.(interface{ Sender() uint32 }); !ok || s.Sender() != 1 {
		t.Errorf("%d %#02x: sender of %#v", v, rt, m)
	}
	for _, l := range []int{n - 1, n + 1} {
		if _, err := dec(inputRec(rt, l, b17)); err == nil {
			t.Errorf("%d %#02x: length %d decodes", v, rt, l)
		}
	}
	for i := range 256 {
		c := byte(i)
		want := c == b17 || c == alt
		if _, err := dec(inputRec(rt, n, c)); (err == nil) != want {
			t.Errorf("%d %#02x: byte 17 %#02x: err %v", v, rt, c, err)
		}
	}
	// dump writes every typed message as JSON, which has no NaN.
	for off := 18; off+4 <= n; off++ {
		p := inputRec(rt, n, b17)
		binary.LittleEndian.PutUint32(p[off:], 0x7fc00000)
		got, decErr := dec(p)
		if decErr != nil {
			continue
		}
		if _, jsonErr := json.Marshal(got); jsonErr != nil {
			t.Errorf("%d %#02x: NaN at offset %d decodes: %v", v, rt, off, jsonErr)
		}
	}
}

func TestInputFixtures(t *testing.T) {
	p1 := Input{Player: 1}
	cases := []struct {
		file  string
		frame int64
		want  Message
	}{
		{"abyss-bot-278f.fwr", 2, &Pointer{Input: p1, X: 45.78173828125, Y: 519.986572265625}},
		{"abyss-bot-278f.fwr", 2, &Hover{Input: p1, Terrain: 0xffff, BuildType: 0xff}},
		{"abyss-bot-278f.fwr", 228, &APM{Input: p1, Rate: 10}},
		{"abyss-bot-278f.fwr", 240, &APM{Input: p1, Rate: 20}},
		{"abyss-bot-974f.fwr", 46, &Hover{Input: p1, NodeA: 287, NodeB: 313, Terrain: 0xffff, HoverKind: 6, BuildType: 0xff}},
		{"abyss-bot-974f.fwr", 150, &Hover{Input: p1, NodeA: 290, Terrain: 0xffff, HoverKind: 1, Material: 1, BuildType: 0xff, Buttons: 1}},
	}
	msgs := map[string][]framed{}
	for _, c := range cases {
		if msgs[c.file] == nil {
			msgs[c.file] = fixtureMessages(t, c.file)
		}
		if got := firstAt(msgs[c.file], c.frame, c.want.MsgType()); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s frame %d: %#v, want %#v", c.file, c.frame, got, c.want)
		}
	}
}

// TestAimFixture: the swarm launcher's aim point while it is selected.
func TestAimFixture(t *testing.T) {
	got := firstAt(fixtureMessages(t, "swarm-missiles.fwr"), 728, TypeAim)
	if want := (&Aim{Player: 1, X: -2279.37109375, Y: -1234.748046875}); !reflect.DeepEqual(got, want) {
		t.Errorf("%#v, want %#v", got, want)
	}
	if _, ok := got.(Command); ok {
		t.Error("Aim is a Command")
	}
}

func TestInputInvalid(t *testing.T) {
	setF := func(p []byte, off int, v float64) []byte {
		binary.LittleEndian.PutUint32(p[off:], math.Float32bits(float32(v)))
		return p
	}
	cases := map[string][]byte{
		"NaN x":      setF(inputRec(TypePointer, 37, TypePointer-1), 28, math.NaN()),
		"infinite y": setF(inputRec(TypePointer, 37, TypePointer-1), 32, math.Inf(1)),
		"NaN rate":   setF(inputRec(TypeAPM, 36, TypeAPM), 28, math.NaN()),
		"NaN aim x":  setF(inputRec(TypeAim, 37, TypeAim), 28, math.NaN()),
	}
	for name, p := range cases {
		data := bytes.Join([][]byte{fwrtest.Record(fixed(TypeSync, 164, 4), false), fwrtest.Record(p, false)}, nil)
		d := NewDecoder(stream.New(data), DefaultRegistry(), 1383, Options{})
		d.Next()
		if !d.Next() {
			t.Fatalf("%s: %v", name, d.Err())
		}
		if _, ok := d.Message().(*Invalid); !ok || d.Frame() != 4 {
			t.Errorf("%s: %T at frame %d", name, d.Message(), d.Frame())
		}
	}
}

// TestInputMultiplayer: bytes 1 to 16 may be non-zero, byte 17 the plain
// type and the player field other than 1 in games with two humans.
func TestInputMultiplayer(t *testing.T) {
	p := inputRec(TypePointer, 37, TypePointer)
	for i := 1; i <= 16; i++ {
		p[i] = byte(i)
	}
	binary.LittleEndian.PutUint32(p[20:], 2)
	m, err := DefaultRegistry().Lookup(TypePointer, 1383)(p)
	if err != nil || !reflect.DeepEqual(m, &Pointer{Player: 2}) {
		t.Errorf("%#v, %v", m, err)
	}
}
