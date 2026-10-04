package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// newHero is the noble the election in TestHeroClaimBeforeElectionRuns
// picks.
const newHero int32 = 9101

// TestHeroClaimBeforeElectionRuns pins a claim made after the Olympiad end
// queued the hero election and before the election ran: the claim's save
// lands behind the election and stores the new era, so only the elected
// noble is a hero of the running era in the heroes table, and the
// outgoing hero is no hero after a restart.
func TestHeroClaimBeforeElectionRuns(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, gameservertest.WithOlympiadSeed(seedStatements(t,
		`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 0 FROM characters WHERE char_name = 'Talker'`,
		`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('others', 9101, 'Newcomer', 0)`,
		`INSERT INTO olympiad_nobles VALUES (9101, 88, 50, 10, 3, 7, 0, 0)`,
	)))

	release := w.srv.HoldPersistenceLane(t, 0)
	w.srv.Olympiad.SelectHeroes()
	w.srv.Settle(t)
	if !w.srv.Heroes.IsInactive(w.player) {
		t.Fatal("the election ran while its lane was held")
	}
	w.talkPage(t, monument, false)
	w.bypass(t, npcCommand(monument, "Olympiad 5"))
	w.bypass(t, npcCommand(monument, "Olympiad 6"))
	if !w.srv.Heroes.IsActive(w.player) {
		t.Fatal("the claim was refused")
	}
	release()
	w.srv.FlushPersistence(t)
	w.srv.Settle(t)
	w.srv.FlushPersistence(t)

	ctx := context.Background()
	rows, err := w.srv.DB.QueryContext(ctx, `SELECT char_id FROM heroes WHERE played = 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var played []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		played = append(played, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(played) != 1 || played[0] != newHero {
		t.Fatalf("heroes of the running era stored = %v, want only %d", played, newHero)
	}

	if err := w.srv.Heroes.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if w.srv.Heroes.IsActive(w.player) || w.srv.Heroes.IsInactive(w.player) {
		t.Fatal("the outgoing hero is a hero again after a restart")
	}
	if !w.srv.Heroes.IsInactive(newHero) {
		t.Fatal("the elected noble is not an inactive hero after a restart")
	}
}
