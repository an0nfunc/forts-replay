// SPDX-License-Identifier: AGPL-3.0-or-later

// Package state decodes the binary data of a state snapshot (message
// 0xe1) for header version 1383: the header's per-team resources and
// damage counters, the player slots, and the world state after the Lua
// sections (nodes, devices and struts). The tail after the struts is not
// decoded.
package state

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// ErrCorrupt reports a snapshot that does not follow the layout.
var ErrCorrupt = errors.New("state: corrupt")

// Version is the header version whose snapshots Parse decodes.
const Version = 1383

// Header offsets into the payload, byte 0 being the type.
const (
	nextNodeAt   = 61
	nextDeviceAt = 65
	teamListAt   = 81
	currentAt    = 229 // per-team tables: 12-byte entries of u32, f32 metal, f32 energy
	maximumAt    = 457
	floatedAt    = 1141
	dealtAt      = 1449
	takenAt      = 1636 // side 1, then side 2
	slotTeamAt   = 1652
	slotBlockAt  = 1688
	slotBlockLen = 148
	headerEnd    = slotBlockAt + numSlots*slotBlockLen

	teamSlots  = 19
	numSlots   = 9
	unusedTeam = 0xffffffff
)

// Caps on what a snapshot declares. The corpus maxima are 1,076 nodes, 318
// devices and 42 materials.
const (
	maxNodes     = 65536
	maxDevices   = 16384
	maxNodeLinks = 64
	maxLinks     = 262144
	maxLinkList  = 64
	maxSegments  = 4096
	maxMaterials = 256
	maxGap       = 1024
	maxName      = 64
	gapTail      = 12 // 0 and 1 bytes the tables can add to the item gap's run
)

// World state layout.
const (
	nodeMark      = "\xff    " // after the node count; each record follows a "    " pad
	nodePad       = 0x20202020
	nodeLen       = 157
	deviceMarker  = 0xdededede
	deviceBody    = 168 // bytes after the marker
	weaponMarker  = 0xf1f1f1f1
	weaponCounts  = 380 // marker-relative offset of the beam count
	linkBody      = 85
	linkBodyStart = 0x01010101
)

// State is the decoded part of one snapshot.
type State struct {
	NextNode, NextDevice uint32     // IDs the next node and device get
	Teams                []Team     // team-list slots in use, team 0 included, in slot order
	DamageTaken          [2]float32 // side 1, side 2
	Slots                [9]Slot    // header player slots
	Nodes                []Node
	Devices              []Device
	Links                []Link
}

// Team holds one team's counters. Team 0 holds 1e12 metal and energy.
type Team struct {
	ID                          int32
	Metal, Energy               float32
	MaxMetal, MaxEnergy         float32
	FloatedMetal, FloatedEnergy float32 // cumulative resources lost to full storage
	DamageDealt                 float32 // cumulative damage to enemies
}

// Slot is one header player slot.
type Slot struct {
	Team   int32 // -3 spectator, 0 empty or left
	APM    float32
	Damage float32 // cumulative; drops to 0 when the player leaves
}

// Node is one node of the node table.
type Node struct {
	ID   uint32
	Team int32 // 0 and -2 are neutral map structures
	X, Y float32
}

// Device is one device of the device table.
type Device struct {
	ID           uint32
	Name         string // the game's save-name, e.g. "reactor"
	Team         int32
	Type         uint16 // type ID; shifts with the game options
	X, Y         float32
	NodeA, NodeB uint32 // 0xffffffff on the ground
	Health       float32
	Progress     float32 // seconds of construction; stops at the build time, which is not stored
	Weapon       bool
}

// Link is one strut of the link table.
type Link struct {
	NodeA, NodeB uint32 // node IDs
	Material     uint32 // index into the snapshot's material table
	HP           float32
	State        uint8 // 0 building, 1 finished, 2 repairing, 3 deconstructing, 4 to 6 shield and portal states
}

