// SPDX-License-Identifier: AGPL-3.0-or-later

// Package match builds the summary of a replay: header fields, players,
// the result, per-player command timelines with action counts and build
// orders, and the economy, damage and fort state the snapshots record.
// Commands are what players asked for; the game may have refused some.
package match

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/header"
	"github.com/an0nfunc/forts-replay/lua"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/stream"
)

const (
	// SchemaVersion is the schema_version of a Summary's JSON form, whose
	// JSON Schema is schema/info-v1.json. Removing a field or changing its
	// meaning bumps it; adding one does not.
	SchemaVersion = 1
	// FramesPerSecond is the simulation rate.
	FramesPerSecond = 25

	// SourceRanked marks a result from the game's ranked result record,
	// exact to the frame.
	SourceRanked = "ranked"
	// SourceSnapshot marks a result from the first snapshot after the end,
	// up to 5 seconds late.
	SourceSnapshot = "snapshot"
)

// Summary is what a replay records about its match. Its JSON form is
// schema v1.
type Summary struct {
	SchemaVersion int               `json:"schema_version"`
	FormatVersion uint32            `json:"format_version"` // header version: 1383 in game v1.38.2, 1390 in v1.39.0
	Frames        uint32            `json:"frames"`
	Duration      float64           `json:"duration"` // seconds
	Ranked        bool              `json:"ranked"`
	Map           string            `json:"map,omitempty"` // .fwe file
	Options       []string          `json:"options,omitempty"`
	Players       []Player          `json:"players,omitempty"`    // header order, spectators included
	AI            []AI              `json:"ai,omitempty"`         // forts the AI plays
	Commanders    []Commander       `json:"commanders,omitempty"` // the sides with a commander
	Result        *Result           `json:"result,omitempty"`
	Timelines     []*PlayerTimeline `json:"timelines,omitempty"` // by player ID
	State         *StateSeries      `json:"state,omitempty"`     // header versions 1383 and 1390 only
	Losses        []DeviceEvent     `json:"losses,omitempty"`
	Builds        []DeviceEvent     `json:"builds,omitempty"`
	DeviceTypes   map[uint16]string `json:"device_types,omitempty"` // type ID to save-name, from the snapshots
	Blocks        int               `json:"blocks"`
	Records       int               `json:"records"`
	Truncated     bool              `json:"truncated"` // decoding stopped early; the rest is what decoded before
	Warnings      []string          `json:"warnings"`
}

// Player is one entry of the header's player table.
type Player struct {
	ID       uint32 `json:"id"` // the player field of the player's records
	Slot     uint32 `json:"slot"`
	Team     int32  `json:"team"`
	Side     int    `json:"side"`                      // 1 or 2; 0 for a spectator
	Name     Name   `json:"name,omitempty"`            // absent for a ranked opponent
	SteamID  uint64 `json:"steam_id,string,omitempty"` // from the player's header entry; absent when it holds none
	Recorder bool   `json:"recorder,omitempty"`        // the client that recorded the replay
}

// Name is a player-chosen name. Its JSON form escapes every rune that
// unicode.IsPrint rejects, so control, format and bidi characters cannot
// act on a terminal or log showing the raw JSON. Decoded, it is unchanged.
type Name string

// MarshalJSON writes n as encoding/json does, with unprintable runes as
// \uXXXX escapes.
func (n Name) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(string(n)) // valid UTF-8: json.Marshal replaces invalid bytes
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(b))
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		switch {
		case r != 0x7f && (r < utf8.RuneSelf || unicode.IsPrint(r)): // json.Marshal escapes every ASCII control but DEL
			out = append(out, b[:size]...)
		case r > 0xffff:
			r1, r2 := utf16.EncodeRune(r)
			out = fmt.Appendf(out, `\u%04x\u%04x`, r1, r2)
		default:
			out = fmt.Appendf(out, `\u%04x`, r)
		}
		b = b[size:]
	}
	return out, nil
}

// AI is a fort the AI plays.
type AI struct {
	Team int32 `json:"team"`
	Side int   `json:"side"`
}

// Commander is a side's commander, by the game's save-name, such as
// "commander-fb-fulminator".
type Commander struct {
	Side int  `json:"side"`
	Name Name `json:"name"`
}

// Result is the winner and where the replay records it.
type Result struct {
	Winner uint32 `json:"winner"` // side 1 or 2
	Frame  int64  `json:"frame"`  // frame of the record that names the winner
	Source string `json:"source"` // SourceRanked or SourceSnapshot
}

// Options configure Build. Zero values take each package's defaults.
type Options struct {
	Container container.Limits
	Lua       lua.Limits // for the snapshot that names the winner
	Timeline  Limits
	// Observe, when set, receives every decoded message in order with its
	// record and the frame of its step.
	Observe func(rec stream.Record, frame int64, m msg.Message)
}

