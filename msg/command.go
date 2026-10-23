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

	TypeRepairLink    byte = 0xcf
	TypeRepairDevice  byte = 0xd4
	TypePortalLink    byte = 0xd7
	TypePortalReverse byte = 0xd8
	TypeFireAt        byte = 0xdb
	TypeAmmo          byte = 0xdc
	TypeRepairArea    byte = 0xe0
)

// Command is a player action. Node IDs and device instances are those of the
// snapshots. A command is what the player asked for; the game may refuse it.
// A Team field is the sender's current team: a player who lost their fort
// plays on in an ally's.
type Command interface {
	Message
	Sender() uint32
	command()
}

// Place places a device or weapon, or moves one (Phantom commander).
type Place struct {
	Input

	Team        int32   `json:"team"`
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

	Team     int32  `json:"team"`
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

	Team  int32  `json:"team"`
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

	Team int32  `json:"team"`
	Node uint32 `json:"node"`
}

// MsgType returns TypeDeleteNode.
func (*DeleteNode) MsgType() byte { return TypeDeleteNode }
func (*DeleteNode) command()      {}

// Scrap scraps a device.
type Scrap struct {
	Input

	Team   int32  `json:"team"`
	Device uint32 `json:"device"`
}

// MsgType returns TypeScrap.
func (*Scrap) MsgType() byte { return TypeScrap }
func (*Scrap) command()      {}

// Upgrade upgrades a device, or downgrades it (Overdrive commander); the
// record is the same. The result is a new device instance.
type Upgrade struct {
	Input

	Team   int32  `json:"team"`
	Device uint32 `json:"device"`
}

// MsgType returns TypeUpgrade.
func (*Upgrade) MsgType() byte { return TypeUpgrade }
func (*Upgrade) command()      {}

// Repair repairs at the hovered node.
type Repair struct {
	Input

	Team int32  `json:"team"`
	Node uint32 `json:"node"`
}

// MsgType returns TypeRepair.
func (*Repair) MsgType() byte { return TypeRepair }
func (*Repair) command()      {}

// Door opens or closes a door or shield strut. The game writes it too,
// around a weapon's shot or scrap.
type Door struct {
	Input

	State  uint32 `json:"state"` // 1 opening, 3 closing
	NodeA  uint32 `json:"node_a"`
	NodeB  uint32 `json:"node_b"`
	Source uint32 `json:"source"` // 11 from the strut's menu; 1 and 3 to 5 for a weapon; 6 and 8 unknown
	Device uint32 `json:"device"` // the weapon the game moves the door for, 0 for none
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
// key with it selected. In single-player games a Fire or FireBlocked follows
// 2 frames later; for a swarm missile launcher a FireAt follows 40 frames later.
type FireStart struct {
	Input

	Device uint32 `json:"device"`
}

// MsgType returns TypeFireStart.
func (*FireStart) MsgType() byte { return TypeFireStart }
func (*FireStart) command()      {}

// FireEnd ends an immediate fire: 40 frames after its FireStart in
// single-player games, 38 to 86 in multiplayer ones, 192 for a swarm missile
// launcher.
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

	Team int32 `json:"team"`
}

// MsgType returns TypeAbility.
func (*Ability) MsgType() byte { return TypeAbility }
func (*Ability) command()      {}

// RepairLink repairs a strut with the wrench.
type RepairLink struct {
	Input

	Team  int32  `json:"team"`
	NodeA uint32 `json:"node_a"`
	NodeB uint32 `json:"node_b"`
}

// MsgType returns TypeRepairLink.
func (*RepairLink) MsgType() byte { return TypeRepairLink }
func (*RepairLink) command()      {}

// RepairDevice repairs a device from its context menu.
type RepairDevice struct {
	Input

	Team   int32  `json:"team"`
	Device uint32 `json:"device"`
}

// MsgType returns TypeRepairDevice.
func (*RepairDevice) MsgType() byte { return TypeRepairDevice }
func (*RepairDevice) command()      {}

// RepairArea repairs the damaged structure around a point: one record per
// click, several per drag, none over undamaged structure.
type RepairArea struct {
	Input

	Team   int32   `json:"team"`
	X      float32 `json:"x"` // world position
	Y      float32 `json:"y"`
	Radius float32 `json:"radius"` // 200 in every sample
}

// MsgType returns TypeRepairArea.
func (*RepairArea) MsgType() byte { return TypeRepairArea }
func (*RepairArea) command()      {}

// PortalLink links a portal strut to another one. Candidate: a link to its
// own nodes turns on passthrough.
type PortalLink struct {
	Input

	NodeA  uint32 `json:"node_a"` // the portal strut
	NodeB  uint32 `json:"node_b"`
	OtherA uint32 `json:"other_a"` // the strut it links to, 0 and 0 to unlink
	OtherB uint32 `json:"other_b"`
}

// MsgType returns TypePortalLink.
func (*PortalLink) MsgType() byte { return TypePortalLink }
func (*PortalLink) command()      {}

// PortalReverse reverses a portal strut.
type PortalReverse struct {
	Input

	NodeA uint32 `json:"node_a"`
	NodeB uint32 `json:"node_b"`
}

// MsgType returns TypePortalReverse.
func (*PortalReverse) MsgType() byte { return TypePortalReverse }
func (*PortalReverse) command()      {}

// FireAt fires a weapon at a world point, such as a swarm launcher at a
// spotter's mark. A group fire writes one per weapon.
type FireAt struct {
	Input

	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	Angle   float32 `json:"angle"` // radians
	Device  uint32  `json:"device"`
	Spotter uint32  `json:"spotter"` // the device that marked the point, 0 for none
}

