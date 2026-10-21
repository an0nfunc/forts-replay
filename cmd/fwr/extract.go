// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/an0nfunc/forts-replay/container"
)

func runExtract(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("o", "", "output directory (required)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *dir == "" || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: fwr extract -o DIR FILE")
		return exitUsage
	}
	if err := os.MkdirAll(*dir, 0o750); err != nil {
		fmt.Fprintf(stderr, "fwr extract: %v\n", err)
		return exitFail
	}
	// O_EXCL: never overwrite, and never follow a symlink in the output directory.
	write := func(name string, data []byte) error {
		f, err := os.OpenFile(filepath.Join(*dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	res, err := walk(ctx, fs.Arg(0), handlers{
		header: func(h container.Header) error { return write("header.bin", h.Raw) },
		block:  func(b container.Block) error { return write(fmt.Sprintf("block%03d.bin", b.Index), b.Data) },
	})
	if err != nil {
		fmt.Fprintf(stderr, "fwr extract: %v\n", err)
		return exitFail
	}
	if res.Stopped != nil {
		fmt.Fprintf(stderr, "fwr extract: warning: decoding stopped early: %s\n", printable(res.Stopped.Error()))
	}
	fmt.Fprintf(stdout, "wrote %d blocks to %s\n", res.Blocks, *dir)
	return exitOK
}
