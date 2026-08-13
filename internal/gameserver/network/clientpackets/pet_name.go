package clientpackets

import "fmt"

// RequestChangePetName asks the server to assign an active pet's custom
// name.
type RequestChangePetName struct {
	Name string
}

// DecodeRequestChangePetName parses a raw RequestChangePetName payload
// (opcode byte included).
func DecodeRequestChangePetName(payload []byte) (RequestChangePetName, error) {
	r := newReader(payload)
	req := RequestChangePetName{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestChangePetName{}, fmt.Errorf("clientpackets: RequestChangePetName: %w", err)
	}
	return req, nil
}
