package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Duel client packet sub-opcodes.
const (
	OpcodeRequestDuelStart       uint16 = 0x0027
	OpcodeRequestDuelAnswerStart uint16 = 0x0028
	OpcodeRequestDuelSurrender   uint16 = 0x0030
)

// RequestDuelStart challenges the player named Target to a duel, or its
// party to a party duel.
type RequestDuelStart struct {
	Target string
	Party  bool
}

// DecodeRequestDuelStart parses a raw RequestDuelStart payload (opcode byte
// and sub-opcode included).
func DecodeRequestDuelStart(payload []byte) (RequestDuelStart, error) {
	r, err := newExtendedReader(payload, "RequestDuelStart", OpcodeRequestDuelStart, 2)
	if err != nil {
		return RequestDuelStart{}, err
	}
	req := RequestDuelStart{Target: r.ReadString()}
	if r.Remaining() < 4 {
		return RequestDuelStart{}, fmt.Errorf("clientpackets: RequestDuelStart: need 4 bytes after the name, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	req.Party = r.ReadInt32() == 1
	if err := r.Err(); err != nil {
		return RequestDuelStart{}, fmt.Errorf("clientpackets: RequestDuelStart: %w", err)
	}
	return req, nil
}

// RequestDuelAnswerStart answers a duel challenge: Accepted takes it. Party
// is the kind of duel the answer claims to take.
type RequestDuelAnswerStart struct {
	Party    bool
	Accepted bool
}

// DecodeRequestDuelAnswerStart parses a raw RequestDuelAnswerStart payload
// (opcode byte and sub-opcode included). Its second field is unused.
func DecodeRequestDuelAnswerStart(payload []byte) (RequestDuelAnswerStart, error) {
	r, err := newExtendedReader(payload, "RequestDuelAnswerStart", OpcodeRequestDuelAnswerStart, 2+12)
	if err != nil {
		return RequestDuelAnswerStart{}, err
	}
	party := r.ReadInt32() == 1
	r.ReadInt32()
	return RequestDuelAnswerStart{Party: party, Accepted: r.ReadInt32() == 1}, nil
}
