// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/an0nfunc/forts-replay/header"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/snapshot"
	"github.com/an0nfunc/forts-replay/state"
)

// StateSeries holds what the snapshots record, one point per decoded
// snapshot. Every array has len(Frames) entries.
type StateSeries struct {
	Frames  []int64        `json:"frames"` // the snapshot's frame; -1 before the first sync
	Teams   []TeamSeries   `json:"teams"`  // by team ID
	Sides   []SideSeries   `json:"sides"`
	Players []PlayerSeries `json:"players"` // header order
}

// TeamSeries is one team's series. A team missing from the snapshot's team
// list gets 0 there. Struts, StrutHP, StrutsLost and Nodes describe the
// fort's stability.
type TeamSeries struct {
	Team          int32     `json:"team"`
	Side          int       `json:"side"`
	Metal         []float32 `json:"metal"`
	Energy        []float32 `json:"energy"`
	MaxMetal      []float32 `json:"max_metal"`
	MaxEnergy     []float32 `json:"max_energy"`
	FloatedMetal  []float32 `json:"floated_metal"`  // cumulative resources lost to full storage
	FloatedEnergy []float32 `json:"floated_energy"` // the same for energy
	DamageDealt   []float32 `json:"damage_dealt"`   // cumulative, to enemies
	ReactorHealth []float64 `json:"reactor_health"` // summed health of the team's reactors
	Struts        []int     `json:"struts"`         // in any state
	StrutHP       []float64 `json:"strut_hp"`       // summed hit points of those struts
	StrutsLost    []int     `json:"struts_lost"`    // lost since the previous point, approximate
	Nodes         []int     `json:"nodes"`          // the team's nodes with at least one strut
	Devices       []int     `json:"devices"`        // weapons and reactors included
	Weapons       []int     `json:"weapons"`
}

// SideSeries is one side's series.
type SideSeries struct {
	Side        int       `json:"side"`         // 1 or 2
	DamageTaken []float32 `json:"damage_taken"` // cumulative
}

// PlayerSeries is one player's series, for every header player on side 1
// or 2.
type PlayerSeries struct {
	Slot   uint32    `json:"slot"`   // equals the player's players[].slot
	Team   []int32   `json:"team"`   // 0 once the player has left
	Damage []float32 `json:"damage"` // cumulative; 0 once the player has left
	APM    []float32 `json:"apm"`    // the game's figure at the snapshot
}

// DeviceEvent is a device lost or finished at a snapshot.
//
// A loss is a device that vanished between two decoded snapshots without an
// upgrade or move naming it in the last two snapshot intervals, and without
// being scrapped; Frame is the snapshot it is missing from. The rule is
// approximate.
//
// A build is a device finished during the match; Frame is the first
// snapshot at which its build progress stands at its final value, so it
// finished in the interval before.
type DeviceEvent struct {
	Frame  int64  `json:"frame"`
	Team   int32  `json:"team"`
	Device uint32 `json:"device"`
	TypeID uint16 `json:"type_id"`
	Name   string `json:"name"`
	Weapon bool   `json:"weapon"`
}

const (
	// stateVersion is the header version whose snapshot state package state
	// decodes.
	stateVersion = 1383

	defaultMaxStates = 8192 // about 11 hours at one snapshot per 5 seconds
	defaultMaxLosses = 1 << 16
	defaultMaxBuilds = 1 << 16
	maxDeviceTypes   = 1024
	// maxNewIDs bounds the window commands per snapshot interval that name
	// an object created after the latest decoded snapshot; the others are
	// bounded by that snapshot's size.
	maxNewIDs = 16384
	// maxTeams bounds the teams with series: the team list's 19 slots,
	// without team 0.
	maxTeams = 18
	// reactorName is the save-name of the device whose health is
	// reactor_health.
	reactorName = "reactor"
	// deconstructing is the strut state after a delete.
	deconstructing = 3
	// ground is a device's node ID when it stands on the ground.
	ground = 0xffffffff
)

