package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Static system message ids used by live summon command feedback.
const (
	SystemMessagePetCannotSentBackDuringBattle = 579
	SystemMessageDeadPetCannotBeReturned       = 589
	SystemMessageYouCannotRestoreHungryPets    = 594
	SystemMessagePetRefusingOrder              = 1864
	SystemMessagePetTooHighToControl           = 1918
)

// OpcodeSystemMessage is the wire opcode for a system message.
const OpcodeSystemMessage = 0x64

// FrameSystemMessage builds a static no-parameter SystemMessage packet.
func FrameSystemMessage(id int) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