// MsgType returns TypeFireAt.
func (*FireAt) MsgType() byte { return TypeFireAt }
func (*FireAt) command()      {}

// Ammo selects a weapon's ammunition.
type Ammo struct {
	Input

	Ammo   int32  `json:"ammo"` // slot from 0; candidate: -1 toggles the ammo queue
	Device uint32 `json:"device"`
}

// MsgType returns TypeAmmo.
func (*Ammo) MsgType() byte { return TypeAmmo }
func (*Ammo) command()      {}

// registerCommands adds the command decoders for version v.
func registerCommands(r *Registry, v uint32) {
	register(r, v, TypeFireStart, inputDecoder(45, func(in Input, b *body) Message {
		return &FireStart{Input: in, Device: b.u32(24)}
	}))
	register(r, v, TypeFireEnd, inputDecoder(28, func(in Input, b *body) Message {
		return &FireEnd{Input: in, Device: b.u32(24)}
	}))
	register(r, v, TypeFireBlocked, inputDecoder(33, func(in Input, b *body) Message {
		return &FireBlocked{Input: in, Device: b.u32(24), Angle: b.f32(29)}
	}))
	register(r, v, TypeFire, inputDecoder(51, func(in Input, b *body) Message {
		return &Fire{Input: in, Device: b.u32(28), Angle: b.f32(33), Power: b.f32(37), Immediate: b.flag(49)}
	}))
	register(r, v, TypeStrut, inputDecoder(74, func(in Input, b *body) Message {
		return &Strut{
			Input: in, Serial: b.u32(24), X: b.f32(32), Y: b.f32(36), DX: b.f32(40), DY: b.f32(44),
			Source: b.u32(48), Material: b.u32(52), First: b.flag(64), Anchored: b.flag(66),
		}
	}))
	register(r, v, TypeDeleteNode, inputDecoder(33, func(in Input, b *body) Message {
		return &DeleteNode{Input: in, Team: b.i32(24), Node: b.u32(28)}
	}))
	register(r, v, TypeLink, inputDecoder(50, func(in Input, b *body) Message {
		return &Link{Input: in, Team: b.i32(24), NodeA: b.u32(28), NodeB: b.u32(32), Material: b.u32(44), First: b.flag(48)}
	}))
	register(r, v, TypeDeleteLink, inputDecoder(41, func(in Input, b *body) Message {
		return &DeleteLink{Input: in, Team: b.i32(24), NodeA: b.u32(28), NodeB: b.u32(32)}
	}))
	register(r, v, TypeRepair, inputDecoder(32, func(in Input, b *body) Message {
		return &Repair{Input: in, Team: b.i32(24), Node: b.u32(28)}
	}))
	register(r, v, TypeDoor, inputDecoder(44, func(in Input, b *body) Message {
		return &Door{Input: in, State: b.u32(24), NodeA: b.u32(28), NodeB: b.u32(32), Source: b.u32(36), Device: b.u32(40)}
	}))
	register(r, v, TypePlace, inputDecoder(67, func(in Input, b *body) Message {
		return &Place{
			Input: in, Team: b.i32(24), T: b.f32(28), TypeID: b.u32(32), NodeA: b.u32(36), NodeB: b.u32(40),
			X: b.f32(44), Y: b.f32(48), Aim: b.f32(53), MovedDevice: b.i32(59),
		}
	}))
	register(r, v, TypeUpgrade, inputDecoder(36, func(in Input, b *body) Message {
		return &Upgrade{Input: in, Team: b.i32(24), Device: b.u32(28)}
	}))
	register(r, v, TypeScrap, inputDecoder(34, func(in Input, b *body) Message {
		return &Scrap{Input: in, Team: b.i32(24), Device: b.u32(28)}
	}))
	register(r, v, TypeAbility, inputDecoder(29, func(in Input, b *body) Message {
		return &Ability{Input: in, Team: b.i32(24)}
	}))
	register(r, v, TypeRepairLink, inputDecoder(36, func(in Input, b *body) Message {
		return &RepairLink{Input: in, Team: b.i32(24), NodeA: b.u32(28), NodeB: b.u32(32)}
	}))
	register(r, v, TypeRepairDevice, inputDecoder(32, func(in Input, b *body) Message {
		return &RepairDevice{Input: in, Team: b.i32(24), Device: b.u32(28)}
	}))
	// u32@36 (0 in every sample) is not decoded.
	register(r, v, TypeRepairArea, inputDecoder(44, func(in Input, b *body) Message {
		return &RepairArea{Input: in, Team: b.i32(24), X: b.f32(28), Y: b.f32(32), Radius: b.f32(40)}
	}))
	register(r, v, TypePortalLink, inputDecoder(40, func(in Input, b *body) Message {
		return &PortalLink{Input: in, NodeA: b.u32(24), NodeB: b.u32(28), OtherA: b.u32(32), OtherB: b.u32(36)}
	}))
	register(r, v, TypePortalReverse, inputDecoder(32, func(in Input, b *body) Message {
		return &PortalReverse{Input: in, NodeA: b.u32(24), NodeB: b.u32(28)}
	}))
	// u32@36 and i32@48 (-1 in every sample) are not decoded.
	register(r, v, TypeFireAt, inputDecoder(52, func(in Input, b *body) Message {
		return &FireAt{Input: in, X: b.f32(24), Y: b.f32(28), Angle: b.f32(32), Device: b.u32(40), Spotter: b.u32(44)}
	}))
	register(r, v, TypeAmmo, inputDecoder(32, func(in Input, b *body) Message {
		return &Ammo{Input: in, Ammo: b.i32(24), Device: b.u32(28)}
	}))
}
