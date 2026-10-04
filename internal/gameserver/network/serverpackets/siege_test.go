package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestSiegeInfoFrame pins SiegeInfo (SiegeInfo.java, castle branch):
// writeC 0xc9, castle id, 1 when the viewer leads the owning clan, owner
// id, then the owner's clan name, leader name, ally id and ally name, or
// "NPC", "", 0, "" for a castle no clan holds, then now and the siege
// date in seconds and a trailing 0.
func TestSiegeInfoFrame(t *testing.T) {
	owned := FrameSiegeInfo(SiegeInfoView{
		CastleID: 1, IsLord: true, OwnerID: 0x10000001, Owner: true,
		OwnerName: "Lords", LeaderName: "Newbie", AllyID: 7, AllyName: "Allies",
		Now: 1_790_000_000, SiegeDate: 1_791_000_000,
	})
	want := cat([]byte{0xc9}, d(1), d(1), d(0x10000001), utf16z("Lords"), utf16z("Newbie"), d(7), utf16z("Allies"),
		d(1_790_000_000), d(1_791_000_000), d(0))
	if got := framePayload(t, owned); !bytes.Equal(got, want) {
		t.Fatalf("owned SiegeInfo = % x\nwant % x", got, want)
	}

	npc := FrameSiegeInfo(SiegeInfoView{CastleID: 3, Now: 5, SiegeDate: 6})
	want = cat([]byte{0xc9}, d(3), d(0), d(0), utf16z("NPC"), utf16z(""), d(0), utf16z(""), d(5), d(6), d(0))
	if got := framePayload(t, npc); !bytes.Equal(got, want) {
		t.Fatalf("NPC SiegeInfo = % x\nwant % x", got, want)
	}
}

// TestSiegeListFrames pins SiegeAttackerList (writeC 0xca) and
// SiegeDefenderList (writeC 0xcb): id, 0, 1, 0, then the row count twice
// (0 and 0 for none) and per clan its id, name, leader name, crest id, 0,
// (defenders only: 1 owner, 2 pending, 3 approved), ally id, ally name, ""
// and ally crest id.
func TestSiegeListFrames(t *testing.T) {
	clanA := SiegeClan{ClanID: 0x10000002, Name: "Rivals", LeaderName: "Red", CrestID: 9, AllyID: 4, AllyName: "Band", AllyCrestID: 11}
	clanB := SiegeClan{ClanID: 0x10000003, Name: "Kings", LeaderName: "Blue"}
	header := func(op byte, id int32, n int32) []byte { return cat([]byte{op}, d(id), d(0), d(1), d(0), d(n), d(n)) }
	row := func(c SiegeClan, side []byte) []byte {
		return cat(d(c.ClanID), utf16z(c.Name), utf16z(c.LeaderName), d(c.CrestID), d(0), side,
			d(c.AllyID), utf16z(c.AllyName), utf16z(""), d(c.AllyCrestID))
	}

	got := framePayload(t, FrameSiegeAttackerList(2, []SiegeClan{clanA, clanB}))
	if want := cat(header(0xca, 2, 2), row(clanA, nil), row(clanB, nil)); !bytes.Equal(got, want) {
		t.Fatalf("SiegeAttackerList = % x\nwant % x", got, want)
	}
	got = framePayload(t, FrameSiegeAttackerList(2, nil))
	if want := header(0xca, 2, 0); !bytes.Equal(got, want) {
		t.Fatalf("empty SiegeAttackerList = % x\nwant % x", got, want)
	}

	got = framePayload(t, FrameSiegeDefenderList(5, []SiegeDefender{
		{SiegeClan: clanA, Side: SiegeDefenderOwner},
		{SiegeClan: clanB, Side: SiegeDefenderPending},
	}))
	if want := cat(header(0xcb, 5, 2), row(clanA, d(1)), row(clanB, d(2))); !bytes.Equal(got, want) {
		t.Fatalf("SiegeDefenderList = % x\nwant % x", got, want)
	}
}

// TestUserInfoSiegeRelation pins UserInfo's relation field
// (UserInfo.java): 0x40 for a clan leader, or-ed with 0x180 for a siege
// attacker and 0x80 for a defender.
func TestUserInfoSiegeRelation(t *testing.T) {
	for _, tc := range []struct {
		leader bool
		state  int32
		want   int32
	}{
		{false, player.SiegeStateNone, 0},
		{true, player.SiegeStateNone, 0x40},
		{false, player.SiegeStateAttacker, 0x180},
		{true, player.SiegeStateAttacker, 0x1c0},
		{false, player.SiegeStateDefender, 0x80},
		{true, player.SiegeStateDefender, 0xc0},
	} {
		if got := userInfoRelation(tc.leader, tc.state); got != tc.want {
			t.Errorf("userInfoRelation(%t, %d) = %#x, want %#x", tc.leader, tc.state, got, tc.want)
		}
	}
}
