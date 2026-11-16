package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAnnounceSpecialCharactersSurviveReload: an announcement whose message
// holds a quote, an ampersand, angle brackets or an entity is saved escaped
// in announcements.xml, so reloading the file brings back every
// announcement, that message as typed.
func TestAnnounceSpecialCharactersSurviveReload(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	special := `Say "hi" & <b>bold</b> it's &amp; 5 > 4`
	exchange(t, gm, encodeBuildCmd("announce add false Welcome"))
	exchange(t, gm, encodeBuildCmd("announce add true "+special))
	exchange(t, gm, encodeBuildCmd("announce add false Goodbye"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Welcome" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`,
		`<announcement message="Say &quot;hi&quot; &amp; &lt;b>bold&lt;/b> it's &amp;amp; 5 > 4" critical="true" auto="false" initial_delay="0" delay="0" limit="0" />`,
		`<announcement message="Goodbye" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload announcement")), "The content of announcements.xml has been reloaded.")
	assertList(t, exchange(t, gm, encodeBuildCmd("announce list")),
		listedAnnouncement("0", "Welcome", "false", "false"),
		listedAnnouncement("1", special, "true", "false"),
		listedAnnouncement("2", "Goodbye", "false", "false"))
}
