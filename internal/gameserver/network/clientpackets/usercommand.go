package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeRequestUserCommand is the wire opcode for RequestUserCommand, sent
// for a slash command the client resolves to a numbered user command
// (/loc, /unstuck, /mount, ...).
const OpcodeRequestUserCommand = 0xaa

const requestUserCommandSize = 4

// RequestUserCommand asks to run user command CommandID.
type RequestUserCommand struct {
	CommandID int32
}

// DecodeRequestUserCommand parses a raw RequestUserCommand payload (opcode
// byte included).
func DecodeRequestUserCommand(payload []byte) (RequestUserCommand, error) {
	r := newReader(payload)
	if r.Remaining() < requestUserCommandSize {
		return RequestUserCommand{}, fmt.Errorf("clientpackets: RequestUserCommand: need %d bytes, got %d: %w", requestUserCommandSize, r.Remaining(), wire.ErrShortPacket)
	}
	return RequestUserCommand{CommandID: r.ReadInt32()}, nil
}
