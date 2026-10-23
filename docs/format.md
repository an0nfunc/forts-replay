# Forts replay format

Status: what is known as of game v1.38.2 (header version 1383) and v1.39.0 (header version 1390). The layouts are those of 1383; a "Version 1390" note says where 1390 differs. All integers are little-endian. "Candidate" marks a reading that is consistent with the samples but not confirmed.

## File layout

A replay is a header followed by blocks up to the end of the file. There is no trailer.

## Header

| Offset | Type | Meaning |
|---|---|---|
| 0 | u32 | 1383 in replays from game v1.38.2, 1390 from v1.39.0. Candidate format version. The game's own version appears only in the default file name, `v1.38.2_<map>_<date>_<time>.fwr`, and from v1.39.0 on `Forts v1.39.0_<map>_<date>_<time>.fwr`. |
| 4 | u8 | 0 |
| 5 | u32 | Total physics frames. The simulation runs at 25 Hz. |
| 9 | u32 | 2, also in games with one player or eight. Candidate side count. |
| 13 | u32 | Offset of the first block, which is also the header length: 1943 in a single-player game on Abyss, up to about 290 KB when the header embeds images. |
| 17 | u32 | 1 when team IDs take the form side + 100 × fort, 0 when they are plain 1 and 2 (see "Teams"). Candidate. |
| 21 | u8 | 0 or 1. Unknown. |
| 22 | u32 | Number of screenshot records (0xe2) in the replay |
| 26 | 200 × u32 | Indices of the blocks holding those records, ascending; unused entries are 0 |
| 0x33a | u8 | 1 in a ranked match, else 0. A ranked header has 128 more bytes after it, not decoded, and every later offset moves by 0x80. |
| 0x33b | 32 × u16 | Name given when saving the replay, UTF-16LE, NUL-terminated; empty in replays saved automatically. Bytes after the NUL are left over. |
| 0x37b | u8 | Unknown |
| 0x37c | 100 × u32 | Fifteen-bit candidate random number generator outputs written at save time. The entry at index 73 seeds the commander-name mask (see "Commander names"). |
| 0x50c | 100 bytes | The sides' commander save-names, XOR-masked; see "Commander names". |
| 0x570 | u32 n, then n str8 | Game options, such as `repair_station`, `weapon_pack` and `dlc3` |
| | 2 str8 | The map's `.fwe` and `.spr` files, such as `maps/Abyss/Abyss.fwe` |
| | | The players, AI forts, images and teams, up to the first block (below) |

A str8 is a u32 byte count followed by that many bytes, with no terminator; a str16 is the same with UTF-16LE text.

**Version 1390** has 22 more bytes before the ranked flag:

| Offset | Type | Meaning |
|---|---|---|
| 21 | 4 bytes | `00 00 00 00` in single-player games, `02 00 00 00` in a lobby game: the 1383 u8 and 3 more bytes, or one u32 |
| 25 | str8 | 15 bytes, the date and time of the recording, `YYYYMMDD_HHMMSS`, as in the default file name |
| 44 | u32 | Number of screenshot records |
| 48 | 200 × u32 | Indices of the blocks holding them |

Every later fixed offset is the 1383 one plus 0x16: the ranked flag at 0x350, the save-name at 0x351, the u8 at 0x391, the number array at 0x392, the commander names at 0x522 and the options at 0x586. Whether a ranked header still has 128 more bytes after the flag is not known; the parser assumes so. From the map files on, the layout is that of 1383.

After the map files:

| Type | Meaning |
|---|---|
| u32 | 0 |
| u32 | 0, 1, 3, 5 or 7; 7 in ranked and single-player games. Candidate: DLC mask, bit k-1 for DLC k. |
| 3 × u8 | Flags: 1 in sandbox games; 1 in multiplayer games; 0 |
| u32 | 0, 1 or 2. Candidate: AI difficulty. |
| u32 | Player ID of the client that recorded the replay |
| i32 | That player's team; -3 when it spectates |
| u32 n | Player entries, spectators included |
| n × 101 bytes | Player entries (below) |
| u32 | 0xa1a1a1a1 |
| 2 × f32 | Unknown. Candidate: the second is half the AI difficulty above. |
| u32 n | AI fort entries |
| n entries | i32 team, str8 the AI fort's script file (may be empty), u32 non-zero for a fort the AI plays. A game without AI has a placeholder (side, "", 0) for each side; sandbox games have no entries. |
| u32 | 0xb7b7b7b7 |
| images | While the next u32 is 0x3c3c3c3c: that marker, u32 size, and size bytes holding a PNG of a custom team avatar or badge after 311 bytes of metadata. Up to 3. |
| 2 team records | str8 name key (`Team1`), str16 custom name, str8 short name key (`T1`), str16 custom short name, str8 avatar texture, i32: -1 for stock teams, a candidate series score for custom ones. Stock teams have empty custom names, custom teams empty keys. |
| str8 | Badge texture. The header ends here. |

Player entry:

