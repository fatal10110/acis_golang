package bbs

import (
	"regexp"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Evidence for the mail writer search (MailBBSManager.getMailCount,
// MailBBSManager.java:486-516): any search type but "title" is a search
// of the sender's name, matched as a lower-cased substring.
//
// Two items of the parity sweep have no scenario because the reference's
// order is that of a Java hash map keyed by object id, which no fixture
// can pin without reproducing the hash function: the friend and block
// boards past 12 ids, and the clan list and clan-hall rows. The existing
// scenarios fix every order the reference sorts explicitly.

// unreadSubject matches a subject listed as unread, as the inbox shows a
// mail not yet opened.
var unreadSubject = regexp.MustCompile(`_bbsmail;view;\d+"><font color="LEVEL">([^<]*)</font>`)

// TestMailWriterSearchMatchesSenderName has Alice mail Bobby twice and
// Bobby search his inbox by writer: a substring of "Alice" in any case
// lists both mails, his own name matches none, and a title search for the
// same text matches no subject.
func TestMailWriterSearchMatchesSenderName(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	for _, subject := range []string{"first", "second"} {
		if frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", subject, "m"); len(frames) != 4 {
			t.Fatalf("send %q answer = %x, want SENT_MAIL then the sent box", subject, opcodes(frames))
		}
		drainFrames(t, p.bobby)
	}

	for _, tc := range []struct {
		searchType, search string
		want               []string
	}{
		{"Writer", "LIC", []string{"first", "second"}},
		{"Writer", "alice", []string{"first", "second"}},
		{"Sender", "al", []string{"first", "second"}},
		{"Writer", "bobby", nil},
		{"Title", "alice", nil},
	} {
		page := pageOf(t, write(t, p.bobby, "Mail", "Search;inbox", "0", tc.searchType, tc.searchType, tc.search))
		var got []string
		for _, m := range unreadSubject.FindAllStringSubmatch(page, -1) {
			got = append(got, m[1])
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%s search %q lists %q, want %q", tc.searchType, tc.search, got, tc.want)
		}
	}
}
