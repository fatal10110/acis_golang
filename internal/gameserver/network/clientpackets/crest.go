package clientpackets

import "fmt"

const (
	// OpcodeRequestPledgeCrest is the wire opcode for RequestPledgeCrest,
	// valid once a client is authenticated.
	OpcodeRequestPledgeCrest = 0x68
	// OpcodeRequestAllyCrest is the wire opcode for RequestAllyCrest, valid
	// once a client is in game.
	OpcodeRequestAllyCrest = 0x88
)

const requestCrestIDSize = 4

// RequestPledgeCrest asks the server to send the small pledge crest data for
// a crest id.
type RequestPledgeCrest struct {
	CrestID int32
}

// RequestAllyCrest asks the server to send the alliance crest data for a
// crest id.
type RequestAllyCrest struct {
	CrestID int32
}

// DecodeRequestPledgeCrest parses a raw RequestPledgeCrest payload (opcode
// byte included).
func DecodeRequestPledgeCrest(payload []byte) (RequestPledgeCrest, error) {
	r := newReader(payload)
	if r.Remaining() < requestCrestIDSize {
		return RequestPledgeCrest{}, fmt.Errorf("clientpackets: RequestPledgeCrest: need %d bytes, got %d", requestCrestIDSize, r.Remaining())
	}
	return RequestPledgeCrest{CrestID: r.ReadInt32()}, nil
}

// DecodeRequestAllyCrest parses a raw RequestAllyCrest payload (opcode byte
// included).
func DecodeRequestAllyCrest(payload []byte) (RequestAllyCrest, error) {
	r := newReader(payload)
	if r.Remaining() < requestCrestIDSize {
		return RequestAllyCrest{}, fmt.Errorf("clientpackets: RequestAllyCrest: need %d bytes, got %d", requestCrestIDSize, r.Remaining())
	}
	return RequestAllyCrest{CrestID: r.ReadInt32()}, nil
}
