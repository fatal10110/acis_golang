package clan

import (
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFoundingClanClearsTitle has a titled character found a clan at the
// village master: its title is cleared in the UserInfo it gets, in the
// clan's roster and in its stored row, as ClanTable.createClan ->
// Player.setClan clears it.
func TestFoundingClanClearsTitle(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanSeed(func(db *sql.DB) {
		seedStatements(t, db, `UPDATE characters SET title = 'Lord' WHERE char_name = 'Founder'`)
	}))
	if got := storedTitle(t, w, w.leaderID); got != "Lord" {
		t.Fatalf("stored title before founding = %q, want Lord", got)
	}
	w.talkToMaster(t)

	frames := w.masterCommand(t, "create_clan Knights")
	if got := leaverTitle(t, frames); got != "" {
		t.Fatalf("founder's UserInfo title = %q, want it cleared", got)
	}
	cl, ok := w.srv.Clans.Table().ByName("Knights")
	if !ok {
		t.Fatal("Knights missing from the registry")
	}
	if m, ok := cl.Member(w.leaderID); !ok || m.Title != "" {
		t.Fatalf("founder's roster row = %+v (found %v), want an empty title", m, ok)
	}
	if got := storedTitle(t, w, w.leaderID); got != "" {
		t.Fatalf("stored title after founding = %q, want it cleared", got)
	}
}
