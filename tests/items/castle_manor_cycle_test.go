package items

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped manors.xml: Gludio's crop Dark Coda (5073) matures as Mature
// Dark Coda (5103), its crop Red Coda (5068) as Mature Red Coda (5098).
const (
	darkCodaMatureID int32 = 5103
	redCodaCropID    int32 = 5068
	redCodaMatureID  int32 = 5098
)

// Reference: CastleManorManager.changeMode and Castle.updateOwnerInDB.
//
// TestCastleManorCycleReachesTheOwner runs Gludio's manor through the
// 20:00 rollover and the end of maintenance with its owner's leader in the
// world: the crops the castle bought land in the clan warehouse, the money
// left for unsold crops comes back as seed income, the next period's lists
// become the running ones and are stored, and at 20:06 the leader is told
// the manor information was updated. Taking the castle from the clan then
// empties its lists.
func TestCastleManorCycleReachesTheOwner(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	manorCfg, _ := shippedManor()
	_, shipped := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfGludioID, lordsCrownID, darkCodaMatureID, redCodaMatureID} {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	cfg := castlemanor.DefaultConfig()
	cfg.SavePeriod = 0
	start := time.Date(2026, 10, 5, 19, 59, 0, 0, time.UTC)

	w := bootCastleWorld(t, 1, func(db *sql.DB) {
		for _, q := range []string{
			"UPDATE castle SET treasury = 5000 WHERE id = 1",
			// The running period: 100 Dark Coda wanted at 50, 40 still
			// wanted; 10 Red Coda at 7, 9 still wanted. The next period:
			// 10 Dark Coda seeds for sale, 100 Dark Coda wanted at 30.
			"INSERT INTO castle_manor_procure VALUES (1, 5073, 40, 100, 50, 1, 0), (1, 5068, 9, 10, 7, 2, 0), (1, 5073, 100, 100, 30, 1, 1)",
			"INSERT INTO castle_manor_production VALUES (1, 5016, 10, 10, 5, 1)",
		} {
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				t.Fatalf("seed manor: %v", err)
			}
		}
	},
		gameservertest.WithManor(manorCfg),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCastleManor(cfg, start, castlemanor.WithRoll(func(int) int { return 0 })),
	)
	w.enter(t)
	m := w.srv.CastleManor

	w.srv.CastleManorClock.Advance(time.Minute)
	if got := m.Status(); got != castlemanor.StatusMaintenance {
		t.Fatalf("status at 20:00 = %s, want MAINTENANCE", got)
	}
	// 60 Dark Coda bought pay (int)(60 * 0.9) = 54; the 1 Red Coda bought
	// truncates to 0 and the roll of 0 makes it 1.
	w.srv.FlushItems(t)
	for _, tc := range []struct {
		itemID int32
		want   int64
	}{{darkCodaMatureID, 54}, {redCodaMatureID, 1}} {
		if got := queryCastleInt(t, w.srv, "SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'CLANWH'", castleClanID, tc.itemID); got != tc.want {
			t.Fatalf("clan warehouse holds %d of %d, want %d", got, tc.itemID, tc.want)
		}
	}
	gludio, _ := w.srv.Castles.Get(1)
	if gludio.SeedIncome() <= 0 {
		t.Fatalf("Gludio seed income = %d, want the taxed share of 40*50 + 9*7", gludio.SeedIncome())
	}
	running := m.CropProcure(1, false)
	if len(running) != 1 || running[0].ID != darkCodaCropID || running[0].Price != 30 {
		t.Fatalf("running crops = %+v, want the former next period's", running)
	}
	if got := queryCastleInt(t, w.srv, "SELECT COUNT(*) FROM castle_manor_procure WHERE castle_id = 1 AND next_period = 0 AND crop_id = ? AND price = 30", darkCodaCropID); got != 1 {
		t.Fatalf("stored running Dark Coda rows at 30 = %d, want 1", got)
	}
	if got := queryCastleInt(t, w.srv, "SELECT COUNT(*) FROM castle_manor_procure WHERE crop_id = ?", redCodaCropID); got != 0 {
		t.Fatalf("stored Red Coda rows = %d, want the finished period's gone", got)
	}
	drainUntilQuiet(t, w.leader)

	w.srv.CastleManorClock.Advance(6 * time.Minute)
	frames := collectFrames(w.leader)
	if !slices.ContainsFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageManorInformationUpdated
	}) {
		t.Fatalf("leader got opcodes %x, want THE_MANOR_INFORMATION_HAS_BEEN_UPDATED", opcodesOf(frames))
	}

	w.castleCommand(t, "remove gludio_castle")
	for _, next := range []bool{false, true} {
		if len(m.SeedProduction(1, next)) != 0 || len(m.CropProcure(1, next)) != 0 {
			t.Fatalf("Gludio manor lists (next=%t) kept after losing its owner", next)
		}
	}
}
