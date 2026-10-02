package sql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/rs/zerolog"
)

func restoredSevenSigns(t *testing.T, db *sql.DB) *sevensigns.State {
	t.Helper()
	state := sevensigns.NewState(NewSevenSignsStore(db), nil, zerolog.Nop(), nil, nil)
	if err := state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	return state
}

type sevenSignsPlayerRow struct {
	cabal, seal         string
	red, green, blue    int
	adena, contribution int64
}

func readSevenSignsPlayer(t *testing.T, db *sql.DB, objectID int32) (sevenSignsPlayerRow, bool) {
	t.Helper()
	var r sevenSignsPlayerRow
	err := db.QueryRow(`SELECT cabal, seal, red_stones, green_stones, blue_stones, ancient_adena_amount, contribution_score
		FROM seven_signs WHERE char_obj_id = ?`, objectID).Scan(&r.cabal, &r.seal, &r.red, &r.green, &r.blue, &r.adena, &r.contribution)
	if err == sql.ErrNoRows {
		return r, false
	}
	if err != nil {
		t.Fatalf("read seven_signs %d: %v", objectID, err)
	}
	return r, true
}

// A sign-up is inserted the moment it is made; stones, festival points and
// the status reach the database with the next save; a restart reads all of
// it back. The festival's own status columns are never touched.
func TestSevenSignsStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	if _, err := db.Exec(`UPDATE seven_signs_status SET festival_cycle = 4, accumulated_bonus2 = 77 WHERE id = 0`); err != nil {
		t.Fatal(err)
	}

	state := restoredSevenSigns(t, db)
	if err := state.SetPlayerInfo(ctx, 501, sevensigns.Dawn, sevensigns.Gnosis); err != nil {
		t.Fatalf("SetPlayerInfo: %v", err)
	}
	if err := state.SetPlayerInfo(ctx, 502, sevensigns.Dusk, sevensigns.Strife); err != nil {
		t.Fatalf("SetPlayerInfo: %v", err)
	}
	if got, ok := readSevenSignsPlayer(t, db, 501); !ok || got != (sevenSignsPlayerRow{cabal: "DAWN", seal: "GNOSIS"}) {
		t.Fatalf("inserted sign-up = %+v (found %v)", got, ok)
	}
	if _, ok := state.AddPlayerStoneContrib(501, 1, 2, 3, 1000000); !ok {
		t.Fatal("stone contribution refused")
	}
	state.AddFestivalScore(sevensigns.Dusk, 25)
	if got, ok := readSevenSignsPlayer(t, db, 501); !ok || got.red != 0 {
		t.Fatalf("stones written before any save: %+v", got)
	}
	before := state.Record(501)
	if err := state.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got, _ := readSevenSignsPlayer(t, db, 501); got != (sevenSignsPlayerRow{cabal: "DAWN", seal: "GNOSIS", red: 3, green: 2, blue: 1, adena: 43, contribution: 43}) {
		t.Fatalf("saved sign-up = %+v", got)
	}
	var (
		dawnStone, duskFestival, gnosisDawn, strifeDusk, festivalCycle, bonus2 int
		date                                                                   int64
	)
	if err := db.QueryRow(`SELECT dawn_stone_score, dusk_festival_score, gnosis_dawn_score, strife_dusk_score, festival_cycle, accumulated_bonus2, date
		FROM seven_signs_status WHERE id = 0`).Scan(&dawnStone, &duskFestival, &gnosisDawn, &strifeDusk, &festivalCycle, &bonus2, &date); err != nil {
		t.Fatal(err)
	}
	if dawnStone != 43 || duskFestival != 25 || gnosisDawn != 1 || strifeDusk != 1 || festivalCycle != 4 || bonus2 != 77 || date == 0 {
		t.Fatalf("saved status = dawn stone %d, dusk festival %d, gnosis dawn %d, strife dusk %d, festival cycle %d, bonus2 %d, date %d",
			dawnStone, duskFestival, gnosisDawn, strifeDusk, festivalCycle, bonus2, date)
	}

	restarted := restoredSevenSigns(t, db)
	if after := restarted.Record(501); after != before {
		t.Fatalf("record after restart = %+v, want %+v", after, before)
	}
	if restarted.PlayerCabal(502) != sevensigns.Dusk || restarted.PlayerSeal(502) != sevensigns.Strife {
		t.Fatal("second sign-up lost across the restart")
	}
}

// A missing status row restores the schema defaults and is written back in
// full by the next save.
func TestSevenSignsStoreMissingStatusRow(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	if _, err := db.Exec(`DELETE FROM seven_signs_status`); err != nil {
		t.Fatal(err)
	}
	state := restoredSevenSigns(t, db)
	if state.CurrentPeriod() != sevensigns.Competition || state.CurrentCycle() != 1 || state.Record(0).Winner != sevensigns.NoCabal {
		t.Fatalf("defaults = cycle %d, %v", state.CurrentCycle(), state.CurrentPeriod())
	}
	if err := state.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var (
		cycle          int
		period, winner string
		festivalCycle  int
	)
	if err := db.QueryRow(`SELECT current_cycle, active_period, previous_winner, festival_cycle FROM seven_signs_status WHERE id = 0`).
		Scan(&cycle, &period, &winner, &festivalCycle); err != nil {
		t.Fatalf("status row after save: %v", err)
	}
	if cycle != 1 || period != "COMPETITION" || winner != "NORMAL" || festivalCycle != 1 {
		t.Fatalf("written row = (%d, %s, %s, festival cycle %d)", cycle, period, winner, festivalCycle)
	}
}

// Deleting a character removes its sign-up.
func TestCharacterPurgeRemovesSevenSignsRow(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	if err := NewSevenSignsStore(db).InsertPlayer(ctx, sevensigns.PlayerRow{ObjectID: 777, Cabal: sevensigns.Dawn, Seal: sevensigns.Avarice}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCharacterStore(db).Purge(ctx, 777); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, ok := readSevenSignsPlayer(t, db, 777); ok {
		t.Fatal("seven_signs row survived the character's deletion")
	}
}
