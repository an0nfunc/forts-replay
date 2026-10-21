// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fixture loads the committed sample replays for tests.
package fixture

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/an0nfunc/forts-replay/container"
)

// Dir holds the sample replays, relative to the repository root.
const Dir = "testdata/fixtures/v1.38.2"

// Names lists the sample replays in Dir.
var Names = []string{"abyss-bot-278f.fwr", "abyss-bot-974f.fwr"}

// Probes lists the probe replays in Dir: short games that each isolate a
// few kinds of command (testdata/probes).
var Probes = []string{
	"probe-delete.fwr", "probe-node.fwr", "probe-doors.fwr", "probe-mine.fwr", "probe-ground.fwr",
	"probe-repair.fwr", "probe-dblclick.fwr", "probe-downgrade-overdrive.fwr", "probe-move-phantom.fwr",
}

// Read returns the header and the decompressed blocks of one sample
// replay. root is the repository root relative to the calling test's
// package directory.
func Read(tb testing.TB, root, name string) (container.Header, [][]byte) {
	tb.Helper()
	f, err := os.Open(filepath.Join(root, Dir, name))
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
