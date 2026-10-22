// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"runtime/debug"
)

// modulePath is the module whose version fwr version prints.
const modulePath = "github.com/an0nfunc/forts-replay"

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: fwr version")
		return exitUsage
	}
	fmt.Fprintln(stdout, parserVersion())
	return exitOK
}

// parserVersion returns the version of the parser module this binary was
// built from.
func parserVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	return moduleVersion(bi)
}

// moduleVersion returns the version bi records for modulePath: the main
// module's, or that of its dependency entry, replaced or not. Without one,
// it is devel; when modulePath is the main module, the VCS revision follows,
// with +dirty for a modified tree. A dependency gets no revision, since
// bi's VCS settings describe the main module.
func moduleVersion(bi *debug.BuildInfo) string {
	known := func(v string) bool { return v != "" && v != "(devel)" }
	if bi.Main.Path != modulePath {
		for _, d := range bi.Deps {
			if d.Path != modulePath {
				continue
			}
			if d.Replace != nil {
				d = d.Replace
			}
			if known(d.Version) {
				return d.Version
			}
		}
		return "devel"
	}
	if known(bi.Main.Version) {
		return bi.Main.Version
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision":
			rev = " " + s.Value
		case s.Key == "vcs.modified" && s.Value == "true":
			dirty = "+dirty"
		}
	}
	return "devel" + rev + dirty
}
