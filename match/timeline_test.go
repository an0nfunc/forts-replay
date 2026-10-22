// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

func fixtureTimeline(t *testing.T, name string) *Timeline {
	t.Helper()
	hdr, blocks := fixture.Read(t, "..", name)
	tl := NewTimeline(Limits{})
	var d *msg.Decoder
	for _, b := range blocks {
		if d == nil {
			d = msg.NewDecoder(stream.New(b), msg.DefaultRegistry(), hdr.Version, msg.Options{})
		} else {
			d.Reset(stream.New(b))
		}
		for d.Next() {
			tl.Add(d.Frame(), d.Message())
		}
		if err := d.Err(); err != nil {
			t.Fatal(err)
		}
	}
	return tl
}

// TestTimelineFixtures checks counts and build orders against the probe
// notes in testdata/probes.
func TestTimelineFixtures(t *testing.T) {
	cases := []struct {
		name   string
		counts map[byte]int
		build  []int64 // frames of the build order
	}{
		{"abyss-bot-278f.fwr", nil, nil},
		{"abyss-bot-974f.fwr", map[byte]int{msg.TypeDeleteNode: 4, msg.TypeDeleteLink: 10}, nil},
		{"probe-delete.fwr", map[byte]int{msg.TypeStrut: 1, msg.TypeLink: 1, msg.TypePlace: 1, msg.TypeDeleteLink: 1, msg.TypeScrap: 1}, []int64{540}},
		{"probe-node.fwr", map[byte]int{msg.TypeStrut: 2, msg.TypeLink: 2, msg.TypeDeleteNode: 2}, nil},
		{"probe-doors.fwr", map[byte]int{msg.TypeStrut: 1, msg.TypeLink: 1, msg.TypeDoor: 4}, nil},
		{"probe-mine.fwr", map[byte]int{msg.TypeScrap: 1, msg.TypePlace: 1}, []int64{516}},
		{"probe-ground.fwr", map[byte]int{msg.TypeStrut: 2}, nil},
		{"probe-repair.fwr", map[byte]int{msg.TypePlace: 2, msg.TypeFire: 4, msg.TypeRepair: 1}, []int64{500, 780}},
		{"probe-dblclick.fwr", map[byte]int{msg.TypePlace: 4, msg.TypeFireBlocked: 1, msg.TypeFire: 3}, []int64{514, 546, 580, 858}},
		{"probe-downgrade-overdrive.fwr", map[byte]int{msg.TypePlace: 1, msg.TypeUpgrade: 2}, []int64{482, 716, 974}},
		{"probe-move-phantom.fwr", map[byte]int{msg.TypePlace: 2}, []int64{288}},
	}
	for _, c := range cases {
		tl := fixtureTimeline(t, c.name)
		ps := tl.Players()
		if len(tl.Warnings()) != 0 {
			t.Errorf("%s: warnings %v", c.name, tl.Warnings())
		}
		if c.counts == nil {
			if len(ps) != 0 {
				t.Errorf("%s: %d players", c.name, len(ps))
			}
			continue
		}
		if len(ps) != 1 || ps[0].ID != 1 || !maps.Equal(ps[0].Counts, c.counts) {
			t.Errorf("%s: players %v", c.name, ps)
			continue
		}
		total := 0
		for _, n := range c.counts {
			total += n
		}
		var build []int64
		for _, e := range ps[0].BuildOrder() {
			build = append(build, e.Frame)
		}
		if len(ps[0].Commands) != total || !slices.Equal(build, c.build) {
			t.Errorf("%s: %d commands, build order at %v", c.name, len(ps[0].Commands), build)
		}
	}
}

func TestTimelineFolding(t *testing.T) {
	p1 := msg.Input{Player: 1}
	blocked := func(device uint32) *msg.FireBlocked { return &msg.FireBlocked{Input: p1, Device: device, Angle: 0.86} }
	adds := []struct {
		frame int64
		m     msg.Message
	}{
		{10, &msg.Pointer{Input: p1}},
		{10, &msg.Raw{Type: msg.TypeScrap}},
		{10, &msg.Invalid{Raw: msg.Raw{Type: msg.TypeScrap}}},
		{12, &msg.Strut{Input: p1, Serial: 1, First: true}},
		{14, &msg.Strut{Input: p1, Serial: 1}},
		{20, &msg.Link{Input: p1, NodeA: 3, NodeB: 4, First: true}},
		{20, &msg.Link{Input: p1, NodeA: 3, NodeB: 4}},
		{30, blocked(14)},
		{30, blocked(14)}, // the game's repeat: folded
		{32, blocked(14)}, // the next step: kept
		{32, blocked(15)}, // another weapon: kept
		{40, &msg.FireStart{Input: p1, Device: 9}},
		{42, &msg.Fire{Input: p1, Device: 9, Power: 0.5, Immediate: true}},
		{80, &msg.FireEnd{Input: p1, Device: 9}},
		{90, &msg.Door{Input: p1, Source: 3, Device: 9}}, // the game's, for a weapon: skipped
		{92, &msg.Door{Input: p1, Source: 11}},
		{94, &msg.Door{Input: p1, Source: 8}},
	}
	tl := NewTimeline(Limits{})
	for _, a := range adds {
		tl.Add(a.frame, a.m)
	}
	ps := tl.Players()
	if len(ps) != 1 {
		t.Fatalf("%d players", len(ps))
	}
	var frames []int64
	for _, e := range ps[0].Commands {
		frames = append(frames, e.Frame)
	}
	want := map[byte]int{msg.TypeStrut: 1, msg.TypeLink: 1, msg.TypeFireBlocked: 3, msg.TypeFire: 1, msg.TypeDoor: 2}
	if !slices.Equal(frames, []int64{14, 20, 30, 32, 32, 42, 92, 94}) || !maps.Equal(ps[0].Counts, want) {
		t.Errorf("frames %v, counts %v", frames, ps[0].Counts)
	}
}

func TestTimelineLimits(t *testing.T) {
	if tl := NewTimeline(Limits{}); tl.lim.MaxPlayers != 16 || tl.lim.MaxCommands != 1<<16 {
		t.Errorf("defaults %+v", tl.lim)
	}
	tl := NewTimeline(Limits{MaxPlayers: 2, MaxCommands: 3})
	for i, player := range []uint32{1, 2, 3, 1, 2, 3} {
		tl.Add(int64(2*i), &msg.Scrap{Player: player, Device: uint32(i)})
	}
	ps := tl.Players()
	if len(ps) != 2 || ps[0].ID != 1 || ps[1].ID != 2 {
		t.Fatalf("players %v", ps)
	}
	if len(ps[0].Commands) != 2 || len(ps[1].Commands) != 1 || ps[0].Counts[msg.TypeScrap] != 2 || ps[1].Counts[msg.TypeScrap] != 2 {
		t.Errorf("player 1 %v, player 2 %v", ps[0], ps[1])
	}
	w := strings.Join(tl.Warnings(), "; ")
	if !strings.Contains(w, "timeline cut at 3 commands, 1 more counted only") || !strings.Contains(w, "2 commands from players beyond the first 2 ignored") {
		t.Errorf("warnings %q", w)
	}

	none := NewTimeline(Limits{MaxCommands: -1})
	for i := range 3 {
		none.Add(int64(2*i), &msg.Scrap{Player: 1, Device: uint32(i)})
	}
	if ps := none.Players(); len(ps) != 1 || len(ps[0].Commands) != 0 || ps[0].Counts[msg.TypeScrap] != 3 || len(none.Warnings()) != 0 {
		t.Errorf("keeping none: players %v, warnings %v", ps, none.Warnings())
	}
}
