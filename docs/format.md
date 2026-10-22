# Forts replay format

Status: what is known as of game v1.38.2. All integers are little-endian. "Candidate" marks a reading that is consistent with the samples but not confirmed.

## File layout

A replay is a header followed by blocks up to the end of the file. There is no trailer.

## Header

| Offset | Type | Meaning |
|---|---|---|
| 0 | u32 | 1383 in replays from game v1.38.2. Candidate format version. The game's own version appears only in the default file name, `v1.38.2_<map>_<date>_<time>.fwr`. |
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

The 100 bytes at 0x50c hold two 50-byte blocks: side 1's commander save-name, then side 2's, each NUL-terminated UTF-8, XOR-masked with a byte stream derived from the u32 array at 0x37c. An empty name means the side had no commander, as in sandbox games; a block with no NUL holds no name. Bytes past a NUL are save-time padding.

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

Of the sync's 17 values, slots 2, 3 and 13 are the same in every sync (1112, 2341, 292270), slot 15 is constant within a replay, slot 16 is 0, slots 5 to 12 look like hashes, and slots 0, 1, 4 and 14 vary between syncs. At the same frame, slots 8 to 10 match in both samples. Candidate state checksums, for desync detection.

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

**0x94, aim point (37 bytes).** Candidate. Byte 17 is the type. u32@24 repeats the player ID, f32@28 x and f32@32 y are a point in world coordinates, and u8@36 is 0. Written while a swarm missile launcher is selected: when the pointer moves (a moment later) and when the selection is cancelled. Selecting a missile launcher or a sniper writes none.

**0x93 (63 bytes).** In single-player games zero except u8@44 = 1, and the snapshot holds the same bytes, so it never changes there. In multiplayer games it changes often, up to about 4,500 records a game: u32@28, the four u32 from offset 44 and u8@60 vary, u32@24 repeats the player ID, and u32@32 to u32@40 stay 0. Meaning unknown.

**0x97, APM (36 bytes).** f32@28 is the game's actions-per-minute figure: ten times the actions in the last 150 frames, sampled every 12 frames. Releasing a mouse button in the world counts as an action (a drag counts once), and so do camera keys. Clicks on the HUD and on context-menu items do not. u32@32 is 0. After a restart the figure starts high and decays, since actions from before the restart still count.

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
| 0xc7 | 33 | Fire while the player's own fort blocks the line of fire | Device | u8@28 1; f32@29 aim angle. The heavy mortar writes it twice in one step. |
| 0xc5 | 45 | Start an immediate fire: a double click on a weapon, or the fire key with it selected. In single-player games a 0xca with u8@49 = 1, or a 0xc7, follows 2 frames later; for a swarm missile launcher a 0xdb follows 40 frames later. | Device | u8@28 1; f32@29 unknown; f32@33 unknown, 5000.05 for machine guns and 2850 for a mortar; 8 zero bytes |
| 0xc6 | 28 | End of an immediate fire: 40 frames after its 0xc5 in single-player games, 38 to 86 in multiplayer ones, 192 for a swarm missile launcher | Device | none |
| 0xda | 29 | Activate the commander ability | Team | u8@28 1 |
| 0xd7 | 40 | Link a portal strut to another, or unlink it | Node A of the portal strut | u32@28 its node B; u32@32 and u32@36 the nodes of the strut it links to, 0 and 0 to unlink. Candidate: its own nodes turn on passthrough (one sample). |
| 0xd8 | 32 | Reverse a portal | Node A of the portal strut | u32@28 its node B |
| 0xdc | 32 | Choose a weapon's ammunition | i32: the ammunition slot from 0; candidate: -1 toggles the ammunition queue | u32@28 device |

The parser leaves 0xd1 records that name a weapon out of timelines and counts: the game writes them, not the player.

0xcb and 0xcd come in pairs, in the same step or 2 frames apart, identical except for the flag byte. The strut is built by the second record.

Node IDs and device instances are the snapshot's own. A new node takes the next free node ID, but projectiles and debris take node IDs too, so the records alone do not tell a new node's ID. An upgrade replaces the device with a new instance.

