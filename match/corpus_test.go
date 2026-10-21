// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

// TestCorpus builds every replay under $FWR_CORPUS, a directory of replays
// outside the repository; CI does not set it. Replays of header version 1383
// must build without warnings or invalid records; in those with more than
// one player entry, every entry must hold a SteamID; ranked ones must carry
// the game's result and the recorder's SteamID.
func TestCorpus(t *testing.T) {
	root := os.Getenv("FWR_CORPUS")
	if root == "" {
		t.Skip("FWR_CORPUS not set")
	}
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // G703: the developer names the corpus in FWR_CORPUS
		if err != nil || d.IsDir() || filepath.Ext(path) != ".fwr" {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		f, err := os.Open(path) //nolint:gosec // G122: a local corpus the developer owns
		if err != nil {
			return err
		}
		defer f.Close()
		invalid := 0
		s, err := Build(t.Context(), f, Options{Observe: func(_ stream.Record, _ int64, m msg.Message) {
			if _, ok := m.(*msg.Invalid); ok {
				invalid++
			}
		}})
		switch {
		case err != nil:
			t.Errorf("%s: %v", rel, err)
			return nil
		case s.FormatVersion != 1383:
			return nil
		case len(s.Warnings) > 0 || invalid > 0:
			t.Errorf("%s: %d invalid, warnings %q", rel, invalid, s.Warnings)
		case len(s.Players) > 1 && slices.ContainsFunc(s.Players, func(p Player) bool { return p.SteamID == 0 }):
			t.Errorf("%s: a player entry without a SteamID", rel)
		case s.Ranked && (s.Result == nil || s.Result.Source != SourceRanked || !slices.ContainsFunc(s.Players, func(p Player) bool { return p.SteamID != 0 })):
			t.Errorf("%s: ranked without the game's result or a SteamID", rel)
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d replays", n)
}
