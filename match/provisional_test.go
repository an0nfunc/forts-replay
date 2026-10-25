// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/internal/corpus"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

const (
	passed1400 = "provisional: header version 1400 is not known to this parser; decoded with the 1390 layout"
	failed1400 = "provisional: header version 1400 did not decode with the 1390 layout: "
)

func TestLayout(t *testing.T) {
	cases := []struct {
		v, layout       uint32
		provisional, ok bool
	}{
		{1383, 1383, false, true},
		{1390, 1390, false, true},
		{1400, 1390, true, true},
		{1386, 1383, true, true},
		{1331, 0, false, false},
		{0, 0, false, false},
	}
	for _, c := range cases {
		if l, p, ok := Layout(c.v); l != c.layout || p != c.provisional || ok != c.ok {
			t.Errorf("Layout(%d) = %d, %v, %v; want %d, %v, %v", c.v, l, p, ok, c.layout, c.provisional, c.ok)
		}
	}
}

// eachReplay calls fn with every replay of the public corpus and, when
// FWR_CORPUS is set, every replay under that directory, as TestCorpus
// reads them.
func eachReplay(t *testing.T, fn func(rel string, data []byte)) {
	t.Helper()
	corpus.Require(t, "..")
	read := func(path, rel string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fn(rel, data)
		return nil
	}
	if err := corpus.Walk("..", read); err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("FWR_CORPUS")
	if root == "" {
		return
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // G703: the developer names the corpus in FWR_CORPUS
		if err != nil || d.IsDir() || filepath.Ext(path) != ".fwr" {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return read(path, "$FWR_CORPUS/"+rel)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func version(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

// relabel returns a copy of data with header version v.
func relabel(data []byte, v uint32) []byte {
	out := slices.Clone(data)
	binary.LittleEndian.PutUint32(out, v)
	return out
}

// decodeAt decodes data as header version v alone, with no provisional
// layout.
func decodeAt(t *testing.T, data []byte, v uint32) *builder {
	t.Helper()
	cr, err := container.NewReader(bufio.NewReader(bytes.NewReader(data)), container.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	bs, err := decode(t.Context(), cr, Options{}, v)
	if err != nil {
		t.Fatal(err)
	}
	return bs[0]
}

// TestLayoutChecks: every replay at 1383 and 1390 passes the checks of a
// provisional decode at its own version.
func TestLayoutChecks(t *testing.T) {
	n := 0
	eachReplay(t, func(rel string, data []byte) {
		v := version(data)
		if v != 1383 && v != 1390 {
			return
		}
		n++
		if failed := decodeAt(t, data, v).layoutCheck(); failed != "" {
			t.Errorf("%s: %s", rel, failed)
		}
	})
	t.Logf("%d replays", n)
}

// TestProvisional1390: a 1390 replay relabelled as 1400 decodes with the
// 1390 layout, and its summary is the 1390 one but for format_version and
// the warning. Observe sees the 1390 syncs.
func TestProvisional1390(t *testing.T) {
	n := 0
	eachReplay(t, func(rel string, data []byte) {
		if version(data) != 1390 {
			return
		}
		n++
		var observed [2]int
		build := func(i int, data []byte) *Summary {
			s, err := Build(t.Context(), bytes.NewReader(data), Options{Observe: func(_ stream.Record, _ int64, m msg.Message) {
				if _, ok := m.(*msg.Sync); ok {
					observed[i]++
				}
			}})
			if err != nil {
				t.Fatalf("%s: %v", rel, err)
			}
			return s
		}
		want := build(0, data)
		want.FormatVersion = 1400
		want.Warnings = append(want.Warnings, passed1400)
		if got := build(1, relabel(data, 1400)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: warnings %q, %d players; want warnings %q, %d players", rel, got.Warnings, len(got.Players), want.Warnings, len(want.Players))
		}
		if observed[0] != observed[1] {
			t.Errorf("%s: Observe saw %d syncs, %d at 1390", rel, observed[1], observed[0])
		}
	})
	t.Logf("%d replays", n)
}

// TestProvisional1383: a 1383 replay relabelled as 1400 fails a check, and
// its summary is the one an unknown version gets, with no players and no
// map, plus the warning.
func TestProvisional1383(t *testing.T) {
	n := 0
	eachReplay(t, func(rel string, data []byte) {
		if version(data) != 1383 {
			return
		}
		n++
		data = relabel(data, 1400)
		got := buildFile(t, data)
		want := decodeAt(t, data, 1400).sum
		if len(got.Warnings) > 0 {
			want.Warnings = append(want.Warnings, got.Warnings[len(got.Warnings)-1])
		}
		if !reflect.DeepEqual(got, want) || got.Players != nil || got.Map != "" ||
			!slices.Contains(got.Warnings, "header fields not decoded for version 1400") ||
			!strings.HasPrefix(got.Warnings[len(got.Warnings)-1], failed1400) {
			t.Errorf("%s: %d players, map %q, warnings %q", rel, len(got.Players), got.Map, got.Warnings)
		}
	})
	t.Logf("%d replays", n)
}

// at1400 returns a replay of header version 1400 with frames frames, the
// header of setup s in the 1390 layout, and one block of recs.
func at1400(s fwrtest.Setup, frames uint32, recs ...[]byte) []byte {
	s.Date = "20261023_195552"
	var b [][]byte
	for i, p := range recs {
		b = append(b, fwrtest.Record(p, i == 0))
	}
	return fwrtest.File(fwrtest.Header{Version: 1400, Frames: frames, Extra: s.Extra1390()}, bytes.Join(b, nil))
}

// steps returns a tick and a sync in version 1390 for each frame.
func steps(frames ...uint32) [][]byte {
	var recs [][]byte
	for _, f := range frames {
		recs = append(recs, fwrtest.In1390(fwrtest.Tick(f+2)), fwrtest.In1390(fwrtest.Sync(f)))
	}
	return recs
}

// TestProvisionalChecks: each check alone turns a provisional decode back
// into the summary of an unknown version.
func TestProvisionalChecks(t *testing.T) {
	valid := steps(2, 4)
	noSteamIDs := duel(false)
	for i := range noSteamIDs.Players {
		noSteamIDs.Players[i].SteamID = 0
	}
	badCommander := duel(false)
	badCommander.Commanders[0] = "commander-\x01"
	slot16 := fwrtest.Sync(4)
	slot16[28+8*16] = 1
	cases := []struct {
		name   string
		data   []byte
		failed string
	}{
		{"zero SteamIDs", at1400(noSteamIDs, 4, valid...), "player 1 has no SteamID"},
		{"commander", at1400(badCommander, 4, valid...), "commander names do not decode"},
		{"renumbered", at1400(duel(false), 4, fwrtest.Tick(4), fwrtest.Sync(2), fwrtest.Tick(6), fwrtest.Sync(4)), "no sync decoded"},
		{"first sync", at1400(duel(false), 4, steps(4)...), "first sync is frame 4"},
		{"tick target", at1400(duel(false), 2, fwrtest.In1390(fwrtest.Tick(5)), fwrtest.In1390(fwrtest.Sync(2))), "tick to frame 5 before the sync of frame 2"},
		{"ticks", at1400(duel(false), 2, fwrtest.In1390(fwrtest.Tick(4)), fwrtest.In1390(fwrtest.Tick(6)), fwrtest.In1390(fwrtest.Sync(2))), "ticks to frames 4 and 6 before one sync"},
		{"last sync", at1400(duel(false), 8, valid...), "header has 8 frames, last sync is frame 4"},
		{"slot 16", at1400(duel(false), 4, append(steps(2), fwrtest.In1390(fwrtest.Tick(6)), fwrtest.In1390(slot16))...), "sync of frame 4 has slot 16 set"},
		{"wrong length", at1400(duel(false), 4, append(valid, fwrtest.In1390(append(fwrtest.Ability(2), 0)))...), "records decoded as invalid: 1"},
	}
	for _, c := range cases {
		got := buildFile(t, c.data)
		want := decodeAt(t, c.data, 1400).sum
		want.Warnings = append(want.Warnings, failed1400+c.failed)
		if !reflect.DeepEqual(got, want) || got.Players != nil || got.Map != "" {
			t.Errorf("%s: %d players, map %q, warnings %q", c.name, len(got.Players), got.Map, got.Warnings)
		}
	}

	// The same records under version 1390 give the summary a provisional
	// decode keeps.
	data := at1400(duel(false), 4, valid...)
	want := buildFile(t, relabel(data, 1390))
	want.FormatVersion = 1400
	want.Warnings = append(want.Warnings, passed1400)
	if got := buildFile(t, data); !reflect.DeepEqual(got, want) || len(got.Players) != 2 {
		t.Errorf("valid: got  %+v\nwant %+v", got, want)
	}
	// Decoding that stops early needs no last sync at the frame count.
	truncated := append(at1400(duel(false), 8, valid...), fwrtest.Block(fwrtest.Record(fwrtest.In1390(fwrtest.Tick(8)), true))[:20]...)
	if s := buildFile(t, truncated); !s.Truncated || len(s.Players) != 2 || !slices.Contains(s.Warnings, passed1400) {
		t.Errorf("truncated: truncated %v, %d players, warnings %q", s.Truncated, len(s.Players), s.Warnings)
	}
}