| Offset | Type | Meaning |
|---|---|---|
| 0 | u32 | Slot, 0 to 8 |
| 4 | u32 | Player ID, the player field (u32@20) of the player's records. A client that rejoins gets a new ID, so records can carry IDs missing from the table. |
| 8 | i32 | Team (see "Teams"); -3 for a spectator |
| 12 | 32 bytes | Name, UTF-16LE, NUL-terminated, up to 15 UTF-16 code units. Bytes after the NUL are left over. |
| 44 | 13 bytes | Zero in single-player games. Not decoded. |
| 57 | 4 × u64 | The second and third hold the player's SteamID: their XOR with 0x314e62818b5f1701, valid when its top 32 bits are 0x01100001. The first and fourth are not used. Zero in single-player games. |
| 89 | 8 bytes | Zero in every sample. Not decoded. |
| 97 | i32 | -3 |

In a ranked replay, the opponent's entry holds a generic localised word for "player" instead of their name.

### Teams

A match has two sides. A team ID is the side plus 100 times the fort: 101, 201, 301 and 401 on side 1, 102 to 402 on side 2. Some lobby games use plain 1 and 2 (header u32@17 is 0), and single-player games use 1 for the player and 102 for the AI. The side is the team ID modulo 100. A player who loses their fort can play on in an ally's, on the same side. The header keeps the team they started with; their commands carry the team they play in at the time (see "Commands").

### Commander names

The 100 bytes at 0x50c (0x522 in version 1390) hold two 50-byte blocks: side 1's commander save-name, then side 2's, each NUL-terminated UTF-8, XOR-masked with a byte stream derived from the u32 array at 0x37c (0x392). The mask is the same in both versions. An empty name means the side had no commander, as in sandbox games; a block with no NUL holds no name. Bytes past a NUL are save-time padding.

The mask: seed = the u32 at index 73 of the array, XOR 0x67e36aed. For each byte, advance the state s = s × 1103515245 + 12345 (mod 2^32); the output is fifteen bits: when bit 31 of s is set, (s + 0xffff, mod 2^32) >> 16, else s >> 16, masked with 0x7fff. The output, 0 to 32767, narrows to the mask byte in 32-bit floats: out × (1/32767) computed as a double and narrowed to a float, times 255 plus 0.5, truncated to an integer.

The mask covers only each name and its NUL, so the stream continues in side 2's block where side 1's name ended, not after 50 bytes. A decoder walks each block byte by byte until the masked byte decodes to the NUL.

Save-names are the game's own, e.g. `commander-fb-fulminator`; a replay reports them verbatim, without display-name mapping.

## Blocks

    u32 0xd1edd1ed   marker
    u32              decompressed size
    u32              size of the payload that follows
    payload:
      u32            decompressed size again
      5 bytes        LZMA properties (0x5d, dictionary 64 KiB in v1.38.2)
      ...            raw LZMA1 stream

The stream has no end marker. It stops once the decompressed size is reached; a decoder that ignores the size produces trailing garbage.

Each block starts with a state snapshot, written every 5 seconds of game time.

## Records

A decompressed block is a sequence of records, none spanning two blocks:

    [u32 0xc0c0c0c0]   optional prefix; only on the first record of the first block
    u32 0xabababab     marker
    u32                payload length
    payload            payload[0] is the message type

Payloads start with the type byte, then 16 bytes that are zero in single-player games and may be non-zero in multiplayer (not decoded), then a second type-like byte at offset 17.

| Type | Typical size | Candidate meaning |
|---|---|---|
| 0xe1 | ~62 KiB | State snapshot, first record of every block |
| 0xbe | 28 | Tick, see below |
| 0xc0 | 164 | Sync, see below |
| 0x92 | 53 | Hover, see "Input state" |
| 0x93 | 63 | Unknown state, see "Input state" |
| 0x94 | 37 | Aim point, see "Input state" |
| 0x95 | 37 | Pointer, see "Input state" |
| 0x97 | 36 | APM, see "Input state" |
| 0xc5 to 0xe0 | 28 to 74 | Commands, see "Commands" |
| 0xe6 | 344 | Script call, see "Script calls" |
| 0x8d, 0x8f, 0x91, 0x9a, 0xb8, 0xba, 0xc8, 0xc9, 0xe3 | 28 to 284 | Not decoded, see "Records not decoded" |
| 0xbb | 85 | Ranked result, see "Ranked result" |
| 0xe2 | about 2.4 MB | Screenshot, see "Screenshots" |

**Version 1390** moves record types: the input records and chat (0x92 to 0x9a) by 3, and every type from the tick (0xbe) to the script call (0xe6) by 4. Lengths, byte 17 and payload layouts stay. The new types overlap old ones, so a decoder picks the table by header version. This document names types by their 1383 IDs.

| 1383 | 1390 | Records |
|---|---|---|
| 0x92 to 0x97 | 0x95 to 0x9a | Input state |
| 0x9a | 0x9d | Chat |
| 0xbe, 0xc0 | 0xc2, 0xc4 | Tick, sync |
| 0xc5 to 0xe0 | 0xc9 to 0xe4 | Commands, 0xc8 and 0xc9 |
| 0xe1 | 0xe5 | Snapshot |
| 0xe2 | 0xe6 | Screenshot |
| 0xe3 | 0xe7 | Not decoded |
| 0xe6 | 0xea | Script call |

No 1390 sample holds the 1383 types 0xc9, 0xcf, 0xd0, 0xdc or 0xe3 yet; the parser decodes 0xcf, 0xd0 and 0xdc at 0xd3, 0xd4 and 0xe0 by the same rule. 0xb8, 0xba and the ranked result 0xbb lie between the two ranges, and their 1390 types are not known. 0x8d keeps its type (candidate), and 0x8f to 0x91 hold other layouts (see "Records not decoded").

