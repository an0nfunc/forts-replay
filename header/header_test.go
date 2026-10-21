// SPDX-License-Identifier: AGPL-3.0-or-later

package header

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
)

func fixtureRaw(tb testing.TB, name string) []byte {
	tb.Helper()
	hdr, _ := fixture.Read(tb, "..", name)
	return hdr.Raw
}

var (
	fixturePlayers = []Player{{Slot: 0, ID: 1, Team: 1, Name: "random🐧"}}
	skirmishAI     = []AI{{Team: 1}, {Team: 102, Script: "maps/Abyss/fort-t1f1-Abyss-ToGA1.lua", Value: 1}}
	// fixtureCommanders lists the fixtures with commanders; the sandbox
	// probes have none.
	fixtureCommanders = map[string][]string{
		"abyss-bot-278f.fwr":            {"commander-fb-fulminator", "commander-da-overclocker"},
		"abyss-bot-974f.fwr":            {"commander-fb-fulminator", "commander-da-builder"},
		"probe-downgrade-overdrive.fwr": {"commander-da-overclocker", "commander-ee-heavy-weapons"},
		"probe-move-phantom.fwr":        {"commander-cf-phantom", "commander-iba-turtle"},
	}
	// longName is a commander name of 49 bytes, the most a block holds.
	longName = "commander-äöü🐧-" + strings.Repeat("z", 49-len("commander-äöü🐧-"))
)

// testSteamID is a synthetic SteamID64 with account ID 0xffffffff, which
// Steam has not assigned; tests count down from it.
const testSteamID = 0x01100001ffffffff

func fixtureCommandersOf(name string) []string {
	if c, ok := fixtureCommanders[name]; ok {
		return c
	}
	return []string{"", ""}
}

func TestFixtures(t *testing.T) {
	want := Header{
		Options:      []string{"repair_station", "weapon_pack", "dlc1_portal_skirmish", "dlc1_weapons_skirmish", "dlc3"},
		Map:          "maps/Abyss/Abyss.fwe",
		MapSPR:       "maps/Abyss/Abyss.spr",
		Rest:         0x5fe,
		Recorder:     1,
		RecorderTeam: 1,
		Players:      fixturePlayers,
		AI:           skirmishAI,
	}
	for _, name := range fixture.Names {
		h, err := Parse(1383, fixtureRaw(t, name))
		want.Commanders = fixtureCommandersOf(name)
		if err != nil || !reflect.DeepEqual(h, &want) {
			t.Errorf("%s: %+v, %v", name, h, err)
		}
	}
	skirmish := map[string]bool{"probe-downgrade-overdrive.fwr": true, "probe-move-phantom.fwr": true}
	for _, name := range fixture.Probes {
		h, err := Parse(1383, fixtureRaw(t, name))
		var ai []AI
		if skirmish[name] {
			ai = skirmishAI
		}
		if err != nil || h.Ranked || h.Recorder != 1 || h.RecorderTeam != 1 || !reflect.DeepEqual(h.Players, fixturePlayers) || !reflect.DeepEqual(h.AI, ai) ||
			!reflect.DeepEqual(h.Commanders, fixtureCommandersOf(name)) {
			t.Errorf("%s: %+v, %v", name, h, err)
		}
	}
}

// setupRaw returns the header a Setup describes.
func setupRaw(s fwrtest.Setup) []byte {
	return fwrtest.File(fwrtest.Header{Version: 1383, Extra: s.Extra()})
}

