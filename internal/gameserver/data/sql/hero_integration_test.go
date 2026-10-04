package sql

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
)

func execAll(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// TestHeroStoreHeroes pins the heroes round trip: a save inserts new heroes
// and updates a stored one's count and flags, keeping its class and
// message; a load joins the character's current name and clan and skips
// heroes whose character no longer exists; resetting the era clears every
// played flag and nothing else.
func TestHeroStoreHeroes(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewHeroStore(db)
	execAll(t, db,
		`INSERT INTO characters (account_name, obj_Id, char_name, clanid) VALUES ('a', 101, 'One', 600), ('a', 102, 'Two', NULL)`,
		`INSERT INTO heroes (char_id, class_id, count, played, active, message) VALUES (101, 93, 2, 0, 1, 'words'), (999, 88, 1, 1, 1, '')`,
	)
	if err := store.SaveHeroes(ctx, map[int32]hero.Hero{
		101: {ClassID: 90, Count: 3, Played: true},
		102: {ClassID: 88, Count: 1, Played: true, Active: true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadHeroes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []hero.Row{
		{ObjectID: 101, Name: "One", ClassID: 93, Count: 3, Played: true, ClanID: 600},
		{ObjectID: 102, Name: "Two", ClassID: 88, Count: 1, Played: true, Active: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("LoadHeroes() = %+v, want %+v", got, want)
	}
	var message string
	if err := db.QueryRowContext(ctx, "SELECT message FROM heroes WHERE char_id = 101").Scan(&message); err != nil || message != "words" {
		t.Fatalf("message = %q, %v; want kept", message, err)
	}

	if err := store.ResetPlayed(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.LoadHeroes(ctx); slices.ContainsFunc(got, func(r hero.Row) bool { return r.Played }) || !got[1].Active {
		t.Fatalf("LoadHeroes() after ResetPlayed = %+v, want no played hero, flags otherwise kept", got)
	}

	if id, found, err := store.ClanID(ctx, 101); err != nil || !found || id != 600 {
		t.Fatalf("ClanID(101) = %d, %v, %v", id, found, err)
	}
	if id, found, err := store.ClanID(ctx, 102); err != nil || !found || id != 0 {
		t.Fatalf("ClanID(102) = %d, %v, %v; want no clan", id, found, err)
	}
	if _, found, err := store.ClanID(ctx, 999); err != nil || found {
		t.Fatalf("ClanID(999) = found %v, %v; want not found", found, err)
	}
}

// TestHeroStoreBestNoble pins the pick of a class's hero to be: points,
// then matches, then wins, all descending, among records whose character
// exists with enough matches and a win; ties on all three go by id.
func TestHeroStoreBestNoble(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewHeroStore(db)
	execAll(t, db,
		`INSERT INTO characters (account_name, obj_Id, char_name) VALUES ('a', 201, 'A'), ('a', 202, 'B'), ('a', 203, 'C'), ('a', 204, 'D'), ('a', 205, 'E')`,
		`INSERT INTO olympiad_nobles VALUES (201, 88, 50, 10, 3, 0, 0, 0), (202, 88, 50, 12, 1, 0, 0, 0), (299, 88, 90, 20, 9, 0, 0, 0),
			(203, 89, 40, 9, 2, 0, 0, 0), (204, 89, 40, 9, 2, 0, 0, 0), (205, 90, 70, 4, 4, 0, 0, 0)`,
	)
	for _, tc := range []struct {
		class, minMatches int
		id                int32
		name              string
		found             bool
	}{
		{88, 5, 202, "B", true},
		{89, 5, 203, "C", true},
		{90, 5, 0, "", false},
		{90, 4, 205, "E", true},
		{91, 5, 0, "", false},
	} {
		id, name, found, err := store.BestNoble(ctx, tc.class, tc.minMatches)
		if err != nil || id != tc.id || name != tc.name || found != tc.found {
			t.Errorf("BestNoble(%d, %d) = %d %q %v %v, want %d %q %v", tc.class, tc.minMatches, id, name, found, err, tc.id, tc.name, tc.found)
		}
	}
}

// TestHeroStoreItemsAndDiary pins the hero item purge, which spares a game
// master's, and the diary insert.
func TestHeroStoreItemsAndDiary(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewHeroStore(db)
	execAll(t, db,
		`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('a', 301, 'Player', 0), ('a', 302, 'Master', 1)`,
		`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES
			(301, 1, 6611, 1, 'PAPERDOLL', 7), (301, 2, 6842, 1, 'WAREHOUSE', 0), (301, 3, 57, 10, 'INVENTORY', 0),
			(302, 4, 6620, 1, 'INVENTORY', 0), (303, 5, 6612, 1, 'INVENTORY', 0)`,
	)
	if err := store.DeleteHeroItems(ctx); err != nil {
		t.Fatal(err)
	}
	var left []int32
	rows, err := db.QueryContext(ctx, "SELECT object_id FROM items ORDER BY object_id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left = append(left, id)
	}
	rows.Close()
	if !slices.Equal(left, []int32{3, 4}) {
		t.Fatalf("items left = %v, want the adena and the game master's hero item", left)
	}

	if err := store.AddDiaryEntry(ctx, 301, 1700000000123, hero.DiaryRaidKilled, 25001); err != nil {
		t.Fatal(err)
	}
	var at int64
	var action, param int
	if err := db.QueryRowContext(ctx, "SELECT time, action, param FROM heroes_diary WHERE char_id = 301").Scan(&at, &action, &param); err != nil ||
		at != 1700000000123 || action != 1 || param != 25001 {
		t.Fatalf("diary row = %d %d %d, %v", at, action, param, err)
	}
}