// Build decodes the replay r and summarises it, checking ctx between
// blocks. It fails when nothing can be decoded. Damage after that stops
// decoding: the summary then holds what decoded before it, with Truncated
// set and a warning.
func Build(ctx context.Context, r io.Reader, opts Options) (*Summary, error) {
	cr, err := container.NewReader(bufio.NewReader(r), opts.Container)
	if err != nil {
		return nil, err
	}
	reg := msg.DefaultRegistry()
	b := newBuilder(cr.Header(), reg, opts)
	var dec *msg.Decoder
	for cr.NextBlock() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		blk := cr.Block()
		b.sum.Blocks++
		s := stream.New(blk.Data)
		if dec == nil {
			dec = msg.NewDecoder(s, reg, b.sum.FormatVersion, msg.Options{})
		} else {
			dec.Reset(s)
		}
		for dec.Next() {
			b.add(dec.Record(), dec.Frame(), dec.Message())
		}
		if err := dec.Err(); err != nil {
			return b.finish(fmt.Errorf("block %d: %w", blk.Index, err)), nil
		}
	}
	if err := cr.Err(); err != nil {
		if b.sum.Blocks == 0 {
			return nil, err
		}
		return b.finish(err), nil
	}
	return b.finish(nil), nil
}

// gameWinnerKey is the key the first snapshot after the end adds to the
// data table.
const gameWinnerKey = "gameWinner"

type builder struct {
	opts    Options
	sum     *Summary
	hdr     *header.Header // nil unless every header field decoded
	hasSync bool           // the version has a sync decoder
	tl      *Timeline
	st      *stateBuilder

	syncs    int
	lastSync uint32

	ranked    *msg.Result // the first result record from player 0
	rankedAt  int64
	differs   bool // a later one from player 0 differs from it
	differsAt int64

	snapSeen bool // a snapshot holding "gameWinner" was parsed
	snapAt   int64
	snapWin  lua.Value
	snapOK   bool // the parsed snapshot has the key
}

func newBuilder(hdr container.Header, reg *msg.Registry, opts Options) *builder {
	b := &builder{
		opts: opts,
		sum: &Summary{
			SchemaVersion: SchemaVersion, FormatVersion: hdr.Version, Frames: hdr.Frames,
			Duration: float64(hdr.Frames) / FramesPerSecond, Warnings: []string{},
		},
		hasSync: decodes(reg, msg.TypeSync, hdr.Version),
		tl:      NewTimeline(opts.Timeline),
	}
	h, err := header.Parse(hdr.Version, hdr.Raw)
	switch {
	case errors.Is(err, header.ErrVersion):
		b.warnf("header fields not decoded for version %d", hdr.Version)
	case err != nil:
		b.warnf("%v", err)
	default:
		b.hdr = h
	}
	b.st = newStateBuilder(hdr.Version, b.hdr, opts.Timeline, b.warnf)
	if h != nil {
		b.sum.Ranked, b.sum.Map, b.sum.Options = h.Ranked, h.Map, h.Options
		if h.Ranked && !decodes(reg, msg.TypeResult, hdr.Version) {
			b.warnf("ranked result not decoded for version %d", hdr.Version)
		}
		if h.Commanders == nil {
			b.warnf("header: commander names do not decode")
		}
		for i, name := range h.Commanders {
			if name != "" {
				b.sum.Commanders = append(b.sum.Commanders, Commander{Side: i + 1, Name: Name(name)})
			}
		}
	}
	return b
}

// decodes reports whether reg decodes messages of type t in version v.
func decodes(reg *msg.Registry, t byte, v uint32) bool {
	rt, ok := msg.RecordType(t, v)
	return ok && reg.Lookup(rt, v) != nil
}

func (b *builder) add(rec stream.Record, frame int64, m msg.Message) {
	b.sum.Records++
	if b.opts.Observe != nil {
		b.opts.Observe(rec, frame, m)
	}
	b.tl.Add(frame, m)
	switch m := m.(type) {
	case *msg.Sync:
		b.syncs++
		b.lastSync = m.Frame
	case *msg.Result:
		switch {
		case m.Player != 0:
		case b.ranked == nil:
			b.ranked, b.rankedAt = m, frame
		case !b.differs && *m != *b.ranked:
			b.differs, b.differsAt = true, frame
		}
	case *msg.Place, *msg.Upgrade, *msg.Scrap, *msg.DeleteLink, *msg.DeleteNode:
		b.st.command(m)
	case *msg.Snapshot:
		b.st.snapshot(frame, m.Payload)
		if !b.snapSeen && bytes.Contains(m.Payload, []byte(gameWinnerKey)) {
			b.snapSeen, b.snapAt = true, frame
			b.snapWin, b.snapOK = snapshotWinner(m.Payload, b.opts.Lua)
		}
	}
}

