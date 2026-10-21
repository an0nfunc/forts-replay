// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

// Command record types.
const (
	TypeFireStart   byte = 0xc5
	TypeFireEnd     byte = 0xc6
	TypeFireBlocked byte = 0xc7
	TypeFire        byte = 0xca
	TypeStrut       byte = 0xcb
	TypeDeleteNode  byte = 0xcc
	TypeLink        byte = 0xcd
	TypeDeleteLink  byte = 0xce
	TypeRepair      byte = 0xd0
	TypeDoor        byte = 0xd1
	TypePlace       byte = 0xd2
	TypeUpgrade     byte = 0xd5
	TypeScrap       byte = 0xd6
	TypeAbility     byte = 0xda
)

// Command is a player action. Node IDs and device instances are those of the
// snapshots. A command is what the player asked for; the game may refuse it.
type Command interface {
	Message
	Sender() uint32
	command()
}

// Place places a device or weapon, or moves one (Phantom commander).
type Place struct {
	Input

	T           float32 `json:"t"`       // position from NodeA to NodeB, 0 to 1; on the ground, the orientation angle
	TypeID      uint32  `json:"type_id"` // index into the match's weapons-then-devices table
	NodeA       uint32  `json:"node_a"`  // 0 on the ground
	NodeB       uint32  `json:"node_b"`  // 0 on the ground
	X           float32 `json:"x"`       // world position
	Y           float32 `json:"y"`
	Aim         float32 `json:"aim"`          // initial aim angle in radians, 0 for most types
	MovedDevice int32   `json:"moved_device"` // the device being moved, -1 for a new one
}

// MsgType returns TypePlace.
func (*Place) MsgType() byte { return TypePlace }
func (*Place) command()      {}

// Strut builds a strut from an existing node to a new one. The game writes
// it twice, First set in the first record; the second one builds.
type Strut struct {
	Input

	Serial   uint32  `json:"serial"` // the player's build serial, from 1 in each match
	X        float32 `json:"x"`      // the new node
	Y        float32 `json:"y"`
	DX       float32 `json:"dx"` // the new node minus the source node
	DY       float32 `json:"dy"`
	Source   uint32  `json:"source"` // node the strut starts at
	Material uint32  `json:"material"`
	First    bool    `json:"first"`
	Anchored bool    `json:"anchored"` // the new node is fixed to terrain
}

// MsgType returns TypeStrut.
func (*Strut) MsgType() byte { return TypeStrut }
func (*Strut) command()      {}

// Link builds a strut between two existing nodes, or gives an existing strut
// a new material. Written twice, like Strut.
type Link struct {
	Input

	NodeA    uint32 `json:"node_a"`
	NodeB    uint32 `json:"node_b"`
	Material uint32 `json:"material"`
	First    bool   `json:"first"`
}

// MsgType returns TypeLink.
func (*Link) MsgType() byte { return TypeLink }
func (*Link) command()      {}

// DeleteLink deconstructs a strut.
type DeleteLink struct {
	Input

	NodeA uint32 `json:"node_a"`
	NodeB uint32 `json:"node_b"`
}

// MsgType returns TypeDeleteLink.
func (*DeleteLink) MsgType() byte { return TypeDeleteLink }
func (*DeleteLink) command()      {}

// DeleteNode deconstructs a node's struts. Candidate: a second one on the
// same node cancels it.
type DeleteNode struct {
	Input

	Node uint32 `json:"node"`
}

// MsgType returns TypeDeleteNode.
func (*DeleteNode) MsgType() byte { return TypeDeleteNode }
func (*DeleteNode) command()      {}

// Scrap scraps a device.
type Scrap struct {
	Input

	Device uint32 `json:"device"`
}

// MsgType returns TypeScrap.
func (*Scrap) MsgType() byte { return TypeScrap }
func (*Scrap) command()      {}

// Upgrade upgrades a device, or downgrades it (Overdrive commander); the
// record is the same. The result is a new device instance.
type Upgrade struct {
	Input

	Device uint32 `json:"device"`
}

// MsgType returns TypeUpgrade.
func (*Upgrade) MsgType() byte { return TypeUpgrade }
func (*Upgrade) command()      {}

// Repair repairs at the hovered node.
type Repair struct {
	Input

	Node uint32 `json:"node"`
}

// MsgType returns TypeRepair.
func (*Repair) MsgType() byte { return TypeRepair }
func (*Repair) command()      {}

// Door opens or closes a door strut.
type Door struct {
	Input

	State uint32 `json:"state"` // candidate: 1 opening, 3 closing
	NodeA uint32 `json:"node_a"`
	NodeB uint32 `json:"node_b"`
}

// MsgType returns TypeDoor.
func (*Door) MsgType() byte { return TypeDoor }
func (*Door) command()      {}

// Fire fires a weapon.
type Fire struct {
	Input

	Device    uint32  `json:"device"`
	Angle     float32 `json:"angle"`     // aim angle in radians, 0 to 2 pi
	Power     float32 `json:"power"`     // 0.5 for an immediate fire
	Immediate bool    `json:"immediate"` // follows a FireStart
}

