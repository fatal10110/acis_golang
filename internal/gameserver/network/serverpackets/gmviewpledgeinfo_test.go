package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameGMViewPledgeInfo pins the game master's clan view: the viewed
// player's name, the clan header, then every member of every sub-unit,
// whose last field is set by a sponsor only.
func TestFrameGMViewPledgeInfo(t *testing.T) {
	got := framePayload(t, FrameGMViewPledgeInfo("Viewed", PledgeMemberList{
		ClanID:      501,
		PledgeType:  1001, // ignored: the view lists the whole clan
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

	want := []byte{OpcodeGMViewPledgeInfo}
	want = append(want, encodeUTF16Z("Viewed")...)
	want = appendD(want, 501)
	want = appendD(want, 0)
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
	want = appendD(want, 2)
	want = append(want, encodeUTF16Z("Rhea")...)
	want = appendD(want, 52)
	want = appendD(want, 16)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 9981)
	want = appendD(want, 1)
	want = append(want, encodeUTF16Z("Main")...)
	want = appendD(want, 60)
	want = appendD(want, 22)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameGMViewPledgeInfo() = %x, want %x", got, want)
	}
}
