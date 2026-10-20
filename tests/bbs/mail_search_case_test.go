package bbs

import (
	"regexp"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

var listedSubject = regexp.MustCompile(`_bbsmail;view;\d+">([^<]*)</a>`)

// TestMailTitleSearchIgnoresCase pins the folder search matching a title in
// any case, letters lowered one code point at a time by the standard
// mapping: a capital sigma always lowers to "σ", never to the final form,
// and a supplementary character matches as itself.
func TestMailTitleSearchIgnoresCase(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	const (
		cyrillic = "ДЖ"
		sigma    = "ΑΣ"
		pair     = "x\U0001F600y"
	)
	for _, subject := range []string{cyrillic, sigma, pair, "plain"} {
		if frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", subject, "m"); len(frames) != 4 {
			t.Fatalf("send %q answer = %x, want SENT_MAIL then the sent box", subject, opcodes(frames))
		}
		drainFrames(t, p.bobby)
	}

	for _, tc := range []struct {
		search string
		want   []string
	}{
		{"дж", []string{cyrillic}},
		{"Ж", []string{cyrillic}},
		{"PLAIN", []string{"plain"}},
		{"ασ", []string{sigma}},
		{"σ", []string{sigma}},
		{"ς", nil},
		{"X\U0001F600", []string{pair}},
		{"\U0001F601", nil},
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
			t.Fatalf("title search %q lists %q, want %q", tc.search, got, want)
		}
	}
}