// stateBuilder folds the snapshots and the commands that remove objects
// into the state series, the losses, the builds and the device types. It
// keeps only aggregates, the latest decoded snapshot's devices and struts,
// and the build progress of its unfinished devices, never whole states.
type stateBuilder struct {
	decode    bool // the version is stateVersion
	version   uint32
	maxStates int
	maxLosses int
	maxBuilds int
	warnf     func(format string, args ...any)

	snapshots int // snapshot records
	failed    int // snapshots whose state does not decode
	failedAt  int64
	failedErr error

	frames  []int64
	teams   map[int32]*teamCols
	sides   [2]SideSeries
	players []PlayerSeries
	skipped []header.Player // side 1 or 2 with a slot past the slot table
	cut     int             // points past maxStates
	teamCut int             // team-list entries past maxTeams

	prev          *snapIndex // the latest decoded snapshot
	older, recent window     // commands since the decoded snapshot before prev, split at prev
	newIDs        int        // window commands this interval naming objects created after prev
	windowCut     int
	// marks holds, per device of prev or created after it, whether an odd
	// number of scraps named it: it is being scrapped, however long ago,
	// since scrapping a large device outlasts the window, and a second
	// scrap cancels the first. Scraps pass the window's filter and cap.
	marks map[uint32]bool

	losses   []DeviceEvent
	lossCut  int
	building map[uint32]*building // devices of prev created during the match, not yet finished
	builds   []DeviceEvent
	buildCut int
	types    map[uint16]string
	typesCut bool
	renamed  bool
}

type teamCols struct {
	TeamSeries

	owned bool // owned a device at a kept point
}

// snapIndex is what losses need of one decoded snapshot.
type snapIndex struct {
	nextNode, nextDevice uint32
	devices              []state.Device
	deviceIDs            map[uint32]struct{}
	held                 map[seat]struct{} // node pairs holding a device, by team
	fresh                map[seat]names    // names of devices created after the snapshot before
	struts               []strut           // one per node pair
	strutKeys            map[[2]uint32]struct{}
	strutNodes           map[uint32]struct{}
}

// seat is a team's device on a node pair.
type seat struct {
	pair [2]uint32
	team int32
}

// seatOf returns the seat of d, false on the ground.
func seatOf(d state.Device) (seat, bool) {
	if d.NodeA == ground || d.NodeB == ground {
		return seat{}, false
	}
	return seat{pairKey(d.NodeA, d.NodeB), d.Team}, true
}

// names keeps up to two distinct names, enough to tell whether any of the
// names added differs from a given one; a seat with many devices then costs
// constant time per vanished device.
type names struct {
	n int
	v [2]string
}

// add returns ns with name added.
func (ns names) add(name string) names {
	if ns.n < 2 && (ns.n == 0 || ns.v[0] != name) {
		ns.v[ns.n] = name
		ns.n++
	}
	return ns
}

// other reports whether a name added differs from name.
func (ns names) other(name string) bool {
	return ns.n == 2 || ns.n == 1 && ns.v[0] != name
}

// placeSeat returns the seat a place command targets, false on the ground.
func placeSeat(c *msg.Place) (seat, bool) {
	if c.NodeA == 0 && c.NodeB == 0 {
		return seat{}, false
	}
	return seat{pairKey(c.NodeA, c.NodeB), c.Team}, true
}

type strut struct {
	key   [2]uint32
	team  int32
	hp    float32
	state uint8
}

// window holds the IDs, node pairs and nodes that upgrades, moves and
// deletes named, and the seats that moves and new placements target.
type window struct {
	devices map[uint32]struct{}
	pairs   map[[2]uint32]struct{}
	nodes   map[uint32]struct{}
	moves   map[seat]struct{} // destinations
	places  map[seat]struct{}
}

func newWindow() window {
	return window{
		devices: map[uint32]struct{}{}, pairs: map[[2]uint32]struct{}{}, nodes: map[uint32]struct{}{},
		moves: map[seat]struct{}{}, places: map[seat]struct{}{},
	}
}