Not recorded: camera moves, pause (paused time has no frames), weapon facing, aim and power keys, laser sights, the weapon door key and the door item in a weapon's menu, area repair over undamaged structure, and clicks the game ignores (on a reloading weapon, outside a weapon's arc, on a device under construction, on a locked technology slot, on a commander dial that is not full). A skirmish has no surrender, and quitting writes no end record.

### IDs

- **Type IDs** (0xd2 u32@32, 0x92 u8@44) index the match's table of weapons followed by devices. The table depends on the loaded options, not on the commander: the battery is 37 in a sandbox game and 38 in a skirmish whose options add `rapidfire`. Ammunition placed like a device has IDs past the weapons and devices (52 and 53 in a sandbox game). The parser reports type IDs as numbers.
- **Materials** (0xcb u32@52, 0xcd u32@44, 0x92 u8@43), by their build bar names: 0 bracing (the wood strut), 1 backbracing, 3 armour, 4 door, 6 shield, 7 rope, 9 portal.

## Script calls (type 0xe6)

| Offset | Type | Meaning |
|---|---|---|
| 20 | u32 | 0 for calls the game makes; the player ID for calls a player's action causes |
| 24 | char[128] | Function name, NUL-terminated |
| 152 | char[192] | Arguments as text, NUL-terminated |

Both fields hold printable ASCII up to their NUL; the parser reports a record that breaks this as invalid. The bytes after a NUL are usually zero but can hold leftovers. Bytes 1 to 16 are zero when the sender is 0.

Functions seen in v1.38.2:

| Function | Sender | Arguments |
|---|---|---|
| `OnStreamComplete` | game | a number and `true`; `1, true` at frame 80 |
| `OnDeviceSelected` | game | the selected device's name in quotes, such as `"reactor"` |
| `OnBuildError` | game | a number |
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

## Ranked result (type 0xbb)

The game writes it when a ranked match ends, 8 to 128 frames after the last snapshot before the end. One sample also holds the other client's record, two frames apart. Other games have none.

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

Written with player 0, about every 60 seconds in lobby games and irregularly in ranked ones; the header lists the blocks that hold them. Candidate: previews for seeking in the replay viewer.

| Offset | Type | Meaning |
|---|---|---|
| 24 | u32 | Width |
| 28 | u32 | Height |
| 32 | u32 | Bytes per pixel: 4 in every sample; the decoder also accepts 3 |
| 36 | u32 | OpenGL pixel format: 0x1908 (RGBA) in every sample |
| 40 | u32 | Candidate: the record's frame plus 1 |
| 44 | u8 | 0 or 1. Unknown. |
| 45 | | Pixels, width × height × bytes per pixel |

## Snapshots (type 0xe1)

A snapshot is binary world state with Lua tables embedded as text. Each Lua section is stored as `u32 length` followed by the text, which always starts with `data = `. Some sections are preceded by a script name stored the same way (`u32 length`, name bytes), for example `dlc_harpoon` and `dlc3`. The binary data between sections is not decoded yet. In v1.38.2 games on Abyss every snapshot has seven sections, and all sections carry the same `data.gameFrame` (see "Ticks and syncs").

The Lua text follows the game's table serializer:

- CRLF line endings, tab indentation, one entry per line: `name = value,`, `[integer] = value,`, or a positional `value,`.
- Tables open with `{` on their own line and close with `},` (the outermost with `}`).
- Strings are wrapped in double quotes and written without escaping.
- Numbers are written the way Lua prints them (`%.14g`), so `inf`, `-inf`, `nan`, `-nan(ind)` and exponents can appear.
- Functions, userdata and C functions cannot be written as values; they appear as comment lines such as `-- key = '!!!FUNCTION!!! name line'`, `'!!!USERDATA!!!'` and `'!!!CFUNCTION!!!'`.

The first snapshot after the match ends adds `gameEnded = true` and `gameWinner = <side>` to the `data` table of several sections; earlier snapshots have neither key. Snapshots come every 5 seconds, so it can be up to that late. A replay that stops before it, such as one a player quit, has no winner in its snapshots; a ranked match still has its result record (type 0xbb).
