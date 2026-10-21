// SPDX-License-Identifier: AGPL-3.0-or-later

// Package msg turns records into typed messages through a registry of
// decoders keyed by message type and game version.
package msg

// Message is a decoded record.
type Message interface {
	MsgType() byte
}

// Raw is a record no registered decoder claims. Payload includes the type byte.
type Raw struct {
	Type    byte
	Payload []byte
}

// MsgType returns the record's type.
func (m *Raw) MsgType() byte { return m.Type }

// Invalid is a record whose decoder failed.
type Invalid struct {
	Raw Raw
	Err error
}

// MsgType returns the record's type.
func (m *Invalid) MsgType() byte { return m.Raw.Type }

// FrameAdvancer is implemented by messages that move game time. The
// decoder passes the current frame (-1 while unknown) and keeps the result.
type FrameAdvancer interface {
	NextFrame(cur int64) int64
}
