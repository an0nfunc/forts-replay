// SPDX-License-Identifier: AGPL-3.0-or-later

package msg_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/fixture"
	"github.com/an0nfunc/forts-replay/lua"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/stream"
)

// TestTimeline pins the frame timeline of the samples: a tick then its sync
// every two frames, syncs running to the header's frame count, and each
// snapshot carrying the frame of the sync before it.
func TestTimeline(t *testing.T) {
	for _, name := range fixture.Names {
		hdr, blocks := fixture.Read(t, "..", name)
		var dec *msg.Decoder
		var prev msg.Message
		var lastSync int64
		ticks := 0
		for i, b := range blocks {
			if dec == nil {
				dec = msg.NewDecoder(stream.New(b), msg.DefaultRegistry(), hdr.Version, msg.Options{})
			} else {
				dec.Reset(stream.New(b))
			}
			for dec.Next() {
				switch m := dec.Message().(type) {
				case *msg.Invalid:
					t.Fatalf("%s block %d: %v", name, i, m.Err)
				case *msg.Tick:
					ticks++
					if int64(m.Target) != lastSync+4 {
						t.Fatalf("%s: tick to %d after sync %d", name, m.Target, lastSync)
					}
				case *msg.Sync:
					tk, ok := prev.(*msg.Tick)
					if !ok || int64(m.Frame) != lastSync+2 || tk.Target != m.Frame+2 {
						t.Fatalf("%s: sync %d after %T and sync %d", name, m.Frame, prev, lastSync)
					}
					lastSync = int64(m.Frame)
				case *msg.Snapshot:
					frame, gameTime := snapshotTime(t, m)
					want := dec.Frame()
					if i == 0 {
						want = 1
					}
					if frame != want || math.Abs(gameTime-float64(frame)*0.04) > 1e-5 {
						t.Errorf("%s block %d: gameFrame %d gameTime %v, decoder frame %d", name, i, frame, gameTime, dec.Frame())
					}
				}
				prev = dec.Message()
			}
			if err := dec.Err(); err != nil {
				t.Fatal(err)
			}
		}
		if lastSync != int64(hdr.Frames) || ticks != int(hdr.Frames)/2 {
			t.Errorf("%s: last sync %d, %d ticks, header frames %d", name, lastSync, ticks, hdr.Frames)
		}
	}
}

// snapshotTime returns data.gameFrame and data.gameTime of a snapshot's
// first section.
func snapshotTime(t *testing.T, m *msg.Snapshot) (int64, float64) {
	t.Helper()
	secs := snapshot.Parse(m.Payload, lua.Limits{}).Sections
	if len(secs) == 0 || secs[0].Err != nil {
		t.Fatalf("snapshot sections %+v", secs)
	}
	data, _ := secs[0].Table.Get("data")
	gf, _ := data.Table.Get("gameFrame")
	gt, _ := data.Table.Get("gameTime")
	frame, err := strconv.ParseInt(gf.Str, 10, 64)
	gameTime, ok := gt.Float()
	if err != nil || !ok {
		t.Fatalf("gameFrame %q gameTime %q", gf.Str, gt.Str)
	}
	return frame, gameTime
}
