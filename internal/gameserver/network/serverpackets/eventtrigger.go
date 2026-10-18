package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeEventTrigger is the wire opcode for EventTrigger, which turns a
// client-side map effect on or off.
const OpcodeEventTrigger = 0xcf

// FrameEventTrigger builds the EventTrigger packet for the map effect
// eventID as an owned frame.
func FrameEventTrigger(eventID int32, active bool) wire.Frame {
	w := newFrameWriter(OpcodeEventTrigger)
	w.WriteInt32(eventID)
	var on byte
	if active {
		on = 1
	}
	w.WriteUint8(on)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
