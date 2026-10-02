package bbs

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seedSentMail files n read mails in Alice's sent box, ids 1 to n, sent two
// days ago so none counts against the daily limit; mail i's subject is
// "s<i>".
func seedSentMail(t *testing.T, n int) gameservertest.Option {
	return gameservertest.WithBoardSeed(func(db *sql.DB) {
		sent := time.Now().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
		for i := 1; i <= n; i++ {
			if _, err := db.ExecContext(context.Background(), `INSERT INTO bbs_mail (id, receiver_id, sender_id, location, recipients, subject, message, sent_date, is_unread)
				SELECT ?, obj_Id, obj_Id, 'sentbox', 'x', ?, 'm', ?, 0 FROM characters WHERE char_name = 'Alice'`, i, "s"+strconv.Itoa(i), sent); err != nil {
				t.Fatalf("seed mail %d: %v", i, err)
			}
		}
	})
}

var (
	pagerNumber  = regexp.MustCompile(`<td>(?:<a action="bypass _bbsmail;SENTBOX;(\d+)[^"]*"> )? ?(\d+) (?:</a>)?</td>`)
	pagerButton  = regexp.MustCompile(`<button action="bypass _bbsmail;SENTBOX;(\d+)" back="l2ui_ch3\.(prev|next)1_down"`)
	mailSubjects = regexp.MustCompile(`_bbsmail;view;\d+">(s\d+)</a>`)
)

// pager reads a folder page's previous and next targets, the page numbers
// it lists, the one shown unlinked, and the subjects it lists.
func pager(t *testing.T, page string) (prev, next int, numbers []int, shown int, subjects []string) {
	t.Helper()
	pagerHTML := page[strings.Index(page, "PAGER="):]
	for _, m := range pagerButton.FindAllStringSubmatch(pagerHTML, -1) {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "prev" {
			prev = n
		} else {
			next = n
		}
	}
	for _, m := range pagerNumber.FindAllStringSubmatch(pagerHTML, -1) {
		n, _ := strconv.Atoi(m[2])
		numbers = append(numbers, n)
		if m[1] == "" {
			shown = n
		}
	}
	for _, m := range mailSubjects.FindAllStringSubmatch(page, -1) {
		subjects = append(subjects, m[1])
	}
	return prev, next, numbers, shown, subjects
}

func span(from, to int) []int {
	var out []int
	for n := from; n <= to; n++ {
		out = append(out, n)
	}
	return out
}

func subjectsFrom(from, to int) []string {
	var out []string
	for n := from; n <= to; n++ {
		out = append(out, "s"+strconv.Itoa(n))
	}
	return out
}

// TestMailListPages pins a folder of 250 mails, 25 pages of ten: page 1
// lists mails 1-10 and page numbers 1-11; page 13 lists mails 121-130 and
// numbers 3-23; page 20, within ten of the last, numbers 10-25; the last
// page has no next. A page past the last shows the last.
func TestMailListPages(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedSentMail(t, 250))
	p.enterAll(t)

	for _, tc := range []struct {
		cmd        string
		shown      int
		prev, next int
		numbers    []int
		subjects   []string
	}{
		{"_bbsmail;sentbox", 1, 1, 2, span(1, 11), subjectsFrom(1, 10)},
		{"_bbsmail;sentbox;13", 13, 12, 14, span(3, 23), subjectsFrom(121, 130)},
		{"_bbsmail;sentbox;20", 20, 19, 21, span(10, 25), subjectsFrom(191, 200)},
		{"_bbsmail;sentbox;99", 25, 24, 25, span(15, 25), subjectsFrom(241, 250)},
	} {
		page := pageOf(t, command(t, p.alice, tc.cmd))
		prev, next, numbers, shown, subjects := pager(t, page)
		if shown != tc.shown || prev != tc.prev || next != tc.next ||
			strings.Join(strconvAll(numbers), ",") != strings.Join(strconvAll(tc.numbers), ",") ||
			strings.Join(subjects, ",") != strings.Join(tc.subjects, ",") {
			t.Fatalf("%s: shown %d prev %d next %d numbers %v subjects %v; want shown %d prev %d next %d numbers %v subjects %v",
				tc.cmd, shown, prev, next, numbers, subjects, tc.shown, tc.prev, tc.next, tc.numbers, tc.subjects)
		}
	}

	// A search lists only the matching mails, and its page links carry it.
	page := pageOf(t, write(t, p.alice, "Mail", "Search;sentbox", "0", "Title", "Title", "s12"))
	_, _, numbers, _, subjects := pager(t, page)
	if strings.Join(subjects, ",") != "s12,s120,s121,s122,s123,s124,s125,s126,s127,s128" || len(numbers) != 2 {
		t.Fatalf("title search s12: subjects %v, numbers %v; want s12 and s120-s128, two pages", subjects, numbers)
	}
	if !strings.Contains(page, `bypass _bbsmail;SENTBOX;2;Title;s12"`) || !strings.Contains(page, "sent=250 ") {
		t.Fatalf("search page does not carry the search in its links or the full counts: %q", page)
	}
	if got := pageOf(t, write(t, p.alice, "Mail", "Search;sentbox", "0", "Writer", "Writer", "nobody")); !strings.Contains(got, "ROWS= ") {
		t.Fatalf("writer search for nobody = %q, want no rows", got)
	}
}

func strconvAll(ns []int) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = strconv.Itoa(n)
	}
	return out
}
