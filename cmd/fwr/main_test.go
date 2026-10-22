// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var update = flag.Bool("update", false, "rewrite golden files")

// fixturePath returns the path of one sample replay.
func fixturePath(tb testing.TB, name string) string {
	tb.Helper()
	return fixture.Path(tb, "../..", name)
}

// f278 and f974 return the paths of the two sample games.
func f278(tb testing.TB) string {
	tb.Helper()
	return fixturePath(tb, "abyss-bot-278f.fwr")
}

func f974(tb testing.TB) string {
	tb.Helper()
	return fixturePath(tb, "abyss-bot-974f.fwr")
}

func runCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestGolden(t *testing.T) {
	type goldenCase struct {
		name string
		args []string
	}
	luaFile := write(t, t.TempDir(), "lua.fwr", fwrtest.File(fwrtest.Header{Version: 1383, Frames: 2}, fwrtest.Record(luaSnapshot(), true)))
	cases := []goldenCase{
		// The fixtures' snapshots hold the game's AI script tables, so
		// goldens print a synthetic snapshot instead.
		{"dump-278f.jsonl", []string{"dump", "--json", "--type", "0x92,0x93,0x95,0x97,0xbe,0xc0,0xe6", f278(t)}},
		{"dump-278f.txt", []string{"dump", f278(t)}},
		{"dump-lua.txt", []string{"dump", "--lua", luaFile}},
		{"dump-lua.jsonl", []string{"dump", "--json", luaFile}},
		{"hex-278f-0x95.txt", []string{"hex", "--type", "0x95", f278(t)}},
		{"info-278f.txt", []string{"info", f278(t)}},
		{"info-974f.txt", []string{"info", f974(t)}},
		{"info-278f.json", []string{"info", "--json", f278(t)}},
		{"info-974f.json", []string{"info", "--json", f974(t)}},
		{"info-probe-dblclick.json", []string{"info", "--json", fixturePath(t, "probe-dblclick.fwr")}},
		{"dump-probe-delete.jsonl", []string{"dump", "--json", "--type", "0xcb,0xcd,0xce,0xd2,0xd6", fixturePath(t, "probe-delete.fwr")}},
		{"dump-probe-dblclick-fire.txt", []string{"dump", "--type", "0xc5,0xc6,0xc7,0xca", fixturePath(t, "probe-dblclick.fwr")}},
	}
	for _, name := range fixture.Probes {
		cases = append(cases, goldenCase{"info-" + strings.TrimSuffix(name, ".fwr") + ".txt", []string{"info", fixturePath(t, name)}})
	}
	for _, c := range cases {
		code, out, errOut := runCmd(t, c.args...)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", c.name, code, errOut)
		}
		golden := filepath.Join("testdata", c.name+".golden")
		if *update {
			if err := os.MkdirAll("testdata", 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(golden, []byte(out), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if out != string(want) {
			t.Errorf("%s differs from %s; rerun with -update and review the diff", c.name, golden)
		}
	}
}

// luaSnapshot returns a snapshot payload whose sections cover what dump
// prints of Lua: nesting, every value kind, a named section, and a section
// that does not parse.
func luaSnapshot() []byte {
	p := fwrtest.Snapshot(`data = { gameFrame = 1, gameTime = 0.04, DoorsOpened = false, ScheduledCalls = {}, }`)
	p = append(p, 0, 1, 2, 3) // binary state between sections
	p = binary.LittleEndian.AppendUint32(p, uint32(len("scripts/test.lua")))
	p = append(p, "scripts/test.lua"...)
	p = append(p, fwrtest.Snapshot(`data = { Name = "a name", List = { 1, 2.5, -3, }, Nested = { Inner = { Flag = true, Big = inf, }, }, [7] = "seven", ["key with space"] = 0, }`)[1:]...)
	return append(p, fwrtest.Snapshot(`data = { broken = , }`)[1:]...)
}

func jsonLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var docs []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		var d map[string]any
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		if d["schema_version"] != 0.0 {
			t.Fatalf("schema_version %v", d["schema_version"])
		}
		docs = append(docs, d)
	}
	return docs
}

