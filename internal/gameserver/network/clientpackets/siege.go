package clientpackets

import "fmt"

// Siege request opcodes.
const (
	OpcodeRequestSiegeAttackerList       = 0xa2
	OpcodeRequestSiegeDefenderList       = 0xa3
	OpcodeRequestJoinSiege               = 0xa4
	OpcodeRequestConfirmSiegeWaitingList = 0xa5
)

// RequestSiegeList asks for the attacking or defending clans of residence
// ID.
type RequestSiegeList struct {
	ID int32
}

// DecodeRequestSiegeList parses a raw RequestSiegeAttackerList or
// RequestSiegeDefenderList payload (opcode byte included).
func DecodeRequestSiegeList(payload []byte) (RequestSiegeList, error) {
	r := newReader(payload)
	req := RequestSiegeList{ID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestSiegeList{}, fmt.Errorf("clientpackets: RequestSiegeList: %w", err)
	}
	return req, nil
}

// RequestJoinSiege registers the sender's clan on residence ID's siege, as
// an attacker when IsAttacker is 1, or drops its registration when
// IsJoining is not 1.
type RequestJoinSiege struct {
	ID         int32
	IsAttacker int32
	IsJoining  int32
}

// DecodeRequestJoinSiege parses a raw RequestJoinSiege payload (opcode byte
// included).
func DecodeRequestJoinSiege(payload []byte) (RequestJoinSiege, error) {
	r := newReader(payload)
	req := RequestJoinSiege{ID: r.ReadInt32(), IsAttacker: r.ReadInt32(), IsJoining: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestJoinSiege{}, fmt.Errorf("clientpackets: RequestJoinSiege: %w", err)
	}
	return req, nil
}

// RequestConfirmSiegeWaitingList is the castle lord's answer to clan
// ClanID's request to defend castle CastleID; Approved 1 approves it.
type RequestConfirmSiegeWaitingList struct {
	CastleID int32
	ClanID   int32
	Approved int32
}

// DecodeRequestConfirmSiegeWaitingList parses a raw
// RequestConfirmSiegeWaitingList payload (opcode byte included).
func DecodeRequestConfirmSiegeWaitingList(payload []byte) (RequestConfirmSiegeWaitingList, error) {
	r := newReader(payload)
	req := RequestConfirmSiegeWaitingList{CastleID: r.ReadInt32(), ClanID: r.ReadInt32(), Approved: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestConfirmSiegeWaitingList{}, fmt.Errorf("clientpackets: RequestConfirmSiegeWaitingList: %w", err)
	}
	return req, nil
}
