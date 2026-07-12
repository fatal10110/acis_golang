package clientpackets

import "fmt"

// OpcodeProtocolVersion is the wire opcode for SendProtocolVersion, valid
// right after a client connects, before authentication.
const OpcodeProtocolVersion = 0x00

const protocolVersionSize = 4

// ProtocolVersion is the client's reported protocol revision.
type ProtocolVersion struct {
	Revision int32
}

// DecodeProtocolVersion parses a raw SendProtocolVersion payload (opcode
// byte included).
func DecodeProtocolVersion(payload []byte) (ProtocolVersion, error) {
	r := newReader(payload)
	if r.Remaining() < protocolVersionSize {
		return ProtocolVersion{}, fmt.Errorf("clientpackets: ProtocolVersion: need %d bytes, got %d", protocolVersionSize, r.Remaining())
	}
	return ProtocolVersion{Revision: r.ReadInt32()}, nil
}
