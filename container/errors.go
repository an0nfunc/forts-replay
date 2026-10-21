// SPDX-License-Identifier: AGPL-3.0-or-later

// Package container reads the replay header and the LZMA-compressed blocks
// that follow it. It knows nothing about what the blocks contain.
package container

import (
	"errors"
	"fmt"
	"io"
)

var (
	// ErrTruncated reports that the input ended inside the header or a block.
	ErrTruncated = errors.New("fwr: truncated")
	// ErrCorrupt reports bytes that do not follow the replay format.
	ErrCorrupt = errors.New("fwr: corrupt")
	// ErrLimit reports a declared size above the configured Limits.
	ErrLimit = errors.New("fwr: limit exceeded")
)

func errAt(kind error, off int64, format string, args ...any) error {
	return fmt.Errorf("%w at offset %d: %s", kind, off, fmt.Sprintf(format, args...))
}

// readErr maps a short read to ErrTruncated and keeps other I/O errors.
func readErr(err error, off int64, what string) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errAt(ErrTruncated, off, "%s", what)
	}
	return fmt.Errorf("fwr: reading %s at offset %d: %w", what, off, err)
}