func newStateBuilder(version uint32, h *header.Header, lim Limits, warnf func(format string, args ...any)) *stateBuilder {
	sb := &stateBuilder{
		decode: version == stateVersion, version: version, warnf: warnf,
		maxStates: lim.MaxStates, maxLosses: lim.MaxLosses, maxBuilds: lim.MaxBuilds,
		teams: map[int32]*teamCols{}, sides: [2]SideSeries{{Side: 1}, {Side: 2}},
		older: newWindow(), recent: newWindow(), marks: map[uint32]bool{},
		building: map[uint32]*building{}, types: map[uint16]string{},
	}
	if sb.maxStates == 0 {
		sb.maxStates = defaultMaxStates
	}
	if sb.maxLosses == 0 {
		sb.maxLosses = defaultMaxLosses
	}
	if sb.maxBuilds == 0 {
		sb.maxBuilds = defaultMaxBuilds
	}
	if h != nil {
		for _, p := range h.Players {
			switch {
			case side(p.Team) != 1 && side(p.Team) != 2:
			case p.Slot >= uint32(len(state.State{}.Slots)):
				sb.skipped = append(sb.skipped, p)
			default:
				sb.players = append(sb.players, PlayerSeries{Slot: p.Slot})
			}
		}
	}
	return sb
}

// command adds a command that may remove a device or strut to the window,
// when it names an object of the latest decoded snapshot or one created
// after it.
func (sb *stateBuilder) command(m msg.Message) {
	if !sb.decode {
		return
	}
	switch c := m.(type) {
	case *msg.Place:
		sb.place(c)
	case *msg.Upgrade:
		sb.device(c.Device)
	case *msg.Scrap:
		sb.scrap(c.Device)
	case *msg.DeleteLink:
		sb.pair(c.NodeA, c.NodeB)
	case *msg.DeleteNode:
		sb.node(c.Node)
	}
}

// device adds id to the window and reports whether it is there.
func (sb *stateBuilder) device(id uint32) bool {
	if _, ok := sb.recent.devices[id]; ok {
		return true
	}
	s := sb.prev
	if s != nil && has(s.deviceIDs, id) || sb.created(s == nil || id >= s.nextDevice) {
		sb.recent.devices[id] = struct{}{}
		return true
	}
	return false
}

// scrap counts a scrap of id in marks.
func (sb *stateBuilder) scrap(id uint32) {
	if odd, ok := sb.marks[id]; ok {
		sb.marks[id] = !odd
		return
	}
	s := sb.prev
	if s != nil && has(s.deviceIDs, id) || sb.created(s == nil || id >= s.nextDevice) {
		sb.marks[id] = true
	}
}

// place adds a move's device and destination, or a new placement's seat.
// A destination counts against maxNewIDs, since one device can be moved
// any number of times; a placement does when its seat held no device in
// the latest decoded snapshot.
func (sb *stateBuilder) place(c *msg.Place) {
	k, seated := placeSeat(c)
	if c.MovedDevice >= 0 {
		if sb.device(uint32(c.MovedDevice)) && seated && !has(sb.recent.moves, k) && sb.created(true) {
			sb.recent.moves[k] = struct{}{}
		}
		return
	}
	if !seated || has(sb.recent.places, k) {
		return
	}
	s := sb.prev
	if s != nil && has(s.held, k) || sb.created(s == nil || k.pair[1] >= s.nextNode) {
		sb.recent.places[k] = struct{}{}
	}
}

func (sb *stateBuilder) pair(a, b uint32) {
	k := pairKey(a, b)
	if _, ok := sb.recent.pairs[k]; ok {
		return
	}
	s := sb.prev
	if s != nil && has(s.strutKeys, k) || sb.created(s == nil || k[1] >= s.nextNode) {
		sb.recent.pairs[k] = struct{}{}
	}
}

