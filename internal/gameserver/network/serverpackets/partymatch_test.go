package serverpackets

import (
	"bytes"
	"testing"
)

// TestPartyMatchFrames pins the party-matching packets byte for byte
// against their reference layouts: PartyMatchList writeC 0x96, writeD
// (rooms empty ? 0 : 1), writeD count, then per room writeD id, writeS
// title, writeD location, minLvl, maxLvl, members, maxMembers, writeS
// leader name; PartyMatchDetail writeC 0x97, writeD id, maxMembers, minLvl,
// maxLvl, loot, location, writeS title; the 0xfe packets' writeH
// sub-opcode, then ExPartyRoomMember writeD mode, writeD count and per
// member writeD id, writeS name, writeD class, level, bbs, status;
// ExManagePartyRoomMember writeD mode and one such row; ExClosePartyRoom
// nothing; ExAskJoinPartyRoom writeS name; ExListPartyMatchingWaitingRoom
// writeD mode, writeD count, then per player writeS name, writeD class,
// level.
func TestPartyMatchFrames(t *testing.T) {
	row := PartyRoomMember{ObjectID: 7, Name: "Ab", ClassID: 9, Level: 20, Location: 3, Status: 2}
	rowBody := cat(d(7), utf16z("Ab"), d(9), d(20), d(3), d(2))
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			"PartyMatchList",
			framePayload(t, FramePartyMatchList([]PartyRoom{{ID: 1, Title: "T", Location: 4, MinLevel: 10, MaxLevel: 30, Members: 2, MaxMembers: 12, LeaderName: "Ab"}})),
			cat([]byte{0x96}, d(1), d(1), d(1), utf16z("T"), d(4), d(10), d(30), d(2), d(12), utf16z("Ab")),
		},
		{"PartyMatchList empty", framePayload(t, FramePartyMatchList(nil)), cat([]byte{0x96}, d(0), d(0))},
		{
			"PartyMatchDetail",
			framePayload(t, FramePartyMatchDetail(PartyRoomTerms{ID: 5, MaxMembers: 12, MinLevel: 10, MaxLevel: 30, Loot: 2, Location: 100, Title: "T"})),
			cat([]byte{0x97}, d(5), d(12), d(10), d(30), d(2), d(100), utf16z("T")),
		},
		{"ExPartyRoomMember", framePayload(t, FrameExPartyRoomMember(1, []PartyRoomMember{row})), cat([]byte{0xfe, 0x0e, 0x00}, d(1), d(1), rowBody)},
		{"ExPartyRoomMember empty", framePayload(t, FrameExPartyRoomMember(0, nil)), cat([]byte{0xfe, 0x0e, 0x00}, d(0), d(0))},
		{"ExManagePartyRoomMember", framePayload(t, FrameExManagePartyRoomMember(2, row)), cat([]byte{0xfe, 0x10, 0x00}, d(2), rowBody)},
		{"ExClosePartyRoom", framePayload(t, FrameExClosePartyRoom()), []byte{0xfe, 0x0f, 0x00}},
		{"ExAskJoinPartyRoom", framePayload(t, FrameExAskJoinPartyRoom("Ab")), cat([]byte{0xfe, 0x34, 0x00}, utf16z("Ab"))},
		{
			"ExListPartyMatchingWaitingRoom",
			framePayload(t, FrameExListPartyMatchingWaitingRoom(1, []WaitingPlayer{{Name: "Ab", ClassID: 9, Level: 20}})),
			cat([]byte{0xfe, 0x35, 0x00}, d(1), d(1), utf16z("Ab"), d(9), d(20)),
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, tt.want)
		}
	}
}
