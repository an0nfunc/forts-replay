# forts-replay

A Go library and the `fwr` CLI that decode replay files (`.fwr`) of the game Forts. AGPL-3.0-or-later. Format knowledge lives in `docs/format.md`.

## Layout

Dependencies point downward only:

- `container`: header, LZMA blocks, size limits
- `header`: version-specific header fields (ranked flag, commanders, game options, map files, players with their SteamIDs, AI forts)
- `stream`: records inside one decompressed block
- `msg`: typed messages through a registry keyed by message type and game version; frame tracking
- `lua`: parser for the Lua table text in snapshots (parses, never evaluates)
- `snapshot`: Lua sections of state snapshots (type 0xe1)
- `match`: the match summary (`Build`): players, commanders, result, per-player command timelines, action counts and build orders
- `cmd/fwr`: the CLI
- `schema/`: the JSON Schema of `fwr info --json`
- `internal/fwrtest` builds synthetic replays; `internal/corpus` locates the corpus submodule; `internal/fixture` loads the v1.38.2 replays tests name
- `testdata/corpus/`: the submodule [forts-replay-corpus](https://github.com/an0nfunc/forts-replay-corpus), the test replays by game version; `testdata/corpus.golden` pins a digest of each
- `testdata/probes/`: how to record probe replays, and a note per probe

## Checks

    go vet ./...
    go test -race ./...
    golangci-lint run
    git submodule update --init                  # the corpus; without it, tests that read replays skip
    go test ./cmd/fwr -run TestGolden -update    # regenerate golden output, then review the diff
    go test ./match -run TestCorpusGolden -update    # after a corpus bump or a decoding change, then review the diff
    go test ./<pkg> -run '^$' -fuzz '^Fuzz<Name>$' -fuzztime 30s -fuzzminimizetime 0
    FWR_CORPUS=<dir> go test ./match ./cmd/fwr -run 'TestCorpus$|TestCorpusSteamIDs'    # every .fwr under <dir>; not run in CI

## Rules

- Every byte of a replay is hostile. Declared sizes never drive an allocation beyond the configured limits, no input may panic, and every library package has a fuzz target seeded from the corpus replays.
- Nothing is dropped: an unknown record decodes to `msg.Raw`, a failed decode to `msg.Invalid`, and container damage after the first block still returns what decoded before it. `dump --json` keeps every payload as hex, except the pixels of a typed screenshot (0xe2) and SteamIDs, printed as zeros (see below).
- A new message decoder is registered in `msg.DefaultRegistry` with the game versions it covers. Packages hold no mutable package-level state.
- `fwr info --json` carries `schema_version` 1, described by `schema/info-v1.json`. Adding a field, to the output and the schema together, keeps the version; removing one or changing its meaning bumps it. `TestInfoSchema` checks the output against a closed copy of the schema. `dump --json` carries 0 (unstable).
- `docs/format.md` records format knowledge only: no disassembly, game code or addresses. The repository ships no EarthWork game data. The corpus replays' snapshots hold the game's AI script tables, so goldens and tests print synthetic snapshots (`fwrtest.Snapshot`), never the replays' Lua text.
- The parser decodes replay files and never contacts EarthWork's servers. Reading any other game file, for example to name devices, needs @an0nfunc's approval first.
- Test replays live only in the corpus repository; tests skip without the submodule and fail without it in CI (`FWR_REQUIRE_CORPUS`). A corpus change lands here as a submodule bump that Renovate proposes; `TestCorpusGolden` fails on it until `testdata/corpus.golden`, which holds no player names or SteamIDs, is regenerated on that branch and its diff reviewed. Unused bytes, such as those after a name's NUL, hold leftover game memory: check a replay's strings before adding it to the corpus.
- Replays hold player names, per-side commander save-names, and every player's SteamID (the header's player entries; ranked result records (0xbb) and some 0x8d records also hold them in clear). SteamIDs appear decoded only in `info --json`, never in text output, warnings or `dump` fields. `dump --json` and `hex` print them as zeros: bytes 32 to 47 of a 0xbb record, 75 to 90 of a 0x8d record, and any 8 bytes whose upper four are `01 00 10 01`. `extract` writes the bytes unchanged. Files in this repository, the corpus submodule aside, hold @an0nfunc's own name and synthetic values only.
- Third-party dependencies: `github.com/ulikunitz/xz`, and in tests only `github.com/santhosh-tekuri/jsonschema/v6` (depguard keeps it there). Ask before adding another.
- Every `.go` file starts with `// SPDX-License-Identifier: AGPL-3.0-or-later`.
- Commits: short conventional subject (`feat(container): ...`), a body only when it adds something.