func (sb *stateBuilder) node(n uint32) {
	if _, ok := sb.recent.nodes[n]; ok {
		return
	}
	s := sb.prev
	if s != nil && has(s.strutNodes, n) || sb.created(s == nil || n >= s.nextNode) {
		sb.recent.nodes[n] = struct{}{}
	}
}

// created reports whether a command naming an object created after the
// latest decoded snapshot (isNew) fits under maxNewIDs.
func (sb *stateBuilder) created(isNew bool) bool {
	switch {
	case !isNew:
		return false
	case sb.newIDs >= maxNewIDs:
		sb.windowCut++
		return false
	}
	sb.newIDs++
	return true
}

// snapshot decodes one snapshot's state and folds it in.
func (sb *stateBuilder) snapshot(frame int64, payload []byte) {
	sb.snapshots++
	if !sb.decode {
		if sb.snapshots == 1 {
			sb.warnf("snapshot state not decoded for version %d", sb.version)
		}
		return
	}
	st, err := decodeState(payload)
	if err != nil {
		if sb.failed == 0 {
			sb.failedAt, sb.failedErr = frame, err
		}
		sb.failed++
		return
	}
	cur := index(st, sb.prev)
	var lost map[int32]int
	if sb.prev != nil {
		lost = sb.diff(frame, cur)
		sb.track(frame, cur)
	}
	maps.DeleteFunc(sb.marks, func(id uint32, _ bool) bool { return !has(cur.deviceIDs, id) })
	sb.addTypes(st.Devices)
	sb.point(frame, st, cur, lost)
	sb.prev, sb.older, sb.recent, sb.newIDs = cur, sb.recent, newWindow(), 0
}

func decodeState(payload []byte) (*state.State, error) {
	end, ok := snapshot.End(payload)
	if !ok {
		return nil, fmt.Errorf("%w: no Lua section", state.ErrCorrupt)
	}
	return state.Parse(payload, end)
}

// index indexes st; prev is the decoded snapshot before it, or nil.
func index(st *state.State, prev *snapIndex) *snapIndex {
	s := &snapIndex{
		nextNode: st.NextNode, nextDevice: st.NextDevice, devices: st.Devices,
		deviceIDs:  make(map[uint32]struct{}, len(st.Devices)),
		held:       map[seat]struct{}{},
		fresh:      map[seat]names{},
		strutKeys:  make(map[[2]uint32]struct{}, len(st.Links)),
		strutNodes: map[uint32]struct{}{},
	}
	for _, d := range st.Devices {
		s.deviceIDs[d.ID] = struct{}{}
		k, ok := seatOf(d)
		if !ok {
			continue
		}
		s.held[k] = struct{}{}
		if prev != nil && d.ID >= prev.nextDevice {
			s.fresh[k] = s.fresh[k].add(d.Name)
		}
	}
	nodeTeam := make(map[uint32]int32, len(st.Nodes))
	for _, n := range st.Nodes {
		if _, ok := nodeTeam[n.ID]; !ok {
			nodeTeam[n.ID] = n.Team
		}
	}
	for _, l := range st.Links {
		k := pairKey(l.NodeA, l.NodeB)
		if has(s.strutKeys, k) {
			continue
		}
		s.strutKeys[k] = struct{}{}
		s.strutNodes[l.NodeA], s.strutNodes[l.NodeB] = struct{}{}, struct{}{}
		team := nodeTeam[l.NodeA]
		if team <= 0 {
			team = nodeTeam[l.NodeB]
		}
		s.struts = append(s.struts, strut{key: k, team: team, hp: l.HP, state: l.State})
	}
	return s
}

