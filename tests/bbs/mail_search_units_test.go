package bbs

import (
	"regexp"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

var listedSubject = regexp.MustCompile(`_bbsmail;view;\d+">([^<]*)</a>`)

// TestMailTitleSearchMatchesByUnit pins the folder search matching client
// text unit by unit after lower-casing, an unpaired surrogate as its own
// unit. "\xED\xA0\xBD" and "\xED\xA0\xBE" spell the lone units D83D and
// D83E, which the test client sends as those single units. A search for
// D83D finds the subject holding it and the one holding the pair D83D
// DE00; a search for D83E, or for three U+FFFD, finds neither; the case of
// the letters around a surrogate is ignored.
func TestMailTitleSearchMatchesByUnit(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	const (
		lone = "a\xED\xA0\xBDb"
		pair = "x\U0001F600y"
	)
	for _, subject := range []string{lone, pair, "plain"} {
		if frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", subject, "m"); len(frames) != 4 {
			t.Fatalf("send %q answer = %x, want SENT_MAIL then the sent box", subject, opcodes(frames))
		}
		drainFrames(t, p.bobby)
	}

	for _, tc := range []struct {
		search string
		want   []string
	}{
		{"\xED\xA0\xBD", []string{lone, pair}},
		{"\xED\xA0\xBE", nil},
		{"���", nil},
		{"A\xED\xA0\xBDB", []string{lone}},
		{"X\U0001F600", []string{pair}},
	} {
		page := pageOf(t, write(t, p.alice, "Mail", "Search;sentbox", "0", "Title", "Title", tc.search))
		var got []string
		for _, m := range listedSubject.FindAllStringSubmatch(page, -1) {
			got = append(got, m[1])
		}
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("title search % X lists %q, want %q", tc.search, got, want)
		}
	}
}
