package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeRestartResponse is the wire opcode for RestartResponse.
	OpcodeRestartResponse = 0x5f
	// OpcodeLeaveWorld is the wire opcode for LeaveWorld.
	OpcodeLeaveWorld = 0x7e
)

// FrameRestartResponse builds a RestartResponse packet as an owned frame.
func FrameRestartResponse(ok bool) wire.Frame {
	w := newFrameWriter(OpcodeRestartResponse)
	if ok {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameLeaveWorld builds the static LeaveWorld packet as an owned frame.
func FrameLeaveWorld() wire.Frame {
	w := newFrameWriter(OpcodeLeaveWorld)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
