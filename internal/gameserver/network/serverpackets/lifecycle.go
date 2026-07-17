package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeRevive is the wire opcode for Revive.
	OpcodeRevive = 0x07
	// OpcodeRestartResponse is the wire opcode for RestartResponse.
	OpcodeRestartResponse = 0x5f
	// OpcodeLeaveWorld is the wire opcode for LeaveWorld.
	OpcodeLeaveWorld = 0x7e
)

// FrameRevive builds the object revive packet.
func FrameRevive(objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeRevive)
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

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
