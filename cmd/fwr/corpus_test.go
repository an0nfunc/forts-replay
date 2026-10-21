// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestCorpusSteamIDs runs dump --json on every replay under $FWR_CORPUS, a
// directory of replays outside the repository; CI does not set it. No payload
// it prints may hold 8 bytes whose upper four are a SteamID64's high half.
func TestCorpusSteamIDs(t *testing.T) {
	root := os.Getenv("FWR_CORPUS")
	if root == "" {
		t.Skip("FWR_CORPUS not set")
	}
	high := []byte{0x01, 0x00, 0x10, 0x01}
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // G703: the developer names the corpus in FWR_CORPUS
		if err != nil || d.IsDir() || filepath.Ext(path) != ".fwr" {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		code, out, _ := runCmd(t, "dump", "--json", path)
		if code != 0 {
			t.Errorf("%s: exit %d", rel, code)
			return nil
		}
		found := 0
		for _, doc := range jsonLines(t, out) {
			s, _ := doc["payload"].(string)
			p, err := hex.DecodeString(s)
			if err != nil {
				t.Errorf("%s: payload is not hex", rel)
				break
			}
			for i := 0; i+8 <= len(p); i++ {
				if bytes.Equal(p[i+4:i+8], high) {
					found++
				}
			}
		}
		if found > 0 {
			t.Errorf("%s: %d SteamID-shaped windows", rel, found)
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d replays", n)
}
