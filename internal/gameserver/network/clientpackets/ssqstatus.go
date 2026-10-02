package clientpackets

import "fmt"

// OpcodeRequestSSQStatus asks for one page of the Record of Seven Signs.
const OpcodeRequestSSQStatus = 0xc7

// RequestSSQStatus is the Record of Seven Signs page the client asks for.
type RequestSSQStatus struct {
	Page byte
}

// DecodeRequestSSQStatus parses a raw RequestSSQStatus payload (opcode byte
// included).
func DecodeRequestSSQStatus(payload []byte) (RequestSSQStatus, error) {
	r := newReader(payload)
	req := RequestSSQStatus{Page: r.ReadUint8()}
	if err := r.Err(); err != nil {
		return RequestSSQStatus{}, fmt.Errorf("clientpackets: RequestSSQStatus: %w", err)
	}
	return req, nil
}