func TestDumpJSON(t *testing.T) {
	code, out, _ := runCmd(t, "dump", "--json", f278(t))
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	docs := jsonLines(t, out)
	end := docs[len(docs)-1]
	if len(docs) != 393 || end["kind"] != "end" || end["records"] != 392.0 || end["blocks"] != 3.0 || end["truncated"] != false {
		t.Errorf("%d docs, end %v", len(docs), end)
	}
	first := docs[0]
	if first["kind"] != "snapshot" || first["type"] != "0xe1" || first["keyframe"] != true || len(first["sections"].([]any)) != 7 {
		t.Errorf("first %v", first)
	}
	kinds := map[string]int{}
	var lastSync any
	for _, d := range docs {
		kinds[d["kind"].(string)]++
		if d["kind"] == "sync" {
			lastSync = d["frame"]
		}
	}
	var pointer map[string]any
	for _, d := range docs {
		if d["kind"] == "pointer" {
			pointer = d
			break
		}
	}
	f, _ := pointer["fields"].(map[string]any)
	if x, _ := f["x"].(float64); f["player"] != 1.0 || math.Abs(x-45.7817) > 1e-3 || pointer["payload"] == "" {
		t.Errorf("first pointer %v", pointer)
	}
	if kinds["tick"] != 139 || kinds["sync"] != 139 || kinds["raw"] != 2 || kinds["script"] != 1 || kinds["pointer"] != 94 || kinds["hover"] != 10 || kinds["apm"] != 4 || kinds["snapshot"] != 3 || lastSync != 278.0 {
		t.Errorf("kinds %v, last sync frame %v", kinds, lastSync)
	}
}

