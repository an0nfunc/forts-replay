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
