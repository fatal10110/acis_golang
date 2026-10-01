package clientpackets

import "fmt"

// Clan packet opcodes.
const (
	OpcodeRequestJoinPledge         = 0x24
	OpcodeRequestAnswerJoinPledge   = 0x25
	OpcodeRequestWithdrawPledge     = 0x26
	OpcodeRequestOustPledgeMember   = 0x27
	OpcodeRequestPledgeMemberList   = 0x3c
	OpcodeRequestPledgeInfo         = 0x66
	OpcodeRequestPledgePower        = 0xc0
	pledgePowerSetPrivilegesAction  = 2
	OpcodeRequestPledgePowerGrades  = uint16(0x001a)
	OpcodeRequestPledgeMemberPower  = uint16(0x001b)
	OpcodeRequestPledgeSetGrade     = uint16(0x001c)
	OpcodeRequestPledgeMemberDetail = uint16(0x001d)
)

// RequestJoinPledge invites a player into the requester's clan.
type RequestJoinPledge struct {
	TargetID   int32
	PledgeType int32
}

// DecodeRequestJoinPledge parses a raw RequestJoinPledge payload (opcode
// byte included).
func DecodeRequestJoinPledge(payload []byte) (RequestJoinPledge, error) {
	r := newReader(payload)
	req := RequestJoinPledge{TargetID: r.ReadInt32(), PledgeType: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestJoinPledge{}, fmt.Errorf("clientpackets: RequestJoinPledge: %w", err)
	}
	return req, nil
}

// RequestAnswerJoinPledge answers a clan invitation: 0 refuses.
type RequestAnswerJoinPledge struct {
	Answer int32
}

// DecodeRequestAnswerJoinPledge parses a raw RequestAnswerJoinPledge
// payload (opcode byte included).
func DecodeRequestAnswerJoinPledge(payload []byte) (RequestAnswerJoinPledge, error) {
	r := newReader(payload)
	req := RequestAnswerJoinPledge{Answer: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestAnswerJoinPledge{}, fmt.Errorf("clientpackets: RequestAnswerJoinPledge: %w", err)
	}
	return req, nil
}

// RequestOustPledgeMember expels the clan member named Name.
type RequestOustPledgeMember struct {
	Name string
}

// DecodeRequestOustPledgeMember parses a raw RequestOustPledgeMember
// payload (opcode byte included).
func DecodeRequestOustPledgeMember(payload []byte) (RequestOustPledgeMember, error) {
	r := newReader(payload)
	req := RequestOustPledgeMember{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestOustPledgeMember{}, fmt.Errorf("clientpackets: RequestOustPledgeMember: %w", err)
	}
	return req, nil
}

// RequestPledgeInfo asks for a clan's name card.
type RequestPledgeInfo struct {
	ClanID int32
}

// DecodeRequestPledgeInfo parses a raw RequestPledgeInfo payload (opcode
// byte included).
func DecodeRequestPledgeInfo(payload []byte) (RequestPledgeInfo, error) {
	r := newReader(payload)
	req := RequestPledgeInfo{ClanID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPledgeInfo{}, fmt.Errorf("clientpackets: RequestPledgeInfo: %w", err)
	}
	return req, nil
}

// RequestPledgePower reads (Action other than 2) or sets (Action 2) a
// rank's privileges.
type RequestPledgePower struct {
	Rank   int32
	Action int32
	Privs  int32
}

// SetsPrivileges reports whether the request sets the rank's privileges.
func (r RequestPledgePower) SetsPrivileges() bool { return r.Action == pledgePowerSetPrivilegesAction }

// DecodeRequestPledgePower parses a raw RequestPledgePower payload (opcode
// byte included). The privileges follow only when they are set.
func DecodeRequestPledgePower(payload []byte) (RequestPledgePower, error) {
	r := newReader(payload)
	req := RequestPledgePower{Rank: r.ReadInt32(), Action: r.ReadInt32()}
	if req.SetsPrivileges() {
		req.Privs = r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		return RequestPledgePower{}, fmt.Errorf("clientpackets: RequestPledgePower: %w", err)
	}
	return req, nil
}

// RequestPledgeMemberName names a clan member, for the member-detail and
// member-rank windows. The pledge type the client sends ahead of it is
// ignored: the member's own is used.
type RequestPledgeMemberName struct {
	Name string
}

// DecodeRequestPledgeMemberName parses a raw RequestPledgeMemberInfo or
// RequestPledgeMemberPowerInfo payload (extended opcode included).
func DecodeRequestPledgeMemberName(payload []byte, opcode uint16) (RequestPledgeMemberName, error) {
	r, err := newExtendedReader(payload, "RequestPledgeMemberName", opcode, 2)
	if err != nil {
		return RequestPledgeMemberName{}, err
	}
	r.ReadInt32()
	req := RequestPledgeMemberName{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestPledgeMemberName{}, fmt.Errorf("clientpackets: RequestPledgeMemberName: %w", err)
	}
	return req, nil
}

// RequestPledgeSetMemberPowerGrade gives a clan member a power grade.
type RequestPledgeSetMemberPowerGrade struct {
	Name       string
	PowerGrade int32
}

// DecodeRequestPledgeSetMemberPowerGrade parses a raw
// RequestPledgeSetMemberPowerGrade payload (extended opcode included).
func DecodeRequestPledgeSetMemberPowerGrade(payload []byte) (RequestPledgeSetMemberPowerGrade, error) {
	r, err := newExtendedReader(payload, "RequestPledgeSetMemberPowerGrade", OpcodeRequestPledgeSetGrade, 2)
	if err != nil {
		return RequestPledgeSetMemberPowerGrade{}, err
	}
	req := RequestPledgeSetMemberPowerGrade{Name: r.ReadString()}
	req.PowerGrade = r.ReadInt32()
	if err := r.Err(); err != nil {
		return RequestPledgeSetMemberPowerGrade{}, fmt.Errorf("clientpackets: RequestPledgeSetMemberPowerGrade: %w", err)
	}
	return req, nil
}
