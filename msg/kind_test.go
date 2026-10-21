// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import "testing"

func TestKindNames(t *testing.T) {
	r := DefaultRegistry()
	seen := map[string]byte{}
	for i := range 256 {
		typ := byte(i)
		switch typ {
		case TypeSnapshot, TypeTick, TypeSync:
			continue
		}
		name := KindName(typ)
		if r.Lookup(typ, 1383) != nil && name == "" {
			t.Errorf("type %#02x has a decoder and no kind name", typ)
		}
		if prev, ok := seen[name]; ok && name != "" {
			t.Errorf("types %#02x and %#02x are both %q", prev, typ, name)
		}
		seen[name] = typ
	}
}