// lobby is a 4v4 lobby game with commanders, a spectator, AI forts, images
// and custom teams.
func lobby(ranked bool) fwrtest.Setup {
	s := fwrtest.Setup{
		Ranked: ranked, Options: []string{"dlc3"}, Map: "maps/Test/Test.fwe",
		Recorder: 37, RecorderTeam: -3,
		AI:     []fwrtest.AI{{Team: 1}, {Team: 202, Script: "maps/Test/ai.lua", Value: 2}, {Team: 302, Value: 6}},
		Images: 3, CustomTeams: true, Commanders: [2]string{"commander-test-alpha", longName},
	}
	for i := range 8 {
		s.Players = append(s.Players, fwrtest.Player{
			Slot: uint32(i), ID: uint32(i + 1), Team: int32(100*(i/2+1) + i%2 + 1), Name: fmt.Sprintf("P%d", i), SteamID: testSteamID - uint64(i),
		})
	}
	s.Players = append(s.Players, fwrtest.Player{Slot: 8, ID: 37, Team: -3, Name: "ÄÖÜ🐧 fifteen u"})
	return s
}

func TestSetup(t *testing.T) {
	for _, ranked := range []bool{false, true} {
		s := lobby(ranked)
		raw := setupRaw(s)
		h, err := Parse(1383, raw)
		if err != nil {
			t.Fatalf("ranked %v: %v", ranked, err)
		}
		var players []Player
		for _, p := range s.Players {
			players = append(players, Player{Slot: p.Slot, ID: p.ID, Team: p.Team, Name: p.Name, SteamID: p.SteamID})
		}
		var ai []AI
		for _, a := range s.AI {
			ai = append(ai, AI{Team: a.Team, Script: a.Script, Value: a.Value})
		}
		rest := optionsOffset + 4 + 8 + 2*(4+len("maps/Test/Test.fwe"))
		if ranked {
			rest += rankedLen
		}
		want := &Header{
			Ranked: ranked, Commanders: s.Commanders[:], Options: []string{"dlc3"}, Map: "maps/Test/Test.fwe", MapSPR: "maps/Test/Test.spr",
			Rest: rest, Recorder: 37, RecorderTeam: -3, Players: players, AI: ai,
		}
		if !reflect.DeepEqual(h, want) {
			t.Errorf("ranked %v:\n got %+v\nwant %+v", ranked, h, want)
		}
	}
}

// TestTailCorrupt damages the fields after MapSPR: Parse keeps the fields
// up to MapSPR, the commanders among them, and drops the rest.
func TestTailCorrupt(t *testing.T) {
	s := lobby(false)
	s.Images, s.Players = 1, s.Players[:1]
	raw := setupRaw(s)
	h0, err := Parse(1383, raw)
	if err != nil {
		t.Fatal(err)
	}
	entry := h0.Rest + 27                             // A, B, flags, C, recorder, recorder team, n
	ai := entry + 101 + 4 + 8                         // the AI count, after a1a1a1a1 and two f32
	imageSize := bytes.Index(raw, []byte("<<<<")) + 4 // the image marker 0x3c3c3c3c, then its size
	put := func(off int, v uint32) []byte {
		b := slices.Clone(raw)
		binary.LittleEndian.PutUint32(b[off:], v)
		return b
	}
	noNUL := slices.Clone(raw)
	for i := entry + 12; i < entry+44; i++ {
		noNUL[i] = 0x41
	}
	cases := map[string][]byte{
		"ranked flag 2":       func() []byte { b := slices.Clone(raw); b[rankedOffset] = 2; return b }(),
		"17 players":          put(entry-4, maxPlayers+1),
		"one player too many": put(entry-4, 2),
		"name without NUL":    noNUL,
		"player marker":       put(entry+97, 0),
		"AI marker":           put(entry+101, 0),
		"17 AI forts":         put(ai, maxAI+1),
		"image size past end": put(imageSize, 1<<30),
		"one byte more":       append(slices.Clone(raw), 0),
		"one byte less":       raw[:len(raw)-1],
	}
	for name, b := range cases {
		h, err := Parse(1383, b)
		if !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: err %v", name, err)
			continue
		}
		if name == "ranked flag 2" {
			if h != nil {
				t.Errorf("%s: header %+v", name, h)
			}
			continue
		}
		if h == nil || h.Map != "maps/Test/Test.fwe" || !slices.Equal(h.Commanders, s.Commanders[:]) ||
			h.Players != nil || h.AI != nil || h.Recorder != 0 || h.RecorderTeam != 0 {
			t.Errorf("%s: %+v", name, h)
		}
	}
}