## Ticks and syncs (types 0xbe and 0xc0)

The simulation advances in steps of two frames. Each step writes a tick, then a sync, then the step's other messages.

| Offset | Tick (28 bytes) | Sync (164 bytes) |
|---|---|---|
| 0 | u8 0xbe | u8 0xc0 |
| 1 | 16 bytes, zero in games against the AI | the same |
| 17 | u8 0xbe | u8 0xc0 |
| 18 | u16 0 | u16 0 |
| 20 | u32 1 | u32 0 |
| 24 | u32 frame the step advances to | u32 frame |
| 28 | | 17 × u64 |

Of the sync's 17 values, slots 2, 3 and 13 are the same in every sync (1112, 2341, 292270), slot 15 is constant within a replay (a small number in version 1383, a full 64-bit value in 1390), slot 16 is 0, slots 5 to 12 look like hashes, and slots 0, 1, 4 and 14 vary between syncs. At the same frame, slots 8 to 10 match in both samples. Candidate state checksums, for desync detection.

The first tick advances to frame 4 and the first sync is frame 2. Each later sync is 2 frames after the one before, and each tick advances to its sync's frame plus 2. The last sync is the header's frame count, or the count minus 1 when the count is odd, which happens in some multiplayer replays. The parser gives every message the frame of the latest sync before it; messages before the first sync have none. Candidate: a command takes effect in the step it is written in (see "Commands").

A snapshot's `data.gameFrame` is the frame of the sync before it, or 1 in the first block. `data.gameTime` is that frame times 0.04 seconds, as a 32-bit float.

## Input records

Only human input is recorded. The AI runs inside the simulation, so the nodes and devices it builds appear in no record.

Input records start like a tick:

| Offset | Type | Meaning |
|---|---|---|
| 0 | u8 | Type |
| 1 | 16 bytes | Zero in single-player games; may be non-zero in multiplayer. Not decoded. |
| 17 | u8 | The type, except in 0x92 and 0x95: there the type minus 1 for player 1 in single-player games and in some multiplayer replays, and the type otherwise |
| 18 | u16 | 0 |
| 20 | u32 | Player ID from the header's player table (1 in single-player games); 0 in records the game writes |
| 24 | u32 | Type-specific; in most commands the sender's current team (see "Commands") |
| 28 | | Body |

Each type has a fixed length. The parser rejects a record of another length, a float that is not finite and a flag byte other than 0 or 1.

A step writes its records in this order: tick, sync, the snapshot when a block starts there, script calls, then input records, with 0x95 before 0x92.

### Input state

Types 0x92, 0x93, 0x95 and 0x97 carry state and are written only when it changes. A match started from map select writes 0x92, 0x93 and 0x95 again at frame 4 with the bodies of frame 2, and 0x97 at frames 12 and 24. After a restart, and in some matches from map select, 0x93 is missing. Every snapshot holds a copy of the latest state, so a decoder carries each player's last value forward.

**0x95, pointer (37 bytes).** f32@28 x and f32@32 y: the mouse pointer in world coordinates, y growing downward, kept inside the camera bounds. u8@36 is 0. Camera moves and zoom write none.

**0x92, hover (53 bytes).**

| Offset | Type | Meaning |
|---|---|---|
| 28 | u16 | Node A: first node of the hovered or selected link, or the node pressed or held |
| 30 | u16 | Node B: second node of that link, 0 when node A is a single node |
| 32 | u32 | Selected device instance, 0 for none |
| 36 | u32 | 0 |
| 40 | u16 | Hovered terrain block, 0xffff for none. Candidate: set only with hover kind 5. |
| 42 | u8 | Hover kind: 0 nothing, 1 node, 5 terrain, 6 link, 7 device. 9 appears while a strut is dragged over terrain, and right before a node is deleted. |
| 43 | u8 | Selected material (see "IDs"), 0xff for none |
| 44 | u8 | Selected device or weapon type to build (see "IDs"), 0xff for none |
| 45 | u8 | Mouse buttons held: 1 left, 2 right |
| 46 | u8 | Candidate cursor feedback: 1 or 2 while a strut is dragged, 3 the selected weapon's target is valid, 6 on a device just placed, 12 on a reloading weapon, 13 the selected weapon is reloading, 14 its line of fire is blocked |
| 47 | 6 bytes | 0 |

**0x94, aim point (37 bytes).** Candidate. Byte 17 is the type. u32@24 repeats the player ID, f32@28 x and f32@32 y are a point in world coordinates, and u8@36 is 0. Written while a swarm missile launcher is selected: when the pointer moves (a moment later) and when the selection is cancelled. Selecting a missile launcher or a sniper writes none. In version 1390 one record, written while a water cannon was selected, has u8@36 = 1.

**0x93 (63 bytes).** In single-player games zero except u8@44 = 1, and the snapshot holds the same bytes, so it never changes there. In multiplayer games it changes often, up to about 4,500 records a game: u32@28, the four u32 from offset 44 and u8@60 vary, u32@24 repeats the player ID, and u32@32 to u32@40 stay 0. Meaning unknown.

**0x97, APM (36 bytes).** f32@28 is the game's actions-per-minute figure: ten times the actions in the last 150 frames, sampled every 12 frames. Releasing a mouse button in the world counts as an action (a drag counts once), and so do camera keys. Clicks on the HUD and on context-menu items do not. u32@32 is 0. After a restart the figure starts high and decays, since actions from before the restart still count. Snapshots hold the figure per player slot too (see "Player slots").