// Parse decodes one snapshot payload. start is the end of its last Lua
// section, where the world state begins.
func Parse(payload []byte, start int) (*State, error) {
	if len(payload) < headerEnd {
		return nil, corrupt("payload of %d bytes ends before offset %d", len(payload), headerEnd)
	}
	if start < headerEnd || start > len(payload) {
		return nil, corrupt("world state start %d outside [%d, %d]", start, headerEnd, len(payload))
	}
	s := &State{NextNode: u32At(payload, nextNodeAt), NextDevice: u32At(payload, nextDeviceAt)}
	if err := s.header(payload); err != nil {
		return nil, err
	}
	if err := s.world(payload, start); err != nil {
		return nil, err
	}
	return s, nil
}

// header reads the team tables, the damage counters and the player slots.
// Damage slots past the last team in use hold garbage, so only slots in
// use are read.
func (s *State) header(b []byte) error {
	for i := range teamSlots {
		if u32At(b, teamListAt+4*i) == unusedTeam {
			continue
		}
		t := Team{
			ID:            i32At(b, teamListAt+4*i),
			Metal:         f32At(b, currentAt+12*i+4),
			Energy:        f32At(b, currentAt+12*i+8),
			MaxMetal:      f32At(b, maximumAt+12*i+4),
			MaxEnergy:     f32At(b, maximumAt+12*i+8),
			FloatedMetal:  f32At(b, floatedAt+12*i+4),
			FloatedEnergy: f32At(b, floatedAt+12*i+8),
			DamageDealt:   f32At(b, dealtAt+4*i),
		}
		if !finite(t.Metal, t.Energy, t.MaxMetal, t.MaxEnergy, t.FloatedMetal, t.FloatedEnergy, t.DamageDealt) {
			return corrupt("non-finite counter of team %d", t.ID)
		}
		s.Teams = append(s.Teams, t)
	}
	s.DamageTaken = [2]float32{f32At(b, takenAt), f32At(b, takenAt+4)}
	if !finite(s.DamageTaken[:]...) {
		return corrupt("non-finite damage taken")
	}
	for k := range numSlots {
		blk := slotBlockAt + slotBlockLen*k
		sl := Slot{Team: i32At(b, slotTeamAt+4*k), APM: f32At(b, blk+132), Damage: f32At(b, blk+144)}
		if !finite(sl.APM, sl.Damage) {
			return corrupt("non-finite counter of player slot %d", k)
		}
		s.Slots[k] = sl
	}
	return nil
}

// world reads the node, device, material and link tables from start.
func (s *State) world(b []byte, start int) error {
	m := bytes.Index(b[start:], []byte(nodeMark))
	if m < 4 {
		return corrupt("no node table after offset %d", start)
	}
	r := &reader{b: b, off: start + m - 4}
	s.nodes(r)
	if r.err != nil {
		return r.err
	}
	devs, off, err := gapAndItems(b, r.off)
	if err != nil {
		return err
	}
	s.Devices = make([]Device, len(devs))
	for i, d := range devs {
		s.Devices[i] = d.dev
		s.Devices[i].Name = string(d.name)
	}
	r.off = off
	s.links(r)
	return r.err
}

// nodes reads the node table. The byte X at +156 of a record sets its
// length; an extended record holds "0000" at +162.
func (s *State) nodes(r *reader) {
	start := r.off
	n := r.u32()
	r.u8() // 0xff, found by the search
	if r.err == nil && (n == 0 || n > maxNodes) {
		r.failf("%d nodes at offset %d", n, start)
		return
	}
	s.Nodes = make([]Node, 0, min(int(n), (len(r.b)-r.off)/(4+nodeLen)))
	for i := range int(n) {
		r.marker(nodePad)
		at := r.off
		rec := r.take(nodeLen)
		if rec == nil {
			return
		}
		switch x := rec[156]; {
		case x == 0xff, x == 1 && i == int(n)-1:
		case x <= 1:
			r.extended(at, 298)
		case x == 2:
			r.extended(at, 371)
		case x == 3:
			r.extended(at, 302)
		default:
			r.failf("node record at offset %d has kind %d", at, x)
		}
		if r.err != nil {
			return
		}
		nd := Node{ID: u32At(rec, 0), Team: i32At(rec, 4), X: f32At(rec, 20), Y: f32At(rec, 24)}
		if !finite(nd.X, nd.Y) {
			r.failf("node record at offset %d has position %v, %v", at, nd.X, nd.Y)
			return
		}
		s.Nodes = append(s.Nodes, nd)
	}
}

