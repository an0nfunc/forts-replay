// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
)

// TestCommandFixtures pins each command's fields against a probe record;
// the probe notes say what the player did there.
func TestCommandFixtures(t *testing.T) {
	p1 := Input{Player: 1}
	cases := []struct {
		file  string
		frame int64
		want  Message
	}{
		{"probe-delete.fwr", 354, &Strut{Input: p1, Serial: 1, X: -1617.851318359375, Y: 110.464111328125, DX: 129.1455078125, DY: 0.7360916137695312, Source: 303, First: true}},
		{"probe-delete.fwr", 456, &Link{Input: p1, NodeA: 317, NodeB: 305}},
		{"probe-delete.fwr", 540, &Place{Input: p1, T: 0.498737633228302, TypeID: 37, NodeA: 288, NodeB: 290, X: -1951.289794921875, Y: 307.03009033203125, MovedDevice: -1}},
		{"probe-delete.fwr", 816, &DeleteLink{Input: p1, NodeA: 317, NodeB: 305}},
		{"probe-delete.fwr", 1016, &Scrap{Input: p1, Device: 9}},
		{"probe-node.fwr", 606, &DeleteNode{Input: p1, Node: 317}},
		{"probe-doors.fwr", 456, &Link{Input: p1, NodeA: 317, NodeB: 305, Material: 4, First: true}},
		{"probe-doors.fwr", 794, &Door{Input: p1, State: 3, NodeA: 303, NodeB: 317}},
		{"probe-mine.fwr", 516, &Place{Input: p1, T: 1.5707963705062866, TypeID: 31, X: -2172.45751953125, Y: 97.26091003417969, MovedDevice: -1}},
		{"probe-ground.fwr", 440, &Strut{Input: p1, Serial: 2, X: -2217.236572265625, Y: 650, DX: -31.8388671875, DY: 26.682373046875, Source: 317, First: true, Anchored: true}},
		{"probe-repair.fwr", 780, &Place{Input: p1, T: 0.3852655291557312, TypeID: 4, NodeA: 302, NodeB: 301, X: -1858.259521484375, Y: 7.30438232421875, Aim: 1.1344640254974365, MovedDevice: -1}},
		{"probe-repair.fwr", 1068, &Fire{Input: p1, Device: 9, Angle: 6.03750467300415, Power: 0.49215561151504517}},
		{"probe-repair.fwr", 2114, &Repair{Input: p1, Node: 287}},
		{"probe-dblclick.fwr", 1128, &FireStart{Input: p1, Device: 10}},
		{"probe-dblclick.fwr", 1130, &FireBlocked{Input: p1, Device: 10, Angle: 6.282497406005859}},
		{"probe-dblclick.fwr", 1168, &FireEnd{Input: p1, Device: 10}},
		{"probe-dblclick.fwr", 1350, &Fire{Input: p1, Device: 11, Angle: 0.0025518368929624557, Power: 0.5, Immediate: true}},
		{"probe-downgrade-overdrive.fwr", 716, &Upgrade{Input: p1, Device: 10}},
		{"probe-move-phantom.fwr", 718, &Place{Input: p1, T: 0.4678429663181305, TypeID: 38, NodeA: 287, NodeB: 291, X: -1951.2061767578125, Y: 207.62594604492188, MovedDevice: 10}},
		{"portal-link.fwr", 636, &PortalLink{Input: p1, NodeA: 303, NodeB: 317, OtherA: 304, OtherB: 318}},
		{"portal-link.fwr", 800, &PortalLink{Input: p1, NodeA: 303, NodeB: 317}},
		{"portal-link.fwr", 1148, &PortalReverse{Input: p1, NodeA: 303, NodeB: 317}},
		{"ammo-select.fwr", 888, &Ammo{Input: p1, Device: 9}},
		{"ammo-select.fwr", 1238, &Ammo{Input: p1, Ammo: 1, Device: 9}},
		{"swarm-missiles.fwr", 1450, &FireAt{Input: p1, X: 1903.1417236328125, Y: -147.45150756835938, Angle: -1.5707963705062866, Device: 1394, Spotter: 1396}},
		{"area-repair-damaged.fwr", 13368, &RepairArea{Input: p1, Team: 1, X: -1835.512451171875, Y: 39.999420166015625, Radius: 200}},
		{"device-repair.fwr", 14660, &RepairDevice{Input: p1, Team: 1, Device: 7}},
		{"strut-repair.fwr", 14020, &RepairLink{Input: p1, Team: 1, NodeA: 298, NodeB: 302}},
		{"swarm-immediate-fire.fwr", 2856, &FireAt{Input: p1, X: 1903.1943359375, Y: -147.40890502929688, Angle: -1.5707963705062866, Device: 1394, Spotter: 1395}},
	}
	msgs := map[string][]framed{}
	for _, c := range cases {
		if msgs[c.file] == nil {
			msgs[c.file] = fixtureMessages(t, c.file)
		}
		got := firstAt(msgs[c.file], c.frame, c.want.MsgType())
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s frame %d: %#v, want %#v", c.file, c.frame, got, c.want)
		}
		if _, ok := got.(Command); !ok {
			t.Errorf("%s frame %d: %T is not a Command", c.file, c.frame, got)
		}
	}
}