### Commands

A command is written in the step the player issues it. Candidate: it takes effect in that step, so a command written after sync N is in effect in the game state of frame N + 2. A record is what the player asked for; the game may still refuse it, for example for lack of resources.

In the u32@24 column, Team is the sender's current team (see "Teams"), 1 in single-player games.

| Type | Bytes | Command | u32@24 | Body |
|---|---|---|---|---|
| 0xd2 | 67 | Place a device or weapon, or move one (Phantom commander) | Team | f32@28 position t from node A to node B (on the ground: the orientation angle); u32@32 type ID; u32@36 node A; u32@40 node B (both 0 on the ground); f32@44 x; f32@48 y; u8@52 1; f32@53 initial aim angle (mortars 1.1345, howitzer 0.4363, otherwise 0); u16@57 1; i32@59 -1 for a new device, else the instance being moved; u32@63 0 |
| 0xcb | 74 | Strut from an existing node to a new one | The player's build serial: 1, 2, 3 in each match. The new node keeps it. | u32@28 1; f32@32, f32@36 the new node; f32@40, f32@44 the new node minus the source node; u32@48 source node; u32@52 material; f32@56 -1; u32@60 0x1000; u8@64 1 in the first record of the pair, 0 in the second; u8@65 0; u8@66 1 when the new node is fixed to terrain; bytes 67 to 73 `00 01 00 00 00 00 01` |
| 0xcd | 50 | Strut between two existing nodes, or a new material for an existing strut | Team | u32@28 node A; u32@32 node B; 8 zero bytes; u32@44 material; u8@48 1 in the first record of the pair, 0 in the second; u8@49 0 |
| 0xce | 41 | Delete a strut. It deconstructs over about 76 frames. | Team | u32@28 node A; u32@32 node B; u8@36 0; f32@37 -1 |
| 0xcc | 33 | Delete a node: its struts deconstruct. Candidate: a second 0xcc on the node cancels this, and they rebuild. | Team | u32@28 node; u8@32 0 |
| 0xd6 | 34 | Scrap a device | Team | u32@28 device; u16@32 0 |
| 0xd5 | 36 | Upgrade a device, or downgrade it (Overdrive commander). Both write the same record. | Team | u32@28 device; u32@32 0 |
| 0xd0 | 32 | Repair at the hovered node | Team | u32@28 node |
| 0xcf | 36 | Repair a strut with the wrench | Team | u32@28 node A; u32@32 node B |
| 0xd4 | 32 | Repair a device from its menu | Team | u32@28 device |
| 0xe0 | 44 | Area repair: one record per click, several per drag. Over undamaged structure nothing is recorded. | Team | f32@28 x; f32@32 y; u32@36 0; f32@40 radius, 200 in every sample |
| 0xd1 | 44 | Open or close a door or shield strut | 1 opening, 3 closing | u32@28 node A; u32@32 node B; u32@36 source: 11 the player, from the strut's menu; 3 and 4 the game opening and closing a weapon's doors, also 1 and 5; 6 and 8 unknown; u32@40 that weapon, 0 for sources 6, 8 and 11 |
| 0xdb | 52 | Fire a weapon at a world point, such as swarm missiles at the point a sniper marks. A group fire writes one record per weapon. | f32: x | f32@28 y; f32@32 angle in radians; u32@36 0 (1.0 as f32 in 2 of 3,327 samples); u32@40 device; u32@44 the spotter device, 0 for none; i32@48 -1 |
| 0xca | 51 | Fire a weapon | Sender | u32@28 device; u8@32 1; f32@33 aim angle in radians, 0 to 2 pi; f32@37 power; 10 zero bytes, except u8@49 = 1 for an immediate fire, whose power is 0.5 |
| 0xc7 | 33 | Fire while the player's own fort blocks the line of fire | Device | u8@28 1, in lobby games 0 or 1; f32@29 aim angle. The heavy mortar writes it twice in one step. |
| 0xc5 | 45 | Start an immediate fire: a double click on a weapon, or the fire key with it selected. In single-player games a 0xca with u8@49 = 1, or a 0xc7, follows 2 frames later; for a swarm missile launcher a 0xdb follows 40 frames later. | Device | u8@28 1, in lobby games 0 or 1; f32@29 unknown; f32@33 unknown, 5000.05 for machine guns and 2850 for a mortar; 8 zero bytes |
| 0xc6 | 28 | End of an immediate fire: 40 frames after its 0xc5 in single-player games, 38 to 86 in multiplayer ones, 192 for a swarm missile launcher | Device | none |
| 0xda | 29 | Activate the commander ability | Team | u8@28 1 |
| 0xd7 | 40 | Link a portal strut to another, or unlink it | Node A of the portal strut | u32@28 its node B; u32@32 and u32@36 the nodes of the strut it links to, 0 and 0 to unlink. Candidate: its own nodes turn on passthrough (one sample). |
| 0xd8 | 32 | Reverse a portal | Node A of the portal strut | u32@28 its node B |
| 0xdc | 32 | Choose a weapon's ammunition | i32: the ammunition slot from 0; candidate: -1 toggles the ammunition queue | u32@28 device |

