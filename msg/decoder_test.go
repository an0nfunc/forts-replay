// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/stream"
)

type known struct{}

func (known) MsgType() byte { return 0x10 }

type tick struct{}

func (tick) MsgType() byte         { return 0x30 }
func (tick) NextFrame(int64) int64 { return 10 }

func testRegistry() *Registry {
	r := NewRegistry()
	r.Register(0x10, Versions{}, func([]byte) (Message, error) { return known{}, nil })
	r.Register(0x20, Versions{}, func([]byte) (Message, error) { return nil, errors.New("bad payload") })
	r.Register(0x30, Versions{}, func([]byte) (Message, error) { return tick{}, nil })
	return r
}

func records(types ...byte) []byte {
	var parts [][]byte
	for i, t := range types {
		parts = append(parts, fwrtest.Record([]byte{t, byte(i)}, i == 0))
	}
	return bytes.Join(parts, nil)
}

func TestDecoder(t *testing.T) {
	d := NewDecoder(stream.New(records(0x10, 0x20, 0x30, 0x40)), testRegistry(), 1383, Options{})
	var kinds []string
	var frames []int64
	for d.Next() {
		kinds = append(kinds, fmt.Sprintf("%T", d.Message()))
		frames = append(frames, d.Frame())
	}
	if err := d.Err(); err != nil {
		t.Fatal(err)
	}
	wantKinds := []string{"msg.known", "*msg.Invalid", "msg.tick", "*msg.Raw"}
	if !slices.Equal(kinds, wantKinds) || !slices.Equal(frames, []int64{-1, -1, 10, 10}) {
		t.Errorf("kinds %v frames %v", kinds, frames)
	}
}

func TestInvalidKeepsPayload(t *testing.T) {
	d := NewDecoder(stream.New(records(0x20)), testRegistry(), 1383, Options{})
	if !d.Next() {
		t.Fatal(d.Err())
	}
	inv, ok := d.Message().(*Invalid)
	if !ok || inv.Raw.Type != 0x20 || !bytes.Equal(inv.Raw.Payload, []byte{0x20, 0}) || inv.Err == nil {
		t.Errorf("%#v", d.Message())
	}
}

func TestStrict(t *testing.T) {
	d := NewDecoder(stream.New(records(0x10, 0x40, 0x10)), testRegistry(), 1383, Options{Strict: true})
	n := 0
	for d.Next() {
		n++
	}
	if n != 1 || !errors.Is(d.Err(), ErrStrict) {
		t.Errorf("%d messages, err %v", n, d.Err())
	}
}

func TestResetKeepsFrame(t *testing.T) {
	d := NewDecoder(stream.New(records(0x30)), testRegistry(), 1383, Options{})
	for d.Next() {
	}
	d.Reset(stream.New(records(0x40)))
	if !d.Next() || d.Frame() != 10 {
		t.Errorf("frame %d after reset", d.Frame())
	}
}

func TestStreamError(t *testing.T) {
	d := NewDecoder(stream.New([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}), testRegistry(), 1383, Options{})
	if d.Next() || !errors.Is(d.Err(), stream.ErrCorrupt) {
		t.Errorf("err %v", d.Err())
	}
	d.Reset(stream.New(records(0x10)))
	if d.Next() {
		t.Error("decoder continued after a stream error")
	}
}
