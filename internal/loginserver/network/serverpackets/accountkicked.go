package serverpackets

// OpcodeAccountKicked is the wire opcode for AccountKicked, sent to
// force-disconnect a client already logged in elsewhere or under sanction.
const OpcodeAccountKicked = 0x02

// AccountKickedReason is the reason code sent in an AccountKicked packet.
type AccountKickedReason int32

// AccountKicked reasons.
const (
	AccountKickedDataStealer        AccountKickedReason = 0x01
	AccountKickedGenericViolation   AccountKickedReason = 0x08
	AccountKickedSevenDaysSuspended AccountKickedReason = 0x10
	AccountKickedPermanentlyBanned  AccountKickedReason = 0x20
)

// EncodeAccountKicked builds the AccountKicked packet for reason.
func EncodeAccountKicked(reason AccountKickedReason) []byte {
	w := newWriter(OpcodeAccountKicked)
	w.WriteInt32(int32(reason))
	return w.Bytes()
}