// extended reads the rest of the extended node record at at, l bytes long.
func (r *reader) extended(at, l int) {
	r.skip(int64(l - nodeLen))
	if r.err == nil && string(r.b[at+162:at+166]) != "0000" {
		r.failf("extended node record at offset %d lacks its check", at)
	}
}

// item is a device whose name is set once the tables around it parse.
type item struct {
	dev  Device
	name []byte
}

// gapAndItems skips the item gap at off and reads the device and material
// tables after it. The gap is a run of 0 and 1 bytes whose length is not
// stored: it is the largest even length, up to the run, at which both tables
// parse. It returns the devices and the offset after the material table.
//
// Past the gap, the run continues only through the u32 counts and name
// lengths before the first name: the device count and the first device
// name's length, or, without devices, the device count, the material count
// and the first material name's length. So the gap is at least the run
// minus gapTail, and at most seven lengths are tried, never one per byte of
// the run.
func gapAndItems(b []byte, off int) ([]item, int, error) {
	run := 0
	for off+run < len(b) && b[off+run] <= 1 {
		if run == maxGap {
			return nil, 0, corrupt("item gap at offset %d above %d bytes", off, maxGap)
		}
		run++
	}
	var devs []item
	var first error
	for g := run &^ 1; g >= max(run-gapTail, 0); g -= 2 {
		var end int
		var err error
		devs, end, err = items(b, off+g, devs[:0])
		if err == nil {
			return devs, end, nil
		}
		if first == nil {
			first = err
		}
	}
	return nil, 0, first
}

// items reads the device and material tables at off, appending the
// devices to devs.
func items(b []byte, off int, devs []item) ([]item, int, error) {
	r := &reader{b: b, off: off}
	n := r.u32()
	if r.err == nil && n > maxDevices {
		r.failf("%d devices at offset %d", n, off)
	}
	for i := uint32(0); i < n && r.err == nil; i++ {
		d := r.device()
		if r.err == nil {
			devs = append(devs, d)
		}
	}
	start := r.off
	m := r.u32()
	if r.err == nil && (m == 0 || m > maxMaterials) {
		r.failf("%d materials at offset %d", m, start)
	}
	for i := uint32(0); i < m && r.err == nil; i++ {
		r.name()
		r.skip(4) // flags
	}
	return devs, r.off, r.err
}

// device reads one device: a header ending in a marker, then a body. A
// weapon's body is longer and holds three counted arrays.
func (r *reader) device() item {
	start := r.off
	name := r.name()
	r.u32() // team, as in the body
	if v := r.u32(); r.err == nil && v > 1 {
		r.failf("device at offset %d has flag %d", start, v)
	}
	progress := math.Float32frombits(r.u32())
	r.marker(deviceMarker)
	body := r.take(deviceBody)
	if body == nil {
		return item{}
	}
	d := Device{
		ID:       u32At(body, 0),
		Team:     i32At(body, 4),
		Type:     binary.LittleEndian.Uint16(body[16:]),
		X:        f32At(body, 20),
		Y:        f32At(body, 24),
		NodeA:    u32At(body, 32),
		NodeB:    u32At(body, 36),
		Health:   f32At(body, 44),
		Progress: progress,
	}
	if !finite(d.Health, d.Progress, d.X, d.Y) {
		r.failf("device at offset %d has health %v, build progress %v and position %v, %v", start, d.Health, d.Progress, d.X, d.Y)
	}
	if r.peek32() == weaponMarker {
		d.Weapon = true
		r.skip(weaponCounts - 4 - deviceBody)
		r.skip(24 * int64(r.u32()))
		r.skip(9 * int64(r.u32()))
		r.skip(9 * int64(r.u32()))
	}
	return item{dev: d, name: name}
}

