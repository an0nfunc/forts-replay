// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

// Synthetic SteamIDs with account IDs 0xffffffff and 0xfffffffe, which
// Steam has not assigned.
const (
	testSteamID     uint64 = 76561202255233023 // the recorder's
	opponentSteamID uint64 = 76561202255233022
)

func buildFile(t *testing.T, data []byte) *Summary {
	t.Helper()
	s, err := Build(t.Context(), bytes.NewReader(data), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fixtureFile(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(fixture.Path(tb, "..", name))
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

// fixtureFile1390 returns one replay of fixture.V1390.
func fixtureFile1390(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(fixture.Path1390(tb, "..", name))
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

// game returns a synthetic two-player replay: header setup s, then two
// steps (frames 2 and 4) and the records in end.
func game(s fwrtest.Setup, end ...[]byte) []byte {
	return gameIn(1383, s, end)
}

// gameIn is game for header version 1383 or 1390, with the records of end,
// given as 1383 writes them, at the version's types.
func gameIn(v uint32, s fwrtest.Setup, end [][]byte) []byte {
	retype, extra := inVersion(v, s)
	recs := [][]byte{
		fwrtest.Record(retype(luaState("data = { gameFrame = 1, }")), true),
		fwrtest.Record(retype(fwrtest.Tick(4)), false), fwrtest.Record(retype(fwrtest.Sync(2)), false),
		fwrtest.Record(retype(fwrtest.Tick(6)), false), fwrtest.Record(retype(fwrtest.Sync(4)), false),
	}
	for _, p := range end {
		recs = append(recs, fwrtest.Record(retype(p), false))
	}
	return fwrtest.File(fwrtest.Header{Version: v, Frames: 4, Extra: extra}, bytes.Join(recs, nil))
}

// inVersion returns what turns a record as header version 1383 writes it
// into one of version v, and the header bytes of setup s in version v.
func inVersion(v uint32, s fwrtest.Setup) (func([]byte) []byte, []byte) {
	if v == 1390 {
		s.Date = "20261023_195552"
		return fwrtest.In1390, s.Extra1390()
	}
	return slices.Clone[[]byte], s.Extra()
}

func duel(ranked bool) fwrtest.Setup {
	return fwrtest.Setup{
		Ranked: ranked, Options: []string{"dlc3"}, Map: "maps/Test/Test.fwe",
		Recorder: 1, RecorderTeam: 101,
		Players: []fwrtest.Player{
			{Slot: 0, ID: 1, Team: 101, Name: "Alpha", SteamID: testSteamID},
			{Slot: 1, ID: 2, Team: 102, Name: "Player", SteamID: opponentSteamID},
		},
		AI:         []fwrtest.AI{{Team: 1}, {Team: 2}},
		Commanders: [2]string{"commander-test-alpha", "commander-test-beta"},
	}
}

func won(side int) []byte {
	return luaState(fmt.Sprintf("data = { gameEnded = true, gameWinner = %d, }", side))
}

// luaState returns a snapshot payload with one Lua section, text, and a
// minimal state: no teams, one node, one material.
func luaState(text string) []byte {
	return fwrtest.State{Lua: []string{text}, Nodes: []fwrtest.Node{{ID: 1, Kind: 0xff}}, Materials: []string{"m"}}.Payload()
}

func TestBuildFixtures(t *testing.T) {
	me := Player{ID: 1, Slot: 0, Team: 1, Side: 1, Name: "random🐧", Recorder: true}
	skirmish := map[string]bool{"abyss-bot-278f.fwr": true, "abyss-bot-974f.fwr": true, "probe-downgrade-overdrive.fwr": true, "probe-move-phantom.fwr": true}
	for _, name := range slices.Concat(fixture.Names, fixture.Probes) {
		s := buildFile(t, fixtureFile(t, name))
		if len(s.Warnings) != 0 || s.Truncated || !reflect.DeepEqual(s.Players, []Player{me}) || s.Map != "maps/Abyss/Abyss.fwe" || s.Ranked {
			t.Errorf("%s: %+v", name, s)
		}
		if skirmish[name] && len(s.Commanders) != 2 || !skirmish[name] && s.Commanders != nil {
			t.Errorf("%s: commanders %+v", name, s.Commanders)
		}
	}
	s := buildFile(t, fixtureFile(t, "abyss-bot-278f.fwr"))
	want := &Summary{
		SchemaVersion: 1, FormatVersion: 1383, Frames: 278, Duration: 11.12,
		Map:     "maps/Abyss/Abyss.fwe",
		Options: []string{"repair_station", "weapon_pack", "dlc1_portal_skirmish", "dlc1_weapons_skirmish", "dlc3"},
		Players: []Player{me}, AI: []AI{{Team: 102, Side: 2}},
		Commanders: []Commander{{Side: 1, Name: "commander-fb-fulminator"}, {Side: 2, Name: "commander-da-overclocker"}},
		State: &StateSeries{
			Frames: []int64{-1, 128, 256},
			Teams: []TeamSeries{
				{
					Team: 1, Side: 1,
					Metal: []float32{600, 600, 600}, Energy: []float32{3000, 3000, 3000},
					MaxMetal: []float32{600, 600, 600}, MaxEnergy: []float32{3000, 3000, 3000},
					FloatedMetal: []float32{1.04, 67.080055, 133.6406}, FloatedEnergy: []float32{8.0001955, 516.0121, 1028.0277},
					DamageDealt: []float32{0, 0, 0}, ReactorHealth: []float64{100, 100, 100},
					Struts: []int{69, 69, 69}, StrutHP: []float64{11100, 11100, 11100}, StrutsLost: []int{0, 0, 0}, Nodes: []int{31, 31, 31},
					Devices: []int{4, 4, 4}, Weapons: []int{1, 1, 1},
				},
				{
					Team: 102, Side: 2,
					Metal: []float32{600, 600, 213.0001}, Energy: []float32{3000, 3000, 1100.0022},
					MaxMetal: []float32{600, 600, 600}, MaxEnergy: []float32{3000, 3000, 3000},
					FloatedMetal: []float32{1.04, 43.384357, 86.88799}, FloatedEnergy: []float32{8.0001955, 397.53354, 759.2619},
					DamageDealt: []float32{0, 0, 0}, ReactorHealth: []float64{100, 100, 100},
					Struts: []int{69, 71, 72}, StrutHP: []float64{11100, 11400, 11300}, StrutsLost: []int{0, 0, 0}, Nodes: []int{31, 32, 32},
					Devices: []int{4, 4, 5}, Weapons: []int{1, 1, 1},
				},
			},
			Sides:   []SideSeries{{Side: 1, DamageTaken: []float32{0, 0, 0}}, {Side: 2, DamageTaken: []float32{0, 0, 0}}},
			Players: []PlayerSeries{{Slot: 0, Team: []int32{1, 1, 1}, Damage: []float32{0, 0, 0}, APM: []float32{0, 0, 20}}},
		},
		DeviceTypes: map[uint16]string{0: "machinegun", 30: "reactor", 31: "mine", 46: "armoury"},
		Blocks:      3, Records: 392, Warnings: []string{},
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("278f:\n got %+v\nwant %+v", s, want)
	}
	s = buildFile(t, fixtureFile(t, "abyss-bot-974f.fwr"))
	if !reflect.DeepEqual(s.Result, &Result{Winner: 2, Frame: 888, Source: SourceSnapshot}) || len(s.Timelines) != 1 {
		t.Errorf("974f: result %+v, %d timelines", s.Result, len(s.Timelines))
	}
}

// TestBuildFixtures1390: the v1.39.0 replays build without warnings or
// invalid records, with a state point for every snapshot up to the first
// that names the winner, and with the header fields of their v1.38.2
// twins. Where a twin's scenario ran the same inputs, the player's
// commands are the same kinds in the same order.
func TestBuildFixtures1390(t *testing.T) {
	sameInputs := map[string]bool{
		"devices.fwr": true, "economy-storage.fwr": true, "probe-delete.fwr": true, "probe-node.fwr": true,
		"skirmish-idle.fwr": true, "skirmish-quit.fwr": true, "struts-materials-1-4.fwr": true,
		"swap-material.fwr": true, "tech.fwr": true, "weapons-1-3.fwr": true,
	}
	kinds := func(s *Summary) [][]string {
		var out [][]string
		for _, tl := range s.Timelines {
			var k []string
			for _, e := range tl.Commands {
				k = append(k, msg.KindName(e.Command.MsgType()))
			}
			out = append(out, k)
		}
		return out
	}
	for _, name := range fixture.V1390 {
		invalid, snaps, ended := 0, 0, false
		s, err := Build(t.Context(), bytes.NewReader(fixtureFile1390(t, name)), Options{Observe: func(_ stream.Record, _ int64, m msg.Message) {
			switch m := m.(type) {
			case *msg.Invalid:
				invalid++
			case *msg.Snapshot:
				if !ended {
					snaps++
				}
				ended = ended || bytes.Contains(m.Payload, []byte(gameWinnerKey))
			}
		}})
		if err != nil {
			t.Fatal(err)
		}
		if s.FormatVersion != 1390 || len(s.Warnings) > 0 || invalid > 0 || s.Truncated || s.State == nil || len(s.State.Frames) != snaps || len(s.DeviceTypes) == 0 {
			t.Errorf("%s: version %d, warnings %q, %d invalid, truncated %v, state %v", name, s.FormatVersion, s.Warnings, invalid, s.Truncated, s.State != nil)
			continue
		}
		twin := buildFile(t, fixtureFile(t, name))
		if !reflect.DeepEqual(s.Options, twin.Options) || s.Map != twin.Map || !reflect.DeepEqual(s.Players, twin.Players) ||
			!reflect.DeepEqual(s.AI, twin.AI) || len(s.Commanders) != len(twin.Commanders) {
			t.Errorf("%s: header fields %+v, twin %+v", name, s, twin)
		}
		if sameInputs[name] && !reflect.DeepEqual(kinds(s), kinds(twin)) {
			t.Errorf("%s: commands %v, twin %v", name, kinds(s), kinds(twin))
		}
	}
}

func TestBuildRanked(t *testing.T) {
	s := buildFile(t, game(duel(true), fwrtest.Result(0, 1, 101, testSteamID), won(1)))
	want := []Player{
		{ID: 1, Slot: 0, Team: 101, Side: 1, Name: "Alpha", SteamID: testSteamID, Recorder: true},
		{ID: 2, Slot: 1, Team: 102, Side: 2, SteamID: opponentSteamID},
	}
	if !s.Ranked || !reflect.DeepEqual(s.Players, want) || len(s.Warnings) != 0 ||
		!reflect.DeepEqual(s.Result, &Result{Winner: 1, Frame: 4, Source: SourceRanked}) || s.AI != nil {
		t.Errorf("%+v", s)
	}
}

// TestBuild1390: a version 1390 replay gives the summary its records give
// in 1383, the ranked result included.
func TestBuild1390(t *testing.T) {
	end := [][]byte{fwrtest.Place(1, 101, 3, 5, 6, -1), fwrtest.Scrap(1, 101, 9), fwrtest.Ability(2), won(2)}
	ranked := append([][]byte{fwrtest.Result(0, 2, 101, testSteamID)}, end...)
	for _, c := range []struct {
		setup  fwrtest.Setup
		end    [][]byte
		source string
	}{{duel(false), end, SourceSnapshot}, {duel(true), ranked, SourceRanked}} {
		want := buildFile(t, game(c.setup, c.end...))
		want.FormatVersion = 1390
		if got := buildFile(t, gameIn(1390, c.setup, c.end)); !reflect.DeepEqual(got, want) || got.Result == nil || got.Result.Source != c.source {
			t.Errorf("got  %+v\nwant %+v", got, want)
		}
	}
	noSync := fwrtest.File(fwrtest.Header{Version: 1390, Frames: 4}, fwrtest.Record(fwrtest.In1390(fwrtest.Tick(4)), true))
	if s := buildFile(t, noSync); !slices.Contains(s.Warnings, "header has 4 frames, no sync decoded") {
		t.Errorf("no sync: warnings %q", s.Warnings)
	}
}

func TestResultRules(t *testing.T) {
	steam := func(s *Summary) uint64 { return s.Players[0].SteamID }
	const differ = "ranked result and recorder entry hold different SteamIDs"
	ids := []string{strconv.FormatUint(testSteamID, 10), strconv.FormatUint(opponentSteamID, 10)}
	otherTeam := duel(true)
	otherTeam.Players[0].Team = 102
	cases := []struct {
		name   string
		setup  fwrtest.Setup
		end    [][]byte
		result *Result
		steam  uint64
		warn   string // "" means no warnings
	}{
		{"snapshot only", duel(false), [][]byte{won(2)}, &Result{2, 4, SourceSnapshot}, testSteamID, ""},
		{"ranked and snapshot agree", duel(true), [][]byte{fwrtest.Result(0, 2, 101, testSteamID), won(2)}, &Result{2, 4, SourceRanked}, testSteamID, ""},
		{"they disagree", duel(true), [][]byte{fwrtest.Result(0, 1, 101, testSteamID), won(2)}, &Result{1, 4, SourceRanked}, testSteamID, "ranked result names side 1, snapshot side 2"},
		{"ranked winner 3", duel(true), [][]byte{fwrtest.Result(0, 3, 101, testSteamID), won(2)}, &Result{2, 4, SourceSnapshot}, testSteamID, "ranked result at frame 4 names side 3"},
		{"snapshot winner 5", duel(false), [][]byte{won(5)}, nil, testSteamID, "snapshot at frame 4 names no side 1 or 2"},
		{"another client's record", duel(true), [][]byte{fwrtest.Result(2, 2, 102, 7)}, nil, testSteamID, ""},
		{"first ranked record wins", duel(true), [][]byte{fwrtest.Result(0, 1, 101, testSteamID), fwrtest.Result(0, 2, 101, 7)}, &Result{1, 4, SourceRanked}, testSteamID, "ranked result at frame 4 differs from the one at frame 4"},
		{"repeated ranked record", duel(true), [][]byte{fwrtest.Result(0, 1, 101, testSteamID), fwrtest.Result(0, 1, 101, testSteamID)}, &Result{1, 4, SourceRanked}, testSteamID, ""},
		{"SteamIDs differ", duel(true), [][]byte{fwrtest.Result(0, 1, 101, opponentSteamID)}, &Result{1, 4, SourceRanked}, testSteamID, differ},
		{"team mismatch", duel(true), [][]byte{fwrtest.Result(0, 1, 102, opponentSteamID)}, &Result{1, 4, SourceRanked}, testSteamID, "ranked result team 102, recorder team 101"},
		{"recorder entry on another team", otherTeam, [][]byte{fwrtest.Result(0, 1, 101, testSteamID)}, &Result{1, 4, SourceRanked}, testSteamID, "ranked result team 101, recorder team 101, recorder entry team 102"},
		{"not ranked", duel(false), [][]byte{fwrtest.Result(0, 1, 101, testSteamID)}, &Result{1, 4, SourceRanked}, testSteamID, "ranked result record in a replay that is not ranked"},
		{"only the first winner snapshot", duel(false), [][]byte{luaState(`data = { note = "gameWinner", }`), won(1)}, nil, testSteamID, "snapshot at frame 4 mentions gameWinner, but no data table holds it"},
	}
	for _, c := range cases {
		s := buildFile(t, game(c.setup, c.end...))
		w := strings.Join(s.Warnings, "; ")
		if !reflect.DeepEqual(s.Result, c.result) || steam(s) != c.steam || c.warn == "" && w != "" || !strings.Contains(w, c.warn) ||
			slices.Contains(s.Warnings, differ) != (c.warn == differ) || strings.Contains(w, ids[0]) || strings.Contains(w, ids[1]) {
			t.Errorf("%s: result %+v, SteamID %d, warnings %q", c.name, s.Result, steam(s), w)
		}
	}
	dup := duel(true)
	dup.Players = []fwrtest.Player{{Slot: 0, ID: 1, Team: 101, Name: "Alpha", SteamID: testSteamID}, {Slot: 1, ID: 1, Team: 102, Name: "Mallory"}, {Slot: 2, ID: 2, Team: 102}}
	s := buildFile(t, game(dup, fwrtest.Result(0, 1, 101, testSteamID)))
	want := []Player{
		{ID: 1, Slot: 0, Team: 101, Side: 1, Name: "Alpha", SteamID: testSteamID, Recorder: true},
		{ID: 1, Slot: 1, Team: 102, Side: 2},
		{ID: 2, Slot: 2, Team: 102, Side: 2},
	}
	if !reflect.DeepEqual(s.Players, want) || !reflect.DeepEqual(s.Warnings, []string{"recorder 1 has 2 entries in the player table"}) {
		t.Errorf("duplicate recorder entry: players %+v, warnings %q", s.Players, s.Warnings)
	}
	lost := duel(false)
	lost.Recorder = 9
	if s := buildFile(t, game(lost)); !slices.Contains(s.Warnings, "recorder 9 is not in the player table") {
		t.Errorf("warnings %q", s.Warnings)
	}
	noNUL := duel(false)
	noNUL.Commanders[0] = strings.Repeat("c", 49)
	data := game(noNUL)
	data[0x50c+49] ^= 'x' // side 1's NUL decodes to 'x'
	if s := buildFile(t, data); s.Commanders != nil || !reflect.DeepEqual(s.Warnings, []string{"header: commander names do not decode"}) {
		t.Errorf("commanders without NUL: %+v, warnings %q", s.Commanders, s.Warnings)
	}
}

func TestBuildDamage(t *testing.T) {
	data := fixtureFile(t, "abyss-bot-974f.fwr")
	s := buildFile(t, data[:len(data)*6/10])
	if !s.Truncated || s.Blocks < 1 || len(s.Warnings) == 0 || !strings.HasPrefix(s.Warnings[0], "decoding stopped early") {
		t.Errorf("cut: %+v", s)
	}
	if _, err := Build(t.Context(), bytes.NewReader(data[:100]), Options{}); err == nil {
		t.Error("header only: no error")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Build(ctx, bytes.NewReader(data), Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
	other := buildFile(t, fwrtest.File(fwrtest.Header{Version: 7, Frames: 4}, fwrtest.Record([]byte{0xbe}, true)))
	if !reflect.DeepEqual(other.Warnings, []string{"header fields not decoded for version 7"}) || other.Players != nil {
		t.Errorf("version 7: %+v", other)
	}
}

// TestNameJSON: a name's JSON, a player's or a commander's, holds only
// printable runes raw and decodes to the name.
func TestNameJSON(t *testing.T) {
	for _, n := range []Name{"a\u202eb", "\x7fx", "\u009b31m", "👩\u200d💻", "\U000e0041", "random🐧"} {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range string(b) {
			if !unicode.IsPrint(r) {
				t.Errorf("%q: raw %U in %s", n, r, b)
			}
		}
		var back Name
		if err := json.Unmarshal(b, &back); err != nil || back != n {
			t.Errorf("%q: %s decodes to %q, %v", n, b, back, err)
		}
		switch n {
		case "a\u202eb":
			if !bytes.Contains(b, []byte(`\u202e`)) {
				t.Errorf("%q: %s, want a \\u202e escape", n, b)
			}
		case "random🐧":
			if string(b) != `"random🐧"` {
				t.Errorf("%q: %s, want it unescaped", n, b)
			}
		}
	}
	b, err := json.Marshal(Commander{Side: 1, Name: "a\u202eb"})
	if err != nil || !bytes.Contains(b, []byte(`\u202e`)) {
		t.Errorf("commander: %s, %v, want a \\u202e escape", b, err)
	}
}

// TestSummaryJSON: field names, omitted empties, arrays never null.
func TestSummaryJSON(t *testing.T) {
	s, err := Build(t.Context(), bytes.NewReader(game(duel(true),
		fwrtest.Result(0, 1, 101, testSteamID),
		fwrtest.Ability(1), fwrtest.Ability(2))), Options{Timeline: Limits{MaxCommands: 1}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"schema_version":1`, fmt.Sprintf(`"steam_id":"%d"`, testSteamID),
		fmt.Sprintf(`{"id":2,"slot":1,"team":102,"side":2,"steam_id":"%d"}`, opponentSteamID),
		`"commanders":[{"side":1,"name":"commander-test-alpha"},{"side":2,"name":"commander-test-beta"}]`,
		`"result":{"winner":1,"frame":4,"source":"ranked"}`,
		`{"player":1,"counts":{"ability":1},"commands":[{"frame":4,"kind":"ability","fields":{"player":1,"team":0}}]}`,
		`{"player":2,"counts":{"ability":1},"commands":[]}`,
	} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("no %s in %s", want, b)
		}
	}
	for _, absent := range []string{"null", `"ai"`, `"name":"Player"`} {
		if bytes.Contains(b, []byte(absent)) {
			t.Errorf("%s in %s", absent, b)
		}
	}
}
