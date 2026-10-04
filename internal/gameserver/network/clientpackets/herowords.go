package clientpackets

import "fmt"

// OpcodeRequestWriteHeroWords is the extended sub-opcode of
// RequestWriteHeroWords.
const OpcodeRequestWriteHeroWords uint16 = 0x000c

// RequestWriteHeroWords sets the requester's hero message.
type RequestWriteHeroWords struct {
	Message string
}

// DecodeRequestWriteHeroWords parses a raw RequestWriteHeroWords payload
// (extended opcode included).
func DecodeRequestWriteHeroWords(payload []byte) (RequestWriteHeroWords, error) {
	r, err := newExtendedReader(payload, "RequestWriteHeroWords", OpcodeRequestWriteHeroWords, 2)
	if err != nil {
		return RequestWriteHeroWords{}, err
	}
	req := RequestWriteHeroWords{Message: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestWriteHeroWords{}, fmt.Errorf("clientpackets: RequestWriteHeroWords: %w", err)
	}
	return req, nil
}
