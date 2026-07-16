package clientpackets

import "fmt"

// OpcodeRequestPledgeCrest is the wire opcode for RequestPledgeCrest, valid
// once a client is authenticated.
const OpcodeRequestPledgeCrest = 0x68

const requestPledgeCrestSize = 4

// RequestPledgeCrest asks the server to send the small pledge crest data for
// a crest id.
type RequestPledgeCrest struct {
	CrestID int32
}

// DecodeRequestPledgeCrest parses a raw RequestPledgeCrest payload (opcode
// byte included).
func DecodeRequestPledgeCrest(payload []byte) (RequestPledgeCrest, error) {
	r := newReader(payload)
	if r.Remaining() < requestPledgeCrestSize {
		return RequestPledgeCrest{}, fmt.Errorf("clientpackets: RequestPledgeCrest: need %d bytes, got %d", requestPledgeCrestSize, r.Remaining())
	}
	return RequestPledgeCrest{CrestID: r.ReadInt32()}, nil
}
