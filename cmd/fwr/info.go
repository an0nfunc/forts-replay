// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/an0nfunc/forts-replay/match"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

// infoCounts are the info rows the summary does not carry.
type infoCounts struct {
	types    [256]int
	syncs    int
	lastSync uint32
}

func runInfo(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "write the summary as one JSON document (schema/info-v1.json)")
	maxCommands := fs.Int("max-commands", 1<<16, "keep at most N timeline commands over all players (0 keeps none); counts stay complete")
	maxStates := fs.Int("max-states", 8192, "keep at most N state series points (0 keeps none)")
	maxLosses := fs.Int("max-losses", 1<<16, "keep at most N lost devices (0 keeps none)")
	maxBuilds := fs.Int("max-builds", 1<<16, "keep at most N finished builds (0 keeps none)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 || *maxCommands < 0 || *maxStates < 0 || *maxLosses < 0 || *maxBuilds < 0 {
		fmt.Fprintln(stderr, "usage: fwr info [--json] [--max-commands N] [--max-states N] [--max-losses N] [--max-builds N] FILE")
		return exitUsage
	}
	keep := func(n int) int {
		if n == 0 {
			return -1 // 0 in Limits takes the default
		}
		return n
	}
	limits := match.Limits{MaxCommands: keep(*maxCommands), MaxStates: keep(*maxStates), MaxLosses: keep(*maxLosses), MaxBuilds: keep(*maxBuilds)}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "fwr info: %v\n", err)
		return exitFail
	}
	defer f.Close()
	var c infoCounts
	sum, err := match.Build(ctx, f, match.Options{Timeline: limits, Observe: func(rec stream.Record, _ int64, m msg.Message) {
		c.types[rec.Type]++
		if s, ok := m.(*msg.Sync); ok {
			c.syncs++
			c.lastSync = s.Frame
		}
	}})
	if err != nil {
		fmt.Fprintf(stderr, "fwr info: %s\n", printable(err.Error()))
		return exitFail
	}
	for _, warn := range sum.Warnings {
		fmt.Fprintf(stderr, "fwr info: warning: %s\n", printable(warn))
	}
	w := bufio.NewWriter(stdout)
	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		err = enc.Encode(struct {
			*match.Summary

			ParserVersion string `json:"parser_version"`
		}{sum, parserVersion()})
	} else {
		writeSummary(w, sum, &c)
	}
	if err == nil {
		err = w.Flush()
	}
	if err != nil {
		fmt.Fprintf(stderr, "fwr info: %v\n", err)
		return exitFail
	}
	return exitOK
}

func writeSummary(w io.Writer, s *match.Summary, c *infoCounts) {
	row := func(name, format string, args ...any) {
		fmt.Fprintf(w, "%-10s %s\n", name, fmt.Sprintf(format, args...))
	}
	row("version", "%d", s.FormatVersion)
	row("frames", "%d (%.2f s)", s.Frames, s.Duration)
	ranked := "no"
	if s.Ranked {
		ranked = "yes"
	}
	row("ranked", "%s", ranked)
	if len(s.Options) > 0 {
		opts := make([]string, len(s.Options))
		for i, o := range s.Options {
			opts[i] = strconv.Quote(o)
		}
		row("options", "%s", strings.Join(opts, " "))
	}
	if s.Map != "" {
		row("map", "%q", s.Map)
	}
	for _, p := range s.Players {
		text := fmt.Sprintf("%d: slot %d, team %d, side %d", p.ID, p.Slot, p.Team, p.Side)
		if p.Name != "" {
			text += ", " + strconv.Quote(string(p.Name))
		}
		if p.Recorder {
			text += ", recorder"
		}
		row("player", "%s", text) // the SteamID stays in --json
	}
	for _, a := range s.AI {
		row("ai", "team %d, side %d", a.Team, a.Side)
	}
	for _, c := range s.Commanders {
		row("commander", "side %d, %q", c.Side, c.Name)
	}
	if r := s.Result; r != nil {
		row("result", "side %d won at frame %d (%s)", r.Winner, r.Frame, r.Source)
	} else {
		row("result", "not recorded")
	}
	row("blocks", "%d", s.Blocks)
	row("records", "%d", s.Records)
	var types []string
	for t, n := range c.types {
		if n > 0 {
			types = append(types, fmt.Sprintf("%s=%d", typeHex(byte(t)), n))
		}
	}
	row("types", "%s", strings.Join(types, " "))
	if c.syncs > 0 {
		row("syncs", "%d, last at frame %d", c.syncs, c.lastSync)
	} else {
		row("syncs", "none")
	}
	writeCommands(row, s.Timelines)
	writeState(row, s)
}

