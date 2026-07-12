package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeVersionCheck is the wire opcode for VersionCheck, the first packet
// this server sends after a client connects: cleartext, since it carries
// the connection's XOR cipher key itself, arming encryption for every
// packet after it in both directions.
const OpcodeVersionCheck = 0x00

// versionCheckKeySize is the length of the XOR cipher key VersionCheck
// carries; must match network.Cipher's key size.
const versionCheckKeySize = 16

// FrameVersionCheck builds the VersionCheck packet as an owned frame,
// carrying key (the connection's cipher key) to the client. key must be
// exactly versionCheckKeySize (16) bytes.
func FrameVersionCheck(key []byte) wire.Frame {
	w := newFrameWriter(OpcodeVersionCheck)
	w.WriteUint8(0x01)
	w.WriteBytes(key)
	w.WriteInt32(0) // Blowfish-over-XOR wrapper: not modeled, always off
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
