package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeVersionCheck is the wire opcode for VersionCheck, the first packet
// this server sends after a client connects: cleartext, since it carries
// the connection's XOR cipher key itself, arming encryption for every
// packet after it in both directions.
const OpcodeVersionCheck = 0x00

// versionCheckKeySize is the random half of the XOR cipher key VersionCheck
// carries. The client appends the fixed static half itself.
const versionCheckKeySize = 8

// FrameVersionCheck builds the VersionCheck packet as an owned frame,
// carrying the random half of key to the client. key must contain at least
// versionCheckKeySize bytes.
func FrameVersionCheck(key []byte) wire.Frame {
	w := newFrameWriter(OpcodeVersionCheck)
	w.WriteUint8(0x01)
	w.WriteBytes(key[:versionCheckKeySize])
	w.WriteInt32(1)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
