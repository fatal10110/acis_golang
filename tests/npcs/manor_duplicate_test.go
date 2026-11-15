package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
)

// TestManorSetRepeatedID pins a setting request naming one seed or crop
// twice, which no setting window sends: the whole request is refused as an
// invalid row refuses it, silently, and the lists stand. Lists that still
// hold a repeated seed or crop save each one once, so the manor save of
// every castle goes through and the stored rows keep the first entry.
func TestManorSetRepeatedID(t *testing.T) {
	t.Parallel()
	w := bootManorSetup(t, residenceSetup{})
	m := w.srv.CastleManor
	spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 30)
	seeds, crops := productions(m.SeedProduction(1, true)), procures(m.CropProcure(1, true))

	wantOnly(t, "repeated seed", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5}, [3]int{5012, 1, 6}, [3]int{5016, 2, 5})))
	wantOnly(t, "repeated crop", w.send(t, encodeSetCrop(1, [4]int{5073, 1, 30, 1}, [4]int{5073, 2, 30, 1})))
	wantOnly(t, "repeated crop, once with no sales", w.send(t, encodeSetCrop(1, [4]int{5073, 1, 30, 1}, [4]int{5073, 0, 30, 1})))
	if got := productions(m.SeedProduction(1, true)); !equalRows(got, seeds) {
		t.Fatalf("next seeds = %v, want unchanged %v", got, seeds)
	}
	if got := procures(m.CropProcure(1, true)); !equalRows(got, crops) {
		t.Fatalf("next crops = %v, want unchanged %v", got, crops)
	}

	m.SetNextSeedProduction(1, []*castlemanor.Production{
		castlemanor.NewProduction(5012, 3, 8, 3), castlemanor.NewProduction(5016, 4, 5, 4), castlemanor.NewProduction(5012, 6, 9, 6),
	})
	m.SetNextCropProcure(1, []*castlemanor.Procure{
		castlemanor.NewProcure(5068, 10, 30, 10, 1), castlemanor.NewProcure(5068, 20, 40, 20, 2),
	})
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("manor save: %v", err)
	}
	if got, want := storedManorRows(t, w, "SELECT castle_id, seed_id, amount, start_amount, price, next_period FROM castle_manor_production ORDER BY next_period, castle_id, seed_id", 6),
		[][7]int32{{1, 5016, 7, 10, 5, 0}, {1, 5012, 3, 3, 8, 1}, {1, 5016, 4, 4, 5, 1}}; !equalRows(got, want) {
		t.Fatalf("stored seeds = %v, want %v", got, want)
	}
	if got, want := storedManorRows(t, w, "SELECT castle_id, crop_id, amount, start_amount, price, reward_type, next_period FROM castle_manor_procure ORDER BY next_period, castle_id, crop_id", 7),
		[][7]int32{{1, 5073, 40, 100, 50, 1, 0}, {1, 5068, 10, 10, 30, 1, 1}}; !equalRows(got, want) {
		t.Fatalf("stored crops = %v, want %v", got, want)
	}
}

// storedManorRows reads the n integer columns of query's rows.
func storedManorRows(t *testing.T, w *folkWorld, query string, n int) [][7]int32 {
	t.Helper()
	rows, err := w.srv.DB.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][7]int32
	for rows.Next() {
		var r [7]int32
		dest := make([]any, n)
		for i := range dest {
			dest[i] = &r[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
