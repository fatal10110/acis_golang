package bbs

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The memo board's page pieces, as the reference builds them.
const (
	memoListHead = `<html><body><br><br><table border=0 width=610><tr><td width=10></td><td width=600 align=left><a action="bypass _bbshome">HOME</a>&nbsp;>&nbsp;<a action="bypass _bbsmemo">Memo Form</a></td></tr></table><img src="L2UI.squareblank" width="1" height="10"><center><table border=0 cellspacing=0 cellpadding=2 bgcolor=888888 width=610><tr><td FIXWIDTH=5></td><td FIXWIDTH=415 align=center>&$413;</td><td FIXWIDTH=120 align=center></td><td FIXWIDTH=70 align=center>&$418;</td></tr></table>`
	memoListBar  = `<br><table width=610 cellspace=0 cellpadding=0><tr><td width=50><button value="&$422;" action="bypass _bbsmemo" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"></td><td width=510 align=center><table border=0><tr>`
	memoPrevOff  = `<td><button action="" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16 ></td>`
	memoNextOff  = `<td><button action="" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td>`
)

// memoTopicRow is one topic of a memo topic list.
func memoTopicRow(forum, topic int, name, date string) string {
	return `<table border=0 cellspacing=0 cellpadding=5 WIDTH=610><tr><td FIXWIDTH=5></td><td FIXWIDTH=415><a action="bypass _bbsposts;read;` +
		strconv.Itoa(forum) + ";" + strconv.Itoa(topic) + `">` + name + `</a></td><td FIXWIDTH=120 align=center></td><td FIXWIDTH=70 align=center>` +
		date + `</td></tr></table><img src="L2UI.Squaregray" width="610" height="1">`
}

// memoPageLink is a link to page n of forum's topic list, as the pager
// and the previous/next buttons write it.
func memoPageLink(forum, n int) string {
	return `<td><a action="bypass _bbstopics;read;` + strconv.Itoa(forum) + ";" + strconv.Itoa(n) + `"> ` + strconv.Itoa(n) + " </a></td>"
}

func memoPrev(forum, n int) string {
	return `<td><button action="bypass _bbstopics;read;` + strconv.Itoa(forum) + ";" + strconv.Itoa(n) + `" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16 ></td>`
}

func memoNext(forum, n int) string {
	return `<td><button action="bypass _bbstopics;read;` + strconv.Itoa(forum) + ";" + strconv.Itoa(n) + `" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td>`
}

// memoListTail closes forum's topic list.
func memoListTail(forum int) string {
	return `</tr></table></td><td align=right><button value = "&$421;" action="bypass _bbstopics;crea;` + strconv.Itoa(forum) +
		`" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td></td><td align=center><table border=0><tr><td></td><td><edit var = "Search" width=130 height=11></td><td><button value="&$420;" action="Write 5 -2 0 Search _ _" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"></td></tr></table></td></tr></table><br><br><br></center></body></html>`
}

// assertLongPage asserts frames are one board page split over its three
// parts, 101, 102 and 103, an unused part carrying "null", and that the
// parts make want.
func assertLongPage(t *testing.T, frames [][]byte, want string) {
	t.Helper()
	if len(frames) != 3 {
		t.Fatalf("board answer = %x, want three ShowBoard parts", opcodes(frames))
	}
	var got strings.Builder
	for i, id := range []string{"101", "102", "103"} {
		part, ok := strings.CutPrefix(boardPart(t, frames[i]), id+"\b")
		if !ok {
			t.Fatalf("part %d is not %s", i, id)
		}
		if part != "null" {
			got.WriteString(part)
		}
	}
	if got.String() != want {
		t.Fatalf("board page = %q, want %q", got.String(), want)
	}
}

// editFields is the 1002 part filling a board form for the viewer.
func editFields(name string, id int32, account, text, title, date string) string {
	var b strings.Builder
	b.WriteString("1002\b")
	for _, f := range []string{"0", "0", "0", "0", "0", "0", name, strconv.Itoa(int(id)), account, "9", title, title, text, date, date, "0", "0"} {
		b.WriteString(f + " \b")
	}
	return b.String()
}

// forumRow is one bbs_forum row.
type forumRow struct {
	id     int
	typ    string
	access string
	owner  int32
}

