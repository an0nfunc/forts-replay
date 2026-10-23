// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/state"
)

// stateReplay returns a synthetic replay of version v from header setup s:
// snapshot i at frame 10(i+1), followed by the payloads in cmds[i], given
// as version 1383 writes them.
func stateReplay(v uint32, s fwrtest.Setup, snaps []fwrtest.State, cmds ...[][]byte) []byte {
	return snapReplay(v, s, payloads(v, snaps), cmds...)
}

// payloads lays out each snapshot for header version v.
func payloads(v uint32, snaps []fwrtest.State) [][]byte {
	out := make([][]byte, len(snaps))
	for i, st := range snaps {
		st.Version = v
		out[i] = st.Payload()
	}
	return out
}

// snapReplay is stateReplay with the snapshot payloads given, as version v
// writes them.
func snapReplay(v uint32, s fwrtest.Setup, snaps [][]byte, cmds ...[][]byte) []byte {
	retype, extra := inVersion(v, s)
	var recs [][]byte
	for i, p := range snaps {
		f := uint32(10 * (i + 1))
		recs = append(recs, fwrtest.Record(retype(fwrtest.Tick(f+2)), i == 0), fwrtest.Record(retype(fwrtest.Sync(f)), false), fwrtest.Record(p, false))
		if i < len(cmds) {
			for _, c := range cmds[i] {
				recs = append(recs, fwrtest.Record(retype(c), false))
			}
		}
	}
	return fwrtest.File(fwrtest.Header{Version: v, Frames: uint32(10 * len(snaps)), Extra: extra}, bytes.Join(recs, nil))
}

// solo is a header setup with one player on team 101.
func solo() fwrtest.Setup {
	return fwrtest.Setup{
		Map: "maps/Test/Test.fwe", Recorder: 1, RecorderTeam: 101,
		Players: []fwrtest.Player{{Slot: 0, ID: 1, Team: 101, Name: "Alpha"}},
	}
}

// testTypes gives each synthetic device name one type ID.
var testTypes = map[string]uint16{"reactor": 1, "gun": 2, "store": 3}

// dev returns a device on the ground; a gun is a weapon.
func dev(id uint32, team int32, name string) fwrtest.Device {
	return fwrtest.Device{ID: id, Name: name, Team: team, Type: testTypes[name], Health: 100, Weapon: name == "gun", NodeA: 0xffffffff, NodeB: 0xffffffff}
}

// snap returns a snapshot with the teams in that team-list order, the
// devices, and one node of team 101 unless nodes are given.
func snap(teams []int32, devs []fwrtest.Device, nodes []fwrtest.Node, links []fwrtest.Link) fwrtest.State {
	st := fwrtest.State{
		NextNode: 100, NextDevice: 100, Lua: []string{"data = { }"},
		Devices: devs, Nodes: nodes, Links: links, Materials: []string{"m"},
	}
	for _, id := range teams {
		st.Teams = append(st.Teams, fwrtest.Team{ID: id, Metal: float32(id), FloatedEnergy: float32(id) / 2, DamageDealt: 2 * float32(id)})
	}
	if len(st.Nodes) == 0 {
		st.Nodes = []fwrtest.Node{{ID: 1, Team: 101, Kind: 0xff}}
	}
	return st
}

