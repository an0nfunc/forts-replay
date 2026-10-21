// SPDX-License-Identifier: AGPL-3.0-or-later

package container

// Limits bounds what a replay may make the reader allocate. Zero fields
// take the defaults.
type Limits struct {
	MaxHeader int   // header bytes before the first block; default 4 MiB, as multiplayer headers can embed images
	MaxBlock  int   // decompressed bytes per block; default 16 MiB
	MaxTotal  int64 // decompressed bytes per file; default 512 MiB
}

func (l Limits) withDefaults() Limits {
	if l.MaxHeader <= 0 {
		l.MaxHeader = 4 << 20
	}
	if l.MaxBlock <= 0 {
		l.MaxBlock = 16 << 20
	}
	if l.MaxTotal <= 0 {
		l.MaxTotal = 512 << 20
	}
	return l
}
