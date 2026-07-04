package serverpackets

// OpcodePlayFail is the wire opcode for PlayFail, sent when a client may not
// play on the chosen game server.
const OpcodePlayFail = 0x06

// PlayFailReason is the reason code sent in a PlayFail packet.
type PlayFailReason byte

// PlayFail reasons. Reason3 and Reason4 carry no more specific meaning in
// the source than their numeric code.
const (
	PlayFailSystemError     PlayFailReason = 0x01
	PlayFailUserOrPassWrong PlayFailReason = 0x02
	PlayFailReason3         PlayFailReason = 0x03
	PlayFailReason4         PlayFailReason = 0x04
	PlayFailTooManyPlayers  PlayFailReason = 0x0f
)

// EncodePlayFail builds the PlayFail packet for reason.
func EncodePlayFail(reason PlayFailReason) []byte {
	w := newWriter(OpcodePlayFail)
	w.writeByte(byte(reason))
	return w.bytes()
}
