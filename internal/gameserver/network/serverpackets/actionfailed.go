package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeActionFailed is the wire opcode for ActionFailed, a one-byte packet
// that tells the client the requested action did not start.
const OpcodeActionFailed = 0x25

// FrameActionFailed builds the static ActionFailed packet as an owned frame.
func FrameActionFailed() wire.Frame {
	w := newFrameWriter(OpcodeActionFailed)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