The parser leaves 0xd1 records that name a weapon out of timelines and counts: the game writes them, not the player.

0xcb and 0xcd come in pairs, in the same step or 2 frames apart, identical except for the flag byte. The strut is built by the second record.

Node IDs and device instances are the snapshot's own. A new node takes the next free node ID, but projectiles and debris take node IDs too, so the records alone do not tell a new node's ID. An upgrade replaces the device with a new instance.

Not recorded: camera moves, pause (paused time has no frames), weapon facing, aim and power keys, laser sights, the weapon door key and the door item in a weapon's menu, area repair over undamaged structure, and clicks the game ignores (on a reloading weapon, outside a weapon's arc, on a device under construction, on a locked technology slot, where v1.39.0 writes an `OnBuildError` script call, on a commander dial that is not full). A skirmish has no surrender, and quitting writes no end record.

### IDs

- **Type IDs** (0xd2 u32@32, 0x92 u8@44) index the match's table of weapons followed by devices. The table depends on the loaded options, not on the commander: the battery is 37 in a sandbox game and 38 in a skirmish whose options add `rapidfire`. Ammunition placed like a device has IDs past the weapons and devices (52 and 53 in a sandbox game). Commands carry only the number; a snapshot's device table pairs each type ID with the device's save-name (see "World state"). The table also changes between game versions: with the options `repair_station`, `weapon_pack`, `dlc1_portal_skirmish`, `dlc1_weapons_skirmish` and `dlc3`, v1.39.0 has `watercannon` at 4, and every later weapon and every device is one higher than in v1.38.2 (`fireball_cannon` 22, battery 38), while a v1.39.0 lobby game without `weapon_pack` has the v1.38.2 IDs.
- **Materials** (0xcb u32@52, 0xcd u32@44, 0x92 u8@43), by their build bar names: 0 bracing (the wood strut), 1 backbracing, 3 armour, 4 door, 6 shield, 7 rope, 9 portal.

## Script calls (type 0xe6)

| Offset | Type | Meaning |
|---|---|---|
| 20 | u32 | 0 for calls the game makes; the player ID for calls a player's action causes |
| 24 | char[128] | Function name, NUL-terminated |
| 152 | char[192] | Arguments as text, NUL-terminated |

Both fields hold printable ASCII up to their NUL; the parser reports a record that breaks this as invalid. The bytes after a NUL are usually zero but can hold leftovers. Bytes 1 to 16 are zero when the sender is 0.

Functions seen in v1.38.2 and v1.39.0:

| Function | Sender | Arguments |
|---|---|---|
| `OnStreamComplete` | game | a number and `true`; `1, true` at frame 80, in v1.38.2 in skirmishes only, in v1.39.0 in every game |
| `OnDeviceSelected` | game | the selected device's name in quotes, such as `"reactor"` |
| `OnBuildError` | game | a number; 8 in v1.39.0 for a click with a locked technology slot selected |
| `FireWeapon` | player | a number, a `Vec` world position and two numbers |
| `OverrideAntiAirOfWeapon` | player | two numbers, then a number or `nil` |

## Records not decoded

These types stay raw (`msg.Raw`). Counts are from 103 v1.38.2 replays: 38 multiplayer games and 65 single-player probes.

| Type | Bytes | Where | What is known |
|---|---|---|---|
| 0x8d | 135 | 150 records in 23 games | Byte 17 repeats the type. Some hold the SteamID64 of a player in the header's player table at bytes 75 and 83, the same ID twice, or at byte 83 only. |
| 0x8f | 32 | 18 records in 5 lobby games | Sent by players |
| 0x91 | 53 | Almost only in ranked games, up to about 700 a game | Sent by players |
| 0x93 | 63 | 68 of the 103 games | See "Input state" |
| 0x9a | 42 to 284 | 250 records in 16 lobby games | Chat: the sender, then UTF-16 text. Almost a third comes from the game itself (sender 0, 284 bytes). |
| 0xb8 | 29 | At most about one per game, near the end, in 21 of the 38 multiplayer games | Sent by players |
| 0xba | 28 | Once | u32@24 2 |
| 0xc8 | 28 | 5 records in 3 games | u32@24 a device. Candidate: with 0xc9, the start and end of an action on one weapon. |
| 0xc9 | 33 | 4 records in 2 games | u32@24 a device, f32@28, u8@32. See 0xc8. |
| 0xe3 | 36 | 10 records in 3 games | Sender 0 |

**Version 1390**, from 13 single-player replays and one lobby game. Here the types are those of 1390. Chat (0x9d) keeps the 1383 layout. The other low types carry SteamIDs at fixed positions, and one SteamID in 0x8d belongs to no player entry.

| Type | Bytes | What is known |
|---|---|---|
| 0x8d | 140 | SteamIDs at 76 and 84, or none. Candidate: the 1383 0x8d, 5 bytes longer, with its `05 00 00 00 00 00 00 00` marker at 92. |
| 0x8f | 36 | A SteamID at 24 and a small number at 32. The 1383 0x8f (32 bytes) has the player ID at 20 and 24 and the small number at 28. |
| 0x90 | 48 | SteamIDs at 24 and 36 |
| 0x91 | 40 | A SteamID at 24 |
| 0xcc | 28 | The 1383 0xc8: u32@24 a device. Written 2 or 4 frames after a water cannon's fire command (0xce), naming the water cannon. |

## Ranked result (type 0xbb)

