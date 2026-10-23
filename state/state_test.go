// SPDX-License-Identifier: AGPL-3.0-or-later

package state

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/stream"
)

// tailLen is the length of the tail fwrtest.State writes after the links.
const tailLen = 34

// synthetic returns a snapshot with every kind of record: team-list slots
// out of the usual order, player slots with gaps, a spectator and a player
// who left, plain and extended nodes, plain and weapon devices, and struts
// with and without a device, an n list and fire segments.
func synthetic() fwrtest.State {
	return fwrtest.State{
		NextNode: 40, NextDevice: 12,
		Lua: []string{"data = { gameFrame = 128, }", "data = { x = 1, }"},
		Teams: []fwrtest.Team{
			{ID: 0, Metal: 1e12, Energy: 1e12, MaxMetal: 1e12, MaxEnergy: 1e12},
			{ID: 1, Metal: 10, Energy: 20, MaxMetal: 600, MaxEnergy: 3000, FloatedMetal: 1.5, FloatedEnergy: 2.5, DamageDealt: 126},
			{ID: 2, Metal: 11, Energy: 21, MaxMetal: 601, MaxEnergy: 3001, DamageDealt: 7},
			{ID: 101, Metal: 12, Energy: 22, MaxMetal: 900, MaxEnergy: 5000, FloatedMetal: 3, FloatedEnergy: 4, DamageDealt: 30},
			{ID: 201, Metal: -0.001, Energy: 23, MaxMetal: 602, MaxEnergy: 3002},
			{ID: 102, Metal: 0, Energy: 0, DamageDealt: 42},
		},
		DamageTaken: [2]float32{49, 156},
		Slots: [9]fwrtest.Slot{
			0: {Team: 101, APM: 30, Damage: 126},
			1: {Team: 201, APM: 12.5, Damage: 7},
			5: {Team: 0, APM: 3}, // left
			6: {Team: 102, APM: 40, Damage: 42},
			7: {Team: -3},
		},
		Nodes: []fwrtest.Node{
			{ID: 1, Team: 101, X: -100, Y: 50, Kind: 0xff},
			{ID: 2, Team: 101, X: -80, Y: 50, Kind: 0},
			{ID: 3, Team: 201, X: 80, Y: 50, Kind: 1},
			{ID: 5, Team: 201, X: 100, Y: 50, Kind: 2},
			{ID: 7, Team: -2, X: 0, Y: 300, Kind: 3},
			{ID: 9, Team: 0, X: 0, Y: 400, Kind: 1},
		},
		Gap: []byte{0, 1, 1, 0, 0, 0},
		Devices: []fwrtest.Device{
			{ID: 7, Name: "reactor", Team: 101, Type: 9, X: -90, Y: 40, NodeA: 1, NodeB: 2, Health: 80.5, Progress: 1},
			{ID: 8, Name: "machinegun", Team: 101, Type: 0, X: -85, Y: 45, NodeA: 1, NodeB: 2, Health: 100, Progress: 20, Weapon: true, Beams: 2, A: 1, B: 3},
			{ID: 10, Name: "battery", Team: 201, Type: 37, X: 90, Y: 0, NodeA: 0xffffffff, NodeB: 0xffffffff, Health: 150, Progress: 7.5},
			{ID: 11, Name: "sniper", Team: 201, Type: 2, X: 95, Y: 45, NodeA: 3, NodeB: 5, Health: 0.25, Weapon: true},
		},
		Materials: []string{"bracing", "backbracing", "armour", "door"},
		Links: []fwrtest.Link{
			{A: 0, B: 1, Material: 0, HP: 150, State: 1, Device: -1},
			{A: 0, B: 4, Material: 2, HP: 400, State: 2, Device: 1, List: []uint32{3, 4}},
			{A: 2, B: 3, Material: 1, HP: 12.5, State: 0, Device: -1, Segments: 3},
			{A: 3, B: 2, Material: 3, HP: 0, State: 3, Device: -1, Segments: -2},
		},
	}
}

