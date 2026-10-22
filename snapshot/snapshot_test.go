// SPDX-License-Identifier: AGPL-3.0-or-later

package snapshot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/corpus"
	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/lua"
	"github.com/an0nfunc/forts-replay/stream"
)

// snapshotsIn returns the 0xe1 payloads in blocks.
func snapshotsIn(blocks [][]byte) [][]byte {
	var out [][]byte
	for _, b := range blocks {
		s := stream.New(b)
		for s.Next() {
			if r := s.Record(); r.Type == 0xe1 {
				out = append(out, r.Payload)
			}
		}
	}
	return out
}

func section(name, text string) []byte {
	var b []byte
	if name != "" {
		b = binary.LittleEndian.AppendUint32(b, uint32(len(name)))
		b = append(b, name...)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(text)))
	return append(b, text...)
}

func TestParse(t *testing.T) {
	payload := bytes.Join([][]byte{
		{0xe1, 0, 0, 0, 2, 0, 0, 0},
		section("", "data = { gameFrame = 128, }"),
		{1, 0, 0, 0},
		section("dlc3", "data = { x = 1, }"),
		[]byte("data = without a length prefix"),
		{0xff, 0xff, 0xff, 0x7f}, []byte("data = bogus length"),
	}, nil)
	s := Parse(payload, lua.Limits{})
	if len(s.Sections) != 2 {
		t.Fatalf("%d sections", len(s.Sections))
	}
	if s.Sections[0].Name != "" || s.Sections[1].Name != "dlc3" {
		t.Errorf("names %q %q", s.Sections[0].Name, s.Sections[1].Name)
	}
	data, _ := s.Sections[0].Table.Get("data")
	if v, ok := data.Table.Get("gameFrame"); !ok || v.Str != "128" {
		t.Errorf("gameFrame %+v", v)
	}
	if got := payload[s.Sections[1].Offset:][:7]; string(got) != "data = " {
		t.Errorf("offset points at %q", got)
	}
}

func TestBrokenSection(t *testing.T) {
	payload := bytes.Join([][]byte{
		section("", "data = { x = ?, }"),
		section("", "data = { y = 2, }"),
	}, nil)
	s := Parse(payload, lua.Limits{})
	if len(s.Sections) != 2 || !errors.Is(s.Sections[0].Err, lua.ErrSyntax) || s.Sections[0].Table != nil || s.Sections[1].Err != nil {
		t.Fatalf("%+v", s.Sections)
	}
}

func TestBudget(t *testing.T) {
	text := "data = { x = 1, }" // 17 bytes
	payload := bytes.Join([][]byte{section("", text), section("", text), section("", text)}, nil)
	s := Parse(payload, lua.Limits{MaxSize: 40})
	if len(s.Sections) != 3 || s.Sections[1].Err != nil || !errors.Is(s.Sections[2].Err, lua.ErrLimit) || s.Sections[2].Table != nil {
		t.Fatalf("%+v", s.Sections)
	}
}

func TestFixtures(t *testing.T) {
	cases := map[string][]string{
		"abyss-bot-278f.fwr": {"1", "128", "256"},
		"abyss-bot-974f.fwr": {"1", "128", "256", "384", "510", "636", "762", "888"},
	}
	wantNames := []string{"", "", "", "dlc_harpoon", "dlc3", "", ""}
	for file, wantFrames := range cases {
		var frames []string
		for i, p := range snapshotsIn(fixture.Blocks(t, "..", file)) {
			var names []string
			for j, sec := range Parse(p, lua.Limits{}).Sections {
				if sec.Err != nil {
					t.Fatalf("%s snapshot %d section %d: %v", file, i, j, sec.Err)
				}
				names = append(names, sec.Name)
				data, _ := sec.Table.Get("data")
				gf, ok := data.Table.Get("gameFrame")
				if !ok {
					t.Fatalf("%s snapshot %d section %d: no gameFrame", file, i, j)
				}
				if j == 0 {
					frames = append(frames, gf.Str)
				}
			}
			if !slices.Equal(names, wantNames) {
				t.Errorf("%s snapshot %d: names %q", file, i, names)
			}
		}
		if !slices.Equal(frames, wantFrames) {
			t.Errorf("%s: frames %v, want %v", file, frames, wantFrames)
		}
	}
}

func TestSectionCap(t *testing.T) {
	payload := bytes.Repeat(section("", "data = "), 100000)
	secs := Parse(payload, lua.Limits{}).Sections
	if len(secs) != maxSections+1 || !errors.Is(secs[maxSections].Err, lua.ErrLimit) {
		t.Fatalf("%d sections, last err %v", len(secs), secs[len(secs)-1].Err)
	}
	if end, _ := End(payload); end != secs[maxSections].Offset+len(secs[maxSections].Text) {
		t.Errorf("End %d after %d sections", end, len(secs))
	}
}

// TestEnd checks End against Parse on every v1.38.2 corpus snapshot.
func TestEnd(t *testing.T) {
	corpus.Require(t, "..")
	n := 0
	err := corpus.Walk("..", func(_, rel string) error {
		name, ok := strings.CutPrefix(rel, "v1.38.2/")
		if !ok {
			return nil
		}
		for i, p := range snapshotsIn(fixture.Blocks(t, "..", name)) {
			secs := Parse(p, lua.Limits{}).Sections
			if len(secs) == 0 {
				t.Fatalf("%s snapshot %d: no section", rel, i)
			}
			last := secs[len(secs)-1]
			if end, ok := End(p); !ok || end != last.Offset+len(last.Text) {
				t.Errorf("%s snapshot %d: End %d, %v; last section ends at %d", rel, i, end, ok, last.Offset+len(last.Text))
			}
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no snapshots")
	}
	for _, p := range [][]byte{nil, {0xe1, 1, 2, 3}, []byte("\x00\x01\x00\x00data = ")} {
		if end, ok := End(p); ok {
			t.Errorf("%q: End %d", p, end)
		}
	}
}
