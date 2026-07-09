package link

import "fmt"

// OpcodePlayerInGame is the wire opcode for PlayerInGame, reporting accounts
// that just entered the game on this server.
const OpcodePlayerInGame = 0x02

// DecodePlayerInGame parses a raw PlayerInGame payload (opcode byte
// included) into the list of accounts that entered the game.
func DecodePlayerInGame(payload []byte) ([]string, error) {
	r := newReader(payload)
	count := int(r.ReadUint16())
	accounts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		accounts = append(accounts, r.ReadString())
	}
	if r.Err() != nil {
		return nil, fmt.Errorf("link: PlayerInGame: %w", r.Err())
	}
	return accounts, nil
}

// EncodePlayerInGame builds the PlayerInGame packet reporting accounts that
// just entered the game on this server.
func EncodePlayerInGame(accounts []string) []byte {
	w := newWriter(OpcodePlayerInGame)
	w.WriteUint16(uint16(len(accounts)))
	for _, account := range accounts {
		w.WriteString(account)
	}
	return w.Bytes()
}
