package bbs

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// joinWolves makes Bobby a member of Alice's clan through the join
// request and its answer.
func (p *pair) joinWolves(t *testing.T) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinPledge)
	w.WriteInt32(p.bobbyID)
	w.WriteInt32(0)
	p.alice.Send(w.Bytes())
	drainFrames(t, p.bobby)
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinPledge)
	answer.WriteInt32(1)
	p.bobby.Send(answer.Bytes())
	joined := drainFrames(t, p.bobby)
	drainFrames(t, p.alice)
	if got := pageOf(t, command(t, p.bobby, "_bbsclan")); got != "MEMBER "+clanID+" Wolves\n" {
		t.Fatalf("Bobby's clan home after joining = %q (join frames %x), want the member page", got, opcodes(joined))
	}
}

// assertLeaderOnly asserts frames are the management page's refusal of a
// player who does not lead the clan: ONLY_THE_CLAN_LEADER_IS_ENABLED, then
// the clan list.
func assertLeaderOnly(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	if len(frames) != 4 {
		t.Fatalf("%s answer = %x, want the system message and the clan list", what, opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageOnlyClanLeaderEnabled)
	if got := pageOf(t, frames[1:]); !strings.HasPrefix(got, "CLANS ") {
		t.Fatalf("%s page = %q, want the clan list", what, got)
	}
}

// TestClanBoardMemberWritesRefused pins the owner-approved divergence of
// #4653: a clan member who does not lead the clan cannot switch the login
// notice, write the notice or write the introduction; each is refused as
// the management page refuses them and the clan row is kept. The leader's
// writes still land.
func TestClanBoardMemberWritesRefused(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, "old notice"))
	p.enterAll(t)
	p.joinWolves(t)

	assertLeaderOnly(t, command(t, p.bobby, "_bbsclan;notice;true"), "member's notice switch")
	assertLeaderOnly(t, write(t, p.bobby, "_bbsclan", "notice", "0", "hijacked", "hijacked", "hijacked"), "member's notice write")
	assertLeaderOnly(t, write(t, p.bobby, "_bbsclan", "intro", clanID, "hijacked", "x", "x"), "member's intro write")
	if enabled, notice, intro := clanRow(t, p.srv); enabled || notice != "old notice" || intro != "We hunt." {
		t.Fatalf("clan board columns after the member's writes = (%v, %q, %q), want them untouched", enabled, notice, intro)
	}

	// The member still reads the notice settings.
	if frames := command(t, p.bobby, "_bbsclan;notice"); len(frames) != 2 || !strings.HasPrefix(boardPart(t, frames[0]), "1001\bNOTICE "+clanID) {
		t.Fatalf("member's notice page = %x, want the 1001 form and its fields", opcodes(frames))
	}

	command(t, p.alice, "_bbsclan;notice;true")
	write(t, p.alice, "_bbsclan", "notice", "0", "new notice", "new notice", "new notice")
	write(t, p.alice, "_bbsclan", "intro", clanID, "Hunting since dawn.", "x", "x")
	if enabled, notice, intro := clanRow(t, p.srv); !enabled || notice != "new notice" || intro != "Hunting since dawn." {
		t.Fatalf("clan board columns after the leader's writes = (%v, %q, %q), want them written", enabled, notice, intro)
	}
}

// TestClanBoardIntroShowsAsText pins the owner-approved divergence of
// #4654: an introduction carrying a link reaches every viewer, a game
// master too, with its markup escaped, so the page holds no live action;
// the stored text is kept as written. Clicking the planted admin link
// runs nothing: no validated page offered it.
func TestClanBoardIntroShowsAsText(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, ""))
	p.setAccessLevel(t, p.bobbyID, 1)
	p.enterAll(t)

	const planted = `<a action="bypass -h admin_admin">Click</a>`
	write(t, p.alice, "_bbsclan", "intro", clanID, planted, "x", "x")
	if _, _, intro := clanRow(t, p.srv); intro != planted {
		t.Fatalf("stored introduction = %q, want it as written", intro)
	}
	const shown = `&lt;a action=&quot;bypass -h admin_admin&quot;&gt;Click&lt;/a&gt;`
	if got, want := pageOf(t, command(t, p.alice, "_bbsclan")), "LEADER "+clanID+" Wolves 2 1 Alice ally= intro="+shown+"\n"; got != want {
		t.Fatalf("leader home = %q, want %q", got, want)
	}
	home := pageOf(t, command(t, p.bobby, "_bbsclan;home;"+clanID))
	if want := "VISITOR " + clanID + " Wolves 2 1 Alice ally= intro=" + shown + "\n"; home != want {
		t.Fatalf("game master's view of the clan home = %q, want %q", home, want)
	}
	if strings.Contains(home, `action="`) {
		t.Fatalf("clan home %q carries a live action", home)
	}
	p.bobby.Send(encodeBypass("admin_admin"))
	assertSilent(t, p.bobby, "planted admin link")
}