// diff finds what vanished between the previous decoded snapshot and cur:
// it adds the devices lost and returns the struts lost per team. A device
// or strut that a command in the window names was removed, as were a
// device being scrapped and a strut that was being deconstructed. Two more
// device rules cover what no command names: the placeholder a Phantom move
// leaves at its destination, and an upgrade the AI makes. A new device of
// the same name on the pair, or one the team placed there, is a rebuild
// after a loss.
func (sb *stateBuilder) diff(frame int64, cur *snapIndex) map[int32]int {
	for _, d := range sb.prev.devices {
		if has(cur.deviceIDs, d.ID) || has(sb.older.devices, d.ID) || has(sb.recent.devices, d.ID) || sb.marks[d.ID] || sb.replaced(d, cur) {
			continue
		}
		sb.addLoss(DeviceEvent{Frame: frame, Team: d.Team, Device: d.ID, TypeID: d.Type, Name: d.Name, Weapon: d.Weapon})
	}
	lost := map[int32]int{}
	for _, s := range sb.prev.struts {
		if has(cur.strutKeys, s.key) || s.state == deconstructing || sb.older.removes(s.key) || sb.recent.removes(s.key) {
			continue
		}
		lost[s.team]++
	}
	return lost
}

// replaced reports whether d, missing from cur, is a Phantom move's
// placeholder or was upgraded without a command.
func (sb *stateBuilder) replaced(d state.Device, cur *snapIndex) bool {
	k, ok := seatOf(d)
	if !ok {
		return false
	}
	if has(sb.older.moves, k) || has(sb.recent.moves, k) {
		return true
	}
	if has(sb.older.places, k) || has(sb.recent.places, k) {
		return false
	}
	return cur.fresh[k].other(d.Name)
}

// removes reports whether the window names the strut with key k or one of
// its nodes.
func (w window) removes(k [2]uint32) bool {
	return has(w.pairs, k) || has(w.nodes, k[0]) || has(w.nodes, k[1])
}

func (sb *stateBuilder) addLoss(l DeviceEvent) {
	if sb.maxLosses < 0 || len(sb.losses) >= sb.maxLosses {
		sb.lossCut++
		return
	}
	sb.losses = append(sb.losses, l)
}

// building is a device created during the match whose build is not
// confirmed finished: its last two sightings, the earlier one valid once
// twice is set.
type building struct {
	build        DeviceEvent
	last, before sighting
	twice        bool
}

// sighting is a device's build progress at a decoded snapshot.
type sighting struct {
	frame    int64
	progress float32
}

// track follows the build progress of the devices in cur created after the
// previous decoded snapshot, adding a build once one is confirmed finished.
// It forgets devices that are gone, so one destroyed or last seen while
// still rising gets none, and never tracks a device of the previous
// snapshot again, so each device gets at most one.
func (sb *stateBuilder) track(frame int64, cur *snapIndex) {
	for _, d := range cur.devices {
		n := sighting{frame, d.Progress}
		b, ok := sb.building[d.ID]
		switch {
		case ok && b.finished(n):
			sb.addBuild(b.build, b.last.frame)
			delete(sb.building, d.ID)
		case ok:
			b.before, b.last, b.twice = b.last, n, true
		case d.ID >= sb.prev.nextDevice && !has(sb.prev.deviceIDs, d.ID):
			sb.building[d.ID] = &building{build: DeviceEvent{Team: d.Team, Device: d.ID, TypeID: d.Type, Name: d.Name, Weapon: d.Weapon}, last: n}
		}
	}
	maps.DeleteFunc(sb.building, func(id uint32, _ *building) bool { return !has(cur.deviceIDs, id) })
}

// finished reports whether sighting n confirms that the build stopped at
// the last one: the progress holds, or it falls after a rise smaller than
// the seconds elapsed. A fall after a full-rate rise, or right after the
// first sighting, is a build cancelled by scrapping.
func (b *building) finished(n sighting) bool {
	l, p := b.last, b.before
	switch {
	case n.progress == l.progress:
		return true
	case n.progress > l.progress || !b.twice || l.progress <= p.progress:
		return false
	}
	return float64(l.progress)-float64(p.progress) < float64(l.frame-p.frame)/FramesPerSecond
}

func (sb *stateBuilder) addBuild(b DeviceEvent, frame int64) {
	if sb.maxBuilds < 0 || len(sb.builds) >= sb.maxBuilds {
		sb.buildCut++
		return
	}
	b.Frame = frame
	sb.builds = append(sb.builds, b)
}

