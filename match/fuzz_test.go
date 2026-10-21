// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

func FuzzTimeline(f *testing.F) {
	for _, name := range fixture.Probes {
		for _, b := range fixture.Blocks(f, "..", name) {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		tl := NewTimeline(Limits{MaxPlayers: 2, MaxCommands: 8})
		d := msg.NewDecoder(stream.New(data), msg.DefaultRegistry(), 1383, msg.Options{})
		for d.Next() {
			tl.Add(d.Frame(), d.Message())
		}
		ps := tl.Players()
		kept := 0
		for _, p := range ps {
			kept += len(p.Commands)
			counted := 0
			for _, n := range p.Counts {
				counted += n
			}
			if counted < len(p.Commands) {
				t.Fatalf("player %d: %d counted, %d kept", p.ID, counted, len(p.Commands))
			}
			for _, e := range p.Commands {
				if e.Command.Sender() != p.ID {
					t.Fatalf("player %d holds a command from %d", p.ID, e.Command.Sender())
				}
			}
		}
		if len(ps) > 2 || kept > 8 {
			t.Fatalf("%d players, %d commands kept", len(ps), kept)
		}
	})
}

func FuzzBuild(f *testing.F) {
	for _, name := range fixture.Names {
		f.Add(fixtureFile(f, name))
	}
	f.Add(game(duel(true), fwrtest.Result(0, 1, 101, testSteamID), won(1)))
	f.Fuzz(func(t *testing.T, data []byte) {
		opts := Options{
			Container: container.Limits{MaxBlock: 1 << 20, MaxTotal: 4 << 20},
			Timeline:  Limits{MaxPlayers: 4, MaxCommands: 64},
		}
		s, err := Build(t.Context(), bytes.NewReader(data), opts)
		if err != nil {
			if s != nil {
				t.Fatalf("summary with error %v", err)
			}
			return
		}
		if s.Blocks < 1 || s.Warnings == nil || len(s.Timelines) > 4 || len(s.Players) > 16 || len(s.AI) > 16 || len(s.Commanders) > 2 {
			t.Fatalf("%d blocks, warnings %v, %d timelines, %d players, %d AI, %d commanders",
				s.Blocks, s.Warnings, len(s.Timelines), len(s.Players), len(s.AI), len(s.Commanders))
		}
		if r := s.Result; r != nil && r.Winner != 1 && r.Winner != 2 {
			t.Fatalf("winner %d", r.Winner)
		}
		if _, err := json.Marshal(s); err != nil {
			t.Fatal(err)
		}
	})
}
