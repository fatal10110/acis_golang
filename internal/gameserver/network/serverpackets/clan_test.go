package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ---- from crest_test.go ----
func TestFramePledgeCrest(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	got := framePayload(t, FramePledgeCrest(101, data))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest() = %x, want %x", got, want)
	}
}

func TestFrameAllyCrest(t *testing.T) {
	data := []byte{0x04, 0x05, 0x06}
	got := framePayload(t, FrameAllyCrest(103, data))

	want := []byte{OpcodeAllyCrest}
	want = binary.LittleEndian.AppendUint32(want, 103)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAllyCrest() = %x, want %x", got, want)
	}
}

func TestFrameExPledgeCrestLarge(t *testing.T) {
	data := []byte{0x07, 0x08, 0x09}
	got := framePayload(t, FrameExPledgeCrestLarge(105, data))

	want := []byte{OpcodeExtended}
	want = binary.LittleEndian.AppendUint16(want, OpcodeExPledgeCrestLarge)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 105)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExPledgeCrestLarge() = %x, want %x", got, want)
	}
}

func TestFramePledgeCrestMissingData(t *testing.T) {
	got := framePayload(t, FramePledgeCrest(101, nil))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest(nil) = %x, want %x", got, want)
	}
}

// ---- from pledgememberlist_test.go ----
func TestFramePledgeShowMemberListUpdate(t *testing.T) {
	got := framePayload(t, FramePledgeShowMemberListUpdate(PledgeMemberListMember{
		Name:           "Rhea",
		Level:          52,
		ClassID:        16,
		Sex:            1,
		Race:           2,
		OnlineObjectID: 9981,
		PledgeType:     1001,
		HasSponsor:     true,
	}))

	want := []byte{OpcodePledgeShowMemberListUpdate}
	want = append(want, encodeUTF16Z("Rhea")...)
	want = appendD(want, 52)
	want = appendD(want, 16)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 9981)
	want = appendD(want, 1001)
	want = appendD(want, 1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeShowMemberListUpdate() = %x, want %x", got, want)
	}
}

func TestFramePledgeShowMemberListAll(t *testing.T) {
	got := framePayload(t, FramePledgeShowMemberListAll(PledgeMemberList{
		ClanID:      501,
		PledgeType:  1001,
		PledgeName:  "Knights",
		LeaderName:  "Captain",
		CrestID:     77,
		Level:       5,
		CastleID:    1,
		ClanHallID:  2,
		Rank:        3,
		Reputation:  4500,
		Dissolving:  true,
		AllyID:      88,
		AllyName:    "Alliance",
		AllyCrestID: 99,
		AtWar:       true,
		Members: []PledgeMemberListMember{
			{Name: "Rhea", Level: 52, ClassID: 16, Sex: 1, Race: 2, OnlineObjectID: 9981, PledgeType: 1001, HasSponsor: true},
			{Name: "Main", Level: 60, ClassID: 22, PledgeType: 0},
		},
	}))

	want := []byte{OpcodePledgeShowMemberListAll}
	want = appendD(want, 1)
	want = appendD(want, 501)
	want = appendD(want, 1001)
	want = append(want, encodeUTF16Z("Knights")...)
	want = append(want, encodeUTF16Z("Captain")...)
	want = appendD(want, 77)
	want = appendD(want, 5)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 3)
	want = appendD(want, 4500)
	want = appendD(want, 3)
	want = appendD(want, 0)
	want = appendD(want, 88)
	want = append(want, encodeUTF16Z("Alliance")...)
	want = appendD(want, 99)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = append(want, encodeUTF16Z("Rhea")...)
	want = appendD(want, 52)
	want = appendD(want, 16)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 9981)
	want = appendD(want, 1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeShowMemberListAll() = %x, want %x", got, want)
	}
}
