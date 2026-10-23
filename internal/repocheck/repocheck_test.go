// SPDX-License-Identifier: AGPL-3.0-or-later

package repocheck

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec" //nolint:depguard // the check lists tracked files with git; it runs in tests only
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestTrackedFiles fails on every individual SteamID64 in a file git tracks
// that steamids.allow does not list. Without git or a checkout it skips,
// unless CI is set.
func TestTrackedFiles(t *testing.T) {
	allowed := allowlist(t)
	files, err := tracked(t.Context(), "../..")
	if err != nil {
		skipUnlessCI(t, err)
	}
	if len(files) == 0 {
		t.Fatal("git tracks no files")
	}
	for _, f := range check(t, "../..", files, allowed) {
		t.Errorf("%q holds SteamID64 %d, not in steamids.allow", f.name, f.id)
	}
}

// TestTrackedFilesOddNames runs the check on a scratch repository whose
// file names hold colons, spaces, $(...) and a line break, with a gitlink
// among its entries.
func TestTrackedFilesOddNames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		skipUnlessCI(t, err)
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // G204: arguments this test builds
			t.Fatalf("git %q: %v: %s", args, err, out)
		}
	}
	git("init", "-q")
	bad := uint64(steamIDHigh)<<32 | 0xfffffffd
	files := map[string]string{
		"a:b.txt":                   strconv.FormatUint(bad, 10),
		"p:q:0x1+found[$(touch X)]": fmt.Sprintf("0x%x", bad),
		"sp ace $(touch X).txt":     "no SteamID",
		"new\nline.txt":             "",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", name)
	}
	git("update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("1", 40)+",sub")
	names, err := tracked(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := slices.Sorted(maps.Keys(files)); !slices.Equal(names, want) {
		t.Fatalf("tracked %q, want %q", names, want)
	}
	want := []finding{{"a:b.txt", bad}, {"p:q:0x1+found[$(touch X)]", bad}}
	if got := check(t, dir, names, allowlist(t)); !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "X")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file name ran as code: %v", err)
	}
}

// tracked returns the paths of the files git tracks under root, the
// gitlinks of submodules left out.
func tracked(ctx context.Context, root string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--stage").Output() //nolint:gosec // G204: root is a test's own directory
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var names []string
	for entry := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		meta, name, ok := strings.Cut(entry, "\t")
		if !ok {
			return nil, fmt.Errorf("git ls-files: entry %q", entry)
		}
		if !strings.HasPrefix(meta, "160000 ") {
			names = append(names, name)
		}
	}
	return slices.Compact(names), nil // a conflicted file has an entry per stage
}

// finding is a SteamID64 in a tracked file that steamids.allow lacks.
type finding struct {
	name string
	id   uint64
}

// check reads each file under root, a symbolic link as its target's name,
// and returns the SteamID64s that allowed lacks. A file it cannot read fails
// the test.
func check(t *testing.T, root string, files []string, allowed map[uint64]bool) []finding {
	t.Helper()
	var found []finding
	for _, name := range files {
		data, err := contents(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		for _, id := range SteamIDs(data) {
			if !allowed[id] {
				found = append(found, finding{name, id})
			}
		}
	}
	return found
}

// contents returns the bytes of the file at path, or of the target's name
// when path is a symbolic link.
func contents(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return os.ReadFile(path)
	}
	target, err := os.Readlink(path)
	return []byte(target), err
}

// skipUnlessCI skips the test on err, or fails it when CI is set.
func skipUnlessCI(t *testing.T, err error) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatal(err)
	}
	t.Skip(err)
}

// allowlist reads steamids.allow: a SteamID64 in decimal at the start of
// each line, then what it is; # starts a comment line.
func allowlist(t *testing.T) map[uint64]bool {
	t.Helper()
	data, err := os.ReadFile("steamids.allow")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[uint64]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, _, _ := strings.Cut(line, " ")
		id, err := strconv.ParseUint(field, 10, 64)
		if err != nil || id>>32 != steamIDHigh {
			t.Fatalf("steamids.allow: %q is not an individual SteamID64", line)
		}
		allowed[id] = true
	}
	return allowed
}
