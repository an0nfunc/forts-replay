// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/corpus"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

var update = flag.Bool("update", false, "rewrite testdata/corpus.golden")

// corpusDigest is the line testdata/corpus.golden holds for one replay: the
// summary without player names, SteamIDs and commands, plus record counts.
type corpusDigest struct {
	Path          string                    `json:"path"`
	Error         string                    `json:"error,omitempty"`
	FormatVersion uint32                    `json:"format_version,omitempty"`
	Frames        uint32                    `json:"frames,omitempty"`
	Ranked        bool                      `json:"ranked,omitempty"`
	Map           string                    `json:"map,omitempty"`
	Options       []string                  `json:"options,omitempty"`
	Players       []digestPlayer            `json:"players,omitempty"`
	AI            []AI                      `json:"ai,omitempty"`
	Commanders    []Commander               `json:"commanders,omitempty"`
	Result        *Result                   `json:"result,omitempty"`
	Actions       map[uint32]map[string]int `json:"actions,omitempty"` // by player ID, then kind
	Blocks        int                       `json:"blocks,omitempty"`
	Records       map[string]int            `json:"records,omitempty"` // by record type
	Invalid       int                       `json:"invalid,omitempty"`
	Truncated     bool                      `json:"truncated,omitempty"`
	Warnings      []string                  `json:"warnings,omitempty"`
}

type digestPlayer struct {
	ID       uint32 `json:"id"`
	Slot     uint32 `json:"slot"`
	Team     int32  `json:"team"`
	Side     int    `json:"side"`
	Named    bool   `json:"named,omitempty"`
	SteamID  bool   `json:"steam_id,omitempty"`
	Recorder bool   `json:"recorder,omitempty"`
}

// TestCorpusGolden builds every replay of the public corpus, the submodule
// testdata/corpus, and compares its digest with testdata/corpus.golden.
func TestCorpusGolden(t *testing.T) {
	corpus.Require(t, "..")
	golden := filepath.Join("..", "testdata", "corpus.golden")
	var got bytes.Buffer
	err := corpus.Walk("..", func(path, rel string) error {
		line, err := json.Marshal(digest(t, path, rel))
		if err != nil {
			return err
		}
		got.Write(line)
		got.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err = os.WriteFile(golden, got.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	wantBy, gotBy := linesByPath(t, want), linesByPath(t, got.Bytes())
	all := maps.Clone(wantBy)
	maps.Copy(all, gotBy)
	for _, p := range slices.Sorted(maps.Keys(all)) {
		switch w, g := wantBy[p], gotBy[p]; {
		case g == "":
			t.Errorf("%s: in the golden file, not in the corpus", p)
		case w == "":
			t.Errorf("%s: not in the golden file", p)
		case w != g:
			t.Errorf("%s: digest differs", p)
		}
	}
	if t.Failed() {
		t.Log("rerun with -update and review the diff of testdata/corpus.golden")
	}
}

// digest builds the replay at path and returns its digest under the name
// rel.
func digest(t *testing.T, path, rel string) corpusDigest {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records := map[string]int{}
	invalid := 0
	s, err := Build(t.Context(), f, Options{Observe: func(rec stream.Record, _ int64, m msg.Message) {
		records[fmt.Sprintf("0x%02x", rec.Type)]++
		if _, ok := m.(*msg.Invalid); ok {
			invalid++
		}
	}})
	if err != nil {
		return corpusDigest{Path: rel, Error: err.Error(), Invalid: invalid}
	}
	d := corpusDigest{
		Path:          rel,
		FormatVersion: s.FormatVersion,
		Frames:        s.Frames,
		Ranked:        s.Ranked,
		Map:           s.Map,
		Options:       s.Options,
		AI:            s.AI,
		Commanders:    s.Commanders,
		Result:        s.Result,
		Actions:       map[uint32]map[string]int{},
		Blocks:        s.Blocks,
		Records:       records,
		Invalid:       invalid,
		Truncated:     s.Truncated,
		Warnings:      s.Warnings,
	}
	for _, p := range s.Players {
		d.Players = append(d.Players, digestPlayer{
			ID:       p.ID,
			Slot:     p.Slot,
			Team:     p.Team,
			Side:     p.Side,
			Named:    p.Name != "",
			SteamID:  p.SteamID != 0,
			Recorder: p.Recorder,
		})
	}
	for _, tl := range s.Timelines {
		kinds := map[string]int{}
		for typ, n := range tl.Counts {
			kinds[msg.KindName(typ)] = n
		}
		d.Actions[tl.ID] = kinds
	}
	return d
}

// linesByPath indexes digest lines by their replay path.
func linesByPath(t *testing.T, b []byte) map[string]string {
	t.Helper()
	m := map[string]string{}
	for line := range strings.Lines(string(b)) {
		var d struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("golden line %q: %v", line, err)
		}
		m[d.Path] = line
	}
	return m
}
