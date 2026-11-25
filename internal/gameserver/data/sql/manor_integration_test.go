package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestManorStoreRoundTrip pins castle_manor_production and
// castle_manor_procure: a save replaces every stored row, and a load
// returns them in primary key order, castle, item, then period. The last
// rollover's time is stored in server_memo, 0 when none is, and a save
// with 0 keeps the stored one.
func TestManorStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewManorStore(sqltest.SharedDB(t))

	stale := []castlemanor.ProductionRow{{CastleID: 9, SeedID: 1, Amount: 1, StartAmount: 1, Price: 1}}
	if err := store.Save(ctx, stale, []castlemanor.ProcureRow{{CastleID: 9, CropID: 1}}, 0); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if _, _, last, err := store.Load(ctx); err != nil || last != 0 {
		t.Fatalf("Load after a save with no rollover = %d, %v; want 0", last, err)
	}

	production := []castlemanor.ProductionRow{
		{CastleID: 2, SeedID: 5017, Amount: 3, StartAmount: 4, Price: 9, NextPeriod: true},
		{CastleID: 1, SeedID: 5016, Amount: 7, StartAmount: 10, Price: 5, NextPeriod: true},
		{CastleID: 1, SeedID: 5016, Amount: 2, StartAmount: 10, Price: 6},
	}
	procure := []castlemanor.ProcureRow{
		{CastleID: 1, CropID: 5073, Amount: 100, StartAmount: 100, Price: 30, RewardType: 1, NextPeriod: true},
		{CastleID: 1, CropID: 5068, Amount: 9, StartAmount: 10, Price: 7, RewardType: 2},
		{CastleID: 1, CropID: 5073, Amount: 40, StartAmount: 100, Price: 50, RewardType: 1},
	}
	const rolledAt = 1_791_230_400_000
	if err := store.Save(ctx, production, procure, rolledAt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(ctx, production, procure, 0); err != nil {
		t.Fatalf("Save with no rollover: %v", err)
	}

	gotProduction, gotProcure, last, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if last != rolledAt {
		t.Fatalf("last rollover = %d, want %d", last, rolledAt)
	}
	wantProduction := []castlemanor.ProductionRow{production[2], production[1], production[0]}
	wantProcure := []castlemanor.ProcureRow{procure[1], procure[2], procure[0]}
	if !slices.Equal(gotProduction, wantProduction) {
		t.Fatalf("production = %+v, want %+v", gotProduction, wantProduction)
	}
	if !slices.Equal(gotProcure, wantProcure) {
		t.Fatalf("procure = %+v, want %+v", gotProcure, wantProcure)
	}
}
