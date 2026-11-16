package npcs

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// rolloverDarkCodaMature is the mature item a Dark Coda crop bought pays
// into the owning clan's warehouse.
const rolloverDarkCodaMature = 5103

// TestManorRolloverChargesAndPaysOnce runs Gludio's manor through four
// rollovers with crops sold by packet. Owner-approved divergences:
//   - #5109: repeated rows of one stack are checked on their total, so a
//     sale past the stack is refused and the castle's count only drops by
//     the crops actually destroyed;
//   - #5108: the next period's lists are charged to the treasury when they
//     go live, and lists the treasury cannot pay for do not go live, so the
//     refund of crops left unbought only gives back what was charged;
//   - #5140: the next period holds its own copies of the entries, so a sale
//     never lowers it, and a rollover the treasury cannot cover leaves no
//     sold-down entry running to pay out again.
func TestManorRolloverChargesAndPaysOnce(t *testing.T) {
	t.Parallel()
	templates := item.NewTable(append(tradeTemplates().All(), &item.Template{
		ID: rolloverDarkCodaMature, Name: "Mature", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Weight: 1,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	}))
	w := bootManorTrade(t, 0, gameservertest.WithItemTemplates(templates))
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)
	m, clock := w.srv.CastleManor, w.srv.CastleManorClock
	gludio, ok := w.srv.Castles.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio")
	}
	setNextCrops := func() {
		m.SetNextCropProcure(taxCastleID, []*castlemanor.Procure{castlemanor.NewProcure(tradeDarkCodaCrop, 100, 50, 100, 1)})
	}

	// 05:58, modifiable. 7 + 7 of the 12 held are refused whole; 2 + 2 sell.
	wantOnly(t, "sell past the stack", w.send(t, encodeProcureCropList(
		[4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 7}, [4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 7},
	)), []byte{serverpackets.OpcodeActionFailed})
	w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 2}, [4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 2}))
	if got := w.amountLeft(t, taxCastleID, tradeDarkCodaCrop, true); got != 36 {
		t.Fatalf("crops wanted after selling 4 = %d, want 36", got)
	}

	// Rollover 1 (day 1, 20:00) with an empty treasury: the 64 crops bought
	// pay (int)(64 * 0.9) = 57, the 36 left give back 36 * 50 = 1800, and
	// the next period's 100 crops at 50 cannot be charged, so nothing goes
	// live.
	setNextCrops()
	clock.Advance(14*time.Hour + 2*time.Minute)
	if got := m.Status(); got != castlemanor.StatusMaintenance {
		t.Fatalf("status at 20:00 = %s, want MAINTENANCE", got)
	}
	assertManorRollover(t, w, gludio, "rollover 1", 57, 1800, 0)
	assertNoCrops(t, w, "rollover 1")

	// Rollover 2 (day 2): 5000 in the treasury pays for the 100 crops at 50,
	// which go live in full while the next period gets its own copy.
	if !gludio.EditTreasury(5000, true) {
		t.Fatal("cannot fill Gludio's treasury")
	}
	clock.Advance(6 * time.Minute)
	setNextCrops()
	clock.Advance(23*time.Hour + 54*time.Minute)
	assertManorRollover(t, w, gludio, "rollover 2", 57, 1800, 0)
	running, okRunning := m.CropProcureOf(taxCastleID, tradeDarkCodaCrop, false)
	next, okNext := m.CropProcureOf(taxCastleID, tradeDarkCodaCrop, true)
	if !okRunning || !okNext || running == next || running.Amount() != 100 || next.Amount() != 100 {
		t.Fatalf("after rollover 2 running = %+v, next = %+v, want 100 of each in separate entries", running, next)
	}

	// A sale of 8 in the running period leaves the next period at 100.
	clock.Advance(6 * time.Minute)
	w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 4}, [4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 4}))
	if got := w.held(t, tradeDarkCodaCrop); got != 0 {
		t.Fatalf("crops held = %d, want 0", got)
	}
	if got := w.amountLeft(t, taxCastleID, tradeDarkCodaCrop, true); got != 92 {
		t.Fatalf("crops wanted after selling 8 = %d, want 92", got)
	}
	if next, ok := m.CropProcureOf(taxCastleID, tradeDarkCodaCrop, true); !ok || next.Amount() != 100 {
		t.Fatalf("next period's crops after a sale = %+v, want 100 wanted", next)
	}

	// Rollover 3 (day 3) with the treasury spent: the 8 bought pay 7, the 92
	// left give back 4600 of the 5000 charged, and the next period, which
	// cannot be charged, does not go live.
	clock.Advance(23*time.Hour + 54*time.Minute)
	assertManorRollover(t, w, gludio, "rollover 3", 64, 1800+4600, 0)
	assertNoCrops(t, w, "rollover 3")

	// Rollover 4 (day 4) pays nothing more.
	clock.Advance(24 * time.Hour)
	assertManorRollover(t, w, gludio, "rollover 4", 64, 1800+4600, 0)

	// The tax period moves the refunds into the treasury: 5000 put in, 5000
	// charged, 4600 of it and the running list's 1800 given back.
	gludio.UpdateTaxes()
	if got := gludio.Treasury(); got != 1800+4600 {
		t.Fatalf("treasury after the tax update = %d, want %d", got, 1800+4600)
	}
}

// assertManorRollover checks the Rivals' warehouse holds mature mature
// crops, Gludio's seed income is income and its treasury treasury, in
// memory and stored.
func assertManorRollover(t *testing.T, w *manorTradeWorld, gludio *castle.Castle, what string, mature, income, treasury int64) {
	t.Helper()
	w.srv.FlushItems(t)
	var held, storedTreasury, storedIncome int64
	ctx := context.Background()
	if err := w.srv.DB.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'CLANWH'", residenceRivalClan, rolloverDarkCodaMature,
	).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != mature {
		t.Fatalf("%s: clan warehouse holds %d mature crops, want %d", what, held, mature)
	}
	if got := gludio.SeedIncome(); got != income {
		t.Fatalf("%s: seed income = %d, want %d", what, got, income)
	}
	if got := gludio.Treasury(); got != treasury {
		t.Fatalf("%s: treasury = %d, want %d", what, got, treasury)
	}
	if err := w.srv.DB.QueryRowContext(ctx, "SELECT treasury, seedIncome FROM castle WHERE id = ?", taxCastleID).Scan(&storedTreasury, &storedIncome); err != nil {
		t.Fatal(err)
	}
	if storedTreasury != treasury || storedIncome != income {
		t.Fatalf("%s: stored treasury %d, income %d, want %d and %d", what, storedTreasury, storedIncome, treasury, income)
	}
}

// assertNoCrops checks Gludio runs no list in either period, stored or in
// memory.
func assertNoCrops(t *testing.T, w *manorTradeWorld, what string) {
	t.Helper()
	m := w.srv.CastleManor
	for _, next := range []bool{false, true} {
		if len(m.CropProcure(taxCastleID, next)) != 0 || len(m.SeedProduction(taxCastleID, next)) != 0 {
			t.Fatalf("%s: Gludio's lists (next=%t) = %v / %v, want none", what, next, m.SeedProduction(taxCastleID, next), m.CropProcure(taxCastleID, next))
		}
	}
	var rows int64
	if err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT (SELECT COUNT(*) FROM castle_manor_procure WHERE castle_id = ?) + (SELECT COUNT(*) FROM castle_manor_production WHERE castle_id = ?)", taxCastleID, taxCastleID,
	).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("%s: Gludio has %d stored manor rows, want none", what, rows)
	}
}
