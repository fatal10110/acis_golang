package clientpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newReader wraps payload for decoding, discarding the leading opcode byte
// every login client packet carries.
func newReader(payload []byte) *wire.Reader {
	return wire.NewPacketReader(payload)
}
