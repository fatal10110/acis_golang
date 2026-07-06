package serverpackets

// OpcodeCharDeleteFail is the wire opcode for CharDeleteFail, reporting why
// a character deletion request was rejected.
const OpcodeCharDeleteFail = 0x24

// CharDeleteFailReason is a client-facing character-deletion rejection
// reason. The values start at 1, matching the client contract; there is no
// 0 reason.
type CharDeleteFailReason int32

const (
	CharDeleteFailReasonDeletionFailed CharDeleteFailReason = iota + 1
	CharDeleteFailReasonClanMemberMayNotDelete
	CharDeleteFailReasonClanLeaderMayNotDelete
)

// EncodeCharDeleteFail builds the CharDeleteFail packet reporting reason.
func EncodeCharDeleteFail(reason CharDeleteFailReason) []byte {
	w := newWriter(OpcodeCharDeleteFail)
	w.WriteInt32(int32(reason))
	return w.Bytes()
}
