package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
)

// TestRelationStoreSaveAcrossChunks saves more upserts and deletes than one
// multi-row statement covers: every row on either side of a chunk boundary
// is written or deleted, and rows the save does not name are kept.
func TestRelationStoreSaveAcrossChunks(t *testing.T) {
	ctx := context.Background()
	store := NewRelationStore(sqltest.SharedDB(t))

	n := 2*relationSaveChunk + 1
	first := make([]relation.Row, 0, n)
	for i := range int32(n) {
		first = append(first, relation.Row{CharID: 1, FriendID: 100 + i, Relation: 1})
	}
	if err := store.Save(ctx, first); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Delete the first chunk and one more row, reflag the rest but one.
	var second []relation.Row
	var want []relation.Row
	for i, r := range first {
		switch {
		case i <= relationSaveChunk:
			second = append(second, relation.Row{CharID: r.CharID, FriendID: r.FriendID})
		case i == n-1:
			want = append(want, r) // not named by the second save
		default:
			r.Relation = 3
			second = append(second, r)
			want = append(want, r)
		}
	}
	if err := store.Save(ctx, second); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Load returned %d rows, want %d (first %+v, last %+v)", len(got), len(want), got[:min(1, len(got))], got[max(0, len(got)-1):])
	}
}
