package clientpackets

import "fmt"

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
		return RequestRestartPoint{}, fmt.Errorf("clientpackets: RequestRestartPoint: need %d bytes, got %d", requestRestartPointSize, r.Remaining())
	}
	return RequestRestartPoint{RequestType: r.ReadInt32()}, nil
}