func TestDumpNonFinite(t *testing.T) {
	p := make([]byte, 37)
	p[0], p[17] = 0x95, 0x94
	binary.LittleEndian.PutUint32(p[28:], 0x7fc00000) // NaN x
	file := filepath.Join(t.TempDir(), "nan.fwr")
	if err := os.WriteFile(file, fwrtest.File(fwrtest.Header{Version: 1383}, fwrtest.Record(p, true)), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCmd(t, "dump", "--json", file)
	docs := jsonLines(t, out)
	if code != 0 || docs[0]["kind"] != "invalid" || !strings.Contains(docs[0]["error"].(string), "NaN") {
		t.Errorf("exit %d, %v", code, docs[0])
	}
}

// TestDumpScreenshot: a typed screenshot leaves its pixels out of dump
// --json; one that fails to decode keeps them.
func TestDumpScreenshot(t *testing.T) {
	shot := func(pixels int) []byte {
		p := make([]byte, 45+pixels)
		p[0], p[17], p[44] = 0xe2, 0xe2, 1
		binary.LittleEndian.PutUint32(p[24:], 4)
		binary.LittleEndian.PutUint32(p[28:], 2)
		binary.LittleEndian.PutUint32(p[32:], 4)
		return p
	}
	data := fwrtest.File(fwrtest.Header{Version: 1383}, bytes.Join([][]byte{fwrtest.Record(shot(32), true), fwrtest.Record(shot(31), false)}, nil))
	code, out, _ := runCmd(t, "dump", "--json", write(t, t.TempDir(), "shot.fwr", data))
	docs := jsonLines(t, out)
	f, _ := docs[0]["fields"].(map[string]any)
	if code != 0 || docs[0]["kind"] != "screenshot" || f["width"] != 4.0 || docs[0]["payload"] != nil {
		t.Errorf("exit %d, typed %v", code, docs[0])
	}
	if docs[1]["kind"] != "invalid" || docs[1]["payload"] == nil {
		t.Errorf("invalid %v", docs[1])
	}
}

func TestTruncatedFile(t *testing.T) {
	data, err := os.ReadFile(f974(t))
	if err != nil {
		t.Fatal(err)
	}
	cut := filepath.Join(t.TempDir(), "cut.fwr")
	if err := os.WriteFile(cut, data[:len(data)*6/10], 0o600); err != nil { //nolint:gosec // G703: path is under t.TempDir()
		t.Fatal(err)
	}
	code, out, errOut := runCmd(t, "dump", "--json", cut)
	if code != 0 || !strings.Contains(errOut, "warning") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	docs := jsonLines(t, out)
	end := docs[len(docs)-1]
	blocks := end["blocks"].(float64)
	if end["truncated"] != true || len(end["warnings"].([]any)) == 0 || blocks < 1 || blocks >= 8 {
		t.Errorf("end %v", end)
	}
	if code, out, errOut := runCmd(t, "info", cut); code != 0 || !strings.Contains(errOut, "decoding stopped early") || !strings.Contains(out, "records") {
		t.Errorf("info: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// write writes data to dir/name and returns the path.
func write(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil { //nolint:gosec // G703: path is under t.TempDir()
		t.Fatal(err)
	}
	return p
}

// Synthetic SteamIDs with account IDs 0xffffffff and 0xfffffffe, which
// Steam has not assigned.
const (
	testSteamID     = 76561202255233023 // the recorder's
	opponentSteamID = 76561202255233022
)

func dec(id uint64) string { return strconv.FormatUint(id, 10) }

// matchFile writes a synthetic two-player replay: setup, two steps, a
// commander ability from player 2, and the end. A ranked one has the
// game's result record before the snapshot naming side 1.
func matchFile(t *testing.T, dir string, ranked bool) string {
	t.Helper()
	s := fwrtest.Setup{
		Ranked: ranked, Options: []string{"dlc3"}, Map: "maps/Test/Test.fwe",
		Recorder: 1, RecorderTeam: 101,
		Players: []fwrtest.Player{
			{Slot: 0, ID: 1, Team: 101, Name: "Alpha", SteamID: testSteamID},
			{Slot: 1, ID: 2, Team: 102, Name: "Beta", SteamID: opponentSteamID},
		},
		AI:         []fwrtest.AI{{Team: 1}, {Team: 202, Script: "maps/Test/ai.lua", Value: 2}},
		Commanders: [2]string{"commander-test-alpha", "commander-test-beta"},
	}
	recs := [][]byte{
		fwrtest.Record(fwrtest.Snapshot("data = { gameFrame = 1, }"), true),
		fwrtest.Record(fwrtest.Tick(4), false), fwrtest.Record(fwrtest.Sync(2), false),
		fwrtest.Record(fwrtest.Ability(2), false),
		fwrtest.Record(fwrtest.Tick(6), false), fwrtest.Record(fwrtest.Sync(4), false),
	}
	if ranked {
		recs = append(recs, fwrtest.Record(fwrtest.Result(0, 1, 101, testSteamID), false))
	}
	recs = append(recs, fwrtest.Record(fwrtest.Snapshot("data = { gameEnded = true, gameWinner = 1, }"), false))
	name := "multiplayer.fwr"
	if ranked {
		name = "ranked.fwr"
	}
	return write(t, dir, name, fwrtest.File(fwrtest.Header{Version: 1383, Frames: 4, Extra: s.Extra()}, bytes.Join(recs, nil)))
}

func TestInfo(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.ReadFile(f278(t))
	if err != nil {
		t.Fatal(err)
	}
	badHeader := slices.Clone(orig)
	binary.LittleEndian.PutUint32(badHeader[0x570:], 1000) // option count
	moreFrames := slices.Clone(orig)
	binary.LittleEndian.PutUint32(moreFrames[5:], 300)
	oddFrames := slices.Clone(orig)
	binary.LittleEndian.PutUint32(oddFrames[5:], 279)
	oddMoreFrames := slices.Clone(orig)
	binary.LittleEndian.PutUint32(oddMoreFrames[5:], 281)
	corruptPlayers := slices.Clone(orig)
	binary.LittleEndian.PutUint32(corruptPlayers[0x67a:], 0)
	var many [][]byte
	for i := range 17 {
		many = append(many, fwrtest.Record(fwrtest.Ability(uint32(i+1)), i == 0))
	}
	cases := []struct {
		name, file  string
		out, notOut string // substrings of stdout
		errOut      string // substring of stderr; "" means stderr stays empty
	}{
		{"fixture", f278(t), "syncs      139, last at frame 278", "", ""},
		{"other version", write(t, dir, "v7.fwr", fwrtest.File(fwrtest.Header{Version: 7, Frames: 4}, fwrtest.Record([]byte{0xbe}, true))), "version    7", "options", "header fields not decoded for version 7"},
		{"bad header", write(t, dir, "bad.fwr", badHeader), "records    392", "options", "warning: header: corrupt"},
		{"frame mismatch", write(t, dir, "frames.fwr", moreFrames), "frames     300 (12.00 s)", "", "header has 300 frames, last sync is frame 278"},
		{"odd frame count", write(t, dir, "odd.fwr", oddFrames), "frames     279 (11.16 s)", "", ""},
		{"odd frame mismatch", write(t, dir, "oddmore.fwr", oddMoreFrames), "frames     281 (11.24 s)", "", "header has 281 frames, last sync is frame 278"},
		{"no sync decoded", write(t, dir, "nosync.fwr", fwrtest.File(fwrtest.Header{Version: 1383, Frames: 4}, fwrtest.Record([]byte{0xc0}, true))), "syncs      none", "", "header has 4 frames, no sync decoded"},
		{"many players", write(t, dir, "many.fwr", fwrtest.File(fwrtest.Header{Version: 1383}, bytes.Join(many, nil))), "commands   player 16: ability=1", "player 17", "commands from players beyond the first 16 ignored"},
		{"commands of another version", write(t, dir, "v1382.fwr", fwrtest.File(fwrtest.Header{Version: 1382}, fwrtest.Record(fwrtest.Ability(1), true))), "commands   none", "build", "header fields not decoded for version 1382"},
		{"multiplayer", matchFile(t, dir, false), "player     2: slot 1, team 102, side 2, \"Beta\"\n", "", ""},
		{"multiplayer result", matchFile(t, dir, false), "result     side 1 won at frame 4 (snapshot)", "", ""},
		{"commanders", matchFile(t, dir, false), "ai         team 202, side 2\ncommander  side 1, \"commander-test-alpha\"\ncommander  side 2, \"commander-test-beta\"\n", dec(testSteamID), ""},
		{"no opponent SteamID", matchFile(t, dir, false), "player     2: slot 1", dec(opponentSteamID), ""},
		{"ranked", matchFile(t, dir, true), "player     1: slot 0, team 101, side 1, \"Alpha\", recorder", dec(testSteamID), ""},
		{"ranked result", matchFile(t, dir, true), "result     side 1 won at frame 4 (ranked)", "Beta", ""},
		{"corrupt player table", write(t, dir, "players.fwr", corruptPlayers), "map        \"maps/Abyss/Abyss.fwe\"", "player ", "warning: header: corrupt"},
	}
	for _, c := range cases {
		code, out, errOut := runCmd(t, "info", c.file)
		if code != 0 || !strings.Contains(out, c.out) || c.notOut != "" && strings.Contains(out, c.notOut) {
			t.Errorf("%s: exit %d, stdout %q", c.name, code, out)
		}
		if c.errOut == "" && errOut != "" || !strings.Contains(errOut, c.errOut) {
			t.Errorf("%s: stderr %q", c.name, errOut)
		}
	}
}

// TestInfoMaxCommands: --max-commands bounds the timeline entries and with
// them the output, while counts stay complete.
func TestInfoMaxCommands(t *testing.T) {
	const n = 70000
	s := fwrtest.Setup{
		Map: "maps/Test/Test.fwe", Recorder: 1, RecorderTeam: 1,
		Players: []fwrtest.Player{{Slot: 0, ID: 1, Team: 1, Name: "Alpha"}},
	}
	recs := [][]byte{fwrtest.Record(fwrtest.Tick(4), true), fwrtest.Record(fwrtest.Sync(2), false)}
	ability := fwrtest.Record(fwrtest.Ability(1), false)
	for range n {
		recs = append(recs, ability)
	}
	file := write(t, t.TempDir(), "many.fwr", fwrtest.File(fwrtest.Header{Version: 1383, Frames: 2, Extra: s.Extra()}, bytes.Join(recs, nil)))
	const cut = "timeline cut at"
	for _, c := range []struct {
		args    []string
		entries int
		cut     bool
	}{
		{nil, 1 << 16, true},
		{[]string{"--max-commands", "100"}, 100, true},
		{[]string{"--max-commands", "0"}, 0, false},
	} {
		code, out, errOut := runCmd(t, slices.Concat([]string{"info", "--json"}, c.args, []string{file})...)
		var d struct {
			Timelines []struct {
				Counts   map[string]int    `json:"counts"`
				Commands []json.RawMessage `json:"commands"`
			} `json:"timelines"`
		}
		if err := json.Unmarshal([]byte(out), &d); code != 0 || err != nil || len(d.Timelines) != 1 {
			t.Fatalf("%v: exit %d, %v, %d timelines", c.args, code, err, len(d.Timelines))
		}
		tl := d.Timelines[0]
		if tl.Commands == nil || len(tl.Commands) != c.entries || tl.Counts["ability"] != n || strings.Contains(errOut, cut) != c.cut {
			t.Errorf("%v: %d entries, counts %v, stderr %q", c.args, len(tl.Commands), tl.Counts, errOut)
		}
		if c.entries == 100 && len(out) >= 64<<10 {
			t.Errorf("%v: %d bytes of output", c.args, len(out))
		}
	}
	if code, _, errOut := runCmd(t, "info", "--json", "--max-commands", "-1", file); code != 2 || !strings.Contains(errOut, "usage: fwr info [--json] [--max-commands N] FILE") {
		t.Errorf("-1: exit %d, stderr %q", code, errOut)
	}
	if code, out, _ := runCmd(t, "info", "--max-commands", "0", file); code != 0 || !strings.Contains(out, "\ncommands   player 1: ability=70000\n") {
		t.Errorf("text: exit %d, %s", code, out)
	}
}

// TestInfoHostileNames sends names with control, format and bidi
// characters through the real info encoders: neither output holds an
// unprintable rune, and the JSON gives the names back unchanged.
func TestInfoHostileNames(t *testing.T) {
	names := []string{
		"\x1b[31mred", "\u202eevil", "a\u2028b\u2029c", "del\x7f\"q\\",
		"\ufeffbom<>&", "zw\u200dj", "soft\u00adhy", "tag\U000E0041", "c1\u009b31m", "cr\r\nlf",
	}
	s := fwrtest.Setup{Map: "maps/Test/Test.fwe", Recorder: 1, RecorderTeam: 101}
	for i, name := range names {
		id := uint32(i + 1)
		s.Players = append(s.Players, fwrtest.Player{Slot: id - 1, ID: id, Team: int32(100*(i/2+1) + i%2 + 1), Name: name})
	}
	recs := bytes.Join([][]byte{fwrtest.Record(fwrtest.Tick(4), true), fwrtest.Record(fwrtest.Sync(2), false)}, nil)
	file := write(t, t.TempDir(), "names.fwr", fwrtest.File(fwrtest.Header{Version: 1383, Frames: 2, Extra: s.Extra()}, recs))
	for _, args := range [][]string{{"info", "--json", file}, {"info", file}} {
		code, out, errOut := runCmd(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d, %s", args, code, errOut)
		}
		for _, r := range out + errOut {
			if r != '\n' && !unicode.IsPrint(r) {
				t.Errorf("%v: rune %U in output", args, r)
			}
		}
	}
	_, out, _ := runCmd(t, "info", "--json", file)
	var d struct {
		Players []struct {
			Name string `json:"name"`
		} `json:"players"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil || len(d.Players) != len(names) {
		t.Fatalf("%v, %d players", err, len(d.Players))
	}
	for i, p := range d.Players {
		if p.Name != names[i] {
			t.Errorf("player %d: name %q, want %q", i+1, p.Name, names[i])
		}
	}
}

func TestInfoJSON(t *testing.T) {
	code, out, errOut := runCmd(t, "info", "--json", matchFile(t, t.TempDir(), true))
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	var s struct {
		SchemaVersion int              `json:"schema_version"`
		Players       []map[string]any `json:"players"`
		Commanders    []map[string]any `json:"commanders"`
		Result        map[string]any   `json:"result"`
		Timelines     []map[string]any `json:"timelines"`
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatal(err)
	}
	if s.SchemaVersion != 1 || len(s.Players) != 2 || s.Players[0]["steam_id"] != dec(testSteamID) ||
		s.Players[1]["steam_id"] != dec(opponentSteamID) || s.Players[1]["name"] != nil ||
		len(s.Commanders) != 2 || s.Commanders[1]["side"] != 2.0 || s.Commanders[1]["name"] != "commander-test-beta" ||
		s.Result["source"] != "ranked" || len(s.Timelines) != 1 || s.Timelines[0]["player"] != 2.0 {
		t.Errorf("%s", out)
	}
	code, out, _ = runCmd(t, "info", matchFile(t, t.TempDir(), true))
	if code != 0 || strings.Contains(out, dec(testSteamID)) || strings.Contains(out, dec(opponentSteamID)) {
		t.Errorf("text output holds a SteamID: %s", out)
	}
}

// TestInfoSchema validates info --json output against a closed copy of the
// schema, so the output cannot hold a field the schema lacks, and checks
// that the published schema accepts a field a later v1 might add.
func TestInfoSchema(t *testing.T) {
	open, strict := compileSchema(t, false), compileSchema(t, true)
	dir := t.TempDir()
	cut, err := os.ReadFile(f974(t))
	if err != nil {
		t.Fatal(err)
	}
	files := []string{
		matchFile(t, dir, false), matchFile(t, dir, true),
		write(t, dir, "cut.fwr", cut[:len(cut)*6/10]),
		write(t, dir, "v7.fwr", fwrtest.File(fwrtest.Header{Version: 7, Frames: 4}, fwrtest.Record([]byte{0xbe}, true))),
	}
	corpusFiles := []string{
		"portal-link.fwr", "ammo-select.fwr", "swarm-missiles.fwr", "toggles.fwr",
		"area-repair-damaged.fwr", "device-repair.fwr", "strut-repair.fwr", "swarm-immediate-fire.fwr",
	}
	for _, name := range slices.Concat(fixture.Names, fixture.Probes, corpusFiles) {
		files = append(files, fixturePath(t, name))
	}
	validate := func(sch *jsonschema.Schema, doc string) error {
		v, err := jsonschema.UnmarshalJSON(strings.NewReader(doc))
		if err != nil {
			return err
		}
		return sch.Validate(v)
	}
	for _, f := range files {
		code, out, _ := runCmd(t, "info", "--json", f)
		if code != 0 {
			t.Fatalf("%s: exit %d", f, code)
		}
		if err := validate(strict, out); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	_, ranked, _ := runCmd(t, "info", "--json", files[1])
	edited := func(edit func(d map[string]any)) string {
		var d map[string]any
		if err := json.Unmarshal([]byte(ranked), &d); err != nil {
			t.Fatal(err)
		}
		edit(d)
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	added := edited(func(d map[string]any) { d["players"].([]any)[0].(map[string]any)["added"] = 1 })
	if err := validate(open, added); err != nil {
		t.Errorf("added field: %v", err)
	}
	if validate(strict, added) == nil {
		t.Error("added field: valid against the closed copy")
	}
	teleport := edited(func(d map[string]any) { cmd0(d)["kind"], cmd0(d)["fields"] = "teleport", map[string]any{"to": 1} })
	if err := validate(open, teleport); err != nil {
		t.Errorf("added kind: %v", err)
	}
	if validate(strict, teleport) == nil {
		t.Error("added kind: valid against the closed copy")
	}
	// Each pair differs in one property of a complete entry.
	for name, c := range map[string]struct {
		kind            string
		complete, wrong map[string]any
	}{
		"door without source": {
			"door",
			map[string]any{"player": 1, "state": 1, "node_a": 2, "node_b": 3, "source": 3, "device": 4},
			map[string]any{"player": 1, "state": 1, "node_a": 2, "node_b": 3, "device": 4},
		},
		"repair_area with a string radius": {
			"repair_area",
			map[string]any{"player": 1, "team": 1, "x": 1.5, "y": 2.5, "radius": 200},
			map[string]any{"player": 1, "team": 1, "x": 1.5, "y": 2.5, "radius": "200"},
		},
	} {
		with := func(fields map[string]any) string {
			return edited(func(d map[string]any) { cmd0(d)["kind"], cmd0(d)["fields"] = c.kind, fields })
		}
		if err := validate(strict, with(c.complete)); err != nil {
			t.Errorf("%s, complete: %v", name, err)
		}
		if validate(open, with(c.wrong)) == nil {
			t.Errorf("%s: valid", name)
		}
	}
	// The published schema rejects what v1 defines differently.
	for name, edit := range map[string]func(d map[string]any){
		"numeric steam_id": func(d map[string]any) { d["players"].([]any)[0].(map[string]any)["steam_id"] = 1 },
		"winner 3":         func(d map[string]any) { d["result"].(map[string]any)["winner"] = 3 },
		"no warnings":      func(d map[string]any) { delete(d, "warnings") },
		"string device": func(d map[string]any) {
			cmd0(d)["kind"] = "upgrade"
			cmd0(d)["fields"].(map[string]any)["device"] = "1"
		},
		"17 players": func(d map[string]any) {
			ps := d["players"].([]any)
			for len(ps) < 17 {
				ps = append(ps, ps[0])
			}
			d["players"] = ps
		},
		"name of 16 characters":  func(d map[string]any) { d["players"].([]any)[0].(map[string]any)["name"] = strings.Repeat("a", 16) },
		"commander side 3":       func(d map[string]any) { commander0(d)["side"] = 3 },
		"empty commander name":   func(d map[string]any) { commander0(d)["name"] = "" },
		"commander without name": func(d map[string]any) { delete(commander0(d), "name") },
		"commander name of 50":   func(d map[string]any) { commander0(d)["name"] = strings.Repeat("c", 50) },
		"three commanders": func(d map[string]any) {
			cs := d["commanders"].([]any)
			d["commanders"] = append(cs, cs[0])
		},
	} {
		if validate(open, edited(edit)) == nil {
			t.Errorf("%s: valid", name)
		}
	}
}

// compileSchema compiles schema/info-v1.json. With strict, every object
// that lists its properties is closed, except the if and then subschemas
// of the entry definition: they list only kind or fields, so closing them
// would reject every entry. The kind list is closed to the known kinds too.
func compileSchema(t *testing.T, strict bool) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open("../../schema/info-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	if strict {
		closeObjects(doc)
		kinds := []any{
			"place", "strut", "link", "delete_link", "delete_node", "scrap", "upgrade", "repair", "door", "fire", "fire_blocked", "ability",
			"repair_link", "repair_device", "repair_area", "portal_link", "portal_reverse", "fire_at", "ammo",
		}
		doc.(map[string]any)["$defs"].(map[string]any)["kind"] = map[string]any{"enum": kinds}
	}
	c := jsonschema.NewCompiler()
	if err = c.AddResource("info-v1.json", doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("info-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func closeObjects(v any) {
	switch v := v.(type) {
	case map[string]any:
		if _, ok := v["properties"]; ok {
			if _, ok := v["additionalProperties"]; !ok {
				v["additionalProperties"] = false
			}
		}
		for k, e := range v {
			if k != "if" && k != "then" {
				closeObjects(e)
			}
		}
	case []any:
		for _, e := range v {
			closeObjects(e)
		}
	}
}

// commander0 returns the first commander.
func commander0(d map[string]any) map[string]any {
	return d["commanders"].([]any)[0].(map[string]any)
}

// cmd0 returns the first command of the first timeline.
func cmd0(d map[string]any) map[string]any {
	return d["timelines"].([]any)[0].(map[string]any)["commands"].([]any)[0].(map[string]any)
}

// TestDumpSteamID: dump and hex print records without SteamIDs, in decimal
// or as little-endian bytes: a ranked result (0xbb), a 0x8d record, and a
// record of an unknown type holding a SteamID-shaped u64, also when a record
// is cut short and fails to decode or ends inside an ID.
func TestDumpSteamID(t *testing.T) {
	dir := t.TempDir()
	const (
		cutSteamID = 1234567890123456789
		rawSteamID = 76561202255233021 // account ID 0xfffffffd
	)
	le := binary.LittleEndian
	lobby := make([]byte, 135)
	lobby[0], lobby[17] = 0x8d, 0x8d
	le.PutUint64(lobby[75:], testSteamID)
	le.PutUint64(lobby[83:], opponentSteamID)
	unknown := make([]byte, 64)
	unknown[0], unknown[17] = 0x77, 0x77
	le.PutUint64(unknown[41:], rawSteamID)
	records := bytes.Join([][]byte{
		fwrtest.Record(fwrtest.Result(0, 1, 101, cutSteamID)[:44], true),
		fwrtest.Record(lobby, false), fwrtest.Record(lobby[:87], false), // the second cut inside its second ID
		fwrtest.Record(unknown, false),
	}, nil)
	type holding struct {
		types   []string // shown with hex
		records int      // records with a SteamID
	}
	files := map[string]holding{
		matchFile(t, dir, true): {[]string{"0xbb"}, 1},
		write(t, dir, "records.fwr", fwrtest.File(fwrtest.Header{Version: 1383}, records)): {[]string{"0xbb", "0x8d", "0x77"}, 4},
	}
	blanked := map[string][2]int{"0xbb": {32, 48}, "0x8d": {75, 91}, "0x77": {41, 49}}
	forms := steamIDForms(testSteamID, opponentSteamID, cutSteamID, rawSteamID)
	for file, want := range files {
		runs := [][]string{{"dump", file}, {"dump", "--json", file}}
		for _, typ := range want.types {
			runs = append(runs, []string{"hex", "--max-bytes", "200", "--type", typ, file})
		}
		var docs []map[string]any
		for _, args := range runs {
			code, out, errOut := runCmd(t, args...)
			if code != 0 {
				t.Fatalf("%v: exit %d: %s", args, code, errOut)
			}
			for _, f := range forms {
				if strings.Contains(out, f) {
					t.Errorf("%v: output holds %q", args, f)
				}
			}
			switch args[1] {
			case "--json":
				docs = jsonLines(t, out)
			case "--max-bytes":
				if !strings.Contains(out, " "+args[4][2:]+" 00") {
					t.Errorf("%v: no row in %q", args, out)
				}
			}
		}
		shown := 0
		for _, d := range docs {
			typ, _ := d["type"].(string)
			r, ok := blanked[typ]
			if !ok {
				continue
			}
			shown++
			payload, _ := d["payload"].(string)
			p, err := hex.DecodeString(payload)
			if err != nil || float64(len(p)) != d["length"] || len(p) > r[0] && slices.ContainsFunc(p[r[0]:min(len(p), r[1])], func(c byte) bool { return c != 0 }) {
				t.Errorf("%s: dump --json: %v", file, d)
			}
		}
		if shown != want.records {
			t.Errorf("%s: dump --json: %d records with SteamIDs", file, shown)
		}
	}
}

// steamIDForms returns each SteamID in decimal and as little-endian bytes in
// hex, plain and spaced.
func steamIDForms(ids ...uint64) []string {
	var forms []string
	for _, id := range ids {
		le := binary.LittleEndian.AppendUint64(nil, id)
		forms = append(forms, dec(id), hex.EncodeToString(le), fmt.Sprintf("% x", le))
	}
	return forms
}

func TestNotAReplay(t *testing.T) {
	dir := t.TempDir()
	rnd := make([]byte, 4096)
	if _, err := rand.Read(rnd); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"random.fwr": rnd, "empty.fwr": nil}
	for name, data := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, errOut := runCmd(t, "dump", p); code != 1 || errOut == "" {
			t.Errorf("%s: exit %d stderr %q", name, code, errOut)
		}
	}
	if code, _, _ := runCmd(t, "dump", filepath.Join(dir, "missing.fwr")); code != 1 {
		t.Errorf("missing file: exit %d", code)
	}
}

// TestUsage names a file that does not exist: usage errors come before the
// file is opened.
func TestUsage(t *testing.T) {
	const f = "missing.fwr"
	for _, args := range [][]string{nil, {"nope"}, {"dump"}, {"dump", "--type", "zz", f}, {"hex", f}, {"extract", f}, {"info"}, {"info", f, f}} {
		if code, _, _ := runCmd(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestExtract(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	if code, out, errOut := runCmd(t, "extract", "-o", dir, f278(t)); code != 0 || !strings.Contains(out, "wrote 3 blocks") {
		t.Fatalf("exit %d %q %q", code, out, errOut)
	}
	want := map[string]int64{"header.bin": 1943, "block000.bin": 78358, "block001.bin": 79167, "block002.bin": 65940}
	for name, size := range want {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if fi.Size() != size {
			t.Errorf("%s: size %d, want %d", name, fi.Size(), size)
		}
	}
}

func TestDumpEscapesControlBytes(t *testing.T) {
	text := "data = { s = \"\x1b[2J\x9b\", }\rhidden\r\n"
	payload := binary.LittleEndian.AppendUint32([]byte{0xe1}, uint32(len(text)))
	payload = append(payload, text...)
	p := filepath.Join(t.TempDir(), "esc.fwr")
	if err := os.WriteFile(p, fwrtest.File(fwrtest.Header{Version: 1383}, fwrtest.Record(payload, true)), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCmd(t, "dump", "--lua", p)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.IndexByte(out, 0x1b) >= 0 || strings.IndexByte(out, 0x9b) >= 0 || strings.Contains(out, "\rhidden") {
		t.Errorf("raw control bytes in output: %q", out)
	}
	if !strings.Contains(out, `\x1b[2J\x9b`) || !strings.Contains(out, "\\x0dhidden\r\n") {
		t.Errorf("escapes missing: %q", out)
	}
}

func TestExtractRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "header.bin")); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCmd(t, "extract", "-o", dir, f278(t)); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "keep" {
		t.Errorf("victim %q, err %v", got, err)
	}
}

func TestRecordDamage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "damaged.fwr")
	if err := os.WriteFile(p, fwrtest.File(fwrtest.Header{Version: 1383}, []byte("not records")), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCmd(t, "dump", "--json", p)
	if code != 0 || !strings.Contains(errOut, "warning") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	docs := jsonLines(t, out)
	end := docs[len(docs)-1]
	if len(docs) != 1 || end["kind"] != "end" || end["truncated"] != true || end["blocks"] != 1.0 || end["records"] != 0.0 {
		t.Errorf("%d docs, end %v", len(docs), end)
	}
}