// MsgType returns TypeFire.
func (*Fire) MsgType() byte { return TypeFire }
func (*Fire) command()      {}

// FireBlocked fires a weapon whose line of fire the player's own fort blocks.
type FireBlocked struct {
	Input

	Device uint32  `json:"device"`
	Angle  float32 `json:"angle"`
}

// MsgType returns TypeFireBlocked.
func (*FireBlocked) MsgType() byte { return TypeFireBlocked }
func (*FireBlocked) command()      {}

// FireStart starts an immediate fire: a double click on a weapon, or the fire
// key with it selected. A Fire or FireBlocked follows 2 frames later.
type FireStart struct {
	Input

	Device uint32 `json:"device"`
}

// MsgType returns TypeFireStart.
func (*FireStart) MsgType() byte { return TypeFireStart }
func (*FireStart) command()      {}

// FireEnd ends an immediate fire, 40 frames after its FireStart.
type FireEnd struct {
	Input

	Device uint32 `json:"device"`
}

// MsgType returns TypeFireEnd.
func (*FireEnd) MsgType() byte { return TypeFireEnd }
func (*FireEnd) command()      {}

// Ability activates the commander ability.
type Ability struct {
	Input
}

// MsgType returns TypeAbility.
func (*Ability) MsgType() byte { return TypeAbility }
func (*Ability) command()      {}

// registerCommands adds the command decoders for v.
func registerCommands(r *Registry, v Versions) {
	r.Register(TypeFireStart, v, inputDecoder(45, TypeFireStart, func(in Input, b *body) Message {
		return &FireStart{Input: in, Device: b.u32(24)}
	}))
	r.Register(TypeFireEnd, v, inputDecoder(28, TypeFireEnd, func(in Input, b *body) Message {
		return &FireEnd{Input: in, Device: b.u32(24)}
	}))
	r.Register(TypeFireBlocked, v, inputDecoder(33, TypeFireBlocked, func(in Input, b *body) Message {
		return &FireBlocked{Input: in, Device: b.u32(24), Angle: b.f32(29)}
	}))
	r.Register(TypeFire, v, inputDecoder(51, TypeFire, func(in Input, b *body) Message {
		return &Fire{Input: in, Device: b.u32(28), Angle: b.f32(33), Power: b.f32(37), Immediate: b.flag(49)}
	}))
	r.Register(TypeStrut, v, inputDecoder(74, TypeStrut, func(in Input, b *body) Message {
		return &Strut{
			Input: in, Serial: b.u32(24), X: b.f32(32), Y: b.f32(36), DX: b.f32(40), DY: b.f32(44),
			Source: b.u32(48), Material: b.u32(52), First: b.flag(64), Anchored: b.flag(66),
		}
	}))
	r.Register(TypeDeleteNode, v, inputDecoder(33, TypeDeleteNode, func(in Input, b *body) Message {
		return &DeleteNode{Input: in, Node: b.u32(28)}
	}))
	r.Register(TypeLink, v, inputDecoder(50, TypeLink, func(in Input, b *body) Message {
		return &Link{Input: in, NodeA: b.u32(28), NodeB: b.u32(32), Material: b.u32(44), First: b.flag(48)}
	}))
	r.Register(TypeDeleteLink, v, inputDecoder(41, TypeDeleteLink, func(in Input, b *body) Message {
		return &DeleteLink{Input: in, NodeA: b.u32(28), NodeB: b.u32(32)}
	}))
	r.Register(TypeRepair, v, inputDecoder(32, TypeRepair, func(in Input, b *body) Message {
		return &Repair{Input: in, Node: b.u32(28)}
	}))
	r.Register(TypeDoor, v, inputDecoder(44, TypeDoor, func(in Input, b *body) Message {
		return &Door{Input: in, State: b.u32(24), NodeA: b.u32(28), NodeB: b.u32(32)}
	}))
	r.Register(TypePlace, v, inputDecoder(67, TypePlace, func(in Input, b *body) Message {
		return &Place{
			Input: in, T: b.f32(28), TypeID: b.u32(32), NodeA: b.u32(36), NodeB: b.u32(40),
			X: b.f32(44), Y: b.f32(48), Aim: b.f32(53), MovedDevice: b.i32(59),
		}
	}))
	r.Register(TypeUpgrade, v, inputDecoder(36, TypeUpgrade, func(in Input, b *body) Message {
		return &Upgrade{Input: in, Device: b.u32(28)}
	}))
	r.Register(TypeScrap, v, inputDecoder(34, TypeScrap, func(in Input, b *body) Message {
		return &Scrap{Input: in, Device: b.u32(28)}
	}))
	r.Register(TypeAbility, v, inputDecoder(29, TypeAbility, func(in Input, _ *body) Message {
		return &Ability{Input: in}
	}))
}
