package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAnnounceAutoRefusesFlood pins the owner ruling on #5126, a divergence
// from the reference (which accepts them and re-arms a zero delay at once):
// //announce add_auto refuses an automatic announcement with a delay under
// a second or a negative limit with a message, then shows the unchanged
// list; nothing is held, written or announced. With auto off the schedule
// values are dropped, so they are not checked.
func TestAnnounceAutoRefusesFlood(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	hello := listedAnnouncement("0", "Hello", "false", "false")
	helloFile := `<announcement message="Hello" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`
	assertList(t, exchange(t, gm, encodeBuildCmd("announce add false Hello")), hello)

	for _, c := range []struct{ args, text string }{
		{"false true 0 0 0 Flood", "Invalid //announce delay ; an automatic announcement needs at least 1 second."},
		{"false true 0 0 3 Flood", "Invalid //announce delay ; an automatic announcement needs at least 1 second."},
		{"false true 5 -1 0 Flood", "Invalid //announce delay ; an automatic announcement needs at least 1 second."},
		{"false true 0 1 -1 Flood", "Invalid //announce limit ; can't be negative."},
	} {
		frames := exchange(t, gm, encodeBuildCmd("announce add_auto "+c.args))
		if len(frames) != 2 {
			t.Fatalf("add_auto %s frames = %x, want refusal then list", c.args, testsupport.FrameOpcodes(frames))
		}
		assertTexts(t, frames[:1], c.text)
		assertList(t, frames, hello)
	}
	assertAnnouncementFile(t, srv, helloFile)
	srv.Advance(t, 1500*time.Millisecond)
	if frames := user.ReadQueued(); len(frames) != 0 {
		t.Fatalf("frame %#x after refused automatic announcements, want none", frames[0][0])
	}

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add_auto true false 0 0 -1 Plain")),
		hello, listedAnnouncement("1", "Plain", "true", "false"))
	assertAnnouncementFile(t, srv, helloFile,
		`<announcement message="Plain" critical="true" auto="false" initial_delay="0" delay="0" limit="0" />`)
}
