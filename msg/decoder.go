// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"errors"
	"fmt"

	"github.com/an0nfunc/forts-replay/stream"
)

// ErrStrict reports a Raw or Invalid message while Options.Strict is set.
var ErrStrict = errors.New("msg: undecoded message in strict mode")

// Options configure a Decoder.
type Options struct {
	Strict bool
}

// Decoder decodes the records of a stream. One decoder serves a whole
// replay: Reset moves it to the next block and keeps the frame.
type Decoder struct {
	s       *stream.Scanner
	reg     *Registry
	version uint32
	opts    Options
	rec     stream.Record
	msg     Message
	frame   int64
	err     error
}

// NewDecoder returns a Decoder for the records of s that uses the decoders
// reg holds for version. Its frame starts unknown.
func NewDecoder(s *stream.Scanner, reg *Registry, version uint32, opts Options) *Decoder {
	return &Decoder{s: s, reg: reg, version: version, opts: opts, frame: -1}
}

// Reset continues with the next block's records. Frame and any earlier
// error are kept.
func (d *Decoder) Reset(s *stream.Scanner) {
	d.s = s
	d.msg = nil
}

// Message returns the message decoded by the last successful Next.
func (d *Decoder) Message() Message { return d.msg }

// Record returns the record of the current message.
func (d *Decoder) Record() stream.Record { return d.rec }

// Frame is the game frame after the current message, or -1 while unknown.
func (d *Decoder) Frame() int64 { return d.frame }

// Err returns the error that stopped decoding, or nil.
func (d *Decoder) Err() error { return d.err }

// Next decodes the next record and reports whether it succeeded.
func (d *Decoder) Next() bool {
	if d.err != nil {
		return false
	}
	if !d.s.Next() {
		d.err = d.s.Err()
		return false
	}
	d.rec = d.s.Record()
	m := d.decode(d.rec)
	if d.opts.Strict {
		switch m.(type) {
		case *Raw, *Invalid:
			d.err = fmt.Errorf("%w: type %#02x at block offset %d", ErrStrict, d.rec.Type, d.rec.Offset)
			return false
		}
	}
	if fa, ok := m.(FrameAdvancer); ok {
		d.frame = fa.NextFrame(d.frame)
	}
	d.msg = m
	return true
}

func (d *Decoder) decode(rec stream.Record) Message {
	raw := Raw{Type: rec.Type, Payload: rec.Payload}
	fn := d.reg.Lookup(rec.Type, d.version)
	if fn == nil {
		return &raw
	}
	m, err := fn(rec.Payload)
	switch {
	case err != nil:
		return &Invalid{Raw: raw, Err: err}
	case m == nil:
		return &Invalid{Raw: raw, Err: errors.New("decoder returned no message")}
	}
	return m
}