The game writes it when a ranked match ends, 8 to 128 frames after the last snapshot before the end. One sample also holds the other client's record, two frames apart. Other games have none.

In version 1390 its type is not known: 0xbb lies between the ranges that moved by 3 and by 4, so 0xbe and 0xbf are the candidates, and no sample holds either with this layout. The parser leaves the record undecoded and warns for a ranked 1390 replay; the result then comes from the snapshots.

| Offset | Type | Meaning |
|---|---|---|
| 20 | u32 | 0 in the recording client's record |
| 24 | u32 | Winning side |
| 28 | i32 | The recording client's team |
| 32 | u64 | The recording client's SteamID |
| 40 | u64 | The same SteamID again |
| 48 | u32 | Candidate: the recording client's leaderboard rank |
| 52 | 33 bytes | Not decoded. Candidate score and rating before and after the match, and season wins and losses. |

## Screenshots (type 0xe2)

Written with player 0, about every 60 seconds in lobby games and irregularly in ranked ones; the header lists the blocks that hold them. The size in every sample is 1024 × 576 in v1.38.2 and 1024 × 640 in v1.39.0. Candidate: previews for seeking in the replay viewer.

| Offset | Type | Meaning |
|---|---|---|
| 24 | u32 | Width |
| 28 | u32 | Height |
| 32 | u32 | Bytes per pixel: 4 in every sample; the decoder also accepts 3 |
| 36 | u32 | OpenGL pixel format: 0x1908 (RGBA) in every v1.38.2 sample, 0x80e1 (BGRA) in v1.39.0. A tool that shows the pixels reads it. |
| 40 | u32 | Candidate: the record's frame plus 1, in some v1.39.0 records plus 2 |
| 44 | u8 | 0 or 1. Unknown. |
| 45 | | Pixels, width × height × bytes per pixel |

## Snapshots (type 0xe1)

A snapshot is binary world state with Lua tables embedded as text. Each Lua section is stored as `u32 length` followed by the text, which always starts with `data = `. Some sections are preceded by a script name stored the same way (`u32 length`, name bytes), for example `dlc_harpoon` and `dlc3`. In v1.38.2 games on Abyss every snapshot has seven sections, and all sections carry the same `data.gameFrame` (see "Ticks and syncs"). Version 1390 also stores script names without a section, followed by a u32 0, such as `faction-asia`, `faction-fb` and `faction-russia`, between and after the sections.

The Lua text follows the game's table serializer:

- CRLF line endings, tab indentation, one entry per line: `name = value,`, `[integer] = value,`, or a positional `value,`.
- Tables open with `{` on their own line and close with `},` (the outermost with `}`).
- Strings are wrapped in double quotes and written without escaping.
- Numbers are written the way Lua prints them (`%.14g`), so `inf`, `-inf`, `nan`, `-nan(ind)` and exponents can appear.
- Functions, userdata and C functions cannot be written as values; they appear as comment lines such as `-- key = '!!!FUNCTION!!! name line'`, `'!!!USERDATA!!!'` and `'!!!CFUNCTION!!!'`.

The first snapshot after the match ends adds `gameEnded = true` and `gameWinner = <side>` to the `data` table of several sections; earlier snapshots have neither key. Snapshots come every 5 seconds, so it can be up to that late. A replay that stops before it, such as one a player quit, has no winner in its snapshots; a ranked match still has its result record (type 0xbb). The game goes on writing snapshots after the end, in version 1390 up to one with every fort emptied: no reactor, device or strut on any team. The match summary stops at the first snapshot that names the winner.

### Snapshot header

The binary layout below is known for header versions 1383 and 1390. It holds in all 8,967 snapshots of 105 v1.38.2 replays and all 676 snapshots of 14 v1.39.0 replays. Offsets are into the payload, byte 0 being the type.

| Offset | Type | Meaning |
|---|---|---|
| 61 | u32 | The ID the next node gets |
| 65 | u32 | The ID the next device gets |
| 81 | 19 × i32 | Team list, see below |
| 229 | 19 × 12 bytes | Metal and energy held, per team |
| 457 | 19 × 12 bytes | Storage cap, per team |
| 1141 | 19 × 12 bytes | Floated: metal and energy lost to full storage, cumulative, per team |
| 1449 | 19 × f32 | Damage dealt, per team (see "Damage") |
| 1628 | 2 × f32 | 100 or 400. Unknown. |
| 1636 | 2 × f32 | Damage taken: side 1, then side 2 |
| 1652 | 9 × i32 | Team per player slot (see "Player slots") |
| 1688 | 9 × 148 bytes | One block per player slot, up to offset 3020 |

**Team list.** Slots 0, 1 and 2 hold teams 0, 1 and 2, then the fort teams follow in ascending order. An unused slot is 0xffffffff. Entry i of every per-team table belongs to slot i of the team list, so a decoder maps teams through the list, not by position.

**Resources.** A per-team table entry is 12 bytes: u32 unused, f32 metal, f32 energy.

- The held values equal the HUD.
- The cap grows by 2000 energy for a battery as soon as it is placed, and by 300 metal for a metal store once it is built.
- Floated resources rise at the team's income while it is at the cap and stop below it. A refund while storage is full floats too.
- Team 0 holds 1e12 metal and energy, held and cap.
- A destroyed fort's entries drop to 0 and stay there.
- Values can drift a little below 0 (about -0.001). All are finite.

