package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeRequestRestartPoint is the wire opcode for RequestRestartPoint,
// sent by a dead client picking a restart option.
const OpcodeRequestRestartPoint = 0x6d

const requestRestartPointSize = 4

// RequestRestartPoint asks to revive and teleport a dead player to the
// location associated with the chosen restart type.
type RequestRestartPoint struct {
	RequestType int32
}

// DecodeRequestRestartPoint parses a raw RequestRestartPoint payload
// (opcode byte included).
func DecodeRequestRestartPoint(payload []byte) (RequestRestartPoint, error) {
	r := newReader(payload)
	if r.Remaining() < requestRestartPointSize {
		return RequestRestartPoint{}, fmt.Errorf("clientpackets: RequestRestartPoint: need %d bytes, got %d: %w", requestRestartPointSize, r.Remaining(), wire.ErrShortPacket)
	}
	return RequestRestartPoint{RequestType: r.ReadInt32()}, nil
}
