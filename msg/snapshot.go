// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

// TypeSnapshot is the message type of the state snapshot that starts every block.
const TypeSnapshot byte = 0xe1

// Snapshot is a state snapshot. Package snapshot reads its Lua sections.
type Snapshot struct {
	Payload []byte
}

// MsgType returns TypeSnapshot.
func (*Snapshot) MsgType() byte { return TypeSnapshot }

func decodeSnapshot(p []byte) (Message, error) { return &Snapshot{Payload: p}, nil }
