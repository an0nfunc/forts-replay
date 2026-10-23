// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"errors"
	"fmt"
)

// Records the game writes itself, with player 0.
const (
	TypeResult     byte = 0xbb
	TypeScreenshot byte = 0xe2

	screenshotHead    = 45
	maxScreenshotSide = 1 << 16 // pixels; 1024 × 576 or 1024 × 640 in every sample
)

// Result is the result of a ranked match, written when it ends. A replay
// can also hold the other client's record.
type Result struct {
	Input

	Winner  uint32 `json:"winner"` // winning side
	Team    int32  `json:"team"`   // the recording client's team
	SteamID uint64 `json:"-"`      // the recording client's SteamID; kept out of dump output
}

// MsgType returns TypeResult.
func (*Result) MsgType() byte { return TypeResult }

// Screenshot is a capture of the game screen, candidate preview for seeking
// in the replay viewer. The pixels follow the fields and are not kept.
type Screenshot struct {
	Input

	Width  uint32 `json:"width"`
	Height uint32 `json:"height"`
	Depth  uint32 `json:"depth"`  // bytes per pixel: 4 in every sample, 3 accepted
	Format uint32 `json:"format"` // OpenGL pixel format: 0x1908 RGBA, 0x80e1 BGRA
	Frame  uint32 `json:"frame"`  // candidate: the record's frame plus 1
	Flag   bool   `json:"flag"`   // u8@44, 0 or 1; meaning unknown
}

// MsgType returns TypeScreenshot.
func (*Screenshot) MsgType() byte { return TypeScreenshot }

func decodeScreenshot(p []byte) (Message, error) {
	if len(p) < screenshotHead {
		return nil, fmt.Errorf("length %d, want at least %d", len(p), screenshotHead)
	}
	if p[17] != p[0] {
		return nil, fmt.Errorf("byte 17 is %#02x, want %#02x", p[17], p[0])
	}
	b := &body{p: p}
	s := &Screenshot{
		Player: b.u32(20), Width: b.u32(24), Height: b.u32(28),
		Depth: b.u32(32), Format: b.u32(36), Frame: b.u32(40), Flag: b.flag(44),
	}
	if b.err != nil {
		return nil, b.err
	}
	if s.Depth != 3 && s.Depth != 4 {
		return nil, fmt.Errorf("%d bytes per pixel", s.Depth)
	}
	// Within the side limit the product is at most 2^34, so int64 holds it.
	if s.Width > maxScreenshotSide || s.Height > maxScreenshotSide {
		return nil, fmt.Errorf("%d × %d pixels", s.Width, s.Height)
	}
	pixels := len(p) - screenshotHead
	if int64(s.Width)*int64(s.Height)*int64(s.Depth) != int64(pixels) {
		return nil, fmt.Errorf("%d bytes of pixels for %d × %d × %d", pixels, s.Width, s.Height, s.Depth)
	}
	return s, nil
}

// registerGame adds the decoders of the game's own records for version v.
func registerGame(r *Registry, v uint32) {
	register(r, v, TypeResult, inputDecoder(85, func(in Input, b *body) Message {
		id := b.u64(32)
		if b.u64(40) != id && b.err == nil {
			b.err = errors.New("the SteamIDs at offsets 32 and 40 differ")
		}
		return &Result{Input: in, Winner: b.u32(24), Team: b.i32(28), SteamID: id}
	}))
	register(r, v, TypeScreenshot, decodeScreenshot)
}
