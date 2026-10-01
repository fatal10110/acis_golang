package bbs

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// clanID is the seeded clan's id as the board links name it.
var clanID = strconv.Itoa(seededClanID)

// clanRow reads the seeded clan's board columns once the queued writes
// have landed.
func clanRow(t *testing.T, srv *gameservertest.Server) (enabled bool, notice, intro string) {
	t.Helper()
	srv.FlushPersistence(t)
	var n, i sql.NullString
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT enabled, notice, introduction FROM clan_data WHERE clan_id = ?", seededClanID).Scan(&enabled, &n, &i); err != nil {
		t.Fatalf("read clan_data: %v", err)
	}
	return enabled, n.String, i.String
}

// clanListRow is one clan of the clan list.
func clanListRow(id, name, leader string, level, members int) string {
	return `<table width=610><tr><td width=5></td><td width=150 align=center><a action="bypass _bbsclan;home;` + id + `">` + name +
		`</a></td><td width=150 align=center>` + leader + `</td><td width=100 align=center>` + strconv.Itoa(level) +
		`</td><td width=200 align=center>` + strconv.Itoa(members) + `</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`
}

// singlePageClanPager is the clan list's page links with one page shown.
const singlePageClanPager = `<table><tr><td><button action="" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16></td>` +
	`<td> 1 </td><td><button action="" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16></td></tr></table>`

// TestClanBoardLeader walks the clan board as the leader of a level 2
// clan: _bbsclan opens the leader's home page; the notice page shows the
// notice settings in a 1001 form filled with the notice text; switching
// the notice on and editing it are stored; the management page's form is
// filled with the introduction, and editing it is stored; the mail form
// names the clan.
func TestClanBoardLeader(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, "old notice"))
	p.enterAll(t)

	if got, want := pageOf(t, command(t, p.alice, "_bbsclan")), "LEADER "+clanID+" Wolves 2 1 Alice ally= intro=We hunt.\n"; got != want {
		t.Fatalf("leader home = %q, want %q", got, want)
	}

	frames := command(t, p.alice, "_bbsclan;notice;true")
	if len(frames) != 2 {
		t.Fatalf("notice page = %x, want the 1001 form and its fields", opcodes(frames))
	}
	if got, want := boardPart(t, frames[0]), "1001\bNOTICE "+clanID+" enabled=[true] flag=false\n"; got != want {
		t.Fatalf("notice form = %q, want %q", got, want)
	}
	wantFields := "1002\b0 \b0 \b0 \b0 \b0 \b0 \bAlice \b" + strconv.Itoa(int(p.aliceID)) + " \bplayer1 \b9 \b \b \bold notice \b \b \b0 \b0 \b"
	if got := boardPart(t, frames[1]); got != wantFields {
		t.Fatalf("notice fields = %q, want %q", got, wantFields)
	}
	if enabled, notice, _ := clanRow(t, p.srv); !enabled || notice != "old notice" {
		t.Fatalf("clan notice = (%v, %q), want (true, old notice)", enabled, notice)
	}

	frames = write(t, p.alice, "_bbsclan", "notice", "0", "new notice", "new notice", "new notice")
	if got, want := boardPart(t, frames[0]), "1001\bNOTICE "+clanID+" enabled=[true] flag=false\n"; got != want {
		t.Fatalf("notice form after edit = %q, want %q", got, want)
	}
	if enabled, notice, _ := clanRow(t, p.srv); !enabled || notice != "new notice" {
		t.Fatalf("clan notice = (%v, %q), want (true, new notice)", enabled, notice)
	}

	frames = write(t, p.alice, "_bbsclan", "intro", clanID, "Hunting since dawn.", "x", "x")
	if len(frames) != 2 {
		t.Fatalf("management answer = %x, want the 1001 form and its fields", opcodes(frames))
	}
	if got, want := boardPart(t, frames[0]), "1001\bMANAGE "+clanID+" Read access/Read access/Read access/Read access\n"; got != want {
		t.Fatalf("management form = %q, want %q", got, want)
	}
	if _, _, intro := clanRow(t, p.srv); intro != "Hunting since dawn." {
		t.Fatalf("clan introduction = %q, want the new one", intro)
	}

	if got, want := pageOf(t, command(t, p.alice, "_bbsclan;mail;"+clanID)), "CLANMAIL "+clanID+" Wolves\n"; got != want {
		t.Fatalf("clan mail form = %q, want %q", got, want)
	}

	// The clan mail goes to every member: here the leader alone, who
	// cannot mail itself. The clan home follows.
	frames = write(t, p.alice, "_bbsclan", "mail", clanID, "x", "Muster", "Tonight")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	if got, want := pageOf(t, frames[1:]), "LEADER "+clanID+" Wolves 2 1 Alice ally= intro=Hunting since dawn.\n"; got != want {
		t.Fatalf("home after clan mail = %q, want %q", got, want)
	}

	// A permission link of the management page shows nothing.
	if frames := command(t, p.alice, "_bbsclan;permission;anno;non"); len(frames) != 0 {
		t.Fatalf("permission link answer = %x, want silence", opcodes(frames))
	}
}

