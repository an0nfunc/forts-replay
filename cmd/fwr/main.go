// SPDX-License-Identifier: AGPL-3.0-or-later

// Command fwr inspects Forts replay files.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	// dumpSchemaVersion 0 marks the dump output as unstable; info --json has schema version 1 (match.SchemaVersion).
	dumpSchemaVersion = 0
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "info":
		return runInfo(ctx, args[1:], stdout, stderr)
	case "dump":
		return runDump(ctx, args[1:], stdout, stderr)
	case "hex":
		return runHex(ctx, args[1:], stdout, stderr)
	case "extract":
		return runExtract(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	}
	fmt.Fprintf(stderr, "fwr: unknown command %q\n", args[0])
	usage(stderr)
	return exitUsage
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage: fwr <command> [flags] FILE

commands:
  info      summary: header fields, players, result, message counts, commands and build orders (--json)
  dump      list every message (--json, --type, --block, --lua)
  hex       align the payloads of one message type (--type, --max-bytes)
  extract   write the header and decompressed blocks to a directory (-o)
`)
}
