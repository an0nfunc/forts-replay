// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"os"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fwrtest"
)

func FuzzReader(f *testing.F) {
	for _, name := range []string{"abyss-bot-278f.fwr", "abyss-bot-974f.fwr"} {
		data, err := os.ReadFile(fixtures + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add(fwrtest.File(fwrtest.Header{Version: 1}, []byte("a"), []byte("bb")))
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := NewReader(bytes.NewReader(data), Limits{MaxBlock: 1 << 20, MaxTotal: 4 << 20})
		if err != nil {
			return
		}
		n := 0
		for r.NextBlock() {
			n++
		}
		if r.Err() == nil && n == 0 {
			t.Fatal("no blocks and no error")
		}
	})
}