func (sb *stateBuilder) addTypes(devs []state.Device) {
	for _, d := range devs {
		name, ok := sb.types[d.Type]
		switch {
		case ok && name != d.Name && !sb.renamed:
			sb.renamed = true
			sb.warnf("device type %d is named %q and %q; the first name stays", d.Type, name, d.Name)
		case ok:
		case len(sb.types) >= maxDeviceTypes:
			if !sb.typesCut {
				sb.typesCut = true
				sb.warnf("device types cut at %d", maxDeviceTypes)
			}
		default:
			sb.types[d.Type] = d.Name
		}
	}
}

// point adds one series point for st, at frame, unless the series is full.
func (sb *stateBuilder) point(frame int64, st *state.State, cur *snapIndex, lost map[int32]int) {
	if sb.maxStates < 0 || len(sb.frames) >= sb.maxStates {
		sb.cut++
		return
	}
	type counts struct {
		devices, weapons, struts, nodes int
		reactor, strutHP                float64
	}
	by := map[int32]*counts{}
	get := func(team int32) *counts {
		c := by[team]
		if c == nil {
			c = &counts{}
			by[team] = c
		}
		return c
	}
	for _, d := range st.Devices {
		c := get(d.Team)
		c.devices++
		if d.Weapon {
			c.weapons++
		}
		if d.Name == reactorName {
			c.reactor += float64(d.Health)
		}
	}
	for _, s := range cur.struts {
		c := get(s.team)
		c.struts++
		c.strutHP += float64(s.hp)
	}
	counted := map[uint32]bool{}
	for _, n := range st.Nodes {
		if has(cur.strutNodes, n.ID) && !counted[n.ID] {
			counted[n.ID] = true
			get(n.Team).nodes++
		}
	}
	i := len(sb.frames)
	sb.frames = append(sb.frames, frame)
	for _, t := range st.Teams {
		if t.ID == 0 {
			continue
		}
		tc := sb.team(t.ID, i)
		if tc == nil || len(tc.Metal) > i { // past maxTeams, or listed twice
			continue
		}
		c := get(t.ID)
		tc.owned = tc.owned || c.devices > 0
		ts := &tc.TeamSeries
		ts.Metal = append(ts.Metal, t.Metal)
		ts.Energy = append(ts.Energy, t.Energy)
		ts.MaxMetal = append(ts.MaxMetal, t.MaxMetal)
		ts.MaxEnergy = append(ts.MaxEnergy, t.MaxEnergy)
		ts.FloatedMetal = append(ts.FloatedMetal, t.FloatedMetal)
		ts.FloatedEnergy = append(ts.FloatedEnergy, t.FloatedEnergy)
		ts.DamageDealt = append(ts.DamageDealt, t.DamageDealt)
		ts.ReactorHealth = append(ts.ReactorHealth, c.reactor)
		ts.Struts = append(ts.Struts, c.struts)
		ts.StrutHP = append(ts.StrutHP, c.strutHP)
		ts.StrutsLost = append(ts.StrutsLost, lost[t.ID])
		ts.Nodes = append(ts.Nodes, c.nodes)
		ts.Devices = append(ts.Devices, c.devices)
		ts.Weapons = append(ts.Weapons, c.weapons)
	}
	for _, tc := range sb.teams {
		if len(tc.Metal) == i {
			tc.pad(i + 1)
		}
	}
	for k := range sb.sides {
		sb.sides[k].DamageTaken = append(sb.sides[k].DamageTaken, st.DamageTaken[k])
	}
	for k := range sb.players {
		p := &sb.players[k]
		sl := st.Slots[p.Slot]
		p.Team = append(p.Team, sl.Team)
		p.Damage = append(p.Damage, sl.Damage)
		p.APM = append(p.APM, sl.APM)
	}
}

