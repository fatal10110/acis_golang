package clientpackets

import "fmt"

// Alliance packet opcodes, valid once a client is in game.
const (
	OpcodeRequestJoinAlly       = 0x82
	OpcodeRequestAnswerJoinAlly = 0x83
	OpcodeAllyLeave             = 0x84
	OpcodeAllyDismiss           = 0x85
	OpcodeRequestDismissAlly    = 0x86
	OpcodeRequestSetAllyCrest   = 0x87
	OpcodeRequestAllyInfo       = 0x8e
)

// AllyCrestMaxLength is the largest alliance crest image an upload
// carries; a longer declared length is read as no image at all.
const AllyCrestMaxLength = 192

// RequestJoinAlly invites the leader of another clan into the requester's
// alliance.
type RequestJoinAlly struct {
	TargetID int32
}

// DecodeRequestJoinAlly parses a raw RequestJoinAlly payload (opcode byte
// included).
func DecodeRequestJoinAlly(payload []byte) (RequestJoinAlly, error) {
	r := newReader(payload)
	req := RequestJoinAlly{TargetID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestJoinAlly{}, fmt.Errorf("clientpackets: RequestJoinAlly: %w", err)
	}
	return req, nil
}

// RequestAnswerJoinAlly answers an alliance invitation: 0 refuses.
type RequestAnswerJoinAlly struct {
	Answer int32
}

// DecodeRequestAnswerJoinAlly parses a raw RequestAnswerJoinAlly payload
// (opcode byte included).
func DecodeRequestAnswerJoinAlly(payload []byte) (RequestAnswerJoinAlly, error) {
	r := newReader(payload)
	req := RequestAnswerJoinAlly{Answer: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestAnswerJoinAlly{}, fmt.Errorf("clientpackets: RequestAnswerJoinAlly: %w", err)
	}
	return req, nil
}

// AllyDismiss dismisses the clan named Name from the requester's alliance.
type AllyDismiss struct {
	Name string
}

// DecodeAllyDismiss parses a raw AllyDismiss payload (opcode byte
// included).
func DecodeAllyDismiss(payload []byte) (AllyDismiss, error) {
	r := newReader(payload)
	req := AllyDismiss{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return AllyDismiss{}, fmt.Errorf("clientpackets: AllyDismiss: %w", err)
	}
	return req, nil
}

// DecodeRequestSetAllyCrest parses a raw RequestSetAllyCrest payload
// (opcode byte included).
func DecodeRequestSetAllyCrest(payload []byte) (CrestUpload, error) {
	return readCrestUpload(newReader(payload), "RequestSetAllyCrest", AllyCrestMaxLength)
}
