// SPDX-License-Identifier: AGPL-3.0-or-later

package corpus

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tree builds a corpus under a new repository root with one replay, a note
// and a link to a file outside the corpus, and returns the root.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	outside := filepath.Join(root, "outside.fwr")
	v := filepath.Join(root, Dir, "v1")
	for _, err := range []error{
		os.MkdirAll(v, 0o750),
		os.WriteFile(filepath.Join(root, Dir, "README.md"), nil, 0o600),
		os.WriteFile(filepath.Join(v, "a.fwr"), nil, 0o600),
		os.WriteFile(filepath.Join(v, "notes.md"), nil, 0o600),
		os.WriteFile(outside, nil, 0o600),
		os.Symlink(outside, filepath.Join(root, Dir, "v1", "link.fwr")),
		os.Symlink(v, filepath.Join(root, Dir, "v2")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWalk(t *testing.T) {
	root := tree(t)
	var seen []string
	err := Walk(root, func(_, rel string) error {
		seen = append(seen, rel)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "link.fwr: not a regular file") || !slices.Equal(seen, []string{"v1/a.fwr"}) {
		t.Errorf("seen %q, err %v", seen, err)
	}
	if err := os.Remove(filepath.Join(root, Dir, "v1", "link.fwr")); err != nil {
		t.Fatal(err)
	}
	if err := Walk(root, func(string, string) error { return nil }); err == nil || !strings.Contains(err.Error(), "v2: not a regular file") {
		t.Errorf("linked directory: %v", err)
	}
}

func TestCheckPath(t *testing.T) {
	dir := filepath.Join(tree(t), Dir)
	if err := checkPath(dir, "v1/a.fwr"); err != nil {
		t.Errorf("regular file: %v", err)
	}
	for _, rel := range []string{"v1/link.fwr", "v2/a.fwr", "v1", "v1/missing.fwr"} {
		if err := checkPath(dir, rel); err == nil {
			t.Errorf("%s: no error", rel)
		}
	}
}
