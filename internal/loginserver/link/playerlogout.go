package link

import "fmt"

// OpcodePlayerLogout is the wire opcode for PlayerLogout, reporting an
// account that just logged out of this server.
const OpcodePlayerLogout = 0x03

// DecodePlayerLogout parses a raw PlayerLogout payload (opcode byte
// included) into the account that logged out.
func DecodePlayerLogout(payload []byte) (string, error) {
	r := newReader(payload)
	account := r.ReadString()
	if r.Err() != nil {
		return "", fmt.Errorf("link: PlayerLogout: %w", r.Err())
	}
	return account, nil
}
