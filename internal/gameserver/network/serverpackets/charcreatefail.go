package serverpackets

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

// EncodeCharCreateFail builds the CharCreateFail packet reporting reason.
func EncodeCharCreateFail(reason CharCreateFailReason) []byte {
	w := newWriter(OpcodeCharCreateFail)
	w.WriteInt32(int32(reason))
	return w.Bytes()
}
