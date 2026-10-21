// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import "testing"

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