### Damage

All damage counters are f32, cumulative, finite and never negative.

- **Dealt per team** at 1449 + 4 × slot, indexed like the team list. Slots past the last team in use hold garbage, so only slots in use mean anything. It counts damage to enemies only: friendly fire is not counted. In `damage-weapons.fwr`, machine gun rounds that hit the player's own cannon show in no counter, and side 2's damage taken equals team 1's dealt at every snapshot.
- **Taken per side** at 1636 (side 1) and 1640 (side 2). It is the game's own counter and need not equal what the other side dealt: in 77 of 89 side comparisons across the replays it is at least that, mostly 0.93 to 1.3 times it, with outliers from 0.48 to 15.5.
- **Per player**, in the player slot block (below).

Damage by weapon is not in the replay. The slot block's other fields are input state, and no other f32 in the payload holds a per-weapon total. Shots per weapon type can be counted from the fire commands (0xca, 0xdb), whose device IDs the device table maps to type IDs.

A second copy of each player's damage sits in the snapshot's player table before the first Lua section, after a list that is non-empty only in single-player games. The table also holds each player's SteamID, masked as in the header's player entries. The parser does not read it.

### Player slots

The nine player slots match the header's player entries: slot k belongs to the player whose entry has slot k. Slots can have gaps, for example 0, 1, 5 and 6.

The i32 at 1652 + 4 × k is slot k's team: -3 for a spectator, 0 for an empty slot or a player who left.

The 148-byte block at 1688 + 148 × k is all zero for an empty slot:

| Offset | Type | Meaning |
|---|---|---|
| 132 | f32 | The game's APM figure, the one 0x97 carries, sampled at the snapshot |
| 136 | 4 bytes | Unknown |
| 140 | i32 | The player's current team; it follows fort switches |
| 144 | f32 | The player's damage dealt, cumulative |

The rest of the block copies the player's input state (0x92 hover, 0x95 pointer).

- In every snapshot the team at 1652 equals the block's team, every team is 0, -3 or one of the team list, and a spectator's damage is 0.
- A player who leaves gets team 0 in both places, and the block's damage drops to 0, while the team counter keeps it. A player's damage dealt is therefore the maximum of their series, not its last value.
- A team's counter and its players' are close but not equal, and either can be larger. In `damage-weapons.fwr` the player ends at 8,496.6 and their team at 7,882.1.
- The APM figure does not equal the latest 0x97 record at the snapshot's frame (they differ by up to 100), but per-player means over a match agree within 1.5 %.

### World state

The world state starts after the last Lua section. Its node table is the first `ff 20 20 20 20` after that point, preceded by the table's u32 node count, within 64 bytes of the section's end in every snapshot. The bytes before it are not decoded. Then come the node table, the item gap, the device table, the material table, the link table and a tail.

**Node table.** u32 count (at least 1), u8 0xff, then per node the pad `20 20 20 20` and a record:

| Offset | Type | Meaning |
|---|---|---|
| 0 | u32 | Node ID |
| 4 | i32 | Team; 0 and -2 are neutral map structures |
| 20 | f32 | x |
| 24 | f32 | y |
| 156 | u8 | X, which sets the record's length |

| X | Record length |
|---|---|
| 0xff | 157 |
| 1 on the table's last node | 157 |
| 0, or 1 on any other node | 298 |
| 2 | 371 |
| 3 | 302 |

No other X occurs. An extended record, one longer than 157 bytes, holds the ASCII `0000` at +162, which checks the length.

**Item gap.** After the node table comes a run of 0 and 1 bytes, R bytes long. Its first G bytes are a gap whose length is not stored, and the device table follows the gap. G is the largest even number up to R at which the device table and the material table both parse. Past the gap, the run goes on only while the tables' first fields happen to hold bytes of 0 and 1, and the first name's printable bytes end it. So it takes in at most:

- the device count and the first device name's length, 8 bytes;
- with no devices, the device count, the material count and the first material name's length, 12 bytes.

So G is at least R - 12, and a decoder tries only the even lengths from R down to R - 12, at most seven. Exactly one length parses in every snapshot. R - G is 0 in 8,964 snapshots and 4 in 3. The parser rejects a run longer than 1024 bytes.

**Device table.** u32 count, then per device a header, a marker and a body. The header:

| Type | Meaning |
|---|---|
| u32 L, then L bytes | The device's save-name, 1 to 64 printable ASCII bytes, such as `reactor` or `machinegun` |
| u32 | Team |
| u32 | 0 or 1. Unknown. |
| f32 | Build progress, see "Build progress" |
| u32 | The marker, `de de de de` |

Fields by offset from the marker:

| Offset | Type | Meaning |
|---|---|---|
| +4 | u32 | Device ID |
| +8 | i32 | Team |
| +20 | u16 | Type ID (see "IDs") |
| +24 | f32 | x |
| +28 | f32 | y |
| +36 | u32 | Node A ID, 0xffffffff on the ground |
| +40 | u32 | Node B ID, 0xffffffff on the ground |
| +44 | f32 | t. Candidate: the position from node A to node B, as in 0xd2. |
| +48 | f32 | Current health; the maximum is not stored |

