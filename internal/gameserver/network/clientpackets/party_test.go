package clientpackets

import (
	"testing"
)

func partyName(name string) []byte {
	var out []byte
	for _, r := range name {
		out = append(out, byte(r), byte(r>>8))
	}
	return append(out, 0, 0)
}

// TestDecodePartyPackets pins the party and command channel requests'
// layouts: RequestJoinParty readS name, readD loot; the answers readD
// response; the name requests readS name; the 0xd0 ones after their
// writeH sub-opcode.
func TestDecodePartyPackets(t *testing.T) {
	join, err := DecodeRequestJoinParty(append(append([]byte{OpcodeRequestJoinParty}, partyName("Ab")...), 3, 0, 0, 0))
	if err != nil || join.Target != "Ab" || join.LootRule != 3 {
		t.Fatalf("RequestJoinParty = %+v, %v", join, err)
	}
	if _, err := DecodeRequestJoinParty(append([]byte{OpcodeRequestJoinParty}, partyName("Ab")...)); err == nil {
		t.Fatal("RequestJoinParty without its loot rule decoded")
	}
	answer, err := DecodeRequestAnswerJoinParty([]byte{OpcodeRequestAnswerJoinParty, 1, 0, 0, 0})
	if err != nil || answer.Response != 1 {
		t.Fatalf("RequestAnswerJoinParty = %+v, %v", answer, err)
	}
	oust, err := DecodeRequestOustPartyMember(append([]byte{OpcodeRequestOustPartyMember}, partyName("Ab")...))
	if err != nil || oust.Name != "Ab" {
		t.Fatalf("RequestOustPartyMember = %+v, %v", oust, err)
	}
	for _, tc := range []struct {
		second uint16
		decode func([]byte) (TargetName, error)
	}{
		{OpcodeRequestChangePartyLeader, DecodeRequestChangePartyLeader},
		{OpcodeRequestExAskJoinMPCC, DecodeRequestExAskJoinMPCC},
		{OpcodeRequestExOustFromMPCC, DecodeRequestExOustFromMPCC},
	} {
		req, err := tc.decode(append([]byte{OpcodeExtended, byte(tc.second), 0}, partyName("Ab")...))
		if err != nil || req.Name != "Ab" {
			t.Fatalf("sub-opcode %#x = %+v, %v", tc.second, req, err)
		}
	}
	accept, err := DecodeRequestExAcceptJoinMPCC([]byte{OpcodeExtended, 0x0e, 0, 1, 0, 0, 0})
	if err != nil || accept.Response != 1 {
		t.Fatalf("RequestExAcceptJoinMPCC = %+v, %v", accept, err)
	}
	info, err := DecodeRequestExMPCCShowPartyMembersInfo([]byte{OpcodeExtended, 0x26, 0, 7, 0, 0, 0})
	if err != nil || info.PartyLeaderID != 7 {
		t.Fatalf("RequestExMPCCShowPartyMembersInfo = %+v, %v", info, err)
	}
}
