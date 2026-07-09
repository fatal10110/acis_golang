package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeAuthLoginFail is the wire opcode for AuthLoginFail, sent to refuse
// a client's presented session keys; the connection is closed right after.
const OpcodeAuthLoginFail = 0x14

// LoginFailReason is the reason code sent in an AuthLoginFail packet.
type LoginFailReason int32

// LoginFailSystemErrorTryLater is sent when the login server rejects a
// client's presented session keys as invalid.
const LoginFailSystemErrorTryLater LoginFailReason = 0x01

// EncodeAuthLoginFail builds the AuthLoginFail packet for reason.
func EncodeAuthLoginFail(reason LoginFailReason) []byte {
	w := newWriter(OpcodeAuthLoginFail)
	writeAuthLoginFail(w, reason)
	return w.Bytes()
}

// FrameAuthLoginFail builds the AuthLoginFail packet as an owned frame.
func FrameAuthLoginFail(reason LoginFailReason) wire.Frame {
	w := newFrameWriter(OpcodeAuthLoginFail)
	writeAuthLoginFail(w, reason)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeAuthLoginFail(w *wire.Writer, reason LoginFailReason) {
	w.WriteInt32(int32(reason))
}
