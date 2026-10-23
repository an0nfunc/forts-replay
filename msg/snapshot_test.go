// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import "testing"

func TestDefaultRegistrySnapshot(t *testing.T) {
	fn := DefaultRegistry().Lookup(TypeSnapshot, 1383)
	if fn == nil {
		t.Fatal("no snapshot decoder")
	}
	m, err := fn([]byte{0xe1, 1})
	if s, ok := m.(*Snapshot); err != nil || !ok || len(s.Payload) != 2 || s.MsgType() != 0xe1 {
		t.Errorf("%#v %v", m, err)
	}
}

// TestSnapshotTypes: snapshots are 0xe5 in version 1390 and 0xe1 in every
// other version.
func TestSnapshotTypes(t *testing.T) {
	r := DefaultRegistry()
	for _, v := range []uint32{0, 1331, 1383, 1389, 1391} {
		if r.Lookup(0xe1, v) == nil || r.Lookup(0xe5, v) != nil {
			t.Errorf("version %d: snapshot decoders at 0xe1 and 0xe5: %v, %v", v, r.Lookup(0xe1, v) != nil, r.Lookup(0xe5, v) != nil)
		}
	}
	fn := r.Lookup(0xe5, 1390)
	if fn == nil || r.Lookup(0xe1, 1390) != nil {
		t.Fatal("version 1390: no snapshot decoder at 0xe5, or one at 0xe1")
	}
	if m, err := fn([]byte{0xe5, 1}); err != nil || m.MsgType() != TypeSnapshot {
		t.Errorf("%#v %v", m, err)
	}
}
