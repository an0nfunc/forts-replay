// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/lua"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/state"
)

type dumpDoc struct {
	SchemaVersion int          `json:"schema_version"`
	Kind          string       `json:"kind"`
	Block         int          `json:"block"`
	Offset        int          `json:"offset"`
	Frame         *int64       `json:"frame,omitempty"`
	Type          string       `json:"type"`
	Keyframe      bool         `json:"keyframe,omitempty"`
	Length        int          `json:"length"`
	Target        *uint32      `json:"target,omitempty"`
	Fields        msg.Message  `json:"fields,omitempty"`
	Payload       string       `json:"payload,omitempty"`
	Error         string       `json:"error,omitempty"`
	Sections      []sectionDoc `json:"sections,omitempty"`
	State         any          `json:"state,omitempty"` // a snapshot's stateDoc, or its stateError
}

type sectionDoc struct {
	Offset int        `json:"offset"`
	Name   string     `json:"name,omitempty"`
	Length int        `json:"length"`
	Table  *lua.Table `json:"table,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// stateDoc is what dump shows of a snapshot's decoded state.
type stateDoc struct {
	Teams   []teamDoc `json:"teams"` // team-list slots in use, team 0 included
	Sides   []sideDoc `json:"sides"`
	Slots   []slotDoc `json:"slots"` // occupied header player slots
	Nodes   int       `json:"nodes"`
	Devices int       `json:"devices"`
	Links   int       `json:"links"`
}

type teamDoc struct {
	Team          int32   `json:"team"`
	Metal         float32 `json:"metal"`
	Energy        float32 `json:"energy"`
	MaxMetal      float32 `json:"max_metal"`
	MaxEnergy     float32 `json:"max_energy"`
	FloatedMetal  float32 `json:"floated_metal"`
	FloatedEnergy float32 `json:"floated_energy"`
	DamageDealt   float32 `json:"damage_dealt"`
}

type sideDoc struct {
	Side        int     `json:"side"`
	DamageTaken float32 `json:"damage_taken"`
}

type slotDoc struct {
	Slot   int     `json:"slot"`
	Team   int32   `json:"team"`
	APM    float32 `json:"apm"`
	Damage float32 `json:"damage"`
}

type stateError struct {
	Error string `json:"error"`
}

type endDoc struct {
	SchemaVersion int      `json:"schema_version"`
	Kind          string   `json:"kind"`
	Blocks        int      `json:"blocks"`
	Records       int      `json:"records"`
	Truncated     bool     `json:"truncated"`
	Warnings      []string `json:"warnings"`
}

func typeHex(t byte) string { return fmt.Sprintf("0x%02x", t) }

// parseTypes reads a comma-separated list such as "0x95,225". Empty means all.
func parseTypes(s string) (map[byte]bool, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // a nil set selects every type
	}
	want := map[byte]bool{}
	for f := range strings.SplitSeq(s, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(f), 0, 8)
		if err != nil {
			return nil, fmt.Errorf("bad message type %q", f)
		}
		want[byte(v)] = true
	}
	return want, nil
}

func runDump(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "write JSON lines")
	types := fs.String("type", "", "only these message types, e.g. 0x95,0xe1")
	block := fs.Int("block", -1, "only this block")
	withLua := fs.Bool("lua", false, "text output: print the Lua text of snapshot sections")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: fwr dump [--json] [--type T,...] [--block N] [--lua] FILE")
		return exitUsage
	}
	want, err := parseTypes(*types)
	if err != nil {
		fmt.Fprintf(stderr, "fwr dump: %v\n", err)
		return exitUsage
	}
	w := bufio.NewWriter(stdout)
	enc := json.NewEncoder(w)
	var version uint32
	setVersion := func(h container.Header) error {
		version = h.Version
		return nil
	}
	show := func(it item) error {
		if *block >= 0 && it.Block != *block || want != nil && !want[it.Record.Type] {
			return nil
		}
		if *asJSON {
			return enc.Encode(dumpDocFor(it, version))
		}
		return writeText(w, it, version, *withLua)
	}
	res, err := walk(ctx, fs.Arg(0), handlers{header: setVersion, msg: show})
	if err != nil {
		w.Flush()
		fmt.Fprintf(stderr, "fwr dump: %v\n", err)
		return exitFail
	}
	end := endDoc{SchemaVersion: dumpSchemaVersion, Kind: "end", Blocks: res.Blocks, Records: res.Records, Warnings: []string{}}
	if res.Stopped != nil {
		fmt.Fprintf(stderr, "fwr dump: warning: decoding stopped early: %s\n", printable(res.Stopped.Error()))
		end.Truncated = true
		end.Warnings = append(end.Warnings, res.Stopped.Error())
	}
	if *asJSON {
		if err := enc.Encode(end); err != nil {
			fmt.Fprintf(stderr, "fwr dump: %v\n", err)
			return exitFail
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "fwr dump: %v\n", err)
		return exitFail
	}
	return exitOK
}

func dumpDocFor(it item, version uint32) dumpDoc {
	d := dumpDoc{
		SchemaVersion: dumpSchemaVersion,
		Block:         it.Block,
		Offset:        it.Record.Offset,
		Type:          typeHex(it.Record.Type),
		Keyframe:      it.Record.Keyframe,
		Length:        len(it.Record.Payload),
	}
	if it.Frame >= 0 {
		f := it.Frame
		d.Frame = &f
	}
	switch m := it.Msg.(type) {
	case *msg.Snapshot:
		d.Kind = "snapshot"
		for _, s := range snapshot.Parse(m.Payload, lua.Limits{}).Sections {
			sd := sectionDoc{Offset: s.Offset, Name: s.Name, Length: len(s.Text), Table: s.Table}
			if s.Err != nil {
				sd.Error = s.Err.Error()
			}
			d.Sections = append(d.Sections, sd)
		}
		if st, err := decodeState(version, m.Payload); err != nil {
			d.State = stateError{err.Error()}
		} else {
			d.State = stateDocFor(st)
		}
		return d
	case *msg.Invalid:
		d.Kind = "invalid"
		d.Error = m.Err.Error()
	case *msg.Tick:
		d.Kind = "tick"
		d.Target = &m.Target
	case *msg.Sync:
		d.Kind = "sync"
	case *msg.Screenshot:
		d.Kind = msg.KindName(m.MsgType())
		d.Fields = m
		return d // the pixels run to megabytes
	case *msg.Raw:
		d.Kind = "raw"
	default:
		d.Kind = msg.KindName(m.MsgType())
		d.Fields = m
	}
	d.Payload = hex.EncodeToString(shownPayload(it.Record.Type, it.Record.Payload))
	return d
}

func writeText(w io.Writer, it item, version uint32, withLua bool) error {
	frame := "-"
	if it.Frame >= 0 {
		frame = strconv.FormatInt(it.Frame, 10)
	}
	head := fmt.Sprintf("blk=%d off=0x%05x frame=%s type=%s len=%d", it.Block, it.Record.Offset, frame, typeHex(it.Record.Type), len(it.Record.Payload))
	if it.Record.Keyframe {
		head += " kf"
	}
	switch m := it.Msg.(type) {
	case *msg.Snapshot:
		snap := snapshot.Parse(m.Payload, lua.Limits{})
		_, err := fmt.Fprintf(w, "%s snapshot sections=%d\n  %s\n", head, len(snap.Sections), stateText(version, m.Payload))
		if err != nil || !withLua {
			return err
		}
		for i, s := range snap.Sections {
			line := fmt.Sprintf("  section %d name=%q off=0x%x len=%d", i, s.Name, s.Offset, len(s.Text))
			if s.Err != nil {
				line += " err=" + printable(s.Err.Error())
			}
			if _, err = fmt.Fprintf(w, "%s\n%s\n", line, printable(string(s.Text))); err != nil {
				return err
			}
		}
		return nil
	case *msg.Tick:
		_, err := fmt.Fprintf(w, "%s tick target=%d\n", head, m.Target)
		return err
	case *msg.Sync:
		_, err := fmt.Fprintf(w, "%s sync\n", head)
		return err
	case *msg.Invalid:
		_, err := fmt.Fprintf(w, "%s invalid: %s %s\n", head, printable(m.Err.Error()), hexPrefix(shownPayload(it.Record.Type, m.Raw.Payload)))
		return err
	case *msg.Raw:
		_, err := fmt.Fprintf(w, "%s raw %s\n", head, hexPrefix(shownPayload(it.Record.Type, it.Record.Payload)))
		return err
	default:
		fields, err := json.Marshal(m)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s %s %s\n", head, msg.KindName(m.MsgType()), printable(string(fields)))
		return err
	}
}

// decodeState decodes the state of a snapshot payload as match does.
func decodeState(version uint32, payload []byte) (*state.State, error) {
	if version != state.Version {
		return nil, fmt.Errorf("not decoded for version %d", version)
	}
	end, ok := snapshot.End(payload)
	if !ok {
		return nil, fmt.Errorf("%w: no Lua section", state.ErrCorrupt)
	}
	return state.Parse(payload, end)
}

func stateDocFor(st *state.State) stateDoc {
	d := stateDoc{Teams: []teamDoc{}, Slots: []slotDoc{}, Nodes: len(st.Nodes), Devices: len(st.Devices), Links: len(st.Links)}
	for _, t := range st.Teams {
		d.Teams = append(d.Teams, teamDoc{t.ID, t.Metal, t.Energy, t.MaxMetal, t.MaxEnergy, t.FloatedMetal, t.FloatedEnergy, t.DamageDealt})
	}
	for k, taken := range st.DamageTaken {
		d.Sides = append(d.Sides, sideDoc{k + 1, taken})
	}
	for k, sl := range st.Slots {
		if sl.Team != 0 {
			d.Slots = append(d.Slots, slotDoc{k, sl.Team, sl.APM, sl.Damage})
		}
	}
	return d
}

// stateText is dump's line for a snapshot's state.
func stateText(version uint32, payload []byte) string {
	st, err := decodeState(version, payload)
	if err != nil {
		return "state error: " + printable(err.Error())
	}
	var b strings.Builder
	b.WriteString("state")
	for _, t := range st.Teams {
		fmt.Fprintf(&b, " team=%d metal=%g/%g floated=%g energy=%g/%g floated=%g dealt=%g",
			t.ID, t.Metal, t.MaxMetal, t.FloatedMetal, t.Energy, t.MaxEnergy, t.FloatedEnergy, t.DamageDealt)
	}
	fmt.Fprintf(&b, " taken=%g,%g", st.DamageTaken[0], st.DamageTaken[1])
	for k, sl := range st.Slots {
		if sl.Team != 0 {
			fmt.Fprintf(&b, " slot=%d team=%d apm=%g damage=%g", k, sl.Team, sl.APM, sl.Damage)
		}
	}
	fmt.Fprintf(&b, " nodes=%d devices=%d links=%d", len(st.Nodes), len(st.Devices), len(st.Links))
	return b.String()
}

// printable escapes, as \xNN, every byte a terminal could act on: invalid
// UTF-8, and anything unprintable other than tab, newline and a carriage
// return that ends a line. Replay text is printed as is otherwise.
func printable(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		ok := unicode.IsPrint(r) && (r != utf8.RuneError || n > 1) ||
			r == '\t' || r == '\n' || r == '\r' && strings.HasPrefix(s[i+n:], "\n")
		if ok {
			b.WriteString(s[i : i+n])
		} else {
			for j := i; j < i+n; j++ {
				fmt.Fprintf(&b, `\x%02x`, s[j])
			}
		}
		i += n
	}
	return b.String()
}

// shownPayload returns payload p of a type t record for printing, with its
// SteamIDs zeroed in a copy: bytes 32 to 47 of a ranked result (0xbb), 75
// to 90 of a 0x8d record, and any 8 bytes whose upper four are 01 00 10 01,
// the high half of every SteamID64. A cut record is zeroed as far as it
// reaches, since it fails to decode and prints in full as Invalid.
func shownPayload(t byte, p []byte) []byte {
	var out []byte
	zero := func(from, to int) {
		if from >= len(p) {
			return
		}
		if out == nil {
			out = bytes.Clone(p)
		}
		clear(out[from:min(len(p), to)])
	}
	switch t {
	case msg.TypeResult:
		zero(32, 48)
	case 0x8d: // not decoded; see docs/format.md
		zero(75, 91)
	}
	high := []byte{0x01, 0x00, 0x10, 0x01}
	for i := 4; i < len(p); {
		j := bytes.Index(p[i:], high)
		if j < 0 {
			break
		}
		i += j
		zero(i-4, i+4)
		i++
	}
	if out == nil {
		return p
	}
	return out
}

func hexPrefix(p []byte) string {
	const n = 32
	if len(p) <= n {
		return hex.EncodeToString(p)
	}
	return hex.EncodeToString(p[:n]) + "..."
}