func TestVersion(t *testing.T) {
	raw := fixtureRaw(t, fixture.Names[0])
	for _, v := range []uint32{0, 1382, 1384} {
		if _, err := Parse(v, raw); !errors.Is(err, ErrVersion) {
			t.Errorf("version %d: err %v", v, err)
		}
	}
}

func TestCorrupt(t *testing.T) {
	raw := fixtureRaw(t, fixture.Names[0])
	put := func(off int, v uint32) []byte {
		b := slices.Clone(raw)
		binary.LittleEndian.PutUint32(b[off:], v)
		return b
	}
	set := func(off int, c byte) []byte {
		b := slices.Clone(raw)
		b[off] = c
		return b
	}
	cases := map[string][]byte{
		"ends before options":  raw[:optionsOffset+2],
		"too many options":     tail(raw, maxOptions+1, make([]string, maxOptions+3)...),
		"string above the cap": tail(raw, 1, strings.Repeat("a", maxString+1), "", ""),
		"string past the end":  put(optionsOffset+4, 1000), // 543 bytes follow
		"control byte":         set(optionsOffset+8, 0x1b),
		"invalid UTF-8":        set(optionsOffset+8, 0xff),
		"bidi override":        tail(raw, 1, "\u202e", "", ""),
		"ends in map file":     raw[:0x5f0],
	}
	for name, b := range cases {
		if h, err := Parse(1383, b); !errors.Is(err, ErrCorrupt) || h != nil {
			t.Errorf("%s: %+v, %v", name, h, err)
		}
	}
}

// TestLimits parses headers exactly at the caps; together with the
// "too many options" and "string above the cap" cases in TestCorrupt it
// pins both bounds.
func TestLimits(t *testing.T) {
	raw := fixtureRaw(t, fixture.Names[0])
	at := func(n int) fwrtest.Setup {
		s := lobby(false)
		s.Players, s.AI, s.Images = nil, nil, 0
		for i := range n {
			s.Players = append(s.Players, fwrtest.Player{Slot: uint32(i), ID: uint32(i + 1), Team: 101})
			s.AI = append(s.AI, fwrtest.AI{Team: 102, Value: 1})
		}
		return s
	}
	cases := map[string][]byte{
		"64 options":              tail(raw, maxOptions, make([]string, maxOptions+2)...),
		"1024-byte string":        tail(raw, 1, strings.Repeat("a", maxString), "", ""),
		"16 players and AI forts": setupRaw(at(maxPlayers)),
	}
	for name, b := range cases {
		if _, err := Parse(1383, b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	images := at(0)
	images.Images = maxImages
	if _, err := Parse(1383, setupRaw(images)); err != nil {
		t.Errorf("%d images: %v", maxImages, err)
	}
	images.Images = maxImages + 1
	if _, err := Parse(1383, setupRaw(images)); !errors.Is(err, ErrCorrupt) {
		t.Errorf("%d images: %v", maxImages+1, err)
	}
	if _, err := Parse(1383, setupRaw(at(maxPlayers+1))); !errors.Is(err, ErrCorrupt) {
		t.Errorf("%d players: %v", maxPlayers+1, err)
	}
}

// tail keeps raw up to the options, writes the option count n and then
// strs (the options followed by the two map files), and ends with raw's
// bytes after its map files.
func tail(raw []byte, n uint32, strs ...string) []byte {
	b := binary.LittleEndian.AppendUint32(slices.Clone(raw[:optionsOffset]), n)
	for _, s := range strs {
		b = binary.LittleEndian.AppendUint32(b, uint32(len(s)))
		b = append(b, s...)
	}
	return append(b, raw[0x5fe:]...)
}
