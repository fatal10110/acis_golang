package bbs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanMailWriteNeedsTheLeader pins the owner-approved divergence of
// #5148: a clan member who does not lead the clan cannot send the clan mail
// by writing it straight to the board; it is refused as the mail form
// refuses it, with ONLY_THE_CLAN_LEADER_IS_ENABLED and the clan list, and
// no mail is stored.
func TestClanMailWriteNeedsTheLeader(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, ""))
	p.enterAll(t)
	p.joinWolves(t)

	assertLeaderOnly(t, write(t, p.bobby, "_bbsclan", "mail", clanID, "x", "Muster", "Tonight"), "member's clan mail")
	assertSilent(t, p.alice, "leader after the member's clan mail")
	if rows := mailRows(t, p.srv); len(rows) != 0 {
		t.Fatalf("bbs_mail rows after the member's clan mail = %+v, want none", rows)
	}
}
