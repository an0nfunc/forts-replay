// SPDX-License-Identifier: AGPL-3.0-or-later

// Package corpus locates the replay corpus, the submodule testdata/corpus,
// for tests. It imports nothing from this module, so container's own tests
// can use it.
package corpus

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
// after Require. It fails the test when rel is not a regular file reached
// through directories only.
func Path(tb testing.TB, root, rel string) string {
	tb.Helper()
	Require(tb, root)
	dir := filepath.Join(root, Dir)
	if err := checkPath(dir, rel); err != nil {
		tb.Fatal(err)
	}
	return filepath.Join(dir, filepath.FromSlash(rel))
}

// Walk calls fn for every .fwr file of the corpus, in lexical order, with
// its path and its slash-separated path inside the corpus. root is the
// repository root, as for Require. Every entry must be a directory or a
// regular file.
func Walk(root string, fn func(path, rel string) error) error {
	dir := filepath.Join(root, Dir)
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil || d.IsDir():
			return err
		case !d.Type().IsRegular():
			return fmt.Errorf("%s: not a regular file", path)
		case filepath.Ext(path) != ".fwr":
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(rel))
	})
}

// checkPath returns an error unless every element of rel below dir is a
// directory and the last a regular file. The corpus takes pull requests, and
// a symlink in it would lead tests to any file on the machine.
func checkPath(dir, rel string) error {
	elems := strings.Split(rel, "/")
	p := dir
	for i, e := range elems {
		p = filepath.Join(p, e)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if last := i == len(elems)-1; last && !fi.Mode().IsRegular() || !last && !fi.IsDir() {
			return fmt.Errorf("%s: not a regular file or directory", p)
		}
	}
	return nil
}