// team returns the series of team id at point i, adding one padded with i
// zeros while there is room, or nil.
func (sb *stateBuilder) team(id int32, i int) *teamCols {
	if tc, ok := sb.teams[id]; ok {
		return tc
	}
	if len(sb.teams) >= maxTeams {
		sb.teamCut++
		return nil
	}
	tc := &teamCols{Team: id, Side: side(id)}
	tc.pad(i)
	sb.teams[id] = tc
	return tc
}

// pad appends zeros to every array up to n entries.
func (tc *teamCols) pad(n int) {
	ts := &tc.TeamSeries
	ts.Metal = padTo(ts.Metal, n)
	ts.Energy = padTo(ts.Energy, n)
	ts.MaxMetal = padTo(ts.MaxMetal, n)
	ts.MaxEnergy = padTo(ts.MaxEnergy, n)
	ts.FloatedMetal = padTo(ts.FloatedMetal, n)
	ts.FloatedEnergy = padTo(ts.FloatedEnergy, n)
	ts.DamageDealt = padTo(ts.DamageDealt, n)
	ts.ReactorHealth = padTo(ts.ReactorHealth, n)
	ts.Struts = padTo(ts.Struts, n)
	ts.StrutHP = padTo(ts.StrutHP, n)
	ts.StrutsLost = padTo(ts.StrutsLost, n)
	ts.Nodes = padTo(ts.Nodes, n)
	ts.Devices = padTo(ts.Devices, n)
	ts.Weapons = padTo(ts.Weapons, n)
}

func padTo[T any](s []T, n int) []T {
	if s == nil {
		s = []T{}
	}
	for len(s) < n {
		var zero T
		s = append(s, zero)
	}
	return s
}

// finish adds the series, losses and device types to s, and the warnings
// for what did not decode or was cut.
func (sb *stateBuilder) finish(s *Summary) {
	if sb.failed > 0 {
		sb.warnf("snapshot state does not decode in %d snapshots; the first, at frame %d: %v", sb.failed, sb.failedAt, sb.failedErr)
	}
	if sb.windowCut > 0 {
		sb.warnf("loss window cut at %d commands naming new objects per snapshot interval, %d more ignored", maxNewIDs, sb.windowCut)
	}
	if sb.lossCut > 0 && sb.maxLosses > 0 {
		sb.warnf("losses cut at %d, %d more counted only", sb.maxLosses, sb.lossCut)
	}
	if sb.buildCut > 0 && sb.maxBuilds > 0 {
		sb.warnf("builds cut at %d, %d more counted only", sb.maxBuilds, sb.buildCut)
	}
	if len(sb.types) > 0 {
		s.DeviceTypes = sb.types
	}
	s.Losses = sb.losses
	slices.SortStableFunc(sb.builds, func(a, b DeviceEvent) int {
		return cmp.Or(cmp.Compare(a.Frame, b.Frame), cmp.Compare(a.Device, b.Device))
	})
	s.Builds = sb.builds
	if len(sb.frames) == 0 {
		return
	}
	if sb.cut > 0 && sb.maxStates > 0 {
		sb.warnf("state series cut at %d points, %d more counted only", sb.maxStates, sb.cut)
	}
	if sb.teamCut > 0 {
		sb.warnf("state series kept for %d teams, %d more team-list entries ignored", maxTeams, sb.teamCut)
	}
	for _, p := range sb.skipped {
		sb.warnf("player %d has slot %d, past the snapshot's player slots; no state series", p.ID, p.Slot)
	}
	ss := &StateSeries{Frames: sb.frames, Teams: []TeamSeries{}, Sides: sb.sides[:], Players: sb.players}
	for _, id := range slices.Sorted(maps.Keys(sb.teams)) {
		if tc := sb.teams[id]; tc.owned {
			ss.Teams = append(ss.Teams, tc.TeamSeries)
		}
	}
	if ss.Players == nil {
		ss.Players = []PlayerSeries{}
	}
	s.State = ss
}

func pairKey(a, b uint32) [2]uint32 { return [2]uint32{min(a, b), max(a, b)} }

func has[K comparable](m map[K]struct{}, k K) bool {
	_, ok := m[k]
	return ok
}
