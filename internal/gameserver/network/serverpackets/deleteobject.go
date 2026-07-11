package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeDeleteObject is the wire opcode for DeleteObject, which removes an
// object from the receiving client's screen.
const OpcodeDeleteObject = 0x12

// FrameDeleteObject builds the DeleteObject packet as an owned frame. It
// tells the client to stop rendering the object with objectID; seated
// selects the removal mode for a sitting character (0 = stand up before
// deleting, 1 = delete outright).
func FrameDeleteObject(objectID int32, seated bool) wire.Frame {
	w := newFrameWriter(OpcodeDeleteObject)
	w.WriteInt32(objectID)
	if seated {
		w.WriteInt32(0)
	} else {
		w.WriteInt32(1)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
