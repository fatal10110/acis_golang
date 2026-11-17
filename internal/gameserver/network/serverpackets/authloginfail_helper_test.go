package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// EncodeAuthLoginFail builds the AuthLoginFail packet for reason as a plain,
// unpooled byte slice.
func EncodeAuthLoginFail(reason LoginFailReason) []byte {
	w := wire.NewPacketWriter(OpcodeAuthLoginFail)
	writeAuthLoginFail(w, reason)
	return w.Bytes()
}
