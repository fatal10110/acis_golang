package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeCharCreateFail is the wire opcode for CharCreateFail, reporting why
// a character creation attempt was rejected.
const OpcodeCharCreateFail = 0x1a

// CharCreateFailReason is a client-facing character-creation rejection
// reason.
type CharCreateFailReason int32

const (
	CharCreateFailReasonCreationFailed CharCreateFailReason = iota
	CharCreateFailReasonTooManyCharacters
	CharCreateFailReasonNameAlreadyExists
	CharCreateFailReason16EngChars
	CharCreateFailReasonIncorrectName
	CharCreateFailReasonCreateNotAllowed
	CharCreateFailReasonChooseAnotherServer
)

// FrameCharCreateFail builds the CharCreateFail packet as an owned frame.
func FrameCharCreateFail(reason CharCreateFailReason) wire.Frame {
	w := newFrameWriter(OpcodeCharCreateFail)
	w.WriteInt32(int32(reason))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
