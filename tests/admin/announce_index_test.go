package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAnnounceIndexNeverReused pins the owner ruling on #4707, a divergence
// from the reference (AnnouncementData.java puts a new announcement at the
// index equal to the count held, replacing a live one and leaving a
// replaced automatic one repeating): after the first of three
// announcements is deleted, a fourth takes an index no announcement holds,
// every other announcement stays in the list and the file, the deleted
// automatic announcement stops at once and the new one repeats.
func TestAnnounceIndexNeverReused(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 0 First")),
		listedAnnouncement("0", "First", "false", "true"))
	assertSay(t, user.ReadWithTimeout(3*time.Second), 0, sayAnnouncement, "", "First")
	assertList(t, exchange(t, gm, encodeBuildCmd("announce add false Second")),
		listedAnnouncement("0", "First", "false", "true"),
		listedAnnouncement("1", "Second", "false", "false"))
	assertList(t, exchange(t, gm, encodeBuildCmd("announce add true Third")),
		listedAnnouncement("0", "First", "false", "true"),
		listedAnnouncement("1", "Second", "false", "false"),
		listedAnnouncement("2", "Third", "true", "false"))

	assertList(t, exchange(t, gm, encodeBuildCmd("announce del 0")),
		listedAnnouncement("1", "Second", "false", "false"),
		listedAnnouncement("2", "Third", "true", "false"))
	drain(t, user)
	srv.Advance(t, 1500*time.Millisecond)
	if frames := user.ReadQueued(); len(frames) != 0 {
		t.Fatalf("frame %#x after the automatic announcement was deleted, want none", frames[0][0])
	}

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 0 Fourth")),
		listedAnnouncement("1", "Second", "false", "false"),
		listedAnnouncement("2", "Third", "true", "false"),
		listedAnnouncement("3", "Fourth", "false", "true"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Second" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`,
		`<announcement message="Third" critical="true" auto="false" initial_delay="0" delay="0" limit="0" />`,
		`<announcement message="Fourth" critical="false" auto="true" initial_delay="0" delay="1" limit="0" />`)
	for i := range 2 {
		frame := user.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("automatic announcement %d never came", i)
		}
		assertSay(t, frame, 0, sayAnnouncement, "", "Fourth")
	}
	drain(t, gm)
	assertList(t, exchange(t, gm, encodeBuildCmd("announce list")),
		listedAnnouncement("1", "Second", "false", "false"),
		listedAnnouncement("2", "Third", "true", "false"),
		listedAnnouncement("3", "Fourth", "false", "true"))
}
