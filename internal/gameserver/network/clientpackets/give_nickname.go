package clientpackets

import "fmt"

// OpcodeRequestGiveNickName is the wire opcode for RequestGiveNickName.
const OpcodeRequestGiveNickName = 0x55

// RequestGiveNickName gives the player named Name the title Title: a noble
// may title itself, a clan member with the title privilege any member of
// its clan.
type RequestGiveNickName struct {
	Name  string
	Title string
}

// DecodeRequestGiveNickName parses a raw RequestGiveNickName payload
// (opcode byte included).
func DecodeRequestGiveNickName(payload []byte) (RequestGiveNickName, error) {
	r := newReader(payload)
	req := RequestGiveNickName{Name: r.ReadString(), Title: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestGiveNickName{}, fmt.Errorf("clientpackets: RequestGiveNickName: %w", err)
	}
	return req, nil
}
