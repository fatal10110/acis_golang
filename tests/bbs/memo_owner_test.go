package bbs

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMemoForumAnswersOnlyItsOwner pins the memo board's ownership check,
// a deliberate hardening over the reference, which serves any forum a
// player names (#3262): every topic and post command or form Bobby aims
// at Alice's memo forum, or at a clan forum, shows the page of a missing
// forum and changes nothing, while Alice's own forum and Bobby's work as
// before.
func TestMemoForumAnswersOnlyItsOwner(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, ""))
	p.enterAll(t)

	command(t, p.alice, "_bbsmemo")
	write(t, p.alice, "Topic", "crea", "3", "", "secret", "Mine")
	forums := forumRows(t, p.srv)
	want := []forumRow{{1, "CLAN_ANN", "READ", seededClanID}, {2, "CLAN_CBB", "READ", seededClanID}, {3, "MEMO", "ALL", p.aliceID}}
	if len(forums) != len(want) || forums[0] != want[0] || forums[1] != want[1] || forums[2] != want[2] {
		t.Fatalf("bbs_forum = %+v, want %+v", forums, want)
	}

	page := func(body string) string { return "<html><body><br><br><center>" + body + "</center></body></html>" }
	for _, cmd := range []string{"_bbstopics;read;3", "_bbstopics;read;3;1", "_bbstopics;crea;3", "_bbstopics;del;3;1", "_bbstopics;del;3;7"} {
		assertPage(t, command(t, p.bobby, cmd), page("The forum #3 doesn't exist."))
	}
	for _, cmd := range []string{"_bbsposts;read;3;1", "_bbsposts;edit;3;1", "_bbsposts;read;3;7", "_bbsposts;edit;3;7"} {
		assertPage(t, command(t, p.bobby, cmd), page("This forum doesn't exist."))
	}
	for _, args := range [][]string{
		{"crea", "3", "", "spam", "Bobby's"},
		{"del", "3", "1"},
		{"crea", "1", "", "spam", "Announcement"},
		{"crea", "2", "", "spam", "Bulletin"},
	} {
		assertPage(t, write(t, p.bobby, "Topic", args...), page("The forum named '"+args[1]+"' doesn't exist."))
	}
	assertPage(t, write(t, p.bobby, "Post", "3;1;0", "_", "x", "rewritten", "x"), page("The forum named '3' doesn't exist."))

	if got := topicPostRows(t, p.srv, 3); len(got) != 1 || got[0] != "1|Mine|Alice|secret" {
		t.Fatalf("Alice's topics = %q, want her one topic untouched", got)
	}
	for _, forum := range []int{1, 2} {
		if got := topicPostRows(t, p.srv, forum); len(got) != 0 {
			t.Fatalf("clan forum %d topics = %q, want none", forum, got)
		}
	}

	// Alice's flows are unchanged.
	if got := pageOf(t, command(t, p.alice, "_bbsposts;read;3;1")); !strings.Contains(got, `<td FIXWIDTH=600 align=left>secret</td>`) {
		t.Fatalf("Alice's post page lacks her text:\n%s", got)
	}
	if got := pageOf(t, write(t, p.alice, "Post", "3;1;0", "_", "x", "edited", "x")); !strings.Contains(got, `<td FIXWIDTH=600 align=left>edited</td>`) {
		t.Fatalf("Alice's post page after her edit lacks the new text:\n%s", got)
	}

	// Bobby's own memo forum, the next one, takes his topics.
	command(t, p.bobby, "_bbsmemo")
	write(t, p.bobby, "Topic", "crea", "4", "", "mine", "Bobby's")
	if got := topicPostRows(t, p.srv, 4); len(got) != 1 || got[0] != "1|Bobby's|Bobby|mine" {
		t.Fatalf("Bobby's topics = %q, want his one topic", got)
	}
	if got := forumRows(t, p.srv); len(got) != 4 || got[3] != (forumRow{4, "MEMO", "ALL", p.bobbyID}) {
		t.Fatalf("bbs_forum = %+v, want Bobby's memo forum 4", got)
	}
}
