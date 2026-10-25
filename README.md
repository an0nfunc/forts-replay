# forts-replay

[![ci](https://github.com/an0nfunc/forts-replay/actions/workflows/ci.yml/badge.svg)](https://github.com/an0nfunc/forts-replay/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/an0nfunc/forts-replay.svg)](https://pkg.go.dev/github.com/an0nfunc/forts-replay)
[![License: AGPL-3.0-or-later](https://img.shields.io/badge/license-AGPL--3.0--or--later-blue.svg)](LICENSE)

A Go library and command line tool for reading replay files (`.fwr`) from [Forts](https://www.earthworkgames.com/forts/), the physics-based RTS by EarthWork Games.

Point it at a replay and you get the map, the players, each side's commander, who won, everything each player did during the match, and how the match went: resources, damage, and what each side built and lost.

## What you can do with it

- Build a match history: map, players, commanders, ranked or not, length and winner.
- Look at build orders: what each player built, upgraded, moved or tore down, and when.
- Collect player stats, like how often someone repaired, fired or used their commander ability, how much damage they dealt, and their APM.
- Follow each team's economy every 5 seconds: metal and energy in stock, storage, and what floated away because storage was full.
- See the damage each team dealt and each side took.
- See what each team finished building and what it lost, such as a sniped weapon, and how its fort held up: its struts, their hit points, and how many fell.
- Index a pile of replays. A damaged or truncated file still gives you everything up to the point where it breaks.
- Use it from any language with `fwr info --json`, or from Go directly.
- Dig into the file format with `fwr dump`, `hex` and `extract`. What we know so far is in [docs/format.md](docs/format.md).

Commands name weapons and devices by numeric IDs, because this project doesn't ship any of the game's data. `info --json` maps those IDs to the game's internal names, such as `machinegun` or `battery`, which it reads from the replay itself.

Only replays from Forts v1.38.2 and v1.39.0 are fully supported for now. A ranked v1.39.0 replay gets its result from the snapshots, with a warning, because the type of its ranked result record is not known yet. A replay from any other version after v1.38.2 is decoded as the closest supported version before it, if the result passes checks that a changed format would fail, with a warning that starts with `provisional: `. Replays that fail those checks, and replays from versions before v1.38.2, still open, but with a warning and without the map, players, commanders, commands or match state.

## Install

    go install github.com/an0nfunc/forts-replay/cmd/fwr@latest

For the library, `go get github.com/an0nfunc/forts-replay`. Both need Go 1.27 or later.

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
| `fwr info FILE` | Summary of the match: header fields, players, commanders, result, record counts, each player's commands and build order, and per team the economy, losses, reactor health, damage and finished builds |
| `fwr info --json FILE` | The same summary as JSON, with a stable schema (see below). `--max-commands N` keeps at most N commands in the timelines (0 keeps none); the counts stay complete. `--max-states N`, `--max-losses N` and `--max-builds N` do the same for the match state, the lost devices and the finished builds. |
| `fwr dump FILE` | One line per record. `--json` gives JSON lines, `--type 0x95,0xe1` and `--block N` filter, `--lua` prints the Lua text of snapshots. |
| `fwr hex --type 0x95 FILE` | The payloads of one record type as aligned hex columns |
| `fwr extract -o DIR FILE` | Writes the header and the decompressed blocks to files |
| `fwr version` | The parser's version, such as `v0.2.0` |

`fwr` exits with 0 on success, also for a truncated replay (it prints a warning), 1 if the file can't be decoded, and 2 on usage errors.

### JSON output

If another program consumes the output, use `fwr info --json`. It includes `"schema_version": 1` and follows [schema/info-v1.json](schema/info-v1.json):

```console
$ fwr info --json abyss-bot-974f.fwr | jq -c '.commanders, .result, .timelines[0].commands[0]'
[{"side":1,"name":"commander-fb-fulminator"},{"side":2,"name":"commander-da-builder"}]
{"winner":2,"frame":888,"source":"snapshot"}
{"frame":168,"kind":"delete_node","fields":{"player":1,"team":1,"node":290}}
```

Besides the header, the players and the commands, the JSON holds:

- `state`: one point every 5 seconds per team, side and player. Per team: metal and energy, storage, floated resources, damage dealt, reactor health, devices and weapons, and fort stability (struts, their hit points, struts lost, nodes). Per side: damage taken. Per player: damage dealt and APM.
- `losses` and `builds`: the devices each team lost and finished, with the time and the device's name.
- `device_types`: the name of each device type ID, for the type IDs in `place` commands.
- `parser_version`: the version `fwr version` prints.
- `warnings`: what the parser could not decode or found inconsistent, as sentences. A warning that starts with `provisional: ` says the replay's header version is not known and whether the closest known layout decoded it.

The state, losses and builds end with the match, at the first snapshot that names the winner. Losses and builds are worked out from the replay and can be off now and then; the schema says how. A player's damage drops to 0 when they leave the game, so use the highest value, not the last.

Later releases may add fields and command kinds to version 1, so ignore the ones you don't know. Removing or changing a field means a new schema version. `dump --json` is meant for exploring and can change at any time (`schema_version` 0).

A replay with a lot of commands makes a large document, about 300 bytes per command in real games, and a long game adds a state point every 5 seconds. A program that runs `fwr` on replays from strangers should pass `--max-commands`, `--max-states`, `--max-losses` and `--max-builds` to bound it.

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

A runnable version is `ExampleBuild` in `match`. The packages, highest level first:

| Package | Contents |
|---|---|
| `match` | The match summary: players, commanders, result, timelines, action counts, build orders, match state, losses and builds |
| `msg` | Decodes single records, by record type and game version, and keeps track of the frame |
| `snapshot` | The Lua sections of state snapshots |
| `state` | The binary world state of snapshots: resources, damage, player slots, nodes, devices and struts |
| `lua` | Parser for the Lua table text in snapshots |
| `header` | The version-specific header fields |
| `stream` | Splits a decompressed block into records |
| `container` | Reads the header and the compressed blocks |

Each package only uses the ones below it, so you can stop at any level. For example, walk the raw records with `container`, `stream` and `msg` and skip the summary.

## Untrusted replays

The parser is written to cope with malicious files. Sizes read from a file are capped (`container.Limits`, `lua.Limits`, `match.Limits`), bad input returns an error instead of a panic, and each package is fuzzed in CI. It never uses the network and never runs anything from a replay.

`fwr info` has four caps: `--max-commands N` for the timelines, `--max-states N` for the state series, and `--max-losses N` and `--max-builds N` for the lost and finished devices. The rest of `info --json` stays under about 850 KB. Each kept command adds at most about 460 bytes, each state point at most about 9 KB, and each loss or build at most about 550 bytes. `dump` output and the time `fwr` takes still grow with a replay's decompressed size, so if you run `fwr` on uploaded files, limit its time, memory and output size.

## Player data

Replays contain player names and SteamIDs. `info --json` lists each player's SteamID, and `dump`, `hex` and `extract` show the records as they are, SteamIDs included. A replay is an ordinary file that anyone can edit, including the names, SteamIDs and result. On its own it doesn't prove who played or who won, so a site that ranks players should confirm the result elsewhere, for example against Steam's ranked leaderboards.

## Contributing

Bug reports and pull requests are welcome. The most useful report is a replay that makes `fwr` warn or fail, so attach it. Replays for the test corpus, especially from other game versions, go to [forts-replay-corpus](https://github.com/an0nfunc/forts-replay-corpus). Before opening a pull request, run:

    git submodule update --init
    go vet ./...
    go test -race ./...
    golangci-lint run

[AGENTS.md](AGENTS.md) covers the layout and the project rules, for example how test replays are recorded and why no game data goes into the repository.

## License

AGPL-3.0-or-later, see [LICENSE](LICENSE).

Forts is a trademark of EarthWork Games. This project is not affiliated with or endorsed by EarthWork Games.