func forumRows(t *testing.T, srv *gameservertest.Server) []forumRow {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT id, type, access, owner_id FROM bbs_forum ORDER BY id")
	if err != nil {
		t.Fatalf("read bbs_forum: %v", err)
	}
	defer rows.Close()
	var out []forumRow
	for rows.Next() {
		var r forumRow
		if err := rows.Scan(&r.id, &r.typ, &r.access, &r.owner); err != nil {
			t.Fatalf("scan bbs_forum: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// topicPostRows reads every topic of forum joined with its post 0 as
// "id|name|owner|text".
func topicPostRows(t *testing.T, srv *gameservertest.Server, forum int) []string {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(), `SELECT t.id, t.name, t.owner_name, COALESCE(p.txt, '<none>')
		FROM bbs_topic t LEFT JOIN bbs_post p ON p.forum_id = t.forum_id AND p.topic_id = t.id AND p.id = 0
		WHERE t.forum_id = ? ORDER BY t.id`, forum)
	if err != nil {
		t.Fatalf("read bbs_topic: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			id                int
			name, owner, text string
		)
		if err := rows.Scan(&id, &name, &owner, &text); err != nil {
			t.Fatalf("scan bbs_topic: %v", err)
		}
		out = append(out, strconv.Itoa(id)+"|"+name+"|"+owner+"|"+text)
	}
	return out
}

// shortDate is ms as the topic list writes it: the US short date and time,
// AM/PM after a narrow no-break space.
func shortDate(ms int64) string {
	at := time.UnixMilli(ms).Local()
	hour := at.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	half := "AM"
	if at.Hour() >= 12 {
		half = "PM"
	}
	return strconv.Itoa(int(at.Month())) + "/" + strconv.Itoa(at.Day()) + "/" + strconv.Itoa(at.Year()%100) + ", " +
		strconv.Itoa(hour) + ":" + twoDigits(at.Minute()) + "\u202f" + half
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// storedTopicDate reads the creation date of topic id of forum.
func storedTopicDate(t *testing.T, srv *gameservertest.Server, forum, id int) int64 {
	t.Helper()
	srv.FlushPersistence(t)
	var ms int64
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT date FROM bbs_topic WHERE forum_id = ? AND id = ?", forum, id).Scan(&ms); err != nil {
		t.Fatalf("read topic date: %v", err)
	}
	return ms
}

// TestMemoBoard walks the memo board: _bbsmemo creates the player's memo
// forum and shows its empty topic list; the new topic form, a topic
// written through it, its post page, its edit form and an edit, then its
// deletion, each stored.
func TestMemoBoard(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	// No clan exists: 0 topics fill 0 pages, which equals the clan count,
	// so the pager has no page and its next button links to page 2.
	emptyList := memoListHead + memoListBar + memoPrevOff + memoNext(1, 2) + memoListTail(1)
	assertPage(t, command(t, p.alice, "_bbsmemo"), emptyList)
	if got := forumRows(t, p.srv); len(got) != 1 || got[0] != (forumRow{1, "MEMO", "ALL", p.aliceID}) {
		t.Fatalf("bbs_forum = %+v, want Alice's memo forum 1", got)
	}
	// A second visit reuses the forum.
	assertPage(t, command(t, p.alice, "_bbsmemo"), emptyList)

	frames := command(t, p.alice, "_bbstopics;crea;1")
	if len(frames) != 2 {
		t.Fatalf("new topic form = %x, want the 1001 form and its fields", opcodes(frames))
	}
	form := boardPart(t, frames[0])
	if !strings.HasPrefix(form, "1001\b<html>") || !strings.Contains(form, `action="Write Topic crea 1 Title Content Title"`) {
		t.Fatalf("new topic form = %q", form)
	}
	if got, want := boardPart(t, frames[1]), editFields("Alice", p.aliceID, "player1", " ", " ", "0"); got != want {
		t.Fatalf("new topic fields = %q, want %q", got, want)
	}

	// Write Topic crea <forum> Title Content Title: the topic is named by
	// the fifth argument and holds the fourth.
	frames = write(t, p.alice, "Topic", "crea", "1", "ignored", "Line one\n<b>two</b>", "First")
	date := storedTopicDate(t, p.srv, 1, 1)
	oneTopic := memoListHead + memoTopicRow(1, 1, "First", shortDate(date)) + memoListBar + memoPrevOff + memoNext(1, 2) + memoListTail(1)
	assertPage(t, frames, oneTopic)
	if got := topicPostRows(t, p.srv, 1); len(got) != 1 || got[0] != "1|First|Alice|Line one\n<b>two</b>" {
		t.Fatalf("topics = %q, want topic 1 with its post", got)
	}

	page := pageOf(t, command(t, p.alice, "_bbsposts;read;1;1"))
	for _, want := range []string{
		`<td fixWIDTH=380 valign=top>First</td>`,
		`<td><font color="AAAAAA">Alice</font></td>`,
		`<td><font color="AAAAAA">` + time.UnixMilli(date).Local().Format("2006-01-02 15:04:05") + `</font></td>`,
		`<td FIXWIDTH=600 align=left>Line one<br1>&lt;b&gt;two&lt;/b&gt;</td>`,
		`action="bypass _bbsposts;edit;1;1;0"`, `action="bypass _bbstopics;del;1;1"`, `action="bypass _bbstopics;crea;1"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("post page lacks %q:\n%s", want, page)
		}
	}

	frames = command(t, p.alice, "_bbsposts;edit;1;1")
	if len(frames) != 2 {
		t.Fatalf("edit form = %x, want the 1001 form and its fields", opcodes(frames))
	}
	if form := boardPart(t, frames[0]); !strings.Contains(form, `<td FIXWIDTH=540>First</td>`) || !strings.Contains(form, `action="Write Post 1;1;0 _ Content Content Content"`) {
		t.Fatalf("edit form = %q", form)
	}
	if got, want := boardPart(t, frames[1]), editFields("Alice", p.aliceID, "player1", "Line one\n<b>two</b>", "First", shortDate(date)); got != want {
		t.Fatalf("edit fields = %q, want %q", got, want)
	}

	// Write Post <forum>;<topic>;<post> _ Content Content Content: the
	// fourth argument is the new text; the post page follows.
	page = pageOf(t, write(t, p.alice, "Post", "1;1;0", "_", "x", "Edited", "y"))
	if !strings.Contains(page, `<td FIXWIDTH=600 align=left>Edited</td>`) {
		t.Fatalf("post page after edit lacks the new text:\n%s", page)
	}
	if got := topicPostRows(t, p.srv, 1); len(got) != 1 || got[0] != "1|First|Alice|Edited" {
		t.Fatalf("topics after edit = %q", got)
	}

	// The search button's form names "5" as its board.
	assertPage(t, write(t, p.alice, "5", "-2", "0", "query", "_", "_"), "<html><body><br><br><center>The command: 5 isn't implemented.</center></body></html>")

	assertPage(t, command(t, p.alice, "_bbstopics;del;1;1"), emptyList)
	if got := topicPostRows(t, p.srv, 1); len(got) != 0 {
		t.Fatalf("topics after delete = %q, want none", got)
	}
	var posts int
	if err := p.srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM bbs_post").Scan(&posts); err != nil || posts != 0 {
		t.Fatalf("bbs_post rows = %d (%v), want none", posts, err)
	}

	// A deleted topic's id is not given out again before a restart.
	write(t, p.alice, "Topic", "crea", "1", "", "again", "Second")
	if got := topicPostRows(t, p.srv, 1); len(got) != 1 || !strings.HasPrefix(got[0], "2|Second|") {
		t.Fatalf("topics = %q, want topic 2", got)
	}
}

// TestMemoBoardRejections pins the pages a memo command or form naming a
// missing forum, topic or post shows, the unknown-command pages of the
// memo board, and the silence of a command whose numbers do not read.
func TestMemoBoardRejections(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)
	command(t, p.alice, "_bbsmemo")
	write(t, p.alice, "Topic", "crea", "1", "", "text", "Topic")

	page := func(body string) string { return "<html><body><br><br><center>" + body + "</center></body></html>" }
	for _, c := range []struct{ cmd, want string }{
		{"_bbstopics;read;9", page("The forum #9 doesn't exist.")},
		{"_bbstopics;crea;9", page("The forum #9 doesn't exist.")},
		{"_bbstopics;del;9;1", page("The forum #9 doesn't exist.")},
		{"_bbstopics;del;1;7", page("The topic #7 doesn't exist.")},
		{"_bbsposts;read;9;1", page("This forum doesn't exist.")},
		{"_bbsposts;read;1;7", page("This topic doesn't exist.")},
		{"_bbsposts;edit;9;1", page("This forum doesn't exist.")},
		{"_bbsposts;edit;1;7", page("This topic doesn't exist.")},
		{"_bbsmemo;x", page("The command: _bbsmemo;x isn't implemented.")},
		{"_bbstopics;view;1", page("The command: _bbstopics;view;1 isn't implemented.")},
		{"_bbsposts;view;1;1", page("The command: _bbsposts;view;1;1 isn't implemented.")},
		{"_bbsposts;read", page("The command: _bbsposts;read isn't implemented.")},
	} {
		assertPage(t, command(t, p.alice, c.cmd), c.want)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"crea", "9", "", "text", "name"}, page("The forum named '9' doesn't exist.")},
		{[]string{"del", "9", "junk"}, page("The forum named '9' doesn't exist.")},
		{[]string{"del", "1", "+7"}, page("The topic named '+7' doesn't exist.")},
		{[]string{"edit", "1"}, page("The command: edit isn't implemented.")},
	} {
		assertPage(t, write(t, p.alice, "Topic", c.args...), c.want)
	}
	for _, c := range []struct {
		arg, want string
	}{
		{"9;1;0", page("The forum named '9' doesn't exist.")},
		{"1;+7;0", page("The topic named '7' doesn't exist.")},
		{"1;1;3", page("The post named '3' doesn't exist.")},
	} {
		assertPage(t, write(t, p.alice, "Post", c.arg, "_", "x", "text", "x"), c.want)
	}
	for _, cmd := range []string{"_bbstopics;read;x", "_bbstopics;read", "_bbstopics;del;1", "_bbsposts;read;1", "_bbsposts;edit;1;x"} {
		if frames := command(t, p.alice, cmd); len(frames) != 0 {
			t.Fatalf("%s answer = %x, want silence", cmd, opcodes(frames))
		}
	}
	for _, args := range [][]string{{"crea", "x"}, {"del", "1", "x"}} {
		if frames := write(t, p.alice, "Topic", args...); len(frames) != 0 {
			t.Fatalf("Topic %q answer = %x, want silence", args, opcodes(frames))
		}
	}
	if frames := write(t, p.alice, "Post", "1;1", "_", "x", "text"); len(frames) != 0 {
		t.Fatalf("short post form answer = %x, want silence", opcodes(frames))
	}
	if got := topicPostRows(t, p.srv, 1); len(got) != 1 || got[0] != "1|Topic|Alice|text" {
		t.Fatalf("topics = %q, want the one topic untouched", got)
	}
}

// seedMemoForum stores forum 3, Alice's memo forum, with n topics, ids 1
// to n, each dated at, and the clan forums a level 2 clan would have.
func seedMemoForum(t *testing.T, n int, at time.Time) gameservertest.Option {
	return gameservertest.WithBoardSeed(func(db *sql.DB) {
		ctx := context.Background()
		if _, err := db.ExecContext(ctx, `INSERT INTO bbs_forum (id, type, access, owner_id) SELECT 3, 'MEMO', 'ALL', obj_Id FROM characters WHERE char_name = 'Alice'`); err != nil {
			t.Fatalf("seed forum: %v", err)
		}
		for id := 1; id <= n; id++ {
			if _, err := db.ExecContext(ctx, `INSERT INTO bbs_topic (id, forum_id, name, date, owner_name, owner_id) VALUES (?, 3, ?, ?, 'Alice', 1)`, id, "T"+strconv.Itoa(id), at.UnixMilli()); err != nil {
				t.Fatalf("seed topic: %v", err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO bbs_post (id, owner_name, owner_id, date, topic_id, forum_id, txt) VALUES (0, 'Alice', 1, ?, ?, 3, ?)`, at.UnixMilli(), id, "text "+strconv.Itoa(id)); err != nil {
				t.Fatalf("seed post: %v", err)
			}
		}
	})
}