The body is 168 bytes after the marker. When the 4 bytes after it, at +172, are `f1 f1 f1 f1`, the device is a weapon, and its body is 388 + 24 × n24 + 9 × (nA + nB) bytes after the marker: u32 n24 at +380 counts 24-byte beam entries, then come u32 nA and nA × 9 bytes, then u32 nB and nB × 9 bytes. Version 1390 has 4 more bytes at +184, 0 in every sample, so every later weapon field is 4 bytes further on, n24 at +384, and the body is 392 + 24 × n24 + 9 × (nA + nB) bytes.

Type IDs shift with the game options, as in commands, so a type ID names a device only within one replay; 81 names occur in the replays. A sequential walk of the device table always ends exactly at the material table.

**Material table.** u32 n, 9 to 42 in the replays, then per material u32 L, a name of L printable bytes (1 to 64), and 4 flag bytes.

**Link table.** For every node, in node table order, a u32 count and that many strut records:

| Type | Meaning |
|---|---|
| u8 | Index of node B in node A's adjacency list |
| u32 | Node B, an index into the node table |
| u32 | Material, an index into the material table |
| u32 n, then n × u32 | Unknown |
| i32 | Device index, or -1. When it is 0 or more, a u8 follows, and when that u8 is 0, an f32 follows. |
| i32 s | Fire segments: when s is above 0, s × 9 bytes follow |
| 85 bytes | Body |

The body starts with `01 01 01 01`; u32 +4 is the material again, f32 +8 the strut's current hit points (the maximum is not stored), and u8 +41 its state: 0 building, 1 finished, 2 repairing, 3 deconstructing, 4 to 6 shield and portal states.

A strut belongs to node A's team, or to node B's when node A's team is 0 or below. Node indices shift between snapshots, so struts are matched across snapshots by their pair of node IDs.

**Tail.** After the link table comes a tail of variable length: area effects, a player table, the music state, the winner and named floats. It is not decoded.

### Build progress

The f32 before a device's marker. It is finite and never negative in all 436,240 device sightings of 105 replays.

- It counts seconds of construction. While a device is built it rises, mostly at 1 per second of game time but at times 2 to 16 times faster (4,153 of 25,625 rising intervals), and stops at the device's build time, which is not stored: for example 20 for a machine gun and 16 for a battery. With the fast-build option it ends at 2.0. Reactors hold 1.0.
- It falls while a device is scrapped, at about 2 to 16 per second, and the device then vanishes. It also falls for other reasons and recovers its earlier value, so a fall alone is no sign of scrapping. Counting each device's first fall, 622 recovered with no scrap command (0xd6) naming the device, 176 ended with the device gone after a 0xd6 named it, and 33 ended with it gone and no 0xd6.
- A second 0xd6 on the same device cancels the scrap: devices named twice recover.
- Since the rate varies, the moment a build finished is known only to the snapshot interval. Of 4,034 intervals where progress rose by less than the time elapsed, 3,683 were followed by a flat interval, 150 by another rise, 34 by a fall and 167 by the device's last sighting.

### Destroyed or removed

When a device or strut is missing from the next snapshot, the replay does not say whether it was destroyed or its owner removed it. The parser reads it from the commands and the snapshots, as below. The rule is approximate. The window is the last two snapshot intervals: the commands since the snapshot before the previous one.

A device was removed by its owner when any of these holds:

- **Upgrade or move:** a 0xd5 upgrade, or a 0xd2 move (instance not -1), named its ID in the window. An upgrade replaces the device with a new instance.
- **Being scrapped:** an odd number of 0xd6 scrap commands named it while it stood, however long ago. A second scrap cancels the first. Scrapping a large device takes longer than two snapshot intervals, so a window would miss it.
- **Phantom placeholder:** it stands on a node pair (in either order, not on the ground), and a 0xd2 move by its team in the window has that pair as its destination (u32@36 and u32@40). A Phantom move creates a temporary device on the destination nodes, which vanishes once the moved device, keeping its ID, arrives there. Matching the command's nodes also covers a move that a second move replaced.
- **Upgrade without a command:** in the snapshot where it is missing, a device of its team with a different name and an ID at or above the previous snapshot's next device ID (u32@65) stands on its node pair, and the window holds no new placement (0xd2 with instance -1) by that team on that pair. The AI's upgrades record no command. A replacement of the same name, or one the team placed there, is a rebuild after a loss, and the loss stands.

Otherwise the device was lost. A weapon lost was sniped or fell with its fort.

A strut was removed when its last state was 3 (deconstructing), or when a 0xce strut delete on its node pair or a 0xcc node delete on either of its nodes falls in the window. Otherwise it was lost.

Known misclassifications:

- A Phantom move on the ground (nodes 0 and 0) leaves a placeholder that no rule explains, which counts as lost.
- A device lost in the same interval as a move onto its node pair counts as a placeholder, so the loss is missed.
- A strut built and lost within one 5-second interval appears in no snapshot.

### Finished builds

A device's build time is not stored, so the snapshot at which it finished is read from its build progress. Devices in the first snapshot, the starting forts, are left out. A device that appears later is followed from its first sighting. Its build finished at the first snapshot where the progress stands at its final value, once the next sighting confirms it:

- the progress is the same; or
- the progress falls, and the rise into the final value was smaller than the seconds elapsed since the sighting before, so the device stopped within that interval.

A fall after a full-rate rise confirms nothing: it is a build cancelled by scrapping. A device destroyed or last seen while its progress still rises never finished. The finishing moment is accurate to one snapshot interval, 5 seconds.
