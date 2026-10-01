package serverpackets

import (
	"bytes"
	"testing"
)

// The fixtures below follow each clan packet's specified field order and
// widths, written out field by field rather than through the encoders.

func TestFramePledgeShowInfoUpdate(t *testing.T) {
	got := framePayload(t, FramePledgeShowInfoUpdate(PledgeHeader{
		ClanID: 501, CrestID: 7, Level: 5, CastleID: 2, ClanHallID: 34, Rank: 3,
		Reputation: 12000, Dissolving: true, AllyID: 600, AllyName: "Ally", AllyCrestID: 9, AtWar: true,
	}))
	want := []byte{0x88}
	for _, v := range []int32{501, 7, 5, 2, 34, 3, 12000, 3, 0, 600} {
		want = appendD(want, v)
	}
	want = append(want, encodeUTF16Z("Ally")...)
	want = appendD(want, 9)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeShowInfoUpdate = %x, want %x", got, want)
	}
}

func TestFramePledgeShowMemberListAdd(t *testing.T) {
	got := framePayload(t, FramePledgeShowMemberListAdd(PledgeMemberListMember{
		Name: "Recruit", Level: 20, ClassID: 4, Sex: 1, Race: 2, OnlineObjectID: 0x10000002, PledgeType: 0,
	}))
	want := append([]byte{0x55}, encodeUTF16Z("Recruit")...)
	for _, v := range []int32{20, 4, 1, 2, 0x10000002, 0} {
		want = appendD(want, v)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeShowMemberListAdd = %x, want %x", got, want)
	}
}

func TestFramePledgeShowMemberListDeleteAndDeleteAll(t *testing.T) {
	if got, want := framePayload(t, FramePledgeShowMemberListDelete("Gone")), append([]byte{0x56}, encodeUTF16Z("Gone")...); !bytes.Equal(got, want) {
		t.Fatalf("PledgeShowMemberListDelete = %x, want %x", got, want)
	}
	if got := framePayload(t, FramePledgeShowMemberListDeleteAll()); !bytes.Equal(got, []byte{0x82}) {
		t.Fatalf("PledgeShowMemberListDeleteAll = %x, want 82", got)
	}
}

func TestFramePledgeInfoAndStatusChanged(t *testing.T) {
	got := framePayload(t, FramePledgeInfo(501, "Knights", ""))
	want := appendD([]byte{0x83}, 501)
	want = append(want, encodeUTF16Z("Knights")...)
	want = append(want, encodeUTF16Z("")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeInfo = %x, want %x", got, want)
	}

	got = framePayload(t, FramePledgeStatusChanged(0x10000001, 501, 7, 600, 9))
	want = []byte{0xcd}
	for _, v := range []int32{0x10000001, 501, 7, 600, 9, 0, 0} {
		want = appendD(want, v)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeStatusChanged = %x, want %x", got, want)
	}
}

func TestFrameAskJoinPledgeAndJoinPledge(t *testing.T) {
	got := framePayload(t, FrameAskJoinPledge(0x10000001, "Knights"))
	want := appendD([]byte{0x32}, 0x10000001)
	want = append(want, encodeUTF16Z("Knights")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("AskJoinPledge = %x, want %x", got, want)
	}
	if got, want := framePayload(t, FrameJoinPledge(501)), appendD([]byte{0x33}, 501); !bytes.Equal(got, want) {
		t.Fatalf("JoinPledge = %x, want %x", got, want)
	}
}

func TestFrameManagePledgePower(t *testing.T) {
	got := framePayload(t, FrameManagePledgePower(3, 1, 66))
	want := []byte{0x30}
	for _, v := range []int32{3, 1, 66} {
		want = appendD(want, v)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ManagePledgePower = %x, want %x", got, want)
	}
}

func TestFramePledgePowerGradeList(t *testing.T) {
	got := framePayload(t, FramePledgePowerGradeList([10]int{1, 0, 0, 0, 0, 0, 4, 0, 0, 2}))
	want := appendH([]byte{0xfe}, 0x3b)
	want = appendD(want, 9)
	for rank, n := range []int32{0, 0, 0, 0, 0, 4, 0, 0, 2} {
		want = appendD(want, int32(rank+1))
		want = appendD(want, n)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgePowerGradeList = %x, want %x", got, want)
	}
}

func TestFramePledgeReceivePowerAndMemberInfo(t *testing.T) {
	got := framePayload(t, FramePledgeReceivePowerInfo(6, "Recruit", 2))
	want := appendH([]byte{0xfe}, 0x3c)
	want = appendD(want, 6)
	want = append(want, encodeUTF16Z("Recruit")...)
	want = appendD(want, 2)
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeReceivePowerInfo = %x, want %x", got, want)
	}

	got = framePayload(t, FramePledgeReceiveMemberInfo(PledgeMemberInfo{
		PledgeType: 0, Name: "Recruit", Title: "Sir", PowerGrade: 6, PledgeName: "Knights", Mentor: "",
	}))
	want = appendH([]byte{0xfe}, 0x3d)
	want = appendD(want, 0)
	want = append(want, encodeUTF16Z("Recruit")...)
	want = append(want, encodeUTF16Z("Sir")...)
	want = appendD(want, 6)
	want = append(want, encodeUTF16Z("Knights")...)
	want = append(want, encodeUTF16Z("")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeReceiveMemberInfo = %x, want %x", got, want)
	}
}