func syntheticWant() *State {
	return &State{
		NextNode: 40, NextDevice: 12,
		Teams: []Team{
			{ID: 0, Metal: 1e12, Energy: 1e12, MaxMetal: 1e12, MaxEnergy: 1e12},
			{ID: 1, Metal: 10, Energy: 20, MaxMetal: 600, MaxEnergy: 3000, FloatedMetal: 1.5, FloatedEnergy: 2.5, DamageDealt: 126},
			{ID: 2, Metal: 11, Energy: 21, MaxMetal: 601, MaxEnergy: 3001, DamageDealt: 7},
			{ID: 101, Metal: 12, Energy: 22, MaxMetal: 900, MaxEnergy: 5000, FloatedMetal: 3, FloatedEnergy: 4, DamageDealt: 30},
			{ID: 201, Metal: -0.001, Energy: 23, MaxMetal: 602, MaxEnergy: 3002},
			{ID: 102, DamageDealt: 42},
		},
		DamageTaken: [2]float32{49, 156},
		Slots: [9]Slot{
			0: {Team: 101, APM: 30, Damage: 126},
			1: {Team: 201, APM: 12.5, Damage: 7},
			5: {Team: 0, APM: 3},
			6: {Team: 102, APM: 40, Damage: 42},
			7: {Team: -3},
		},
		Nodes: []Node{
			{ID: 1, Team: 101, X: -100, Y: 50},
			{ID: 2, Team: 101, X: -80, Y: 50},
			{ID: 3, Team: 201, X: 80, Y: 50},
			{ID: 5, Team: 201, X: 100, Y: 50},
			{ID: 7, Team: -2, X: 0, Y: 300},
			{ID: 9, Team: 0, X: 0, Y: 400},
		},
		Devices: []Device{
			{ID: 7, Name: "reactor", Team: 101, Type: 9, X: -90, Y: 40, NodeA: 1, NodeB: 2, Health: 80.5, Progress: 1},
			{ID: 8, Name: "machinegun", Team: 101, Type: 0, X: -85, Y: 45, NodeA: 1, NodeB: 2, Health: 100, Progress: 20, Weapon: true},
			{ID: 10, Name: "battery", Team: 201, Type: 37, X: 90, Y: 0, NodeA: 0xffffffff, NodeB: 0xffffffff, Health: 150, Progress: 7.5},
			{ID: 11, Name: "sniper", Team: 201, Type: 2, X: 95, Y: 45, NodeA: 3, NodeB: 5, Health: 0.25, Weapon: true},
		},
		Links: []Link{
			{NodeA: 1, NodeB: 2, Material: 0, HP: 150, State: 1},
			{NodeA: 1, NodeB: 7, Material: 2, HP: 400, State: 2},
			{NodeA: 3, NodeB: 5, Material: 1, HP: 12.5, State: 0},
			{NodeA: 5, NodeB: 3, Material: 3, HP: 0, State: 3},
		},
	}
}

// parse decodes a payload from the end of its last Lua section.
func parse(payload []byte) (*State, error) {
	end, ok := snapshot.End(payload)
	if !ok {
		return nil, errors.New("no Lua section")
	}
	return Parse(payload, end)
}