// links reads the link table: per node in node order, a count and that
// many struts.
func (s *State) links(r *reader) {
	for _, a := range s.Nodes {
		start := r.off
		n := r.u32()
		switch {
		case r.err != nil:
			return
		case n > maxNodeLinks:
			r.failf("%d struts at offset %d", n, start)
			return
		case len(s.Links)+int(n) > maxLinks:
			r.failf("more than %d struts at offset %d", maxLinks, start)
			return
		}
		for range n {
			l := r.link(len(s.Nodes))
			if r.err != nil {
				return
			}
			l.NodeA, l.NodeB = a.ID, s.Nodes[l.NodeB].ID
			s.Links = append(s.Links, l)
		}
	}
}

// link reads one strut, returning node B's index in NodeB.
func (r *reader) link(nodes int) Link {
	start := r.off
	r.u8() // index of B in A's adjacency list
	bi := r.u32()
	mat := r.u32()
	if r.err == nil && int64(bi) >= int64(nodes) {
		r.failf("strut at offset %d to node index %d of %d", start, bi, nodes)
	}
	list := r.u32()
	if r.err == nil && list > maxLinkList {
		r.failf("strut at offset %d lists %d values", start, list)
	}
	r.skip(4 * int64(list))
	if dev := r.i32(); dev >= 0 && r.u8() == 0 {
		r.skip(4)
	}
	if seg := r.i32(); r.err == nil && seg > maxSegments {
		r.failf("strut at offset %d has %d fire segments", start, seg)
	} else if seg > 0 {
		r.skip(9 * int64(seg))
	}
	body := r.take(linkBody)
	if body == nil {
		return Link{}
	}
	l := Link{NodeB: bi, Material: mat, HP: f32At(body, 8), State: body[41]}
	switch {
	case u32At(body, 0) != linkBodyStart:
		r.failf("strut body at offset %d does not start with its marker", start)
	case u32At(body, 4) != mat:
		r.failf("strut at offset %d has material %d and %d", start, mat, u32At(body, 4))
	case !finite(l.HP):
		r.failf("strut at offset %d has hit points %v", start, l.HP)
	}
	return l
}

// reader reads fields in order and keeps the first error; reads after an
// error return zero values.
type reader struct {
	b   []byte
	off int
	err error
}

// take returns the next n bytes, or nil after an error.
func (r *reader) take(n int64) []byte {
	if r.err != nil {
		return nil
	}
	if n > int64(len(r.b)-r.off) {
		r.failf("payload ends at %d, before offset %d", len(r.b), int64(r.off)+n)
		return nil
	}
	b := r.b[r.off : r.off+int(n)]
	r.off += int(n)
	return b
}

func (r *reader) skip(n int64) { r.take(n) }

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *reader) i32() int32 {
	if b := r.take(4); b != nil {
		return i32At(b, 0)
	}
	return 0
}

// peek32 returns the next u32 without reading it, or 0 when fewer than 4
// bytes are left.
func (r *reader) peek32() uint32 {
	if r.err != nil || len(r.b)-r.off < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(r.b[r.off:])
}

// marker reads a u32 that must equal want.
func (r *reader) marker(want uint32) {
	start := r.off
	if v := r.u32(); r.err == nil && v != want {
		r.failf("%#08x at offset %d, want %#08x", v, start, want)
	}
}

// name reads a u32 length and a name of 1 to maxName printable ASCII
// bytes.
func (r *reader) name() []byte {
	start := r.off
	n := r.u32()
	if r.err == nil && (n == 0 || n > maxName) {
		r.failf("name of %d bytes at offset %d", n, start)
	}
	s := r.take(int64(n))
	if r.err != nil {
		return nil
	}
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			r.failf("name at offset %d is not printable", start)
			return nil
		}
	}
	return s
}

func (r *reader) failf(format string, args ...any) {
	r.err = corrupt(format, args...)
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

func u32At(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

func i32At(b []byte, off int) int32 {
	return int32(binary.LittleEndian.Uint32(b[off:])) //nolint:gosec // G115: the field is a two's-complement i32
}

func f32At(b []byte, off int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))
}

// finite reports whether every value is finite; JSON carries no NaN or
// Inf.
func finite(vs ...float32) bool {
	for _, v := range vs {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}