// TestMemoTopicListPages pins the stored memo forum restored at boot and
// its topic list pages: 12 topics a page, newest first, dated short; the
// page count is the topic count over 8, plus one unless that times 8
// equals the clan count; the new topic takes the id after the highest.
func TestMemoTopicListPages(t *testing.T) {
	at := time.Date(2026, 1, 5, 14, 7, 0, 0, time.Local)
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedMemoForum(t, 13, at))
	p.enterAll(t)

	const date = "1/5/26, 2:07\u202fPM"
	var first strings.Builder
	first.WriteString(memoListHead)
	for id := 13; id >= 2; id-- {
		first.WriteString(memoTopicRow(3, id, "T"+strconv.Itoa(id), date))
	}
	// 13 topics over 8 is 1 page; 1 times 8 is not 0 clans, so 2 pages.
	first.WriteString(memoListBar + memoPrevOff + "<td> 1 </td>" + memoPageLink(3, 2) + memoNext(3, 2) + memoListTail(3))
	// The 12 topics take the page past one part.
	assertLongPage(t, command(t, p.alice, "_bbsmemo"), first.String())
	assertLongPage(t, command(t, p.alice, "_bbstopics;read;3"), first.String())
	assertLongPage(t, command(t, p.alice, "_bbstopics;read;3;1"), first.String())

	second := memoListHead + memoTopicRow(3, 1, "T1", date) + memoListBar + memoPrev(3, 1) + memoPageLink(3, 1) + "<td> 2 </td>" + memoNextOff + memoListTail(3)
	assertPage(t, command(t, p.alice, "_bbstopics;read;3;2"), second)

	if got := pageOf(t, command(t, p.alice, "_bbsposts;read;3;7")); !strings.Contains(got, `<td FIXWIDTH=600 align=left>text 7</td>`) {
		t.Fatalf("restored post page lacks its text:\n%s", got)
	}

	// Bobby reaches Alice's memo forum by its id, as the reference allows.
	assertPage(t, command(t, p.bobby, "_bbstopics;read;3;2"), second)

	write(t, p.alice, "Topic", "crea", "3", "", "new", "T14")
	if got := topicPostRows(t, p.srv, 3); len(got) != 14 || got[13] != "14|T14|Alice|new" {
		t.Fatalf("topics = %q, want topic 14 last", got)
	}
	if got := forumRows(t, p.srv); len(got) != 1 {
		t.Fatalf("bbs_forum = %+v, want the seeded memo forum alone", got)
	}
}