// writeCommands adds each player's command counts, then each player's build
// order.
func writeCommands(row func(name, format string, args ...any), players []*match.PlayerTimeline) {
	if len(players) == 0 {
		row("commands", "none")
		return
	}
	for _, p := range players {
		counts := make([]string, 0, len(p.Counts))
		for t, n := range p.Counts {
			counts = append(counts, fmt.Sprintf("%s=%d", msg.KindName(t), n))
		}
		slices.Sort(counts)
		row("commands", "player %d: %s", p.ID, strings.Join(counts, " "))
	}
	for _, p := range players {
		for _, e := range p.BuildOrder() {
			row("build", "player %d at %.2f s: %s", p.ID, float64(e.Frame)/match.FramesPerSecond, buildText(e.Command))
		}
	}
}

func buildText(c msg.Command) string {
	switch c := c.(type) {
	case *msg.Place:
		return fmt.Sprintf("place type %d", c.TypeID)
	case *msg.Upgrade:
		return fmt.Sprintf("upgrade device %d", c.Device)
	}
	return msg.KindName(c.MsgType())
}

// writeState adds what the snapshots give: per team the economy, losses,
// reactor health, damage dealt and builds, per side the damage taken, and
// per player the damage dealt and the mean APM.
func writeState(row func(name, format string, args ...any), s *match.Summary) {
	st := s.State
	series := map[int32]*match.TeamSeries{}
	if st != nil {
		for i := range st.Teams {
			series[st.Teams[i].Team] = &st.Teams[i]
		}
	}
	type events struct{ weapons, others, builds int }
	by := map[int32]*events{}
	get := func(team int32) *events {
		e := by[team]
		if e == nil {
			e = &events{}
			by[team] = e
		}
		return e
	}
	for id := range series {
		get(id)
	}
	for _, l := range s.Losses {
		if e := get(l.Team); l.Weapon {
			e.weapons++
		} else {
			e.others++
		}
	}
	firstWeapon := map[int32]match.DeviceEvent{}
	for _, b := range s.Builds { // by frame
		get(b.Team).builds++
		if _, ok := firstWeapon[b.Team]; b.Weapon && !ok {
			firstWeapon[b.Team] = b
		}
	}
	teams := slices.Sorted(maps.Keys(by))
	for _, id := range teams {
		if ts := series[id]; ts != nil {
			row("economy", "team %d: floated metal %.1f, energy %.1f; peak storage metal %.1f, energy %.1f",
				id, last(ts.FloatedMetal), last(ts.FloatedEnergy), peak(ts.MaxMetal), peak(ts.MaxEnergy))
		}
	}
	for _, id := range teams {
		e := by[id]
		text := fmt.Sprintf("team %d: weapons %d, other devices %d", id, e.weapons, e.others)
		if ts := series[id]; ts != nil {
			lost := 0
			for _, n := range ts.StrutsLost {
				lost += n
			}
			text += fmt.Sprintf(", struts %d", lost)
		}
		row("losses", "%s", text)
	}
	for _, id := range teams {
		if ts := series[id]; ts != nil {
			row("reactor", "team %d: health %.1f", id, last(ts.ReactorHealth))
		}
	}
	for _, id := range teams {
		if ts := series[id]; ts != nil {
			row("damage", "team %d: dealt %.1f", id, last(ts.DamageDealt))
		}
	}
	for _, id := range teams {
		text := fmt.Sprintf("team %d: %d", id, by[id].builds)
		if b, ok := firstWeapon[id]; ok {
			text += fmt.Sprintf(", first weapon %q at frame %d", b.Name, b.Frame)
		}
		row("builds", "%s", text)
	}
	if st == nil {
		return
	}
	for _, sd := range st.Sides {
		row("taken", "side %d: %.1f", sd.Side, last(sd.DamageTaken))
	}
	for _, p := range st.Players {
		text := fmt.Sprintf("%s: %.1f", playerLabel(s.Players, p.Slot), peak(p.Damage))
		sum, n := 0.0, 0
		for i, team := range p.Team {
			if team != 0 && i < len(p.APM) {
				sum += float64(p.APM[i])
				n++
			}
		}
		if n > 0 {
			text += fmt.Sprintf(", mean APM %.1f", sum/float64(n))
		}
		row("damage", "%s", text)
	}
}

// playerLabel names the player on side 1 or 2 with slot.
func playerLabel(players []match.Player, slot uint32) string {
	for _, p := range players {
		if p.Slot == slot && (p.Side == 1 || p.Side == 2) {
			return fmt.Sprintf("player %d", p.ID)
		}
	}
	return fmt.Sprintf("slot %d", slot)
}

// last returns the last value of a series, 0 when it is empty.
func last[T any](s []T) T {
	var v T
	if len(s) > 0 {
		v = s[len(s)-1]
	}
	return v
}

// peak returns the largest value of a series, 0 when it is empty.
func peak[T float32 | float64](s []T) T {
	if len(s) == 0 {
		return 0
	}
	return slices.Max(s)
}
