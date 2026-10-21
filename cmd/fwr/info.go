// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: fwr info [--json] FILE")
		return exitUsage
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "fwr info: %v\n", err)
		return exitFail
	}
	defer f.Close()
	var c infoCounts
	sum, err := match.Build(ctx, f, match.Options{Observe: func(rec stream.Record, _ int64, m msg.Message) {
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
		err = enc.Encode(sum)
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
