// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/an0nfunc/forts-replay/msg"
)

// Limits bound a Timeline. Zero fields take the defaults.
type Limits struct {
	MaxPlayers  int // distinct player values; default 16
	MaxCommands int // timeline entries over all players; 0 takes the default 65536, a negative value keeps none
}

// Entry is one command of a timeline.
type Entry struct {
	Frame   int64 // frame of the step it was written in; -1 before the first sync
	Command msg.Command
}

// MarshalJSON writes the entry as its frame, kind name and command fields.
func (e Entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Frame  int64       `json:"frame"`
		Kind   string      `json:"kind"`
		Fields msg.Command `json:"fields"`
	}{e.Frame, msg.KindName(e.Command.MsgType()), e.Command})
}

// PlayerTimeline is one player's part of a timeline.
type PlayerTimeline struct {
	ID       uint32
	Commands []Entry      // in replay order, up to the timeline's limit
	Counts   map[byte]int // commands per message type, including those past the limit

	prev      msg.Command
	prevFrame int64
}

// BuildOrder returns the player's new placements and upgrades, in order.
// Moves are left out.
func (p *PlayerTimeline) BuildOrder() []Entry {
	var out []Entry
	for _, e := range p.Commands {
		switch c := e.Command.(type) {
		case *msg.Place:
			if c.MovedDevice < 0 {
				out = append(out, e)
			}
		case *msg.Upgrade:
			out = append(out, e)
		}
	}
	return out
}

// MarshalJSON writes counts keyed by kind name and commands as an array,
// empty when the limit left none.
func (p *PlayerTimeline) MarshalJSON() ([]byte, error) {
	counts := make(map[string]int, len(p.Counts))
	for t, n := range p.Counts {
		counts[msg.KindName(t)] = n
	}
	cmds := p.Commands
	if cmds == nil {
		cmds = []Entry{}
	}
	return json.Marshal(struct {
		Player   uint32         `json:"player"`
		Counts   map[string]int `json:"counts"`
		Commands []Entry        `json:"commands"`
	}{p.ID, counts, cmds})
}

// repeat reports whether c is a FireBlocked equal to the player's previous
// command in the same frame: the heavy mortar writes it twice.
func (p *PlayerTimeline) repeat(frame int64, c msg.Command) bool {
	fb, ok := c.(*msg.FireBlocked)
	if !ok || p.prevFrame != frame {
		return false
	}
	prev, ok := p.prev.(*msg.FireBlocked)
	return ok && *prev == *fb
}

// Timeline collects the commands of a replay per player.
type Timeline struct {
	lim     Limits
	players map[uint32]*PlayerTimeline
	kept    int // entries over all players
	cut     int // commands past MaxCommands, counted only
	ignored int // commands of players past MaxPlayers
}

// NewTimeline returns an empty timeline.
func NewTimeline(lim Limits) *Timeline {
	if lim.MaxPlayers <= 0 {
		lim.MaxPlayers = 16
	}
	if lim.MaxCommands == 0 {
		lim.MaxCommands = 1 << 16
	}
	return &Timeline{lim: lim, players: map[uint32]*PlayerTimeline{}}
}

// Add folds one decoded message, written in the step of frame. It keeps
// commands only, and counts an action once: it skips the first record of a
// strut or link pair, the start and end records around an immediate fire,
// a repeated FireBlocked, and a door the game opens or closes for a weapon.
func (t *Timeline) Add(frame int64, m msg.Message) {
	c, ok := m.(msg.Command)
	if !ok {
		return
	}
	switch v := c.(type) {
	case *msg.Strut:
		if v.First {
			return
		}
	case *msg.Link:
		if v.First {
			return
		}
	case *msg.FireStart, *msg.FireEnd:
		return
	case *msg.Door:
		if v.Device != 0 {
			return
		}
	}
	p := t.player(c.Sender())
	if p == nil {
		t.ignored++
		return
	}
	if p.repeat(frame, c) {
		return
	}
	p.prev, p.prevFrame = c, frame
	p.Counts[c.MsgType()]++
	if t.kept >= t.lim.MaxCommands {
		t.cut++
		return
	}
	p.Commands = append(p.Commands, Entry{Frame: frame, Command: c})
	t.kept++
}

// Players returns the players that issued commands, by ascending ID.
func (t *Timeline) Players() []*PlayerTimeline {
	return slices.SortedFunc(maps.Values(t.players), func(a, b *PlayerTimeline) int { return cmp.Compare(a.ID, b.ID) })
}

// Warnings reports commands the limits left out.
func (t *Timeline) Warnings() []string {
	var w []string
	if t.cut > 0 && t.lim.MaxCommands > 0 {
		w = append(w, fmt.Sprintf("timeline cut at %d commands, %d more counted only", t.lim.MaxCommands, t.cut))
	}
	if t.ignored > 0 {
		w = append(w, fmt.Sprintf("%d commands from players beyond the first %d ignored", t.ignored, t.lim.MaxPlayers))
	}
	return w
}

// player returns the player with id, adding it while there is room, or nil.
func (t *Timeline) player(id uint32) *PlayerTimeline {
	if p, ok := t.players[id]; ok {
		return p
	}
	if len(t.players) >= t.lim.MaxPlayers {
		return nil
	}
	p := &PlayerTimeline{ID: id, Counts: map[byte]int{}}
	t.players[id] = p
	return p
}
