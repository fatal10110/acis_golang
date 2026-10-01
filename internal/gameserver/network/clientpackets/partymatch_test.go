package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func joinBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// TestDecodePartyMatchPackets pins the party-matching requests' layouts:
// RequestListPartyWaiting readD auto, bbs, level mode; RequestManagePartyRoom
// readD room, max members, min level, max level, loot, readS title;
// RequestJoinPartyRoom readD room, bbs, level mode; then, after their
// writeH sub-opcode, RequestOustFromPartyRoom readD target; the dismiss and
// withdraw requests readD room, readD unused; RequestAskJoinPartyRoom readS
// name; AnswerJoinPartyRoom readD answer; RequestListPartyMatchingWaitingRoom
// readD page, min level, max level, mode. A short body is refused.
func TestDecodePartyMatchPackets(t *testing.T) {
	list, err := DecodeRequestListPartyWaiting(joinBytes([]byte{OpcodeRequestListPartyWaiting}, le32(1, -2, 0)))
	if err != nil || list != (RequestListPartyWaiting{Auto: 1, Location: -2, LevelMode: 0}) {
		t.Fatalf("RequestListPartyWaiting = %+v, %v", list, err)
	}
	manage, err := DecodeRequestManagePartyRoom(joinBytes([]byte{OpcodeRequestManagePartyRoom}, le32(3, 12, 10, 30, 2), partyName("Go")))
	if err != nil || manage != (RequestManagePartyRoom{RoomID: 3, MaxMembers: 12, MinLevel: 10, MaxLevel: 30, Loot: 2, Title: "Go"}) {
		t.Fatalf("RequestManagePartyRoom = %+v, %v", manage, err)
	}
	if _, err := DecodeRequestManagePartyRoom(joinBytes([]byte{OpcodeRequestManagePartyRoom}, le32(3, 12, 10, 30, 2))); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("RequestManagePartyRoom without a title: %v", err)
	}
	join, err := DecodeRequestJoinPartyRoom(joinBytes([]byte{OpcodeRequestJoinPartyRoom}, le32(0, -1, 1)))
	if err != nil || join != (RequestJoinPartyRoom{RoomID: 0, Location: -1, LevelMode: 1}) {
		t.Fatalf("RequestJoinPartyRoom = %+v, %v", join, err)
	}
	if _, err := DecodeRequestJoinPartyRoom(joinBytes([]byte{OpcodeRequestJoinPartyRoom}, le32(0, -1))); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestJoinPartyRoom: %v", err)
	}

	ext := func(second uint16, body ...[]byte) []byte {
		return joinBytes(append([][]byte{{OpcodeExtended, byte(second), byte(second >> 8)}}, body...)...)
	}
	oust, err := DecodeRequestOustFromPartyRoom(ext(OpcodeRequestOustFromPartyRoom, le32(77)))
	if err != nil || oust.ObjectID != 77 {
		t.Fatalf("RequestOustFromPartyRoom = %+v, %v", oust, err)
	}
	dismiss, err := DecodeRequestDismissPartyRoom(ext(OpcodeRequestDismissPartyRoom, le32(4, 0)))
	if err != nil || dismiss.RoomID != 4 {
		t.Fatalf("RequestDismissPartyRoom = %+v, %v", dismiss, err)
	}
	if _, err := DecodeRequestDismissPartyRoom(ext(OpcodeRequestDismissPartyRoom, le32(4))); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("RequestDismissPartyRoom without its unused int: %v", err)
	}
	withdraw, err := DecodeRequestWithdrawPartyRoom(ext(OpcodeRequestWithdrawPartyRoom, le32(5, 0)))
	if err != nil || withdraw.RoomID != 5 {
		t.Fatalf("RequestWithdrawPartyRoom = %+v, %v", withdraw, err)
	}
	ask, err := DecodeRequestAskJoinPartyRoom(ext(OpcodeRequestAskJoinPartyRoom, partyName("Ab")))
	if err != nil || ask.Name != "Ab" {
		t.Fatalf("RequestAskJoinPartyRoom = %+v, %v", ask, err)
	}
	answer, err := DecodeAnswerJoinPartyRoom(ext(OpcodeAnswerJoinPartyRoom, le32(1)))
	if err != nil || answer.Response != 1 {
		t.Fatalf("AnswerJoinPartyRoom = %+v, %v", answer, err)
	}
	waiting, err := DecodeRequestListPartyMatchingWaitingRoom(ext(OpcodeRequestListPartyMatchingWaitingRoom, le32(1, 10, 20, 0)))
	if err != nil || waiting != (RequestListPartyMatchingWaitingRoom{Page: 1, MinLevel: 10, MaxLevel: 20, Mode: 0}) {
		t.Fatalf("RequestListPartyMatchingWaitingRoom = %+v, %v", waiting, err)
	}
	if _, err := DecodeRequestListPartyMatchingWaitingRoom(ext(OpcodeRequestListPartyMatchingWaitingRoom, le32(1, 10, 20))); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestListPartyMatchingWaitingRoom: %v", err)
	}
}
