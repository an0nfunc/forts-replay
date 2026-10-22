// SPDX-License-Identifier: AGPL-3.0-or-later

package state

import (
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/an0nfunc/forts-replay/snapshot"
)

func FuzzParse(f *testing.F) {
	seeds := [][]byte{synthetic().Payload()}
	gap := synthetic()
	gap.Gap, gap.Devices = []byte{1, 0}, gap.Devices[:1]
	seeds = append(seeds, gap.Payload())
	for _, c := range []struct {
		name  string
		frame int64
	}{{"economy-storage.fwr", 6100}, {"damage-weapons.fwr", 7364}} {
		seeds = append(seeds, snapshots(f, c.name)[c.frame])
	}
	for _, p := range seeds {
		end, _ := snapshot.End(p)
		f.Add(p, end)
	}
	f.Fuzz(func(t *testing.T, payload []byte, start int) {
		s, err := Parse(payload, start)
		if err != nil {
			if !errors.Is(err, ErrCorrupt) || s != nil {
				t.Fatalf("err %v, state %v", err, s != nil)
			}
			return
		}
		if len(s.Teams) > teamSlots || len(s.Nodes) == 0 || len(s.Nodes) > maxNodes || len(s.Devices) > maxDevices || len(s.Links) > maxLinks {
			t.Fatalf("%d teams, %d nodes, %d devices, %d struts", len(s.Teams), len(s.Nodes), len(s.Devices), len(s.Links))
		}
		for _, tm := range s.Teams {
			if !finite(tm.Metal, tm.Energy, tm.MaxMetal, tm.MaxEnergy, tm.FloatedMetal, tm.FloatedEnergy, tm.DamageDealt) {
				t.Fatalf("team %+v", tm)
			}
		}
		for _, sl := range s.Slots {
			if !finite(sl.APM, sl.Damage) {
				t.Fatalf("slot %+v", sl)
			}
		}
		if !finite(s.DamageTaken[:]...) {
			t.Fatalf("taken %v", s.DamageTaken)
		}
		bad := func(r rune) bool { return r > unicode.MaxASCII || !unicode.IsPrint(r) }
		for _, d := range s.Devices {
			if d.Name == "" || len(d.Name) > maxName || strings.ContainsFunc(d.Name, bad) || !finite(d.Health, d.Progress) {
				t.Fatalf("device %+v", d)
			}
		}
		ids := make(map[uint32]bool, len(s.Nodes))
		for _, n := range s.Nodes {
			ids[n.ID] = true
		}
		for _, l := range s.Links {
			if !ids[l.NodeA] || !ids[l.NodeB] || !finite(l.HP) {
				t.Fatalf("strut %+v", l)
			}
		}
	})
}
