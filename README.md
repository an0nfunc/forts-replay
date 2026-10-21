# forts-replay

[![ci](https://github.com/an0nfunc/forts-replay/actions/workflows/ci.yml/badge.svg)](https://github.com/an0nfunc/forts-replay/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/an0nfunc/forts-replay.svg)](https://pkg.go.dev/github.com/an0nfunc/forts-replay)
[![License: AGPL-3.0-or-later](https://img.shields.io/badge/license-AGPL--3.0--or--later-blue.svg)](LICENSE)

A toolbox for replay files (`.fwr`) of [Forts](https://www.earthworkgames.com/forts/), the physics-based RTS by EarthWork Games: a Go library and the `fwr` command.

It reads who played on which map, with which commanders, who won and when, and every command each player gave, frame by frame: what they built and where, what they upgraded, repaired and fired. Use it for match histories and stats, build-order analysis, replay archives, or to study the format itself.

## What it reads

| Part | Contents |
|---|---|
| Container | The header and the LZMA-compressed blocks. A damaged or cut-off file still yields everything before the damage. |
| Header | Game options, map, ranked flag, players with names and SteamIDs, AI forts, both sides' commanders |
| Result | The winning side and the frame it is recorded at, from the ranked result record or the end-of-match snapshot |
| Commands | Per player: struts, devices and weapons built, moved and deleted, upgrades, repairs, doors, firing, commander abilities; action counts and a build order |
| Records | Frame ticks and syncs, input state, script calls, screenshots |
| Snapshots | The Lua tables in the periodic state snapshots, parsed and never evaluated |

Device and weapon types are numeric IDs: the toolbox ships no name tables or other data from the game's files. A record it does not decode is kept as raw bytes, never dropped. [docs/format.md](docs/format.md) describes the format byte by byte.

Replays from game v1.38.2 (header version 1383) decode fully. For another header version, the container, the record stream and snapshots still decode, while the header fields and the other record types stay raw, with a warning.

## Install

    go install github.com/an0nfunc/forts-replay/cmd/fwr@latest

As a library: `go get github.com/an0nfunc/forts-replay`. Both need Go 1.27 or later.

## The `fwr` command

```console
$ fwr info probe-downgrade-overdrive.fwr
version    1383
frames     1304 (52.16 s)
ranked     no
options    "repair_station" "unlimited-energy" "unlimited-metal" "weapon_pack" "dlc1_portal_skirmish" "dlc1_weapons_skirmish" "dlc3" "fast-build" "rapidfire" "unlock-all-weapons"
map        "maps/Abyss/Abyss.fwe"
player     1: slot 0, team 1, side 1, "random🐧", recorder
ai         team 102, side 2
commander  side 1, "commander-da-overclocker"
commander  side 2, "commander-ee-heavy-weapons"
result     not recorded
blocks     11
records    1491
types      0x92=104 0x93=2 0x95=50 0x97=16 0xbe=652 0xc0=652 0xd2=1 0xd5=2 0xe1=11 0xe6=1
syncs      652, last at frame 1304
commands   player 1: place=1 upgrade=2
build      player 1 at 19.28 s: place type 0
build      player 1 at 28.64 s: upgrade device 10
build      player 1 at 38.96 s: upgrade device 12
```

| Command | Output |
|---|---|
| `fwr info FILE` | Header fields, players, commanders, result, record counts, and commands and build order per player |
| `fwr info --json FILE` | The same summary as one JSON document, with a stable schema (below) |
| `fwr dump FILE` | One line per record. `--json` for JSON lines, `--type 0x95,0xe1` and `--block N` to filter, `--lua` for the Lua text of snapshots. |
| `fwr hex --type 0x95 FILE` | The payloads of one record type in aligned hex columns, for format work |
| `fwr extract -o DIR FILE` | The header and the decompressed blocks as files |

Exit status: 0 on success, including a cut-off replay (with a warning on stderr); 1 when the file cannot be decoded; 2 for usage errors.

### JSON output

`fwr info --json` is the interface for programs in any language. It carries `"schema_version": 1`, described by [schema/info-v1.json](schema/info-v1.json):

```console
$ fwr info --json abyss-bot-974f.fwr | jq -c '.commanders, .result, .timelines[0].commands[0]'
[{"side":1,"name":"commander-fb-fulminator"},{"side":2,"name":"commander-da-builder"}]
{"winner":2,"frame":888,"source":"snapshot"}
{"frame":168,"kind":"delete_node","fields":{"player":1,"node":290}}
```

A later release can add fields within version 1, so ignore fields you do not know; removing a field or changing its meaning bumps the version. `dump --json` is for exploring and can change between releases (`schema_version` 0).

## Library

```go
f, err := os.Open("replay.fwr")
if err != nil {
	log.Fatal(err)
}
s, err := match.Build(ctx, f, match.Options{})
f.Close()
if err != nil {
	log.Fatal(err)
}
for _, p := range s.Players {
	fmt.Printf("player %d, side %d: %q\n", p.ID, p.Side, p.Name)
}
if s.Result != nil {
	fmt.Printf("side %d won at frame %d\n", s.Result.Winner, s.Result.Frame)
}
```

`ExampleBuild` in `match` is the runnable version. The packages, from the top down:

| Package | Contents |
|---|---|
| `match` | The match summary: players, commanders, result, per-player timelines, action counts and build orders |
| `msg` | Typed records from a registry of decoders keyed by record type and game version, and frame tracking |
| `snapshot` | The Lua sections of state snapshots |
| `lua` | A parser for the Lua table text in snapshots |
| `header` | The version-specific header fields |
| `stream` | The records inside one decompressed block |
| `container` | The header and the blocks, with size limits |

A package imports only those below it, so a tool can stop at any layer: walk the records with `container`, `stream` and `msg` and skip the summary.

## Untrusted replays

Every byte of a replay is treated as hostile. Declared sizes never allocate beyond configurable limits (`container.Limits`, `lua.Limits`, `match.Limits`), no input may panic, and every library package has a fuzz target that CI runs on each change. The toolbox only reads files: it opens no network connections and runs no code from a replay.

Output still grows with the input: a crafted file of a few kilobytes can produce a timeline of tens of megabytes. A service that runs `fwr` on uploaded files should cap its time, memory and output.

## Player data

Replays hold player names and every player's SteamID. `fwr` prints SteamIDs only in `info --json`: text output, `dump` and `hex` print them as zeros, while `extract` writes the bytes unchanged. Names, SteamIDs and the result come from the file and can be forged, so verify them before crediting a match to anyone.

## Contributing

Bug reports and pull requests are welcome. A replay that makes `fwr` warn or fail is the most useful report; attach it only if everyone who played in it agrees. Before a pull request:

    go vet ./...
    go test -race ./...
    golangci-lint run

[AGENTS.md](AGENTS.md) has the layout and the rules, such as how fixtures are recorded and that no game data goes into the repository.

## Licence

AGPL-3.0-or-later, see [LICENSE](LICENSE). The replays under `testdata/fixtures` are written by the game and are not covered by it.

Forts is a trademark of EarthWork Games. This is an independent project, not affiliated with, endorsed or sponsored by EarthWork Games.
