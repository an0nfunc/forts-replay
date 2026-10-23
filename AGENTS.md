# forts-replay

A Go library and the `fwr` CLI that decode replay files (`.fwr`) of the game Forts. AGPL-3.0-or-later. Format knowledge lives in `docs/format.md`.

## Layout

Dependencies point downward only:

- `container`: header, LZMA blocks, size limits
- `header`: version-specific header fields (ranked flag, commanders, game options, map files, players with their SteamIDs, AI forts)
- `stream`: records inside one decompressed block
- `msg`: typed messages through a registry keyed by message type and game version; frame tracking
- `lua`: parser for the Lua table text in snapshots (parses, never evaluates)
- `snapshot`: Lua sections of state snapshots (type 0xe1, 0xe5 in header version 1390)
- `state`: the binary data of state snapshots for header versions 1383 and 1390 (resources, damage, player slots, nodes, devices, struts); imports nothing from this module
- `match`: the match summary (`Build`): players, commanders, result, per-player command timelines, action counts, build orders, and the state series, losses, builds and device types from snapshots
- `cmd/fwr`: the CLI
- `schema/`: the JSON Schema of `fwr info --json`
- `internal/fwrtest` builds synthetic replays; `internal/corpus` locates the corpus submodule; `internal/fixture` loads the v1.38.2 and v1.39.0 replays tests name; `internal/repocheck` checks the tracked files for SteamIDs
- `testdata/corpus/`: the submodule [forts-replay-corpus](https://github.com/an0nfunc/forts-replay-corpus), the test replays by game version; `testdata/corpus.golden` pins a digest of each
- `testdata/probes/`: how to record probe replays, and a note per probe

## Checks

    git submodule update --init                  # the corpus; without it, tests that read replays skip
    go vet ./...
    go test -race ./...
    golangci-lint run
    go test ./cmd/fwr -run TestGolden -update    # regenerate golden output, then review the diff
    go test ./match -run TestCorpusGolden -update    # after a corpus bump or a decoding change, then review the diff
    go test ./<pkg> -run '^$' -fuzz '^Fuzz<Name>$' -fuzztime 30s -fuzzminimizetime 0
    FWR_CORPUS=<dir> go test ./match -run 'TestCorpus$'    # every .fwr under <dir>; not run in CI

## Rules

- Every byte of a replay is hostile. Declared sizes never drive an allocation beyond the configured limits, no input may panic, and every library package has a fuzz target seeded from the corpus replays.
- Nothing is dropped: an unknown record decodes to `msg.Raw`, a failed decode to `msg.Invalid`, and container damage after the first block still returns what decoded before it. `dump --json` keeps every payload as hex, except the pixels of a typed screenshot (0xe2).
- A new message decoder is registered in `msg.DefaultRegistry` with the game versions it covers. Packages hold no mutable package-level state.
- `fwr info --json` carries `schema_version` 1, described by `schema/info-v1.json`. Adding a field or a command kind, to the output and the schema together, keeps the version; removing one or changing its meaning bumps it. `TestInfoSchema` checks the output against a closed copy of the schema, with the kind list closed too. `dump --json` carries 0 (unstable).
- `docs/format.md` records format knowledge only: no disassembly, game code or addresses. The repository ships no EarthWork game data. The corpus replays' snapshots hold the game's AI script tables, so goldens and tests print synthetic snapshots (`fwrtest.Snapshot`, `fwrtest.State`), never the replays' Lua text.
- Test replays live only in the corpus repository; tests skip without the submodule and fail without it in CI (`FWR_REQUIRE_CORPUS`). A corpus change lands here as a submodule bump, which Renovate proposes, to a commit on the corpus's main branch; `TestCorpusGolden` fails on it until `testdata/corpus.golden`, which holds no player names or SteamIDs, is regenerated on that branch and its diff reviewed. Unused bytes, such as those after a name's NUL, hold leftover game memory: check a replay's strings before adding it to the corpus.
- Replays hold player names, per-side commander save-names, and every player's SteamID (the header's player entries; ranked result records (0xbb) and some 0x8d records also hold them in clear, and in version 1390 0x8f to 0x91 too; snapshots' player tables hold them masked as the header does). `info --json` reports the SteamIDs the parser decodes; `dump`, `hex` and `extract` show the bytes as they are. Files in this repository, the corpus submodule aside, hold @an0nfunc's own name and synthetic values only. `go test ./internal/repocheck` fails on an individual SteamID64 (upper 32 bits 0x01100001) in a tracked file, binary or text, that `internal/repocheck/steamids.allow` does not list, in the raw and printed forms `repocheck.SteamIDs` names. It cannot see a SteamID masked as the header's player entries and the snapshots' player tables hold it, or one in another encoding, such as base64 or an account ID alone.
- Third-party dependencies: `github.com/ulikunitz/xz`, and in tests only `github.com/santhosh-tekuri/jsonschema/v6` (depguard keeps it there). Ask before adding another.
- Every `.go` file starts with `// SPDX-License-Identifier: AGPL-3.0-or-later`.
- Commits: short conventional subject (`feat(container): ...`), a body only when it adds something.
- Releases: release-please keeps a pull request open with the next version and `CHANGELOG.md`, built from the conventional commits since the last tag; merging it tags the release. Below 1.0, a breaking change bumps the minor version. Tag rulesets let only the release app create tags and nobody move or delete one.
