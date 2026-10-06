package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const requestQuestAbortSize = 4

// RequestQuestAbort asks to abort the quest QuestID from the quest window.
type RequestQuestAbort struct {
	QuestID int32
}

// DecodeRequestQuestAbort parses a raw RequestQuestAbort payload (opcode
// byte included).
func DecodeRequestQuestAbort(payload []byte) (RequestQuestAbort, error) {
	r := newReader(payload)
	if r.Remaining() < requestQuestAbortSize {
		return RequestQuestAbort{}, fmt.Errorf("clientpackets: RequestQuestAbort: need %d bytes, got %d: %w", requestQuestAbortSize, r.Remaining(), wire.ErrShortPacket)
	}
	return RequestQuestAbort{QuestID: r.ReadInt32()}, nil
}
