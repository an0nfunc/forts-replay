// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/an0nfunc/forts-replay/container"
	"github.com/an0nfunc/forts-replay/match"
	"github.com/an0nfunc/forts-replay/msg"
	"github.com/an0nfunc/forts-replay/stream"
)

type item struct {
	Block  int
	Record stream.Record
	Frame  int64
	Msg    msg.Message
}

// handlers receive the replay as it decodes. Records are only decoded when
// msg is set. An error from a handler stops the walk.
type handlers struct {
	header func(container.Header) error
	block  func(container.Block) error
	msg    func(item) error
}

type walkResult struct {
	Blocks  int
	Records int
	Stopped error  // decoding ended early after at least one block
	Note    string // set when records decode with a provisional layout
}

// decodeVersion returns the version to decode the records of header
// version v as, which is v unless match.Layout picks a provisional layout,
// and the note for that layout.
func decodeVersion(v uint32) (uint32, string) {
	layout, provisional, _ := match.Layout(v)
	if !provisional {
		return v, ""
	}
	return layout, fmt.Sprintf("%sheader version %d is not known to this parser; decoded with the %d layout", match.ProvisionalPrefix, v, layout)
}

// walk returns an error when nothing could be decoded, a handler failed or
// ctx ended. Damage after the first block is reported in Stopped.
func walk(ctx context.Context, path string, h handlers) (walkResult, error) {
	var res walkResult
	f, err := os.Open(path)
	if err != nil {
		return res, err
	}
	defer f.Close()
	cr, err := container.NewReader(bufio.NewReader(f), container.Limits{})
	if err != nil {
		return res, err
	}
	hdr := cr.Header()
	version, note := decodeVersion(hdr.Version)
	res.Note = note
	if h.header != nil {
		if err := h.header(hdr); err != nil {
			return res, err
		}
	}
	var dec *msg.Decoder
	for cr.NextBlock() {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		b := cr.Block()
		res.Blocks++
		if h.block != nil {
			if err := h.block(b); err != nil {
				return res, err
			}
		}
		if h.msg == nil {
			continue
		}
		s := stream.New(b.Data)
		if dec == nil {
			dec = msg.NewDecoder(s, msg.DefaultRegistry(), version, msg.Options{})
		} else {
			dec.Reset(s)
		}
		for dec.Next() {
			res.Records++
			if err := h.msg(item{Block: b.Index, Record: dec.Record(), Frame: dec.Frame(), Msg: dec.Message()}); err != nil {
				return res, err
			}
		}
		if err := dec.Err(); err != nil {
			res.Stopped = fmt.Errorf("block %d: %w", b.Index, err)
			return res, nil
		}
	}
	if err := cr.Err(); err != nil {
		if res.Blocks == 0 {
			return res, err
		}
		res.Stopped = err
	}
	return res, nil
}