// TestClanForums pins the clan forums: a level 2 clan gets its
// announcement and bulletin forums, read-only, at boot, after the stored
// forums; the clan management page shows each forum's stored access.
func TestClanForums(t *testing.T) {
	seed := gameservertest.WithBoardSeed(func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `INSERT INTO bbs_forum (id, type, access, owner_id) VALUES (4, 'CLAN_ANN', 'ALL', ?)`, seededClanID); err != nil {
			t.Fatalf("seed forum: %v", err)
		}
	})
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, ""), seed)
	p.enterAll(t)

	want := []forumRow{{4, "CLAN_ANN", "ALL", seededClanID}, {5, "CLAN_CBB", "READ", seededClanID}}
	if got := forumRows(t, p.srv); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("bbs_forum = %+v, want %+v", got, want)
	}
	frames := command(t, p.alice, "_bbsclan;management;"+clanID)
	if got, want := boardPart(t, frames[0]), "1001\bMANAGE "+clanID+" All access/All access/Read access/Read access\n"; got != want {
		t.Fatalf("management form = %q, want %q", got, want)
	}

	// A clan forum is no memo forum: its topics neither list nor show.
	page := func(body string) string { return "<html><body><br><br><center>" + body + "</center></body></html>" }
	assertPage(t, command(t, p.alice, "_bbstopics;read;4"), page("The forum #4 doesn't exist."))
	write(t, p.alice, "Topic", "crea", "4", "", "text", "Notice")
	assertPage(t, command(t, p.alice, "_bbsposts;read;4;1"), page("The forum is off-limits."))
}

