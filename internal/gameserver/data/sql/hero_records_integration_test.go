package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
)

// TestHeroStoreRecords pins what a hero's pages read: its diary oldest
// first; the fights it fought on either side before the given instant,
// oldest first, with each side's current name, a side whose character is
// gone having none; and its message, saved back for each hero given.
func TestHeroStoreRecords(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewHeroStore(db)
	execAll(t, db,
		`INSERT INTO characters (account_name, obj_Id, char_name) VALUES ('a', 101, 'One'), ('a', 102, 'Two')`,
		`INSERT INTO heroes (char_id, class_id, count, played, active, message) VALUES (101, 88, 1, 1, 1, 'words'), (102, 89, 1, 1, 0, '')`,
		`INSERT INTO heroes_diary (char_id, time, action, param) VALUES (101, 300, 1, 25001), (101, 100, 2, 0), (102, 200, 3, 1)`,
		`INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed) VALUES
			(101, 102, 88, 89, 1, 2000, 60000, 1), (103, 101, 90, 88, 2, 1000, 5000, 0), (101, 102, 88, 89, 0, 5000, 1, 0), (102, 104, 89, 91, 1, 1500, 1, 0)`,
	)

	diary, err := store.LoadDiary(ctx, 101)
	if err != nil {
		t.Fatal(err)
	}
	if want := []hero.DiaryRow{{At: 100, Action: 2}, {At: 300, Action: 1, Param: 25001}}; !slices.Equal(diary, want) {
		t.Fatalf("LoadDiary(101) = %+v, want %+v", diary, want)
	}

	fights, err := store.LoadFights(ctx, 101, 5000)
	if err != nil {
		t.Fatal(err)
	}
	want := []hero.FightRow{
		{OneID: 103, TwoID: 101, OneClass: 90, TwoClass: 88, TwoName: "One", TwoFound: true, Winner: 2, Start: 1000, Time: 5000},
		{OneID: 101, TwoID: 102, OneClass: 88, TwoClass: 89, OneName: "One", TwoName: "Two", OneFound: true, TwoFound: true, Winner: 1, Start: 2000, Time: 60000, Classed: 1},
	}
	if !slices.Equal(fights, want) {
		t.Fatalf("LoadFights(101, 5000) = %+v, want %+v", fights, want)
	}

	if msg, found, err := store.LoadMessage(ctx, 101); err != nil || !found || msg != "words" {
		t.Fatalf("LoadMessage(101) = %q, %v, %v; want words", msg, found, err)
	}
	if _, found, err := store.LoadMessage(ctx, 103); err != nil || found {
		t.Fatalf("LoadMessage(103) found = %v, %v; want no hero", found, err)
	}
	if err := store.SaveMessages(ctx, map[int32]string{101: "", 102: "Glory"}); err != nil {
		t.Fatal(err)
	}
	for id, wantMsg := range map[int32]string{101: "", 102: "Glory"} {
		if msg, _, err := store.LoadMessage(ctx, id); err != nil || msg != wantMsg {
			t.Fatalf("message of %d = %q, %v; want %q", id, msg, err, wantMsg)
		}
	}
}
