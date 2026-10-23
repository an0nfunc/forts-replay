// SPDX-License-Identifier: AGPL-3.0-or-later

package header

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/an0nfunc/forts-replay/internal/fixture"
)

func FuzzParse(f *testing.F) {
	for _, name := range fixture.Names {
		f.Add(fixtureRaw(f, name))
	}
	for _, name := range fixture.V1390[:2] {
		hdr, _ := fixture.Read1390(f, "..", name)
		f.Add(hdr.Raw)
	}
	f.Add(setupRaw(lobby(false)))
	f.Add(setupRaw(lobby(true)))
	ranked := lobby(true)
	ranked.Date = "20261023_195552"
	f.Add(setupRaw1390(ranked))
	f.Fuzz(func(t *testing.T, raw []byte) {
		checkParse(t, 1383, raw)
		checkParse(t, 1390, raw)
	})
}

// checkParse parses raw as a header of version v and checks the result.
func checkParse(t *testing.T, v uint32, raw []byte) {
	t.Helper()
	h, err := Parse(v, raw)
	if err != nil && !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err %v, want ErrCorrupt", err)
	}
	if h == nil {
		return
	}
	if err != nil && (h.Players != nil || h.AI != nil || h.Recorder != 0 || h.RecorderTeam != 0) {
		t.Fatalf("tail fields kept after %v: %+v", err, h)
	}
	if h.Rest > len(raw) || len(h.Options) > maxOptions || len(h.Players) > maxPlayers || len(h.AI) > maxAI {
		t.Fatalf("rest %d of %d, %d options, %d players, %d AI", h.Rest, len(raw), len(h.Options), len(h.Players), len(h.AI))
	}
	rest := optionsOffset + 4
	if h.Ranked {
		rest += rankedLen
	}
	if v == 1390 {
		if len(h.Date) != dateLen {
			t.Fatalf("date %q", h.Date)
		}
		rest += dateShift
	}
	strs := append(slices.Clone(h.Options), h.Map, h.MapSPR)
	for _, a := range h.AI {
		strs = append(strs, a.Script)
	}
	for i, s := range strs {
		if i < len(h.Options)+2 {
			rest += 4 + len(s)
		}
		if len(s) > maxString || !utf8.ValidString(s) {
			t.Fatalf("bad string %q", s)
		}
		for _, c := range s {
			if !strconv.IsPrint(c) {
				t.Fatalf("unprintable %q in %q", c, s)
			}
		}
	}
	if h.Rest != rest {
		t.Fatalf("rest %d, strings end at %d", h.Rest, rest)
	}
	for _, p := range h.Players {
		if !utf8.ValidString(p.Name) || len(utf16.Encode([]rune(p.Name))) > nameUnits-1 {
			t.Fatalf("name %q", p.Name)
		}
	}
	checkMasked(t, h)
}

// checkMasked checks the fields decoded from masked bytes: the commanders
// and the SteamIDs.
func checkMasked(t *testing.T, h *Header) {
	t.Helper()
	if h.Commanders != nil && len(h.Commanders) != 2 {
		t.Fatalf("commanders %q", h.Commanders)
	}
	for _, c := range h.Commanders {
		if len(c) >= nameBlock || !utf8.ValidString(c) || strings.ContainsFunc(c, func(r rune) bool { return !strconv.IsPrint(r) }) {
			t.Fatalf("commander %q", c)
		}
	}
	for _, p := range h.Players {
		if p.SteamID != 0 && p.SteamID>>32 != steamIDHigh {
			t.Fatalf("SteamID %#x", p.SteamID)
		}
	}
}
