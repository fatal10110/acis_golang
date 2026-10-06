package clientpackets

import (
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// The tutorial window's opcodes.
const (
	// OpcodeRequestTutorialLinkHTML follows a link of the tutorial window.
	OpcodeRequestTutorialLinkHTML = 0x7b
	// OpcodeRequestTutorialPassCmdToServer sends a command of the
	// tutorial window.
	OpcodeRequestTutorialPassCmdToServer = 0x7c
	// OpcodeRequestTutorialQuestionMark reports a clicked question mark.
	OpcodeRequestTutorialQuestionMark = 0x7d
	// OpcodeRequestTutorialClientEvent reports a client event the server
	// enabled.
	OpcodeRequestTutorialClientEvent = 0x7e
)

// TutorialEvent is one of the four tutorial requests, as the event name it
// hands the player's tutorial quest.
type TutorialEvent struct {
	Name string
}

// DecodeRequestTutorialLinkHTML parses RequestTutorialLinkHtml (opcode byte
// included): the event is the link as sent.
func DecodeRequestTutorialLinkHTML(payload []byte) (TutorialEvent, error) {
	return decodeTutorialString(payload, "RequestTutorialLinkHtml")
}

// DecodeRequestTutorialPassCmdToServer parses RequestTutorialPassCmdToServer
// (opcode byte included): the event is the command as sent.
func DecodeRequestTutorialPassCmdToServer(payload []byte) (TutorialEvent, error) {
	return decodeTutorialString(payload, "RequestTutorialPassCmdToServer")
}

// DecodeRequestTutorialQuestionMark parses RequestTutorialQuestionMark
// (opcode byte included): the event is "QM" and the mark's number.
func DecodeRequestTutorialQuestionMark(payload []byte) (TutorialEvent, error) {
	return decodeTutorialNumber(payload, "RequestTutorialQuestionMark", "QM")
}

// DecodeRequestTutorialClientEvent parses RequestTutorialClientEvent
// (opcode byte included): the event is "CE" and the client event's number.
func DecodeRequestTutorialClientEvent(payload []byte) (TutorialEvent, error) {
	return decodeTutorialNumber(payload, "RequestTutorialClientEvent", "CE")
}

func decodeTutorialString(payload []byte, packet string) (TutorialEvent, error) {
	r := newReader(payload)
	e := TutorialEvent{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return TutorialEvent{}, fmt.Errorf("clientpackets: %s: %w", packet, err)
	}
	return e, nil
}

func decodeTutorialNumber(payload []byte, packet, prefix string) (TutorialEvent, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return TutorialEvent{}, fmt.Errorf("clientpackets: %s: need 4 bytes, got %d: %w", packet, r.Remaining(), wire.ErrShortPacket)
	}
	return TutorialEvent{Name: prefix + strconv.Itoa(int(r.ReadInt32()))}, nil
}
