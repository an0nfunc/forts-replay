// SPDX-License-Identifier: AGPL-3.0-or-later

// Package corpus locates the replay corpus, the submodule testdata/corpus,
// for tests. It imports nothing from this module, so container's own tests
// can use it.
package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

// Dir is the corpus checkout, relative to the repository root.
const Dir = "testdata/corpus"

// RequireEnv names the variable that, when set, makes a missing corpus fail
// tests instead of skipping them. CI sets it.
const RequireEnv = "FWR_REQUIRE_CORPUS"

// Require skips the test when the corpus is not checked out, or fails it
// when RequireEnv is set. root is the repository root relative to the
// calling test's package directory.
func Require(tb testing.TB, root string) {
	tb.Helper()
	if _, err := os.Stat(filepath.Join(root, Dir, "README.md")); err == nil {
		return
	}
	if os.Getenv(RequireEnv) != "" {
		tb.Fatal("testdata/corpus is not checked out")
	}
	tb.Skip("testdata/corpus is not checked out: git submodule update --init")
}

// Path returns the path of rel, a slash-separated path inside the corpus,
// after Require.
func Path(tb testing.TB, root, rel string) string {
	tb.Helper()
	Require(tb, root)
	return filepath.Join(root, Dir, filepath.FromSlash(rel))
}
