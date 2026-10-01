package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Party client packet opcodes.
const (
	OpcodeRequestJoinParty                = 0x29
	OpcodeRequestAnswerJoinParty          = 0x2a
	OpcodeRequestWithdrawParty            = 0x2b
	OpcodeRequestOustPartyMember          = 0x2c
	OpcodeRequestChangePartyLeader uint16 = 0x0004

	OpcodeRequestExAskJoinMPCC              uint16 = 0x000d
	OpcodeRequestExAcceptJoinMPCC           uint16 = 0x000e
	OpcodeRequestExOustFromMPCC             uint16 = 0x000f
	OpcodeRequestExMPCCShowPartyMembersInfo uint16 = 0x0026
)

// RequestJoinParty invites the player named Target into the sender's
// party, offering LootRule when the sender has none yet.
type RequestJoinParty struct {
	Target   string
	LootRule int32
}

// DecodeRequestJoinParty parses a raw RequestJoinParty payload (opcode
// byte included).
func DecodeRequestJoinParty(payload []byte) (RequestJoinParty, error) {
	r := newReader(payload)
	req := RequestJoinParty{Target: r.ReadString()}
	if r.Remaining() < 4 {
		return RequestJoinParty{}, fmt.Errorf("clientpackets: RequestJoinParty: need 4 bytes after the name, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	req.LootRule = r.ReadInt32()
	if err := r.Err(); err != nil {
		return RequestJoinParty{}, fmt.Errorf("clientpackets: RequestJoinParty: %w", err)
	}
	return req, nil
}

// Response is an answer to a party or command channel invitation; 1
// accepts.
type Response struct {
	Response int32
}

// DecodeRequestAnswerJoinParty parses a raw RequestAnswerJoinParty payload
// (opcode byte included).
func DecodeRequestAnswerJoinParty(payload []byte) (Response, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return Response{}, fmt.Errorf("clientpackets: RequestAnswerJoinParty: need 4 bytes, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	return Response{Response: r.ReadInt32()}, nil
}

// TargetName names the player a party or command channel request acts on.
type TargetName struct {
	Name string
}

// DecodeRequestOustPartyMember parses a raw RequestOustPartyMember payload
// (opcode byte included).
func DecodeRequestOustPartyMember(payload []byte) (TargetName, error) {
	r := newReader(payload)
	req := TargetName{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return TargetName{}, fmt.Errorf("clientpackets: RequestOustPartyMember: %w", err)
	}
	return req, nil
}

// DecodeRequestChangePartyLeader parses a raw RequestChangePartyLeader
// payload (opcode byte and sub-opcode included).
func DecodeRequestChangePartyLeader(payload []byte) (TargetName, error) {
	return decodeExtendedName(payload, "RequestChangePartyLeader", OpcodeRequestChangePartyLeader)
}

// DecodeRequestExAskJoinMPCC parses a raw RequestExAskJoinMPCC payload
// (opcode byte and sub-opcode included).
func DecodeRequestExAskJoinMPCC(payload []byte) (TargetName, error) {
	return decodeExtendedName(payload, "RequestExAskJoinMPCC", OpcodeRequestExAskJoinMPCC)
}

// DecodeRequestExOustFromMPCC parses a raw RequestExOustFromMPCC payload
// (opcode byte and sub-opcode included).
func DecodeRequestExOustFromMPCC(payload []byte) (TargetName, error) {
	return decodeExtendedName(payload, "RequestExOustFromMPCC", OpcodeRequestExOustFromMPCC)
}

// DecodeRequestExAcceptJoinMPCC parses a raw RequestExAcceptJoinMPCC
// payload (opcode byte and sub-opcode included).
func DecodeRequestExAcceptJoinMPCC(payload []byte) (Response, error) {
	r, err := newExtendedReader(payload, "RequestExAcceptJoinMPCC", OpcodeRequestExAcceptJoinMPCC, 2+4)
	if err != nil {
		return Response{}, err
	}
	return Response{Response: r.ReadInt32()}, nil
}

// RequestExMPCCShowPartyMembersInfo asks for the member list of the party
// the player PartyLeaderID is in.
type RequestExMPCCShowPartyMembersInfo struct {
	PartyLeaderID int32
}

// DecodeRequestExMPCCShowPartyMembersInfo parses a raw
// RequestExMPCCShowPartyMembersInfo payload (opcode byte and sub-opcode
// included).
func DecodeRequestExMPCCShowPartyMembersInfo(payload []byte) (RequestExMPCCShowPartyMembersInfo, error) {
	r, err := newExtendedReader(payload, "RequestExMPCCShowPartyMembersInfo", OpcodeRequestExMPCCShowPartyMembersInfo, 2+4)
	if err != nil {
		return RequestExMPCCShowPartyMembersInfo{}, err
	}
	return RequestExMPCCShowPartyMembersInfo{PartyLeaderID: r.ReadInt32()}, nil
}

func decodeExtendedName(payload []byte, name string, opcode uint16) (TargetName, error) {
	r, err := newExtendedReader(payload, name, opcode, 2)
	if err != nil {
		return TargetName{}, err
	}
	req := TargetName{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return TargetName{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return req, nil
}