// TestAbility pins a recorded ability record: a game long enough to charge
// the ability is over the fixture size limit.
func TestAbility(t *testing.T) {
	p, err := hex.DecodeString("da00000000000000000000000000000000da0000010000000100000001")
	if err != nil {
		t.Fatal(err)
	}
	m, err := DefaultRegistry().Lookup(TypeAbility, 1383)(p)
	if err != nil || !reflect.DeepEqual(m, &Ability{Player: 1}) {
		t.Errorf("%#v, %v", m, err)
	}
}

func TestCommandFlags(t *testing.T) {
	setB := func(p []byte, off int, v byte) []byte { p[off] = v; return p }
	setF := func(p []byte, off int) []byte {
		binary.LittleEndian.PutUint32(p[off:], math.Float32bits(float32(math.Inf(-1))))
		return p
	}
	cases := map[string][]byte{
		"strut first 2":      setB(inputRec(TypeStrut, 74, TypeStrut), 64, 2),
		"strut anchored 2":   setB(inputRec(TypeStrut, 74, TypeStrut), 66, 2),
		"link first 2":       setB(inputRec(TypeLink, 50, TypeLink), 48, 2),
		"fire immediate 2":   setB(inputRec(TypeFire, 51, TypeFire), 49, 2),
		"place x":            setF(inputRec(TypePlace, 67, TypePlace), 44),
		"place aim":          setF(inputRec(TypePlace, 67, TypePlace), 53),
		"strut dx":           setF(inputRec(TypeStrut, 74, TypeStrut), 40),
		"fire angle":         setF(inputRec(TypeFire, 51, TypeFire), 33),
		"fire blocked angle": setF(inputRec(TypeFireBlocked, 33, TypeFireBlocked), 29),
		"repair area x":      setF(inputRec(TypeRepairArea, 44, TypeRepairArea), 28),
		"fire at angle":      setF(inputRec(TypeFireAt, 52, TypeFireAt), 32),
	}
	r := DefaultRegistry()
	for name, p := range cases {
		if m, err := r.Lookup(p[0], 1383)(p); err == nil {
			t.Errorf("%s: decodes to %#v", name, m)
		}
	}
}

// TestFixturesTyped decodes every sample replay: no record is Invalid, and
// only 0x93 stays Raw.
func TestFixturesTyped(t *testing.T) {
	for _, name := range slices.Concat(fixture.Names, fixture.Probes) {
		for _, f := range fixtureMessages(t, name) {
			switch m := f.m.(type) {
			case *Invalid:
				t.Errorf("%s frame %d: %v", name, f.frame, m.Err)
			case *Raw:
				if m.Type != 0x93 {
					t.Errorf("%s frame %d: raw %#02x", name, f.frame, m.Type)
				}
			}
		}
	}
}
