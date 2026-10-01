package sql

import (
	"context"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

func seedRecommendationRow(t *testing.T, store *CharacterStore, id int32, name string, level, have, left int) {
	t.Helper()
	ctx := context.Background()
	c := testCharacter(id, name)
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE characters SET level = ?, rec_have = ?, rec_left = ? WHERE obj_Id = ?`, level, have, left, id); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

// TestRecommendationStoreGiveAndReload covers a recommendation's three
// writes and their reload: the record, the target's count and the giver's
// remaining count, which the character row read restores.
func TestRecommendationStoreGiveAndReload(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	chars := NewCharacterStore(db)
	store := NewRecommendationStore(db)
	seedRecommendationRow(t, chars, 0x10000001, "Giver", 30, 0, 6)
	seedRecommendationRow(t, chars, 0x10000002, "Target", 30, 4, 0)

	if ids, err := store.ListRecommended(ctx, 0x10000001); err != nil || len(ids) != 0 {
		t.Fatalf("ListRecommended before any = %v, %v; want none", ids, err)
	}
	if err := store.Give(ctx, 0x10000001, 0x10000002, 5, 5); err != nil {
		t.Fatalf("Give: %v", err)
	}
	ids, err := store.ListRecommended(ctx, 0x10000001)
	if err != nil || !reflect.DeepEqual(ids, []int32{0x10000002}) {
		t.Fatalf("ListRecommended = %v, %v; want [target]", ids, err)
	}
	giver, err := chars.Get(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("Get giver: %v", err)
	}
	target, err := chars.Get(ctx, 0x10000002)
	if err != nil {
		t.Fatalf("Get target: %v", err)
	}
	if giver.RecommendationsLeft() != 5 || target.RecommendationsHave() != 5 {
		t.Fatalf("reloaded giver left %d, target have %d; want 5 and 5", giver.RecommendationsLeft(), target.RecommendationsHave())
	}
	// A second record of the same pair is a duplicate key: the remaining
	// writes are skipped and the failure reported.
	if err := store.Give(ctx, 0x10000001, 0x10000002, 6, 4); err == nil {
		t.Fatal("duplicate Give returned no error")
	}
}

// TestRecommendationStoreRefreshDaily pins the stored half of the daily
// refresh: the record is emptied and each row's counters are reset by its
// stored level (under 20: 3 left, 1 lost; under 40: 6 and 2; else 9 and 3),
// never below zero.
func TestRecommendationStoreRefreshDaily(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	chars := NewCharacterStore(db)
	store := NewRecommendationStore(db)
	rows := []struct {
		id                 int32
		level, have, left  int
		wantHave, wantLeft int
	}{
		{0x10000011, 19, 0, 0, 0, 3},
		{0x10000012, 20, 10, 1, 8, 6},
		{0x10000013, 39, 1, 0, 0, 6},
		{0x10000014, 40, 255, 2, 252, 9},
	}
	for i, r := range rows {
		seedRecommendationRow(t, chars, r.id, "Daily"+string(rune('A'+i)), r.level, r.have, r.left)
	}
	if err := store.Give(ctx, 0x10000011, 0x10000012, 11, 0); err != nil {
		t.Fatalf("Give: %v", err)
	}
	rows[1].wantHave = 9 // 11 held before the refresh

	if err := store.RefreshDaily(ctx); err != nil {
		t.Fatalf("RefreshDaily: %v", err)
	}
	if ids, err := store.ListRecommended(ctx, 0x10000011); err != nil || len(ids) != 0 {
		t.Fatalf("ListRecommended after refresh = %v, %v; want none", ids, err)
	}
	for _, r := range rows {
		c, err := chars.Get(ctx, r.id)
		if err != nil {
			t.Fatalf("Get %d: %v", r.id, err)
		}
		if c.RecommendationsHave() != r.wantHave || c.RecommendationsLeft() != r.wantLeft {
			t.Errorf("level %d: have %d left %d, want %d and %d", r.level, c.RecommendationsHave(), c.RecommendationsLeft(), r.wantHave, r.wantLeft)
		}
	}
}
