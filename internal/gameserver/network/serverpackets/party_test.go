package serverpackets

import (
	"bytes"
	"testing"
)

// utf16z is name as a null-terminated UTF-16LE string (writeS).
func utf16z(name string) []byte {
	var out []byte
	for _, r := range name {
		out = append(out, byte(r), byte(r>>8))
	}
	return append(out, 0, 0)
}

func d(v int32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// TestPartyFrames pins the party and command channel packets byte for
// byte against their reference layouts: AskJoinParty writeC 0x39, writeS
// requester, writeD loot; JoinParty writeC 0x3a, writeD response; the
// small-window rows writeD id, writeS name, writeD cp, maxCp, hp, maxHp,
// mp, maxMp, level, class, then (All) writeD 0, writeD race or (Add)
// writeD 0, writeD 0; Delete writeD id, writeS name; DeleteAll writeC 0x50;
// PartyMemberPosition writeD count then writeD id, x, y, z per member;
// the 0xfe packets' writeH sub-opcode and their bodies.
func TestPartyFrames(t *testing.T) {
	row := PartyMember{ObjectID: 7, Name: "Ab", CP: 1, MaxCP: 2, HP: 3, MaxHP: 4, MP: 5, MaxMP: 6, Level: 20, ClassID: 9, Race: 3}
	rowBody := cat(d(7), utf16z("Ab"), d(1), d(2), d(3), d(4), d(5), d(6), d(20), d(9))
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"AskJoinParty", framePayload(t, FrameAskJoinParty("Ab", 4)), cat([]byte{0x39}, utf16z("Ab"), d(4))},
		{"JoinParty", framePayload(t, FrameJoinParty(1)), cat([]byte{0x3a}, d(1))},
		{"WindowAll", framePayload(t, FramePartySmallWindowAll(11, 2, []PartyMember{row})), cat([]byte{0x4e}, d(11), d(2), d(1), rowBody, d(0), d(3))},
		{"WindowAll empty", framePayload(t, FramePartySmallWindowAll(11, 0, nil)), cat([]byte{0x4e}, d(11), d(0), d(0))},
		{"WindowAdd", framePayload(t, FramePartySmallWindowAdd(11, 2, row)), cat([]byte{0x4f}, d(11), d(2), rowBody, d(0), d(0))},
		{"WindowDeleteAll", framePayload(t, FramePartySmallWindowDeleteAll()), []byte{0x50}},
		{"WindowDelete", framePayload(t, FramePartySmallWindowDelete(7, "Ab")), cat([]byte{0x51}, d(7), utf16z("Ab"))},
		{"WindowUpdate", framePayload(t, FramePartySmallWindowUpdate(row)), cat([]byte{0x52}, rowBody)},
		{"MemberPosition", framePayload(t, FramePartyMemberPosition([]PartyMemberAt{{ObjectID: 7, X: -1, Y: 2, Z: 3}})), cat([]byte{0xa7}, d(1), d(7), d(-1), d(2), d(3))},
		{"ExAskJoinMPCC", framePayload(t, FrameExAskJoinMPCC("Ab")), cat([]byte{0xfe, 0x27, 0x00}, utf16z("Ab"))},
		{"ExOpenMPCC", framePayload(t, FrameExOpenMPCC()), []byte{0xfe, 0x25, 0x00}},
		{"ExCloseMPCC", framePayload(t, FrameExCloseMPCC()), []byte{0xfe, 0x26, 0x00}},
		{"ExMPCCPartyInfoUpdate", framePayload(t, FrameExMPCCPartyInfoUpdate("Ab", 7, 3, true)), cat([]byte{0xfe, 0x5a, 0x00}, utf16z("Ab"), d(7), d(3), d(1))},
		{"ExMPCCPartyInfoUpdate remove", framePayload(t, FrameExMPCCPartyInfoUpdate("Ab", 7, 3, false)), cat([]byte{0xfe, 0x5a, 0x00}, utf16z("Ab"), d(7), d(3), d(0))},
		{"ExMPCCShowPartyMemberInfo", framePayload(t, FrameExMPCCShowPartyMemberInfo([]PartyChannelMember{{Name: "Ab", ObjectID: 7, ClassID: 9}})), cat([]byte{0xfe, 0x4a, 0x00}, d(1), utf16z("Ab"), d(7), d(9))},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, tt.want)
		}
	}
}
