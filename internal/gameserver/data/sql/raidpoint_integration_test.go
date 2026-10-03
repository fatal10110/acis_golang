package sql

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/rs/zerolog"
)

// TestRaidPointStoreRoundTrip pins character_raid_points: an empty table
// loads nothing, a save inserts a row, a later save of the same character
// and boss replaces its total, and rows load ordered by character then
// boss.
func TestRaidPointStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewRaidPointStore(db)

	if got, err := store.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Load() on an empty table = %v, %v; want none", got, err)
	}
	for _, row := range []raidpoint.Row{
		{CharID: 0x10000002, BossID: 25001, Points: 7},
		{CharID: 0x10000001, BossID: 29001, Points: 30},
		{CharID: 0x10000001, BossID: 25002, Points: 4},
		{CharID: 0x10000001, BossID: 25002, Points: 9},
	} {
		if err := store.Save(ctx, row); err != nil {
			t.Fatalf("Save(%+v): %v", row, err)
		}
	}
	want := []raidpoint.Row{
		{CharID: 0x10000001, BossID: 25002, Points: 9},
		{CharID: 0x10000001, BossID: 29001, Points: 30},
		{CharID: 0x10000002, BossID: 25001, Points: 7},
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, want) {
		t.Fatalf("Load() = %v, %v; want %v", got, err, want)
	}
}

// TestRaidPointsWriteThroughStore drives raidpoint.Points over the store:
// each addition stores the boss's new total, a negative amount stores
// nothing, a restore reads the totals back, and equal totals rank by
// object id.
func TestRaidPointsWriteThroughStore(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewRaidPointStore(db)
	points := raidpoint.New(store, nil, zerolog.Nop())

	points.Add(0x10000001, 25001, 10)
	points.Add(0x10000001, 25001, 5)
	points.Add(0x10000001, 25002, -3)
	points.Add(0x10000002, 25002, 0)
	want := []raidpoint.Row{
		{CharID: 0x10000001, BossID: 25001, Points: 15},
		{CharID: 0x10000002, BossID: 25002, Points: 0},
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, want) {
		t.Fatalf("stored rows = %v, %v; want %v", got, err, want)
	}

	again := raidpoint.New(store, nil, zerolog.Nop())
	if err := again.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if got := again.Record(0x10000001); got.Total != 15 || got.Rank != 1 || !got.Found {
		t.Fatalf("restored record = %+v, want rank 1, total 15", got)
	}
	if got := again.Record(0x10000002); got.Total != 0 || got.Rank != 0 || !got.Found {
		t.Fatalf("restored zero-point record = %+v, want found, unranked", got)
	}

	// Equal totals rank by object id.
	again.Add(0x10000003, 25003, 15)
	if a, b := again.Record(0x10000001).Rank, again.Record(0x10000003).Rank; a != 1 || b != 2 {
		t.Fatalf("tied ranks = %d, %d; want 1, 2", a, b)
	}
}

// TestBossZoneStoreRoundTrip pins grandboss_list: an empty table loads
// nothing, a save writes every zone's players, and a later save replaces
// them all.
func TestBossZoneStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewBossZoneStore(db)

	if got, err := store.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Load() on an empty table = %v, %v; want none", got, err)
	}
	first := map[int][]int32{110000: {0x10000002, 0x10000001}, 110012: {0x10000003}}
	if err := store.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	want := map[int][]int32{110000: {0x10000001, 0x10000002}, 110012: {0x10000003}}
	got, err := store.Load(ctx)
	if err != nil || !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("Load() = %v, %v; want %v", got, err, want)
	}
	second := map[int][]int32{110001: {0x10000004}}
	if err := store.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(ctx); err != nil || !maps.EqualFunc(got, second, slices.Equal) {
		t.Fatalf("Load() after the second save = %v, %v; want %v", got, err, second)
	}
}

// TestCharacterPurgeRemovesRaidPoints pins that deleting a character
// removes its raid points and leaves everyone else's.
func TestCharacterPurgeRemovesRaidPoints(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	chars := NewCharacterStore(db)
	c := testCharacter(0x10000001, "Newbie")
	if err := chars.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	points := NewRaidPointStore(db)
	for _, row := range []raidpoint.Row{{CharID: c.ID, BossID: 25001, Points: 3}, {CharID: c.ID, BossID: 25002, Points: 4}, {CharID: 0x10000002, BossID: 25001, Points: 5}} {
		if err := points.Save(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chars.Purge(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	want := []raidpoint.Row{{CharID: 0x10000002, BossID: 25001, Points: 5}}
	if got, err := points.Load(ctx); err != nil || !slices.Equal(got, want) {
		t.Fatalf("rows after purge = %v, %v; want %v", got, err, want)
	}
}
