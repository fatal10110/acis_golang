package serverpackets

// OpcodeLoginFail is the wire opcode for LoginFail, sent when login
// authentication is rejected.
const OpcodeLoginFail = 0x01

// LoginFailReason is the reason code sent in a LoginFail packet.
type LoginFailReason int32

// LoginFail reasons, in the order the client's string table expects.
const (
	LoginFailSystemError       LoginFailReason = 0x01
	LoginFailPasswordWrong     LoginFailReason = 0x02
	LoginFailUserOrPassWrong   LoginFailReason = 0x03
	LoginFailAccessFailed      LoginFailReason = 0x04
	LoginFailAccountInUse      LoginFailReason = 0x07
	LoginFailServerOverloaded  LoginFailReason = 0x0f
	LoginFailServerMaintenance LoginFailReason = 0x10
	LoginFailTempPassExpired   LoginFailReason = 0x11
	LoginFailDualBox           LoginFailReason = 0x23
)

// EncodeLoginFail builds the LoginFail packet for reason.
func EncodeLoginFail(reason LoginFailReason) []byte {
	w := newWriter(OpcodeLoginFail)
	w.writeInt32(int32(reason))
	return w.bytes()
}
