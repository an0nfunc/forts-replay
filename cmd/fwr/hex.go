// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
)

func runHex(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hex", flag.ContinueOnError)
	fs.SetOutput(stderr)
	typ := fs.String("type", "", "message type, e.g. 0x95 (required)")
	maxBytes := fs.Int("max-bytes", 64, "bytes per row")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	t, err := strconv.ParseUint(*typ, 0, 8)
	if err != nil || fs.NArg() != 1 || *maxBytes < 1 {
		fmt.Fprintln(stderr, "usage: fwr hex --type T [--max-bytes N] FILE")
		return exitUsage
	}
	type row struct {
		label string
		data  []byte
	}
	var rows []row
	collect := func(it item) error {
		if uint64(it.Record.Type) == t {
			p := it.Record.Payload
			// Clone, so a matching row does not keep its whole block alive.
			rows = append(rows, row{fmt.Sprintf("b%d+0x%05x", it.Block, it.Record.Offset), bytes.Clone(p[:min(len(p), *maxBytes)])})
		}
		return nil
	}
	res, err := walk(ctx, fs.Arg(0), handlers{msg: collect})
	if err != nil {
		fmt.Fprintf(stderr, "fwr hex: %v\n", err)
		return exitFail
	}
	if res.Stopped != nil {
		fmt.Fprintf(stderr, "fwr hex: warning: decoding stopped early: %s\n", printable(res.Stopped.Error()))
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r.data))
	}
	w := bufio.NewWriter(stdout)
	const labelWidth = 14
	fmt.Fprintf(w, "%-*s", labelWidth, "")
	for i := range width {
		fmt.Fprintf(w, " %02x", i)
	}
	fmt.Fprintln(w)
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s", labelWidth, r.label)
		for _, c := range r.data {
			fmt.Fprintf(w, " %02x", c)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%-*s", labelWidth, "const")
	for i := range width {
		same := len(rows) > 0
		for _, r := range rows {
			if i >= len(r.data) || r.data[i] != rows[0].data[i] {
				same = false
				break
			}
		}
		if same {
			fmt.Fprint(w, " ==")
		} else {
			fmt.Fprint(w, " ..")
		}
	}
	fmt.Fprintln(w)
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "fwr hex: %v\n", err)
		return exitFail
	}
	return exitOK
}
