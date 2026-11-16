package olympiad

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHeroMessageShowsAsText pins the owner-approved divergence of #4654:
// a hero's words carrying a link reach the diary page with their markup
// escaped, so the page holds no live action of theirs.
func TestHeroMessageShowsAsText(t *testing.T) {
	t.Parallel()
	gained := time.Date(2025, time.March, 1, 9, 15, 0, 0, time.Local)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		heroPageOptions(t),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithOlympiadSeed(seedStatements(t, []string{
			`INSERT INTO heroes (char_id, class_id, count, played, active, message) SELECT obj_Id, 88, 1, 1, 1, 'Hail' FROM characters WHERE char_name = 'Login'`,
			fmt.Sprintf(`INSERT INTO heroes_diary (char_id, time, action, param) SELECT obj_Id, %d, 2, 0 FROM characters WHERE char_name = 'Login'`, gained.UnixMilli()),
		})),
	)
	c := srv.Client
	startInWorld(t, c)

	c.Send(encodeWriteHeroWords(`<a action="bypass -h admin_admin">Hail</a>`))
	page := heroPage(t, c, "_diary?class=88&page=1")
	if want := "Hero Login's Message:<br>\n&lt;a action=&quot;bypass -h admin_admin&quot;&gt;Hail&lt;/a&gt;<br>\n"; !strings.Contains(page, want) {
		t.Fatalf("diary page =\n%q\nwant it to hold %q", page, want)
	}
	if strings.Contains(page, "admin_admin\"") {
		t.Fatalf("diary page %q carries the hero's link", page)
	}
}
