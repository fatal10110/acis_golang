package clientpackets

import "fmt"

// Clan war and sub-unit packet opcodes.
const (
	OpcodeRequestStartPledgeWar          = 0x4d
	OpcodeRequestReplyStartPledgeWar     = 0x4e
	OpcodeRequestStopPledgeWar           = 0x4f
	OpcodeRequestReplyStopPledgeWar      = 0x50
	OpcodeRequestSurrenderPledgeWar      = 0x51
	OpcodeRequestReplySurrenderPledgeWar = 0x52

	OpcodeRequestPledgeSetAcademyMaster = uint16(0x0019)
	OpcodeRequestPledgeWarList          = uint16(0x001e)
	OpcodeRequestPledgeReorganizeMember = uint16(0x0024)
)

// RequestPledgeWarName names the clan a war request is about: a
// declaration, a stop or a surrender.
type RequestPledgeWarName struct {
	PledgeName string
}

// DecodeRequestPledgeWarName parses a raw RequestStartPledgeWar,
// RequestStopPledgeWar or RequestSurrenderPledgeWar payload (opcode byte
// included).
func DecodeRequestPledgeWarName(payload []byte) (RequestPledgeWarName, error) {
	r := newReader(payload)
	req := RequestPledgeWarName{PledgeName: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestPledgeWarName{}, fmt.Errorf("clientpackets: RequestPledgeWarName: %w", err)
	}
	return req, nil
}

// RequestPledgeWarReply answers a war, stop or surrender proposal.
type RequestPledgeWarReply struct {
	Answer int32
}

// DecodeRequestPledgeWarReply parses a raw RequestReplyStartPledgeWar,
// RequestReplyStopPledgeWar or RequestReplySurrenderPledgeWar payload
// (opcode byte included).
func DecodeRequestPledgeWarReply(payload []byte) (RequestPledgeWarReply, error) {
	r := newReader(payload)
	req := RequestPledgeWarReply{Answer: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPledgeWarReply{}, fmt.Errorf("clientpackets: RequestPledgeWarReply: %w", err)
	}
	return req, nil
}

// RequestPledgeWarList asks for a page of one tab of the clan war window:
// tab 0 the clans the clan declared war on, others the clans that declared
// war on it.
type RequestPledgeWarList struct {
	Page int32
	Tab  int32
}

// DecodeRequestPledgeWarList parses a raw RequestPledgeWarList payload
// (extended opcode included).
func DecodeRequestPledgeWarList(payload []byte) (RequestPledgeWarList, error) {
	r, err := newExtendedReader(payload, "RequestPledgeWarList", OpcodeRequestPledgeWarList, 2)
	if err != nil {
		return RequestPledgeWarList{}, err
	}
	req := RequestPledgeWarList{Page: r.ReadInt32(), Tab: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPledgeWarList{}, fmt.Errorf("clientpackets: RequestPledgeWarList: %w", err)
	}
	return req, nil
}

// RequestPledgeReorganizeMember moves the member named MemberName into
// sub-unit NewPledgeType, swapping SelectedMemberName into its old one;
// with Selected 0 it only asks for MemberName's card.
type RequestPledgeReorganizeMember struct {
	Selected           int32
	MemberName         string
	NewPledgeType      int32
	SelectedMemberName string
}

// DecodeRequestPledgeReorganizeMember parses a raw
// RequestPledgeReorganizeMember payload (extended opcode included).
func DecodeRequestPledgeReorganizeMember(payload []byte) (RequestPledgeReorganizeMember, error) {
	r, err := newExtendedReader(payload, "RequestPledgeReorganizeMember", OpcodeRequestPledgeReorganizeMember, 2)
	if err != nil {
		return RequestPledgeReorganizeMember{}, err
	}
	req := RequestPledgeReorganizeMember{Selected: r.ReadInt32(), MemberName: r.ReadString()}
	req.NewPledgeType = r.ReadInt32()
	req.SelectedMemberName = r.ReadString()
	if err := r.Err(); err != nil {
		return RequestPledgeReorganizeMember{}, fmt.Errorf("clientpackets: RequestPledgeReorganizeMember: %w", err)
	}
	return req, nil
}

// RequestPledgeSetAcademyMaster links (Set non-zero) or unlinks (Set 0)
// the members named CurrentName and TargetName as sponsor and apprentice.
type RequestPledgeSetAcademyMaster struct {
	Set         int32
	CurrentName string
	TargetName  string
}

// DecodeRequestPledgeSetAcademyMaster parses a raw
// RequestPledgeSetAcademyMaster payload (extended opcode included).
func DecodeRequestPledgeSetAcademyMaster(payload []byte) (RequestPledgeSetAcademyMaster, error) {
	r, err := newExtendedReader(payload, "RequestPledgeSetAcademyMaster", OpcodeRequestPledgeSetAcademyMaster, 2)
	if err != nil {
		return RequestPledgeSetAcademyMaster{}, err
	}
	req := RequestPledgeSetAcademyMaster{Set: r.ReadInt32(), CurrentName: r.ReadString()}
	req.TargetName = r.ReadString()
	if err := r.Err(); err != nil {
		return RequestPledgeSetAcademyMaster{}, fmt.Errorf("clientpackets: RequestPledgeSetAcademyMaster: %w", err)
	}
	return req, nil
}