func TestParseSynthetic(t *testing.T) {
	got, err := parse(synthetic().Payload())
	if err != nil {
		t.Fatal(err)
	}
	if want := syntheticWant(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// TestParse1390: a version 1390 snapshot, whose weapons have 4 more bytes
// before their counts, decodes with ParseVersion(1390), and neither layout
// decodes as the other.
func TestParse1390(t *testing.T) {
	s := synthetic()
	s.Version = 1390
	p := s.Payload()
	end, _ := snapshot.End(p)
	if got, err := ParseVersion(1390, p, end); err != nil || !reflect.DeepEqual(got, syntheticWant()) {
		t.Errorf("got  %+v, %v\nwant %+v", got, err, syntheticWant())
	}
	if st, err := ParseVersion(1383, p, end); !errors.Is(err, ErrCorrupt) || st != nil {
		t.Errorf("as 1383: %+v, %v", st, err)
	}
	old := synthetic().Payload()
	oldEnd, _ := snapshot.End(old)
	if st, err := ParseVersion(1390, old, oldEnd); !errors.Is(err, ErrCorrupt) || st != nil {
		t.Errorf("1383 payload as 1390: %+v, %v", st, err)
	}
	if !Decodes(1383) || !Decodes(1390) {
		t.Error("1383 or 1390 not decoded")
	}
	for _, v := range []uint32{0, 1331, 1382, 1384, 1389, 1391} {
		if st, err := ParseVersion(v, old, oldEnd); Decodes(v) || !errors.Is(err, ErrVersion) || st != nil {
			t.Errorf("version %d: decodes %v, %+v, %v", v, Decodes(v), st, err)
		}
	}
}

// TestParseCorpus1390: every snapshot of the v1.39.0 replays decodes as
// version 1390; as 1383, every one holding a weapon fails.
func TestParseCorpus1390(t *testing.T) {
	for _, name := range fixture.V1390 {
		for frame, p := range snapshotsAt(t, fixture.Path1390(t, "..", name)) {
			end, ok := snapshot.End(p)
			if !ok {
				t.Fatalf("%s frame %d: no Lua section", name, frame)
			}
			st, err := ParseVersion(1390, p, end)
			if err != nil {
				t.Errorf("%s frame %d: %v", name, frame, err)
				continue
			}
			if slices.ContainsFunc(st.Devices, func(d Device) bool { return d.Weapon }) {
				if _, err := Parse(p, end); err == nil {
					t.Errorf("%s frame %d: decodes as 1383", name, frame)
				}
			}
		}
	}
}

// TestGap checks that the gap is the largest even length at which the
// tables parse, also when the 0 and 1 run reaches into the device count.
func TestGap(t *testing.T) {
	for _, gap := range [][]byte{nil, {1, 1}, {0, 1, 0, 1, 1, 0, 1, 1}} {
		s := synthetic()
		s.Gap = gap
		s.Devices = s.Devices[:1] // count 01 00 00 00 continues the run
		got, err := parse(s.Payload())
		if err != nil {
			t.Fatalf("gap %v: %v", gap, err)
		}
		if len(got.Devices) != 1 || got.Devices[0].Name != "reactor" || len(got.Links) != 4 {
			t.Errorf("gap %v: %+v", gap, got)
		}
	}
}

// TestGapTail checks the most 0 and 1 bytes the tables add to the gap's
// run: 8 with devices (a count of 1 and a 1-byte name), 12 without (counts
// of 0 devices and 1 material, and a 1-byte material name). Both parse; no
// table adds more, so no shorter gap than the run minus 12 can parse.
func TestGapTail(t *testing.T) {
	for _, c := range []struct {
		name    string
		devices int
		edit    func(*fwrtest.State)
	}{
		{"device", 1, func(s *fwrtest.State) { s.Devices = s.Devices[:1]; s.Devices[0].Name = "r" }},
		{"no device", 0, func(s *fwrtest.State) { s.Devices, s.Materials = nil, []string{"m"} }},
	} {
		s := synthetic()
		s.Gap = []byte{1, 0, 1, 1}
		c.edit(&s)
		got, err := parse(s.Payload())
		if err != nil || len(got.Devices) != c.devices || len(got.Links) != 4 {
			t.Errorf("%s: %v, %+v", c.name, err, got)
		}
	}
}

// at returns the offset of the first sub after the Lua sections.
func at(t *testing.T, p []byte, sub string) int {
	t.Helper()
	end, _ := snapshot.End(p)
	i := bytes.Index(p[end:], []byte(sub))
	if i < 0 {
		t.Fatalf("%q not found", sub)
	}
	return end + i
}

// TestParseCorrupt pins the checks and caps: each case gives ErrCorrupt,
// and a case at a cap still parses.
func TestParseCorrupt(t *testing.T) {
	t.Run("fields", corruptFields)
	t.Run("bytes", corruptBytes)
	t.Run("truncated", truncated)
	t.Run("struts in total", strutCap)
}

func corruptFields(t *testing.T) {
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	cases := []struct {
		name string
		ok   bool
		edit func(*fwrtest.State)
	}{
		{"nodes at cap", true, func(s *fwrtest.State) { s.Nodes = nodes(maxNodes) }},
		{"nodes above cap", false, func(s *fwrtest.State) { s.Nodes = nodes(maxNodes + 1) }},
		{"no nodes", false, func(s *fwrtest.State) { s.Nodes, s.Links = nil, nil }},
		{"unknown node kind", false, func(s *fwrtest.State) { s.Nodes[1].Kind = 4 }},
		{"NaN node x", false, func(s *fwrtest.State) { s.Nodes[2].X = nan }},
		{"Inf node y", false, func(s *fwrtest.State) { s.Nodes[5].Y = -inf }},
		{"devices at cap", true, func(s *fwrtest.State) { s.Devices = devices(maxDevices) }},
		{"devices above cap", false, func(s *fwrtest.State) { s.Devices = devices(maxDevices + 1) }},
		{"gap at cap", true, func(s *fwrtest.State) { s.Gap = make([]byte, maxGap) }},
		{"gap above cap", false, func(s *fwrtest.State) { s.Gap = make([]byte, maxGap+2) }},
		{"gap byte not 0 or 1", false, func(s *fwrtest.State) { s.Gap = []byte{0, 0, 2, 0} }},
		{"name at cap", true, func(s *fwrtest.State) { s.Devices[0].Name = strings.Repeat("r", maxName) }},
		{"name above cap", false, func(s *fwrtest.State) { s.Devices[0].Name = strings.Repeat("r", maxName+1) }},
		{"empty name", false, func(s *fwrtest.State) { s.Devices[2].Name = "" }},
		{"name not printable", false, func(s *fwrtest.State) { s.Devices[2].Name = "batt\x00ry" }},
		{"materials at cap", true, func(s *fwrtest.State) { s.Materials = slices.Repeat([]string{"m"}, maxMaterials) }},
		{"materials above cap", false, func(s *fwrtest.State) { s.Materials = slices.Repeat([]string{"m"}, maxMaterials+1) }},
		{"no materials", false, func(s *fwrtest.State) { s.Materials = nil }},
		{"empty material name", false, func(s *fwrtest.State) { s.Materials[1] = "" }},
		{"material name above cap", false, func(s *fwrtest.State) { s.Materials[1] = strings.Repeat("m", maxName+1) }},
		{"material name not printable", false, func(s *fwrtest.State) { s.Materials[1] = "ar\x7fmour" }},
		{"struts per node at cap", true, func(s *fwrtest.State) { s.Links = links(1, maxNodeLinks) }},
		{"struts per node above cap", false, func(s *fwrtest.State) { s.Links = links(1, maxNodeLinks+1) }},
		{"strut list at cap", true, func(s *fwrtest.State) { s.Links[0].List = make([]uint32, maxLinkList) }},
		{"strut list above cap", false, func(s *fwrtest.State) { s.Links[0].List = make([]uint32, maxLinkList+1) }},
		{"segments at cap", true, func(s *fwrtest.State) { s.Links[0].Segments = maxSegments }},
		{"segments above cap", false, func(s *fwrtest.State) { s.Links[0].Segments = maxSegments + 1 }},
		{"strut to a missing node", false, func(s *fwrtest.State) { s.Links[2].B = len(s.Nodes) }},
		{"NaN metal", false, func(s *fwrtest.State) { s.Teams[3].Metal = nan }},
		{"Inf energy", false, func(s *fwrtest.State) { s.Teams[1].Energy = inf }},
		{"NaN maximum metal", false, func(s *fwrtest.State) { s.Teams[2].MaxMetal = nan }},
		{"Inf maximum energy", false, func(s *fwrtest.State) { s.Teams[2].MaxEnergy = -inf }},
		{"NaN floated metal", false, func(s *fwrtest.State) { s.Teams[4].FloatedMetal = nan }},
		{"Inf floated energy", false, func(s *fwrtest.State) { s.Teams[5].FloatedEnergy = inf }},
		{"NaN damage dealt", false, func(s *fwrtest.State) { s.Teams[1].DamageDealt = nan }},
		{"Inf damage taken", false, func(s *fwrtest.State) { s.DamageTaken[1] = inf }},
		{"NaN APM", false, func(s *fwrtest.State) { s.Slots[6].APM = nan }},
		{"Inf player damage", false, func(s *fwrtest.State) { s.Slots[0].Damage = inf }},
		{"NaN health", false, func(s *fwrtest.State) { s.Devices[3].Health = nan }},
		{"NaN build progress", false, func(s *fwrtest.State) { s.Devices[1].Progress = nan }},
		{"Inf build progress", false, func(s *fwrtest.State) { s.Devices[2].Progress = -inf }},
		{"Inf device x", false, func(s *fwrtest.State) { s.Devices[0].X = inf }},
		{"NaN device y", false, func(s *fwrtest.State) { s.Devices[3].Y = nan }},
		{"Inf strut hit points", false, func(s *fwrtest.State) { s.Links[3].HP = inf }},
	}
	for _, c := range cases {
		s := synthetic()
		c.edit(&s)
		_, err := parse(s.Payload())
		if c.ok && err != nil || !c.ok && !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func corruptBytes(t *testing.T) {
	le := binary.LittleEndian
	cases := []struct {
		name string
		edit func([]byte)
	}{
		{"extended node without its check", func(p []byte) { p[at(t, p, "0000")] = '1' }},
		{"beam count overflowing", func(p []byte) { le.PutUint32(p[at(t, p, "\xf1\xf1\xf1\xf1")+208:], 0xffffffff) }},
		{"A count overflowing", func(p []byte) { le.PutUint32(p[at(t, p, "\xf1\xf1\xf1\xf1")+212+48:], 0xfffffff0) }},
		{"device flag", func(p []byte) { le.PutUint32(p[at(t, p, "reactor")+7+4:], 2) }},
		{"device marker", func(p []byte) { p[at(t, p, "\xde\xde\xde\xde")] = 0 }},
		{"node pad", func(p []byte) { p[at(t, p, "\xff    ")+5+157] = 0 }},
		{"strut body marker", func(p []byte) { p[at(t, p, "\x01\x01\x01\x01")] = 2 }},
		{"strut body material", func(p []byte) { p[at(t, p, "\x01\x01\x01\x01")+4] = 9 }},
	}
	for _, c := range cases {
		p := synthetic().Payload()
		c.edit(p)
		if _, err := parse(p); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// truncated cuts the payload at every length short of the tail, and passes
// starts outside the world state.
func truncated(t *testing.T) {
	p := synthetic().Payload()
	end, _ := snapshot.End(p)
	for n := range len(p) - tailLen {
		if _, err := Parse(p[:n], min(end, n)); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("cut at %d of %d: %v", n, len(p), err)
		}
	}
	if _, err := Parse(p[:len(p)-tailLen], end); err != nil {
		t.Errorf("without the tail: %v", err)
	}
	for _, start := range []int{-1, 0, headerEnd - 1, len(p) + 1} {
		if _, err := Parse(p, start); !errors.Is(err, ErrCorrupt) {
			t.Errorf("start %d: %v", start, err)
		}
	}
}

// strutCap fills 4096 nodes with 64 struts each, the total cap, then adds
// one more.
func strutCap(t *testing.T) {
	s := synthetic()
	s.Nodes = nodes(maxLinks / maxNodeLinks)
	s.Links = links(len(s.Nodes), maxNodeLinks)
	got, err := parse(s.Payload())
	if err != nil || len(got.Links) != maxLinks {
		t.Fatalf("at cap: %v", err)
	}
	s.Nodes = append(s.Nodes, fwrtest.Node{ID: 1 << 20, Kind: 0xff})
	s.Links = append(s.Links, fwrtest.Link{A: len(s.Nodes) - 1, Device: -1})
	if _, err := parse(s.Payload()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("above cap: %v", err)
	}
}

func nodes(n int) []fwrtest.Node {
	out := make([]fwrtest.Node, n)
	for i := range out {
		out[i] = fwrtest.Node{ID: uint32(i), Team: 1, Kind: 0xff}
	}
	return out
}

func devices(n int) []fwrtest.Device {
	out := make([]fwrtest.Device, n)
	for i := range out {
		out[i] = fwrtest.Device{ID: uint32(i), Name: "store", Team: 1}
	}
	return out
}

// links returns per struts from each of nodes 0 to nodes-1, all to node 0.
func links(nodes, per int) []fwrtest.Link {
	out := make([]fwrtest.Link, 0, nodes*per)
	for a := range nodes {
		for range per {
			out = append(out, fwrtest.Link{A: a, Device: -1})
		}
	}
	return out
}

// snapshots returns one corpus replay's snapshot payloads by the frame of
// the last sync before each.
func snapshots(tb testing.TB, name string) map[int64][]byte {
	tb.Helper()
	return snapshotsAt(tb, fixture.Path(tb, "..", name))
}

// snapshotsAt returns the snapshot payloads of the replay at path by frame.
func snapshotsAt(tb testing.TB, path string) map[int64][]byte {
	tb.Helper()
	hdr, blocks := fixture.ReadPath(tb, path)
	out := map[int64][]byte{}
	var dec *msg.Decoder
	for _, b := range blocks {
		s := stream.New(b)
		if dec == nil {
			dec = msg.NewDecoder(s, msg.DefaultRegistry(), hdr.Version, msg.Options{})
		} else {
			dec.Reset(s)
		}
		for dec.Next() {
			if m, ok := dec.Message().(*msg.Snapshot); ok {
				out[dec.Frame()] = m.Payload
			}
		}
		if err := dec.Err(); err != nil {
			tb.Fatal(err)
		}
	}
	return out
}

// corpusStates parses the snapshots of one corpus replay at the frames
// asked for.
func corpusStates(t *testing.T, name string, frames ...int64) map[int64]*State {
	t.Helper()
	snaps := snapshots(t, name)
	out := map[int64]*State{}
	for _, f := range frames {
		p, ok := snaps[f]
		if !ok {
			t.Fatalf("%s: no snapshot at frame %d", name, f)
		}
		s, err := parse(p)
		if err != nil {
			t.Fatalf("%s frame %d: %v", name, f, err)
		}
		out[f] = s
	}
	return out
}

// team1 returns team 1's counters.
func (s *State) team1(t *testing.T) Team {
	t.Helper()
	for _, tm := range s.Teams {
		if tm.ID == 1 {
			return tm
		}
	}
	t.Fatal("no team 1")
	return Team{}
}

func (s *State) device(id uint32) (Device, bool) {
	for _, d := range s.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return Device{}, false
}

func TestParseCorpus(t *testing.T) {
	t.Run("economy", corpusEconomy)
	t.Run("losses", corpusLosses)
	t.Run("damage", corpusDamage)
}

func corpusEconomy(t *testing.T) {
	st := corpusStates(t, "economy-storage.fwr", 1518, 2904, 6100)
	for _, c := range []struct {
		frame         int64
		metal, energy float32
	}{
		{1518, 789.8690185546875, 6076.05908203125},
		{6100, 2686.447265625, 20578.888671875},
	} {
		if tm := st[c.frame].team1(t); tm.FloatedMetal != c.metal || tm.FloatedEnergy != c.energy {
			t.Errorf("frame %d: floated %v, %v", c.frame, tm.FloatedMetal, tm.FloatedEnergy)
		}
	}
	if tm := st[2904].team1(t); tm.MaxEnergy != 5000 {
		t.Errorf("frame 2904: maximum energy %v", tm.MaxEnergy)
	}
	if tm := st[6100].team1(t); tm.MaxMetal != 900 {
		t.Errorf("frame 6100: maximum metal %v", tm.MaxMetal)
	}
}

func corpusLosses(t *testing.T) {
	st := corpusStates(t, "area-repair-damaged.fwr", 12910, 13038, 13166, 14318)
	if d, ok := st[12910].device(8); !ok || d.Name != "machinegun" || d.Team != 1 || !d.Weapon {
		t.Errorf("frame 12910: device 8 %+v", d)
	}
	if _, ok := st[13038].device(8); ok {
		t.Error("frame 13038: device 8 still there")
	}
	for f, want := range map[int64]float32{13038: 100, 13166: 80.89825439453125, 14318: 56.16320037841797} {
		if d, ok := st[f].device(7); !ok || d.Name != "reactor" || d.Team != 1 || d.Health != want {
			t.Errorf("frame %d: device 7 %+v", f, d)
		}
		for _, d := range st[f].Devices {
			if d.Team == 102 && d.Name == "reactor" && d.Health != 100 {
				t.Errorf("frame %d: team 102 reactor %+v", f, d)
			}
		}
	}
}

func corpusDamage(t *testing.T) {
	st := corpusStates(t, "damage-weapons.fwr", 2904, 3796, 4052, 7364, 7490)
	if d, ok := st[2904].device(13); !ok || d.Name != "cannon" || d.Health != 264 || st[2904].DamageTaken[0] != 0 {
		t.Errorf("frame 2904: device 13 %+v, side 1 taken %v", d, st[2904].DamageTaken[0])
	}
	if apm := st[3796].Slots[0].APM; apm != 30 {
		t.Errorf("frame 3796: slot 0 APM %v", apm)
	}
	s := st[4052]
	if s.team1(t).DamageDealt != 126 || s.DamageTaken != [2]float32{0, 126} || s.Slots[0].Team != 1 || s.Slots[0].Damage != 126 {
		t.Errorf("frame 4052: dealt %v, taken %v, slot 0 %+v", s.team1(t).DamageDealt, s.DamageTaken, s.Slots[0])
	}
	s = st[7364]
	if d := s.team1(t).DamageDealt; d != 1574.033203125 || s.DamageTaken[1] != d || s.Slots[0].Damage != d {
		t.Errorf("frame 7364: dealt %v, side 2 taken %v, slot 0 %+v", d, s.DamageTaken[1], s.Slots[0])
	}
	s = st[7490]
	if d := s.team1(t).DamageDealt; d != 7882.13134765625 || s.Slots[0].Damage != 8496.6171875 {
		t.Errorf("frame 7490: dealt %v, slot 0 %+v", d, s.Slots[0])
	}
}
