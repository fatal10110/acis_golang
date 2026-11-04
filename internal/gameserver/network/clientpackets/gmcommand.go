package clientpackets

import "fmt"

// OpcodeRequestGMCommand asks for a game master's view of a character, sent
// from the character window (Alt+G).
const OpcodeRequestGMCommand = 0x6e

// RequestGMCommand names the player, or for a warehouse view also the clan,
// to show, and the view to show of it.
type RequestGMCommand struct {
	TargetName string
	Command    int32
}

// DecodeRequestGMCommand parses a raw RequestGMCommand payload (opcode byte
// included).
func DecodeRequestGMCommand(payload []byte) (RequestGMCommand, error) {
	r := newReader(payload)
	req := RequestGMCommand{TargetName: r.ReadString(), Command: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestGMCommand{}, fmt.Errorf("clientpackets: RequestGMCommand: %w", err)
	}
	return req, nil
}