// TestMemoRestoreStopsAtOrphanPost pins the boot load of the posts: it
// stops at the first post whose topic its forum does not hold, so no post
// after it is restored. Here that post comes first, and topic 1 is left
// without its text: its post page shows nothing and its edit form names
// the post missing.
func TestMemoRestoreStopsAtOrphanPost(t *testing.T) {
	seed := gameservertest.WithBoardSeed(func(db *sql.DB) {
		for _, q := range []string{
			`INSERT INTO bbs_forum (id, type, access, owner_id) SELECT 3, 'MEMO', 'ALL', obj_Id FROM characters WHERE char_name = 'Alice'`,
			`INSERT INTO bbs_topic (id, forum_id, name, date, owner_name, owner_id) VALUES (1, 3, 'Kept', 0, 'Alice', 1)`,
			`INSERT INTO bbs_post (id, owner_name, owner_id, date, topic_id, forum_id, txt) VALUES (-1, 'Alice', 1, 0, 9, 3, 'orphan')`,
			`INSERT INTO bbs_post (id, owner_name, owner_id, date, topic_id, forum_id, txt) VALUES (0, 'Alice', 1, 0, 1, 3, 'text')`,
		} {
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seed)
	p.enterAll(t)

	if frames := command(t, p.alice, "_bbsposts;read;3;1"); len(frames) != 0 {
		t.Fatalf("post page of a topic without its post = %x, want silence", opcodes(frames))
	}
	assertPage(t, command(t, p.alice, "_bbsposts;edit;3;1"), "<html><body><br><br><center>This post doesn't exist.</center></body></html>")
}
