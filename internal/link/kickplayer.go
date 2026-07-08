package link

import "fmt"

// OpcodeKickPlayer is the wire opcode for KickPlayer, telling a game server
// to disconnect an account.
const OpcodeKickPlayer = 0x04

// EncodeKickPlayer builds the KickPlayer packet for account.
func EncodeKickPlayer(account string) []byte {
	w := newWriter(OpcodeKickPlayer)
	w.WriteString(account)
	return w.Bytes()
}

// DecodeKickPlayer parses a raw KickPlayer payload (opcode byte included)
// into the account to disconnect.
func DecodeKickPlayer(payload []byte) (string, error) {
	r := newReader(payload)
	account := r.ReadString()
	if r.Err() != nil {
		return "", fmt.Errorf("link: KickPlayer: %w", r.Err())
	}
	return account, nil
}
