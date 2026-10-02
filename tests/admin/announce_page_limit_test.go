package admin

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// announceListTooLong is the page NpcHtmlMessage.setHtml sends in place of
// one longer than 8192 characters.
const announceListTooLong = "<html><body>Html was too long.</body></html>"

// manyAnnouncements returns an announcements.xml holding n login
// announcements, and their rows on the announcement list.
func manyAnnouncements(n int) (string, []string) {
	var file strings.Builder
	file.WriteString("<list>")
	rows := make([]string, n)
	for i := range n {
		msg := "Login announcement " + strconv.Itoa(i) + " " + strings.Repeat("-", 40)
		file.WriteString(`<announcement message="` + msg + `" critical="false" auto="false" />`)
		rows[i] = listedAnnouncement(strconv.Itoa(i), msg, "false", "false")
	}
	file.WriteString("</list>")
	return file.String(), rows
}

// TestAnnounceListGrownPastPageLimitIsSentWhole pins AnnouncementData.
// listAnnouncements over NpcHtmlMessage: the page limit holds the list
// page as loaded, then %announces% is filled; a list the rows grow past
// 8192 characters is sent whole.
func TestAnnounceListGrownPastPageLimitIsSentWhole(t *testing.T) {
	t.Parallel()
	file, rows := manyAnnouncements(40)
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)),
		gameservertest.WithAnnouncements(file))
	gm := srv.Client
	enterWorld(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("announce list"))
	if len(frames) != 1 {
		t.Fatalf("//announce list frames = %d, want the one list page", len(frames))
	}
	if units := len(utf16.Encode([]rune(htmlBody(t, frames[0])))); units <= 8192 {
		t.Fatalf("list page = %d characters, want one past the 8192 limit", units)
	}
	assertList(t, frames, rows...)
}

// TestAnnounceListPageLimitAppliesToLoadedPage pins NpcHtmlMessage.setHtml
// on the page as loaded: a list page longer than 8192 characters before
// its rows are filled in is replaced by the notice, and one of fewer than
// 8192 characters is sent even when its UTF-8 form passes 8192 bytes.
func TestAnnounceListPageLimitAppliesToLoadedPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		padding string
		want    func(page string) string
	}{
		{
			name:    "over the limit",
			padding: strings.Repeat("x", 8192),
			want:    func(string) string { return announceListTooLong },
		},
		{
			// 6000 three-byte characters: 18000 bytes, 6000 characters.
			name:    "non-ASCII under the limit",
			padding: strings.Repeat("漢", 6000),
			want: func(page string) string {
				return strings.ReplaceAll(page, "%announces%", "<br><tr><td>The XML file doesn't contain any content.</td></tr>")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			page := "<html><body>" + tt.padding + "%announces%</body></html>\n"
			srv, _ := bootAdmin(t, adminLevel,
				gameservertest.WithHTMLPages(map[string]string{"admin/announce_list.htm": page}))
			gm := srv.Client
			enterWorld(t, gm)

			frames := exchange(t, gm, encodeBuildCmd("announce list"))
			if len(frames) != 1 {
				t.Fatalf("//announce list frames = %d, want the one list page", len(frames))
			}
			if got, want := htmlBody(t, frames[0]), tt.want(page); got != want {
				t.Fatalf("list page = %d bytes starting %.60q, want %d bytes starting %.60q", len(got), got, len(want), want)
			}
		})
	}
}
