// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fixture loads the v1.38.2 sample replays of the corpus for tests.
package fixture

import (
	"bufio"
	"os"
	"testing"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/internal/corpus"
)

// version is the corpus folder that holds the sample replays.
const version = "v1.38.2"

// Names lists the sample replays: two games against the AI.
var Names = []string{"abyss-bot-278f.fwr", "abyss-bot-974f.fwr"}

// Probes lists the probe replays: short games that each isolate a
// few kinds of command (testdata/probes).
var Probes = []string{
	"probe-delete.fwr", "probe-node.fwr", "probe-doors.fwr", "probe-mine.fwr", "probe-ground.fwr",
	"probe-repair.fwr", "probe-dblclick.fwr", "probe-downgrade-overdrive.fwr", "probe-move-phantom.fwr",
}

// Path returns the path of one sample replay, skipping the test without
// the corpus (corpus.Require). root is the repository root relative to the
// calling test's package directory.
func Path(tb testing.TB, root, name string) string {
	tb.Helper()
	return corpus.Path(tb, root, version+"/"+name)
}

// Read returns the header and the decompressed blocks of one sample
// replay.
func Read(tb testing.TB, root, name string) (container.Header, [][]byte) {
	tb.Helper()
	f, err := os.Open(Path(tb, root, name))
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	r, err := container.NewReader(bufio.NewReader(f), container.Limits{})
	if err != nil {
		tb.Fatal(err)
	}
	var blocks [][]byte
	for r.NextBlock() {
		blocks = append(blocks, r.Block().Data)
	}
	if err := r.Err(); err != nil {
		tb.Fatal(err)
	}
	return r.Header(), blocks
}

// Blocks returns the decompressed blocks of one sample replay.
func Blocks(tb testing.TB, root, name string) [][]byte {
	tb.Helper()
	_, blocks := Read(tb, root, name)
	return blocks
}

// AllBlocks returns the blocks of every sample replay, file by file.
func AllBlocks(tb testing.TB, root string) [][]byte {
	tb.Helper()
	var all [][]byte
	for _, name := range Names {
		all = append(all, Blocks(tb, root, name)...)
	}
	return all
}
