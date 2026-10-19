package bbs

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMailTitleSearchLowersByUnicode15 pins the folder search lower-casing
// by Unicode 15.0: U+A7CB, encoded in Unicode 16.0 with lower case U+0264,
// has no lower case there, so a search for U+0264 finds only the subject
// spelled with U+0264 and a search for U+A7CB only the one holding it.
func TestMailTitleSearchLowersByUnicode15(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	const (
		ramsHorn  = "x\uA7CBy"
		ramsLower = "x\u0264y"
	)
	for _, subject := range []string{ramsHorn, ramsLower} {
		if frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", subject, "m"); len(frames) != 4 {
			t.Fatalf("send %+q answer = %x, want SENT_MAIL then the sent box", subject, opcodes(frames))
		}
		drainFrames(t, p.bobby)
	}

	for _, tc := range []struct {
		search string
		want   []string
	}{
		{"\uA7CB", []string{ramsHorn}},
		{"\u0264", []string{ramsLower}},
		{"X\u0264Y", []string{ramsLower}},
	} {
		page := pageOf(t, write(t, p.alice, "Mail", "Search;sentbox", "0", "Title", "Title", tc.search))
		var got []string
		for _, m := range listedSubject.FindAllStringSubmatch(page, -1) {
			got = append(got, m[1])
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("title search %+q lists %+q, want %+q", tc.search, got, tc.want)
		}
	}
}
