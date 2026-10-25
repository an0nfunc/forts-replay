// SPDX-License-Identifier: AGPL-3.0-or-later

package match

import "fmt"

// ProvisionalPrefix starts the warning of a provisional decode, whether it
// passed or not.
const ProvisionalPrefix = "provisional: "

// Layout returns the layout to decode header version v with, and whether
// it is provisional: v itself when known, else the closest known version
// below v. A version below the oldest known one has no layout: the last
// result, ok, is then false.
func Layout(v uint32) (uint32, bool, bool) {
	for _, l := range [...]uint32{1390, 1383} { // the known layouts, newest first
		if l <= v {
			return l, l != v, true
		}
	}
	return 0, false, false
}

// layoutCheck returns the first check that b, a finished builder, fails, or
// "" when it passes them all. A game update can raise the header version
// without changing the format; these checks tell that apart from a layout
// that only seems to decode:
//   - the header decodes;
//   - each player has a SteamID when there are two or more, and the
//     commander names decode;
//   - the first sync is at frame 2, each tick advances to the next sync's
//     frame plus 2, the last sync is at the header's frame count rounded
//     down to even unless decoding stopped early, and slot 16 of each sync
//     is 0;
//   - no record decoded as Invalid.
func (b *builder) layoutCheck() string {
	h := b.hdr
	if h == nil {
		return b.hdrErr.Error()
	}
	if len(h.Players) >= 2 {
		for _, p := range h.Players {
			if p.SteamID == 0 {
				return fmt.Sprintf("player %d has no SteamID", p.ID)
			}
		}
	}
	frames := b.sum.Frames
	switch {
	case h.Commanders == nil:
		return "commander names do not decode"
	case b.syncs == 0:
		return "no sync decoded"
	case b.firstSync != 2:
		return fmt.Sprintf("first sync is frame %d", b.firstSync)
	case b.stepErr != "":
		return b.stepErr
	case !b.sum.Truncated && b.lastSync != frames&^1:
		return fmt.Sprintf("header has %d frames, last sync is frame %d", frames, b.lastSync)
	case b.invalid > 0:
		return fmt.Sprintf("records decoded as invalid: %d", b.invalid)
	}
	return ""
}