// TestClanBoardVisitor pins a player outside the clan: _bbsclan opens the
// clan list, the clan's home shows the visitor's page, and the leader-only
// pages answer ONLY_THE_CLAN_LEADER_IS_ENABLED with the clan list. Its
// notice page and a form naming the clan show nothing.
func TestClanBoardVisitor(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, ""))
	p.enterAll(t)

	list := "CLANS bar= list=" + clanListRow(clanID, "Wolves", "Alice", 2, 1) + singlePageClanPager + "\n"
	if got := pageOf(t, command(t, p.bobby, "_bbsclan")); got != list {
		t.Fatalf("clan list = %q, want %q", got, list)
	}
	if got, want := pageOf(t, command(t, p.bobby, "_bbsclan;home;"+clanID)), "VISITOR "+clanID+" Wolves 2 1 Alice ally= intro=We hunt.\n"; got != want {
		t.Fatalf("visitor home = %q, want %q", got, want)
	}
	for _, cmd := range []string{"_bbsclan;management;" + clanID, "_bbsclan;mail;" + clanID} {
		frames := command(t, p.bobby, cmd)
		assertSystemMessage(t, frames[0], serverpackets.SystemMessageOnlyClanLeaderEnabled)
		if got := pageOf(t, frames[1:]); got != list {
			t.Fatalf("%s list = %q, want %q", cmd, got, list)
		}
	}
	for _, f := range [][][]byte{
		command(t, p.bobby, "_bbsclan;notice;true"),
		write(t, p.bobby, "_bbsclan", "intro", clanID, "x"),
		write(t, p.bobby, "_bbsclan", "notice", "0", "x", "x"),
	} {
		if len(f) != 0 {
			t.Fatalf("outsider clan command answer = %x, want silence", opcodes(f))
		}
	}
	if enabled, notice, intro := clanRow(t, p.srv); enabled || notice != "" || intro != "We hunt." {
		t.Fatalf("clan board columns = (%v, %q, %q), want them untouched", enabled, notice, intro)
	}
}

// TestClanBoardLowLevelClan pins a clan below level 2: its pages answer
// NO_CB_IN_MY_CLAN with the clan list, whose home bar links the player's
// clan.
func TestClanBoardLowLevelClan(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClanAt(t, 1, false, ""))
	p.enterAll(t)

	frames := command(t, p.alice, "_bbsclan")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageNoCommunityBoardInClan)
	want := "CLANS bar=" + `<table width=610 bgcolor=A7A19A><tr><td width=5></td><td width=605><a action="bypass _bbsclan;home;` + clanID + `">[GO TO MY CLAN]</a></td></tr></table>` +
		" list=" + clanListRow(clanID, "Wolves", "Alice", 1, 1) + singlePageClanPager + "\n"
	if got := pageOf(t, frames[1:]); got != want {
		t.Fatalf("clan list = %q, want %q", got, want)
	}
}