func buildOpts(t *testing.T, data []byte, lim Limits) *Summary {
	t.Helper()
	s, err := Build(t.Context(), bytes.NewReader(data), Options{Timeline: lim})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// checkLengths fails unless every series array has one entry per frame.
func checkLengths(tb testing.TB, ss *StateSeries) {
	tb.Helper()
	n := len(ss.Frames)
	for _, ts := range ss.Teams {
		for i, l := range []int{
			len(ts.Metal), len(ts.Energy), len(ts.MaxMetal), len(ts.MaxEnergy), len(ts.FloatedMetal), len(ts.FloatedEnergy),
			len(ts.DamageDealt), len(ts.ReactorHealth), len(ts.Struts), len(ts.StrutHP), len(ts.StrutsLost), len(ts.Nodes),
			len(ts.Devices), len(ts.Weapons),
		} {
			if l != n {
				tb.Fatalf("team %d: array %d has %d entries, %d frames", ts.Team, i, l, n)
			}
		}
	}
	for _, sd := range ss.Sides {
		if len(sd.DamageTaken) != n {
			tb.Fatalf("side %d: %d entries, %d frames", sd.Side, len(sd.DamageTaken), n)
		}
	}
	for _, p := range ss.Players {
		if len(p.Team) != n || len(p.Damage) != n || len(p.APM) != n {
			tb.Fatalf("player slot %d: %d, %d, %d entries, %d frames", p.Slot, len(p.Team), len(p.Damage), len(p.APM), n)
		}
	}
}

// TestStateTeams: teams map through the team list in any slot order; a
// team that appears later is padded at the front, one missing from the
// list or dead gets 0, teams without a device are left out, and past 18
// teams the rest get none, with a warning.
func TestStateTeams(t *testing.T) {
	s0 := snap([]int32{0, 1, 2, 201, 101, 102}, []fwrtest.Device{dev(1, 101, "reactor"), dev(2, 102, "reactor"), dev(3, 201, "reactor")}, nil, nil)
	s1 := snap([]int32{0, 2, 1, 102, 202, 101}, []fwrtest.Device{dev(1, 101, "reactor"), dev(4, 202, "reactor")}, nil, nil)
	s1.Teams[3] = fwrtest.Team{ID: 102} // dead: its entries drop to 0
	s := buildFile(t, stateReplay(1383, solo(), []fwrtest.State{s0, s1}))
	if len(s.Warnings) != 0 || s.State == nil {
		t.Fatalf("warnings %q, state %v", s.Warnings, s.State)
	}
	checkLengths(t, s.State)
	type row struct {
		team    int32
		side    int
		metal   []float32
		dealt   []float32
		reactor []float64
		devices []int
	}
	var got []row
	for _, ts := range s.State.Teams {
		got = append(got, row{ts.Team, ts.Side, ts.Metal, ts.DamageDealt, ts.ReactorHealth, ts.Devices})
	}
	want := []row{
		{101, 1, []float32{101, 101}, []float32{202, 202}, []float64{100, 100}, []int{1, 1}},
		{102, 2, []float32{102, 0}, []float32{204, 0}, []float64{100, 0}, []int{1, 0}},
		{201, 1, []float32{201, 0}, []float32{402, 0}, []float64{100, 0}, []int{1, 0}},
		{202, 2, []float32{0, 202}, []float32{0, 404}, []float64{0, 100}, []int{0, 1}},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(s.State.Frames, []int64{10, 20}) {
		t.Errorf("frames %v, teams\n got %+v\nwant %+v", s.State.Frames, got, want)
	}

	// 19 teams: the first 18 in team-list order keep series.
	var ids []int32
	var devs []fwrtest.Device
	for i := range 19 {
		ids = append(ids, int32(301+i))
		devs = append(devs, dev(uint32(10+i), int32(301+i), "reactor"))
	}
	s = buildFile(t, stateReplay(1383, solo(), []fwrtest.State{snap(ids, devs, nil, nil)}))
	if s.State == nil || len(s.State.Teams) != 18 || s.State.Teams[17].Team != 318 ||
		!slices.Equal(s.Warnings, []string{"state series kept for 18 teams, 1 more team-list entries ignored"}) {
		t.Errorf("19 teams: state %+v, warnings %q", s.State, s.Warnings)
	}
}

// TestLosses: a scrap, an upgrade or a move in the window, also a scrap of a
// device created after the latest snapshot, and a scrap three intervals
// before the device vanishes all mean no loss. A device that vanishes
// without one is lost, also when an upgrade named it before the window, a
// second scrap cancelled its scrap, or its build progress fell and
// recovered.
func TestLosses(t *testing.T) {
	devs := func(ids ...uint32) []fwrtest.Device {
		var out []fwrtest.Device
		for _, id := range ids {
			name := "store"
			if id == 13 {
				name = "gun"
			}
			out = append(out, dev(id, 101, name))
		}
		return out
	}
	snaps := []fwrtest.State{
		snap([]int32{101}, devs(10, 11, 12, 13, 14, 15, 16, 17, 18), nil, nil),
		snap([]int32{101}, devs(10, 11, 12, 13, 14, 15, 16, 17, 18, 20), nil, nil),
		snap([]int32{101}, devs(14, 15, 16, 17, 18, 21), nil, nil),
		snap([]int32{101}, devs(21), nil, nil),
	}
	snaps[0].NextDevice, snaps[1].NextDevice, snaps[2].NextDevice, snaps[3].NextDevice = 20, 21, 22, 22
	for i, p := range []float32{5, 3, 5} { // device 18 dips and recovers
		ds := snaps[i].Devices
		ds[slices.IndexFunc(ds, func(d fwrtest.Device) bool { return d.ID == 18 })].Progress = p
	}
	cmds := [][][]byte{{
		fwrtest.Scrap(1, 101, 10), fwrtest.Upgrade(1, 101, 11), fwrtest.Place(1, 101, 3, 0, 0, 12),
		fwrtest.Place(1, 101, 3, 0, 0, -1), fwrtest.Scrap(1, 101, 20), fwrtest.Upgrade(1, 101, 15),
		fwrtest.Scrap(1, 101, 16), fwrtest.Scrap(1, 101, 17),
	}, {fwrtest.Scrap(1, 101, 17)}}
	s := buildFile(t, stateReplay(1383, solo(), snaps, cmds...))
	want := []DeviceEvent{
		{Frame: 30, Team: 101, Device: 13, TypeID: 2, Name: "gun", Weapon: true},
		{Frame: 40, Team: 101, Device: 14, TypeID: 3, Name: "store"},
		{Frame: 40, Team: 101, Device: 15, TypeID: 3, Name: "store"},
		{Frame: 40, Team: 101, Device: 17, TypeID: 3, Name: "store"},
		{Frame: 40, Team: 101, Device: 18, TypeID: 3, Name: "store"},
	}
	if !reflect.DeepEqual(s.Losses, want) || len(s.Warnings) != 0 {
		t.Errorf("losses %+v, warnings %q", s.Losses, s.Warnings)
	}
}

// TestScrapMarks: scraps naming devices created after the latest decoded
// snapshot count against the window's cap, with one warning; those naming
// its devices always count; marks are pruned to the next decoded
// snapshot's devices.
func TestScrapMarks(t *testing.T) {
	var warnings []string
	sb := newStateBuilder(state.Version, nil, Limits{}, func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	s0 := snap([]int32{101}, []fwrtest.Device{dev(1, 101, "store"), dev(2, 101, "store")}, nil, nil)
	s0.NextDevice = 10
	sb.snapshot(10, s0.Payload())
	for i := range 20000 {
		sb.command(&msg.Scrap{Device: 10 + uint32(i)})
	}
	sb.command(&msg.Scrap{Device: 1})
	if len(sb.marks) != maxNewIDs+1 || !sb.marks[1] || sb.marks[10+maxNewIDs] {
		t.Errorf("%d marks, device 1 %v", len(sb.marks), sb.marks[1])
	}
	s1 := snap([]int32{101}, []fwrtest.Device{dev(2, 101, "store"), dev(10, 101, "store")}, nil, nil)
	s1.NextDevice = 11
	sb.snapshot(20, s1.Payload())
	if !reflect.DeepEqual(sb.marks, map[uint32]bool{10: true}) || sb.losses != nil {
		t.Errorf("marks %v, losses %+v", sb.marks, sb.losses)
	}
	sb.finish(&Summary{})
	want := []string{fmt.Sprintf("loss window cut at %d commands naming new objects per snapshot interval, %d more ignored", maxNewIDs, 20000-maxNewIDs)}
	if !slices.Equal(warnings, want) {
		t.Errorf("warnings %q", warnings)
	}
}

// TestWindowCap: upgrades, moves, new placements, strut deletes and node
// deletes naming objects created after the latest decoded snapshot count
// against the window's cap, with one warning. TestScrapMarks covers scraps.
func TestWindowCap(t *testing.T) {
	for _, c := range []struct {
		name string
		cmd  func(i uint32) msg.Message
	}{
		{"upgrade", func(i uint32) msg.Message { return &msg.Upgrade{Team: 101, Device: 100 + i} }},
		{"move", func(i uint32) msg.Message {
			return &msg.Place{Team: 101, NodeA: 1, NodeB: 2 + i, MovedDevice: int32(100 + i)}
		}},
		{"new placement", func(i uint32) msg.Message { return &msg.Place{Team: 101, NodeA: 1, NodeB: 100 + i, MovedDevice: -1} }},
		{"delete_link", func(i uint32) msg.Message { return &msg.DeleteLink{Team: 101, NodeA: 100 + i, NodeB: 1} }},
		{"delete_node", func(i uint32) msg.Message { return &msg.DeleteNode{Team: 101, Node: 100 + i} }},
	} {
		var warnings []string
		sb := newStateBuilder(1383, nil, Limits{}, func(format string, args ...any) {
			warnings = append(warnings, fmt.Sprintf(format, args...))
		})
		sb.snapshot(10, snap([]int32{101}, []fwrtest.Device{dev(1, 101, "store")}, nil, nil).Payload())
		for i := range uint32(20000) {
			sb.command(c.cmd(i))
		}
		w := sb.recent
		n := len(w.devices) + len(w.pairs) + len(w.nodes) + len(w.moves) + len(w.places)
		sb.finish(&Summary{})
		if n > maxNewIDs || len(warnings) != 1 || !strings.HasPrefix(warnings[0], fmt.Sprintf("loss window cut at %d commands", maxNewIDs)) {
			t.Errorf("%s: %d window entries, warnings %q", c.name, n, warnings)
		}
	}
}

// TestBuilds: a build is confirmed by a flat sighting, or by a fall after a
// rise slower than the time elapsed, at the frame of its final value. A
// device destroyed or last seen while rising, cancelled after a full-rate
// rise, or first seen falling gets none, and so do the starting forts.
// MaxBuilds keeps the earliest confirmed.
func TestBuilds(t *testing.T) {
	// Progress per snapshot, -1 when absent; snapshots are 0.4 s apart.
	const gone = -1
	devices := []struct {
		dev      fwrtest.Device
		progress []float32
	}{
		{dev(1, 101, "reactor"), []float32{1, 1, 1, 1, 1, 1}},             // starting fort
		{dev(100, 101, "gun"), []float32{gone, 1, 2, 3, 3, 3}},            // finished at frame 40
		{dev(101, 101, "store"), []float32{gone, 1, 2, gone}},             // destroyed while rising
		{dev(102, 101, "store"), []float32{gone, gone, 5, 5, 5, 5}},       // seen once at its final value: frame 30
		{dev(103, 101, "store"), []float32{gone, 1, 2, 1, gone}},          // full-rate rise, then scrapped
		{dev(104, 101, "store"), []float32{gone, 1, 1.25, 1, 0.5, gone}},  // partial rise, then a fall: frame 30
		{dev(105, 101, "store"), []float32{gone, gone, 2, 1, 0.5, gone}},  // first seen falling
		{dev(106, 101, "store"), []float32{gone, gone, gone, gone, 1, 2}}, // last seen rising
	}
	var snaps []fwrtest.State
	for i := range 6 {
		var devs []fwrtest.Device
		for _, d := range devices {
			if i < len(d.progress) && d.progress[i] != gone {
				d.dev.Progress = d.progress[i]
				devs = append(devs, d.dev)
			}
		}
		snaps = append(snaps, snap([]int32{101}, devs, nil, nil)) // devices from 100 on are new
	}
	data := stateReplay(1383, solo(), snaps)
	builds := []DeviceEvent{
		{Frame: 30, Team: 101, Device: 102, TypeID: 3, Name: "store"},
		{Frame: 30, Team: 101, Device: 104, TypeID: 3, Name: "store"},
		{Frame: 40, Team: 101, Device: 100, TypeID: 2, Name: "gun", Weapon: true},
	}
	for _, c := range []struct {
		lim    Limits
		builds []DeviceEvent
		warn   []string
	}{
		{Limits{}, builds, nil},
		{Limits{MaxStates: -1}, builds, nil},
		{Limits{MaxBuilds: 2}, builds[:2], []string{"builds cut at 2, 1 more counted only"}},
		{Limits{MaxBuilds: -1}, nil, nil},
	} {
		s := buildOpts(t, data, c.lim)
		if !reflect.DeepEqual(s.Builds, c.builds) || !slices.Equal(s.Warnings, c.warn) {
			t.Errorf("%+v: builds %+v, warnings %q", c.lim, s.Builds, s.Warnings)
		}
	}
}

// TestBuildingPruned: a device that vanishes while rising leaves the build
// tracker.
func TestBuildingPruned(t *testing.T) {
	sb := newStateBuilder(1383, nil, Limits{}, func(string, ...any) {})
	at := func(progress float32) fwrtest.State {
		d := dev(100, 101, "store")
		d.Progress = progress
		return snap([]int32{101}, []fwrtest.Device{d}, nil, nil)
	}
	empty := snap([]int32{101}, nil, nil, nil)
	for i, st := range []fwrtest.State{empty, at(1), at(2), empty} {
		sb.snapshot(int64(10*(i+1)), st.Payload())
		if want := []int{0, 1, 1, 0}[i]; len(sb.building) != want {
			t.Errorf("snapshot %d: %d devices tracked, want %d", i, len(sb.building), want)
		}
	}
}

// TestReplaced: a Phantom move's placeholder, also of a move that a later
// move supersedes, and a device the AI upgrades without a command are no
// losses, as in the corpus's Phantom probe; a rebuild of the same name, a
// placement on the pair, an empty pair and a pair another team holds leave
// a loss.
func TestReplaced(t *testing.T) {
	type step struct {
		devs []fwrtest.Device
		next uint32 // next device ID
		cmds [][]byte
	}
	for _, c := range []struct {
		name  string
		steps []step
		lost  []uint32
	}{
		{"phantom placeholder", []step{
			{[]fwrtest.Device{on(10, 101, "store", 1, 2)}, 13, [][]byte{fwrtest.Place(1, 101, 3, 3, 4, 10)}},
			{[]fwrtest.Device{on(10, 101, "store", 1, 2), on(13, 101, "store", 4, 3)}, 14, nil},
			{[]fwrtest.Device{on(10, 101, "store", 3, 4)}, 14, nil},
		}, nil},
		{"superseded move", []step{
			{[]fwrtest.Device{on(10, 101, "store", 1, 2)}, 13, [][]byte{fwrtest.Place(1, 101, 3, 3, 4, 10)}},
			{[]fwrtest.Device{on(10, 101, "store", 1, 2), on(13, 101, "store", 3, 4)}, 14, [][]byte{fwrtest.Place(1, 101, 3, 6, 5, 10)}},
			{[]fwrtest.Device{on(10, 101, "store", 5, 6)}, 15, nil},
		}, nil},
		{"upgrade without a command", []step{
			{[]fwrtest.Device{on(10, 102, "gun", 1, 2)}, 11, nil},
			{[]fwrtest.Device{on(11, 102, "store", 2, 1)}, 12, nil},
		}, nil},
		{"same-name rebuild", []step{
			{[]fwrtest.Device{on(10, 102, "gun", 1, 2)}, 11, nil},
			{[]fwrtest.Device{on(11, 102, "gun", 1, 2)}, 12, nil},
		}, []uint32{10}},
		{"placement on the pair", []step{
			{[]fwrtest.Device{on(10, 101, "gun", 1, 2)}, 11, [][]byte{fwrtest.Place(1, 101, 3, 2, 1, -1)}},
			{[]fwrtest.Device{on(11, 101, "store", 1, 2)}, 12, nil},
		}, []uint32{10}},
		{"empty pair and another team's", []step{
			{[]fwrtest.Device{on(10, 101, "gun", 1, 2), on(11, 101, "gun", 3, 4)}, 12, nil},
			{[]fwrtest.Device{on(12, 102, "store", 3, 4)}, 13, nil},
		}, []uint32{10, 11}},
	} {
		var snaps []fwrtest.State
		var cmds [][][]byte
		for _, st := range c.steps {
			sn := snap([]int32{101, 102}, st.devs, nodeRun(6), nil)
			sn.NextDevice = st.next
			snaps = append(snaps, sn)
			cmds = append(cmds, st.cmds)
		}
		s := buildFile(t, stateReplay(1383, solo(), snaps, cmds...))
		var lost []uint32
		for _, l := range s.Losses {
			lost = append(lost, l.Device)
		}
		if !slices.Equal(lost, c.lost) || len(s.Warnings) != 0 {
			t.Errorf("%s: losses %+v, warnings %q", c.name, s.Losses, s.Warnings)
		}
	}
	if s := buildFile(t, fixtureFile(t, "probe-move-phantom.fwr")); s.Losses != nil {
		t.Errorf("probe-move-phantom.fwr: losses %+v", s.Losses)
	}
}

// TestReplacedNames: with one name or two among the devices new on a pair,
// a vanished device of another name was upgraded, and one of the only new
// name was lost; several new devices of one name count as one name.
func TestReplacedNames(t *testing.T) {
	for _, c := range []struct {
		fresh []string // names of the new devices on the pair
		gone  string
		lost  bool
	}{
		{[]string{"store"}, "gun", false},
		{[]string{"store"}, "store", true},
		{[]string{"store", "store", "store"}, "store", true},
		{[]string{"store", "reactor"}, "gun", false},
		{[]string{"store", "reactor"}, "store", false},
		{[]string{"store", "store", "reactor"}, "reactor", false},
	} {
		devs := []fwrtest.Device{on(10, 102, c.gone, 1, 2)}
		next := []fwrtest.Device{on(11, 102, "old", 1, 2)} // not new, so its name does not count
		for i, name := range c.fresh {
			next = append(next, on(12+uint32(i), 102, name, 2, 1))
		}
		devs = append(devs, next[0])
		var snaps []fwrtest.State
		for i, ds := range [][]fwrtest.Device{devs, next} {
			sn := snap([]int32{101, 102}, ds, nodeRun(6), nil)
			sn.NextDevice = 12 + uint32(i*len(c.fresh))
			snaps = append(snaps, sn)
		}
		s := buildFile(t, stateReplay(1383, solo(), snaps))
		if lost := len(s.Losses) == 1 && s.Losses[0].Device == 10; lost != c.lost || len(s.Losses) > 1 {
			t.Errorf("%v, %s gone: losses %+v", c.fresh, c.gone, s.Losses)
		}
	}
}

// on returns a device on the node pair a, b.
func on(id uint32, team int32, name string, a, b uint32) fwrtest.Device {
	d := dev(id, team, name)
	d.NodeA, d.NodeB = a, b
	return d
}

// nodeRun returns nodes 1 to n of team 101.
func nodeRun(n int) []fwrtest.Node {
	var out []fwrtest.Node
	for i := range uint32(n) {
		out = append(out, fwrtest.Node{ID: i + 1, Team: 101, Kind: 0xff})
	}
	return out
}

// TestStrutsLost: struts removed by a strut delete (named in either
// order), a node delete, or a deconstruction already under way are not
// lost; one that vanishes without them is.
func TestStrutsLost(t *testing.T) {
	nodes := nodeRun(9)
	nodes[8].Team = 0 // a strut from it counts for node B's team
	link := func(a, b int, hp float32, state byte) fwrtest.Link {
		return fwrtest.Link{A: a, B: b, HP: hp, State: state, Device: -1}
	}
	keep := []fwrtest.Link{link(0, 2, 10, 1), link(8, 0, 20, 1)}
	gone := []fwrtest.Link{link(0, 1, 30, 1), link(2, 3, 40, 1), link(4, 5, 50, 3), link(6, 7, 60, 1)}
	reactor := []fwrtest.Device{dev(1, 101, "reactor")}
	snaps := []fwrtest.State{
		snap([]int32{101}, reactor, nodes, slices.Concat(keep, gone)),
		snap([]int32{101}, reactor, nodes, keep),
	}
	cmds := [][][]byte{{fwrtest.DeleteLink(1, 101, 2, 1), fwrtest.DeleteNode(1, 101, 4)}}
	s := buildFile(t, stateReplay(1383, solo(), snaps, cmds...))
	if len(s.Warnings) != 0 || s.State == nil || len(s.State.Teams) != 1 {
		t.Fatalf("warnings %q, state %+v", s.Warnings, s.State)
	}
	ts := s.State.Teams[0]
	if !reflect.DeepEqual(ts.StrutsLost, []int{0, 1}) || !reflect.DeepEqual(ts.Struts, []int{6, 2}) ||
		!reflect.DeepEqual(ts.StrutHP, []float64{210, 30}) || !reflect.DeepEqual(ts.Nodes, []int{8, 2}) {
		t.Errorf("lost %v, struts %v, hp %v, nodes %v", ts.StrutsLost, ts.Struts, ts.StrutHP, ts.Nodes)
	}
	if s.Losses != nil {
		t.Errorf("losses %+v", s.Losses)
	}
}

// TestBuildStateErrors: snapshots whose state does not decode give one
// warning with their count and no series point; losses diff against the
// last decoded snapshot, with the window anchored to decoded ones; the
// timelines do not change.
func TestBuildStateErrors(t *testing.T) {
	two := []fwrtest.Device{dev(1, 101, "store"), dev(2, 101, "store")}
	good := []fwrtest.State{
		snap([]int32{101}, two, nil, nil), snap([]int32{101}, two, nil, nil), snap([]int32{101}, two, nil, nil),
		snap([]int32{101}, two, nil, nil), snap([]int32{101}, nil, nil, nil),
	}
	snaps := payloads(1383, good)
	noNodes := good[2]
	noNodes.Nodes = []fwrtest.Node{}
	snaps[2] = noNodes.Payload()
	snaps[3] = fwrtest.Snapshot("data = { }") // ends inside the header tables
	cmds := [][][]byte{nil, {fwrtest.Upgrade(1, 101, 1)}, {fwrtest.Ability(1)}, {fwrtest.Ability(1)}}
	s := buildFile(t, snapReplay(1383, solo(), snaps, cmds...))
	ref := buildFile(t, stateReplay(1383, solo(), good, cmds...))
	if len(s.Warnings) != 1 || !strings.HasPrefix(s.Warnings[0], "snapshot state does not decode in 2 snapshots; the first, at frame 30: state: corrupt") {
		t.Errorf("warnings %q", s.Warnings)
	}
	if s.State == nil || !reflect.DeepEqual(s.State.Frames, []int64{10, 20, 50}) {
		t.Fatalf("state %+v", s.State)
	}
	checkLengths(t, s.State)
	if want := []DeviceEvent{{Frame: 50, Team: 101, Device: 2, TypeID: 3, Name: "store"}}; !reflect.DeepEqual(s.Losses, want) {
		t.Errorf("losses %+v", s.Losses)
	}
	if !reflect.DeepEqual(s.Timelines, ref.Timelines) || len(s.Timelines) != 1 || len(s.Timelines[0].Commands) != 3 {
		t.Errorf("timelines %+v, want %+v", s.Timelines, ref.Timelines)
	}
}

// TestStateLimit: MaxStates and MaxLosses bound the series and the losses;
// past a positive limit a warning says so, and with MaxStates negative the
// losses are still found.
func TestStateLimit(t *testing.T) {
	var snaps []fwrtest.State
	for i := range 5 {
		var devs []fwrtest.Device
		if i < 4 {
			devs = []fwrtest.Device{dev(1, 101, "reactor"), dev(2, 101, "gun")}
		}
		snaps = append(snaps, snap([]int32{101}, devs, nil, nil))
	}
	data := stateReplay(1383, solo(), snaps)
	lost := []DeviceEvent{
		{Frame: 50, Team: 101, Device: 1, TypeID: 1, Name: "reactor"},
		{Frame: 50, Team: 101, Device: 2, TypeID: 2, Name: "gun", Weapon: true},
	}
	for _, c := range []struct {
		lim    Limits
		points int // -1: no state
		losses []DeviceEvent
		warn   []string
	}{
		{Limits{}, 5, lost, nil},
		{Limits{MaxStates: 3}, 3, lost, []string{"state series cut at 3 points, 2 more counted only"}},
		{Limits{MaxStates: -1}, -1, lost, nil},
		{Limits{MaxLosses: 1}, 5, lost[:1], []string{"losses cut at 1, 1 more counted only"}},
		{Limits{MaxLosses: -1}, 5, nil, nil},
	} {
		s := buildOpts(t, data, c.lim)
		switch {
		case c.points < 0 && s.State != nil:
			t.Errorf("%+v: state %+v", c.lim, s.State)
		case c.points >= 0 && (s.State == nil || len(s.State.Frames) != c.points):
			t.Errorf("%+v: state %+v", c.lim, s.State)
		case !reflect.DeepEqual(s.Losses, c.losses) || !slices.Equal(s.Warnings, c.warn):
			t.Errorf("%+v: losses %+v, warnings %q", c.lim, s.Losses, s.Warnings)
		}
		if s.State != nil {
			checkLengths(t, s.State)
		}
	}
}

// TestStateVersion: snapshots of another header version are not decoded;
// the first one adds a warning, and a replay without snapshots none.
func TestStateVersion(t *testing.T) {
	snaps := []fwrtest.State{snap([]int32{101}, []fwrtest.Device{dev(1, 101, "gun")}, nil, nil), snap([]int32{101}, nil, nil, nil)}
	s := buildFile(t, stateReplay(1331, solo(), snaps))
	want := []string{"header fields not decoded for version 1331", "snapshot state not decoded for version 1331"}
	if s.State != nil || s.Losses != nil || s.DeviceTypes != nil || !reflect.DeepEqual(s.Warnings, want) {
		t.Errorf("state %+v, losses %+v, device types %v, warnings %q", s.State, s.Losses, s.DeviceTypes, s.Warnings)
	}
	steps := bytes.Join([][]byte{fwrtest.Record(fwrtest.Tick(12), true), fwrtest.Record(fwrtest.Sync(10), false)}, nil)
	s = buildFile(t, fwrtest.File(fwrtest.Header{Version: 1331, Frames: 10, Extra: solo().Extra()}, steps))
	if !reflect.DeepEqual(s.Warnings, want[:1]) {
		t.Errorf("no snapshots: warnings %q", s.Warnings)
	}
}

// TestDeviceTypes: a type ID keeps its first name, with one warning for a
// second one, and the map stops at 1024 entries.
func TestDeviceTypes(t *testing.T) {
	named := func(name string) fwrtest.State {
		return snap([]int32{101}, []fwrtest.Device{{ID: 1, Name: name, Team: 101, Type: 7, Health: 1}}, nil, nil)
	}
	s := buildFile(t, stateReplay(1383, solo(), []fwrtest.State{named("alpha"), named("beta"), named("gamma")}))
	if !reflect.DeepEqual(s.DeviceTypes, map[uint16]string{7: "alpha"}) ||
		!reflect.DeepEqual(s.Warnings, []string{`device type 7 is named "alpha" and "beta"; the first name stays`}) {
		t.Errorf("device types %v, warnings %q", s.DeviceTypes, s.Warnings)
	}
	var devs []fwrtest.Device
	for i := range 1025 {
		devs = append(devs, fwrtest.Device{ID: uint32(i + 1), Name: fmt.Sprintf("d%d", i), Team: 101, Type: uint16(i), Health: 1})
	}
	s = buildFile(t, stateReplay(1383, solo(), []fwrtest.State{snap([]int32{101}, devs, nil, nil)}))
	if len(s.DeviceTypes) != 1024 || s.DeviceTypes[1023] != "d1023" || !reflect.DeepEqual(s.Warnings, []string{"device types cut at 1024"}) {
		t.Errorf("%d device types, warnings %q", len(s.DeviceTypes), s.Warnings)
	}
}

// TestStatePlayers: player series follow the header's slots, gaps
// included; a spectator gets none, a player who leaves keeps an entry per
// frame with team and damage 0, and a slot past the table gets none and a
// warning.
func TestStatePlayers(t *testing.T) {
	setup := fwrtest.Setup{
		Map: "maps/Test/Test.fwe", Recorder: 1, RecorderTeam: 101,
		Players: []fwrtest.Player{
			{Slot: 0, ID: 1, Team: 101}, {Slot: 1, ID: 2, Team: 102}, {Slot: 2, ID: 3, Team: -3},
			{Slot: 5, ID: 4, Team: 201}, {Slot: 6, ID: 5, Team: 202}, {Slot: 9, ID: 6, Team: 101},
		},
	}
	snaps := make([]fwrtest.State, 3)
	for i := range snaps {
		st := snap([]int32{101, 102, 201, 202}, []fwrtest.Device{dev(1, 101, "reactor")}, nil, nil)
		f := float32(i)
		st.Slots[0] = fwrtest.Slot{Team: 101, APM: 10 + f, Damage: 1 + f}
		st.Slots[1] = fwrtest.Slot{Team: 102, APM: 20 + f, Damage: 2 + f}
		st.Slots[2] = fwrtest.Slot{Team: -3}
		st.Slots[5] = fwrtest.Slot{Team: 201, APM: 50 + f, Damage: 5 + f}
		st.Slots[6] = fwrtest.Slot{Team: 202, APM: 60 + f, Damage: 6 + f}
		if i == 2 {
			st.Slots[5] = fwrtest.Slot{} // left
		}
		snaps[i] = st
	}
	s := buildFile(t, stateReplay(1383, setup, snaps))
	want := []PlayerSeries{
		{Slot: 0, Team: []int32{101, 101, 101}, Damage: []float32{1, 2, 3}, APM: []float32{10, 11, 12}},
		{Slot: 1, Team: []int32{102, 102, 102}, Damage: []float32{2, 3, 4}, APM: []float32{20, 21, 22}},
		{Slot: 5, Team: []int32{201, 201, 0}, Damage: []float32{5, 6, 0}, APM: []float32{50, 51, 0}},
		{Slot: 6, Team: []int32{202, 202, 202}, Damage: []float32{6, 7, 8}, APM: []float32{60, 61, 62}},
	}
	if s.State == nil || !reflect.DeepEqual(s.State.Players, want) {
		t.Fatalf("players %+v", s.State)
	}
	checkLengths(t, s.State)
	if !reflect.DeepEqual(s.Warnings, []string{"player 6 has slot 9, past the snapshot's player slots; no state series"}) {
		t.Errorf("warnings %q", s.Warnings)
	}
}

// TestStateCorpus pins series, losses and device types from the corpus.
func TestStateCorpus(t *testing.T) {
	at := func(t *testing.T, s *Summary, frame int64) int {
		t.Helper()
		i := slices.Index(s.State.Frames, frame)
		if i < 0 {
			t.Fatalf("no point at frame %d", frame)
		}
		return i
	}
	team := func(t *testing.T, s *Summary, id int32) TeamSeries {
		t.Helper()
		for _, ts := range s.State.Teams {
			if ts.Team == id {
				return ts
			}
		}
		t.Fatalf("no team %d", id)
		return TeamSeries{}
	}
	t.Run("economy", func(t *testing.T) {
		s := buildFile(t, fixtureFile(t, "economy-storage.fwr"))
		t1 := team(t, s, 1)
		for _, c := range []struct {
			frame         int64
			metal, energy float32
		}{{1518, 789.8690185546875, 6076.05908203125}, {6100, 2686.447265625, 20578.888671875}} {
			if i := at(t, s, c.frame); t1.FloatedMetal[i] != c.metal || t1.FloatedEnergy[i] != c.energy {
				t.Errorf("frame %d: floated %v, %v", c.frame, t1.FloatedMetal[i], t1.FloatedEnergy[i])
			}
		}
		for i, f := range s.State.Frames {
			if f >= 2904 && t1.MaxEnergy[i] != 5000 {
				t.Errorf("frame %d: maximum energy %v", f, t1.MaxEnergy[i])
			}
		}
		if i := at(t, s, 6100); t1.MaxMetal[i] != 900 {
			t.Errorf("frame 6100: maximum metal %v", t1.MaxMetal[i])
		}
	})
	t.Run("losses", func(t *testing.T) {
		s := buildFile(t, fixtureFile(t, "area-repair-damaged.fwr"))
		if !slices.Contains(s.Losses, DeviceEvent{Frame: 13038, Team: 1, Device: 8, TypeID: 0, Name: "machinegun", Weapon: true}) {
			t.Errorf("losses %+v", s.Losses)
		}
		if i := at(t, s, 13166); team(t, s, 1).ReactorHealth[i] != 80.89825439453125 || team(t, s, 102).ReactorHealth[i] != 100 {
			t.Errorf("frame 13166: reactor health %v, %v", team(t, s, 1).ReactorHealth[i], team(t, s, 102).ReactorHealth[i])
		}
	})
	t.Run("damage", func(t *testing.T) {
		s := buildFile(t, fixtureFile(t, "damage-weapons.fwr"))
		t1, p := team(t, s, 1), s.State.Players[0]
		taken := func(side, i int) float32 { return s.State.Sides[side-1].DamageTaken[i] }
		if i := at(t, s, 2904); taken(1, i) != 0 {
			t.Errorf("frame 2904: side 1 taken %v", taken(1, i))
		}
		if i := at(t, s, 3796); p.Slot != 0 || p.APM[i] != 30 {
			t.Errorf("frame 3796: slot %d APM %v", p.Slot, p.APM[i])
		}
		if i := at(t, s, 4052); t1.DamageDealt[i] != 126 || taken(2, i) != 126 || taken(1, i) != 0 || p.Team[i] != 1 || p.Damage[i] != 126 {
			t.Errorf("frame 4052: dealt %v, taken %v and %v, player team %d damage %v", t1.DamageDealt[i], taken(1, i), taken(2, i), p.Team[i], p.Damage[i])
		}
		if i := at(t, s, 7364); t1.DamageDealt[i] != 1574.033203125 || taken(2, i) != t1.DamageDealt[i] || p.Damage[i] != t1.DamageDealt[i] {
			t.Errorf("frame 7364: dealt %v, side 2 taken %v, player damage %v", t1.DamageDealt[i], taken(2, i), p.Damage[i])
		}
		if i := at(t, s, 7490); t1.DamageDealt[i] != 7882.13134765625 || p.Damage[i] != 8496.6171875 {
			t.Errorf("frame 7490: dealt %v, player damage %v", t1.DamageDealt[i], p.Damage[i])
		}
	})
	t.Run("device types", func(t *testing.T) {
		for name, want := range map[string]map[uint16]string{
			"weapons-1-3.fwr": {0: "machinegun", 2: "sniper"},
			"devices.fwr":     {34: "turbine", 37: "battery"},
		} {
			s := buildFile(t, fixtureFile(t, name))
			for id, n := range want {
				if s.DeviceTypes[id] != n {
					t.Errorf("%s: device types %v", name, s.DeviceTypes)
				}
			}
		}
	})
}

// TestStateCorpus1390 pins series, the result, builds and device types
// from the v1.39.0 replays.
func TestStateCorpus1390(t *testing.T) {
	team1 := func(t *testing.T, s *Summary) TeamSeries {
		t.Helper()
		for _, ts := range s.State.Teams {
			if ts.Team == 1 {
				return ts
			}
		}
		t.Fatal("no team 1")
		return TeamSeries{}
	}
	t.Run("economy", func(t *testing.T) {
		s := buildFile(t, fixtureFile1390(t, "economy-storage.fwr"))
		t1, last := team1(t, s), len(s.State.Frames)-1
		if t1.FloatedMetal[last] != 4820.06 || t1.FloatedEnergy[last] != 41138.85 {
			t.Errorf("floated %v, %v at the end", t1.FloatedMetal[last], t1.FloatedEnergy[last])
		}
		for i, f := range s.State.Frames {
			want := [2]float32{600, 3000}
			switch {
			case f >= 6608:
				want = [2]float32{900, 5000}
			case f >= 3412:
				want = [2]float32{600, 5000}
			}
			if got := [2]float32{t1.MaxMetal[i], t1.MaxEnergy[i]}; got != want {
				t.Errorf("frame %d: maximum %v", f, got)
			}
		}
	})
	t.Run("damage", func(t *testing.T) {
		s := buildFile(t, fixtureFile1390(t, "damage-weapons.fwr"))
		t1, last := team1(t, s), len(s.State.Frames)-1
		taken := s.State.Sides[1].DamageTaken
		for i, f := range s.State.Frames {
			if f < 6608 && t1.DamageDealt[i] != taken[i] {
				t.Errorf("frame %d: dealt %v, side 2 taken %v", f, t1.DamageDealt[i], taken[i])
			}
		}
		if t1.DamageDealt[last] != 5152.5083 || taken[last] != 6245.5874 || slices.Max(s.State.Players[0].Damage) != 5174.08 {
			t.Errorf("at the end: dealt %v, taken %v, player %v", t1.DamageDealt[last], taken[last], slices.Max(s.State.Players[0].Damage))
		}
		if !reflect.DeepEqual(s.Result, &Result{Winner: 1, Frame: 6608, Source: SourceSnapshot}) {
			t.Errorf("result %+v", s.Result)
		}
		var got []DeviceEvent
		for _, b := range s.Builds {
			if b.Team == 1 {
				got = append(got, b)
			}
		}
		want := []DeviceEvent{
			{Frame: 1140, Team: 1, Device: 12, TypeID: 2, Name: "sniper", Weapon: true},
			{Frame: 1644, Team: 1, Device: 13, TypeID: 22, Name: "fireball_cannon", Weapon: true},
			{Frame: 1896, Team: 1, Device: 14, TypeID: 4, Name: "watercannon", Weapon: true},
			{Frame: 2148, Team: 1, Device: 15, TypeID: 0, Name: "machinegun", Weapon: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("team 1 builds %+v", got)
		}
	})
	t.Run("device types", func(t *testing.T) {
		for name, want := range map[string]map[uint16]string{
			"fire-mg-mortar.fwr": {0: "machinegun", 4: "watercannon"},
			"devices.fwr":        {35: "turbine", 38: "battery"},
		} {
			s := buildFile(t, fixtureFile1390(t, name))
			for id, n := range want {
				if s.DeviceTypes[id] != n {
					t.Errorf("%s: device types %v", name, s.DeviceTypes)
				}
			}
		}
	})
}

// TestBuildsCorpus pins team 1's finished devices in the economy and damage
// probes.
func TestBuildsCorpus(t *testing.T) {
	for name, want := range map[string][]DeviceEvent{
		"damage-weapons.fwr": {
			{Frame: 510, Team: 1, Device: 11, TypeID: 2, Name: "sniper", Weapon: true},
			{Frame: 1014, Team: 1, Device: 13, TypeID: 22, Name: "cannon", Weapon: true},
			{Frame: 1266, Team: 1, Device: 14, TypeID: 4, Name: "mortar", Weapon: true},
			{Frame: 1644, Team: 1, Device: 15, TypeID: 0, Name: "machinegun", Weapon: true},
		},
		"economy-storage.fwr": {
			{Frame: 2148, Team: 1, Device: 9, TypeID: 3, Name: "sniper2", Weapon: true},
			{Frame: 2904, Team: 1, Device: 10, TypeID: 37, Name: "battery"},
			{Frame: 6100, Team: 1, Device: 11, TypeID: 36, Name: "store"},
			{Frame: 9128, Team: 1, Device: 12, TypeID: 34, Name: "turbine"},
		},
	} {
		s := buildFile(t, fixtureFile(t, name))
		var got []DeviceEvent
		for _, b := range s.Builds {
			if b.Team == 1 {
				got = append(got, b)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: team 1 builds %+v", name, got)
		}
	}
}