// snapshotWinner returns data.gameWinner from the first section that has
// it.
func snapshotWinner(payload []byte, lim lua.Limits) (lua.Value, bool) {
	for _, sec := range snapshot.Parse(payload, lim).Sections {
		if sec.Table == nil {
			continue
		}
		if data, ok := sec.Table.Get("data"); ok && data.Kind == lua.KindTable {
			if v, ok := data.Table.Get(gameWinnerKey); ok {
				return v, true
			}
		}
	}
	return lua.Value{}, false
}

// finish completes the summary; stopped is the damage that ended decoding
// early, or nil.
func (b *builder) finish(stopped error) *Summary {
	s := b.sum
	frames := s.Frames
	switch {
	case stopped != nil:
		s.Truncated = true
		b.warnf("decoding stopped early: %v", stopped)
	case b.syncs == 0 && b.hasSync:
		b.warnf("header has %d frames, no sync decoded", frames)
	case b.syncs > 0 && b.lastSync != frames&^1: // syncs are at even frames
		b.warnf("header has %d frames, last sync is frame %d", frames, b.lastSync)
	}
	s.Timelines = b.tl.Players()
	s.Warnings = append(s.Warnings, b.tl.Warnings()...)
	b.result()
	b.players()
	b.st.finish(s)
	return s
}

func (b *builder) result() {
	var ranked, snap *Result
	if r := b.ranked; r != nil {
		if r.Winner == 1 || r.Winner == 2 {
			ranked = &Result{Winner: r.Winner, Frame: b.rankedAt, Source: SourceRanked}
		} else {
			b.warnf("ranked result at frame %d names side %d", b.rankedAt, r.Winner)
		}
		if b.differs {
			b.warnf("ranked result at frame %d differs from the one at frame %d", b.differsAt, b.rankedAt)
		}
	}
	switch f, ok := b.snapWin.Float(); {
	case !b.snapSeen:
	case !b.snapOK:
		b.warnf("snapshot at frame %d mentions gameWinner, but no data table holds it", b.snapAt)
	case ok && (f == 1 || f == 2):
		snap = &Result{Winner: uint32(f), Frame: b.snapAt, Source: SourceSnapshot}
	default:
		b.warnf("snapshot at frame %d names no side 1 or 2", b.snapAt)
	}
	switch {
	case ranked != nil:
		b.sum.Result = ranked
		if snap != nil && snap.Winner != ranked.Winner {
			b.warnf("ranked result names side %d, snapshot side %d", ranked.Winner, snap.Winner)
		}
	case snap != nil:
		b.sum.Result = snap
	}
}

func (b *builder) players() {
	h := b.hdr
	if h == nil {
		return
	}
	r := b.ranked
	if r != nil && !h.Ranked {
		b.warnf("ranked result record in a replay that is not ranked")
	}
	recorders := 0
	for _, hp := range h.Players {
		p := Player{ID: hp.ID, Slot: hp.Slot, Team: hp.Team, Side: side(hp.Team), Name: Name(hp.Name), SteamID: hp.SteamID}
		if hp.ID == h.Recorder {
			recorders++
			p.Recorder = recorders == 1 // a later entry with the ID is treated as an opponent
		}
		if h.Ranked && !p.Recorder {
			p.Name = "" // the header holds a generic word, not the opponent's name
		}
		if p.Recorder && r != nil && h.Ranked {
			switch {
			case r.Team != h.RecorderTeam || hp.Team != h.RecorderTeam:
				b.warnf("ranked result team %d, recorder team %d, recorder entry team %d", r.Team, h.RecorderTeam, hp.Team)
			case r.SteamID != hp.SteamID: // the entry's SteamID stays
				b.warnf("ranked result and recorder entry hold different SteamIDs")
			}
		}
		b.sum.Players = append(b.sum.Players, p)
	}
	switch recorders {
	case 0:
		b.warnf("recorder %d is not in the player table", h.Recorder)
	case 1:
	default:
		b.warnf("recorder %d has %d entries in the player table", h.Recorder, recorders)
	}
	for _, a := range h.AI {
		if a.Value != 0 {
			b.sum.AI = append(b.sum.AI, AI{Team: a.Team, Side: side(a.Team)})
		}
	}
}

func (b *builder) warnf(format string, args ...any) {
	b.sum.Warnings = append(b.sum.Warnings, fmt.Sprintf(format, args...))
}

// side returns the side of a team ID: the ID modulo 100, and 0 for a
// spectator.
func side(team int32) int {
	if team <= 0 {
		return 0
	}
	return int(team % 100)
}
