package sql

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestOlympiadStoreClassLeaders pins Olympiad.getClassLeaderBoard's
// SELECT_CLASS_LEADER: the month's standings of one class, nobles with at
// least the minimum of matches, by points, then matches, then wins, ten at
// most, named by their current character name; a standing whose character
// is gone is skipped.
func TestOlympiadStoreClassLeaders(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewOlympiadStore(db)
	chars := NewCharacterStore(db)
	// id, class, points, done, won.
	rows := [][5]int{
		{1, 88, 50, 9, 5},
		{2, 88, 50, 9, 6},
		{3, 88, 50, 10, 1},
		{4, 88, 80, 5, 0},
		{5, 88, 99, 4, 4}, // too few matches
		{6, 89, 99, 9, 9}, // another class
		{7, 88, 10, 5, 0},
		{8, 88, 11, 5, 0},
		{9, 88, 12, 5, 0},
		{10, 88, 13, 5, 0},
		{11, 88, 14, 5, 0},
		{12, 88, 15, 5, 0},
		{13, 88, 70, 5, 0}, // character gone
	}
	for _, r := range rows {
		id := int32(0x10000000 + r[0])
		if r[0] != 13 {
			if err := chars.Create(ctx, testCharacter(id, fmt.Sprintf("Noble%d", r[0]))); err != nil {
				t.Fatalf("create %d: %v", r[0], err)
			}
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO olympiad_nobles_eom VALUES (?, ?, ?, ?, ?, 0, 0)", id, r[1], r[2], r[3], r[4]); err != nil {
			t.Fatalf("seed olympiad_nobles_eom: %v", err)
		}
	}
	got, err := store.ClassLeaders(ctx, 88, 5)
	if err != nil {
		t.Fatalf("ClassLeaders(): %v", err)
	}
	want := []string{"Noble4", "Noble3", "Noble2", "Noble1", "Noble12", "Noble11", "Noble10", "Noble9", "Noble8", "Noble7"}
	if !slices.Equal(got, want) {
		t.Fatalf("ClassLeaders(88, 5) = %v, want %v", got, want)
	}
	if got, err := store.ClassLeaders(ctx, 90, 5); err != nil || len(got) != 0 {
		t.Fatalf("ClassLeaders(90, 5) = %v, %v; want none", got, err)
	}
}
