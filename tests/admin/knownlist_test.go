package admin

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// knownlistRow is one object row of AdminKnownlist.showKnownlist.
func knownlistRow(row int, name, class string) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 bgcolor=000000><tr>"
	}
	return open + "<td width=160>" + name + "</td><td width=120>" + class + `</td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// knownlistPage is the shipped knownlist.htm around target listing rows,
// the page padded to fifteen rows.
func knownlistPage(t *testing.T, target string, rows ...string) string {
	t.Helper()
	content := strings.Join(rows, "") + strings.Repeat("<img height=20>", 15-len(rows)) + pageBar("bypass admin_knownlist %page%", 1, min(len(rows), 1))
	return shippedPage(t, "knownlist.htm", "%target%", target, "%content%", content)
}

// TestAdminKnownlist pins AdminKnownlist.java: //knownlist lists the
// objects around the GM's target, or around the GM without one, each by
// name and class; a page that does not read is answered with the usage
// and the first page, page 0 sends nothing.
func TestAdminKnownlist(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "knownlist.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	other, otherID := addPlayer(t, srv, "other", "Other", userLevel)
	drain(t, gm)
	drain(t, other)

	self := knownlistPage(t, "Admin", knownlistRow(0, "Other", "Player"))
	for _, cmd := range []string{"knownlist", "knownlist 1", "knownlist 7"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want one page", cmd, testsupport.FrameOpcodes(frames))
		}
		if got := htmlBody(t, frames[0]); got != self {
			t.Fatalf("//%s page =\n%s\nwant\n%s", cmd, got, self)
		}
	}

	frames := exchange(t, gm, encodeBuildCmd("knownlist x"))
	if len(frames) != 2 {
		t.Fatalf("//knownlist x frames = %x, want the usage and the page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Usage: //knownlist [page]")
	if got := htmlBody(t, frames[1]); got != self {
		t.Fatalf("//knownlist x page =\n%s\nwant\n%s", got, self)
	}

	if frames := exchange(t, gm, encodeBuildCmd("knownlist 0")); len(frames) != 0 {
		t.Fatalf("//knownlist 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}

	exchange(t, gm, encodeAction(otherID))
	frames = exchange(t, gm, encodeBypass("admin_knownlist"))
	if len(frames) != 1 {
		t.Fatalf("//knownlist on Other frames = %x, want one page", testsupport.FrameOpcodes(frames))
	}
	if got, want := htmlBody(t, frames[0]), knownlistPage(t, "Other", knownlistRow(0, "Admin", "Player")); got != want {
		t.Fatalf("//knownlist on Other page =\n%s\nwant\n%s", got, want)
	}
}
