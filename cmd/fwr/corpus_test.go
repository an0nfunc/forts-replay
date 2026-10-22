// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestCorpusSteamIDs runs dump --json on every replay under $FWR_CORPUS, a
// directory of replays outside the repository; CI does not set it. No payload
// it prints may hold 8 bytes whose upper four are a SteamID64's high half, and
// no field or payload text a SteamID64 in decimal: script calls (0xe6) carry
// free text, printed as recorded.
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
		found, decimal := 0, 0
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
			delete(doc, "payload")
			fields, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			decimal += decimalSteamIDs(string(fields)) + decimalSteamIDs(string(p))
		}
		if found > 0 || decimal > 0 {
			t.Errorf("%s: %d SteamID-shaped windows, %d decimal SteamIDs", rel, found, decimal)
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d replays", n)
}

var digitRun = regexp.MustCompile(`[0-9]{17,}`)

// decimalSteamIDs counts the 17-digit windows of s that read as a SteamID64:
// a number whose upper 32 bits are 0x01100001.
func decimalSteamIDs(s string) int {
	n := 0
	for _, run := range digitRun.FindAllString(s, -1) {
		for i := 0; i+17 <= len(run); i++ {
			if id, err := strconv.ParseUint(run[i:i+17], 10, 64); err == nil && id>>32 == 0x01100001 {
				n++
			}
		}
	}
	return n
}

func TestDecimalSteamIDs(t *testing.T) {
	for s, want := range map[string]int{
		"76561197960265728":                      1, // the lowest
		"76561202255233023":                      1, // the highest: upper half 0x01100001, digits 765612
		`"args":"1, 76561198000000000, true"`:    1,
		"x76561198000000000y":                    1,
		"12345678901234567":                      0,
		"7656119800000000":                       0, // 16 digits
		"00000000000000000000000000000000000000": 0,
	} {
		if got := decimalSteamIDs(s); got != want {
			t.Errorf("%q: %d, want %d", s, got, want)
		}
	}
}
