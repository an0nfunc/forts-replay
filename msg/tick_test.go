// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/stream"
)

// fixed returns a tick or sync payload of n >= 28 bytes carrying frame at offset 24.
func fixed(t byte, n int, frame uint32) []byte {
	p := make([]byte, n)
	p[0], p[17] = t, t
	binary.LittleEndian.PutUint32(p[24:], frame)
	return p
}

func TestTickSync(t *testing.T) {
	sync := fixed(TypeSync, 164, 38)
	for i := range 17 {
		binary.LittleEndian.PutUint64(sync[28+8*i:], uint64(i)<<56|uint64(i))
	}
	data := bytes.Join([][]byte{fwrtest.Record(fixed(TypeTick, 28, 40), false), fwrtest.Record(sync, false)}, nil)
	d := NewDecoder(stream.New(data), DefaultRegistry(), 1383, Options{Strict: true})
	if !d.Next() {
		t.Fatal(d.Err())
	}
	if tk, ok := d.Message().(*Tick); !ok || tk.Target != 40 || tk.MsgType() != TypeTick || d.Frame() != -1 {
		t.Errorf("tick %#v at frame %d", d.Message(), d.Frame())
	}
	if !d.Next() {
		t.Fatal(d.Err())
	}
	s, ok := d.Message().(*Sync)
	if !ok || s.Frame != 38 || s.MsgType() != TypeSync || d.Frame() != 38 || s.Values[0] != 0 || s.Values[16] != 16<<56|16 {
		t.Errorf("sync %#v at frame %d", d.Message(), d.Frame())
	}
}

func TestTickSyncInvalid(t *testing.T) {
	retype := func(p []byte) []byte { p[17] = 0; return p }
	cases := map[string][]byte{
		"short tick":   fixed(TypeTick, 28, 6)[:27],
		"long tick":    append(fixed(TypeTick, 28, 6), 0),
		"short sync":   fixed(TypeSync, 164, 6)[:163],
		"long sync":    append(fixed(TypeSync, 164, 6), 0),
		"tick byte 17": retype(fixed(TypeTick, 28, 6)),
		"sync byte 17": retype(fixed(TypeSync, 164, 6)),
		"type only":    {TypeSync},
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

func TestTickSyncVersions(t *testing.T) {
	r := DefaultRegistry()
	for _, v := range []uint32{0, 1382, 1384} {
		if r.Lookup(TypeTick, v) != nil || r.Lookup(TypeSync, v) != nil {
			t.Errorf("version %d has tick or sync decoders", v)
		}
	}
	if r.Lookup(TypeSnapshot, 1384) == nil {
		t.Error("snapshot decoder missing at 1384")
	}
}
