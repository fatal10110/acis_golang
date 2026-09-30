package trade

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const potionID = 20

// TestTradeWritesBothSidesWithoutTheTick pins that a settled trade's own
// write lands every row it changed, on both sides, before the lazy item tick
// ever runs.
func TestTradeWritesBothSidesWithoutTheTick(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, potionID, 3)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, potions, 2)

	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	h.srv.Settle(t)
	h.srv.FlushPersistence(t)

	assertItemRows(t, h, h.firstID, "57x60 20x2")
	assertItemRows(t, h, h.secondID, "57x40 20x1")
}

// TestTradeRowsCommitOrRollBackTogether pins that a trade's rows reach the
// database as one unit. The database refuses the receiver's new adena row
// partway through the write; written row by row, the giver's reduced stack
// and the potion rows would still commit and 40 adena would exist nowhere.
// The whole write rolls back instead, both sides keep their pre-trade rows,
// and the lazy item tick — which still holds every changed item — writes the
// traded state once the database takes it again.
func TestTradeRowsCommitOrRollBackTogether(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, potionID, 3)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, potions, 2)

	dropFault := refuseAdenaRowFor(t, h, h.secondID)
	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	h.srv.Settle(t)
	h.srv.FlushPersistence(t)

	assertItemRows(t, h, h.firstID, "57x100")
	assertItemRows(t, h, h.secondID, "20x3")

	dropFault()
	h.srv.FlushItems(t)
	assertItemRows(t, h, h.firstID, "57x60 20x2")
	assertItemRows(t, h, h.secondID, "57x40 20x1")
}

// TestTradeWritesAStackOnBothLegsOnce covers a row one trade touches twice:
// both players offer adena, so each adena stack gives on one leg and receives
// on the other. The trade's write carries each row once, with its final count.
func TestTradeWritesAStackOnBothLegsOnce(t *testing.T) {
	h := bootTraders(t)
	firstAdena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	secondAdena := h.srv.GiveItem(t, h.secondID, item.AdenaID, 50)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, firstAdena, 40, secondAdena, 10)

	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	h.srv.Settle(t)
	h.srv.FlushPersistence(t)

	assertItemRows(t, h, h.firstID, "57x70")
	assertItemRows(t, h, h.secondID, "57x80")
}

// refuseAdenaRowFor makes the database refuse any adena row written for
// ownerID until the returned func drops the trigger. The trigger is dropped at
// cleanup too, so a pooled database never carries it into another test.
func refuseAdenaRowFor(t *testing.T, h *traders, ownerID int32) func() {
	t.Helper()
	ctx := context.Background()
	drop := func() {
		if _, err := h.srv.DB.ExecContext(ctx, "DROP TRIGGER IF EXISTS trade_atomic_fault"); err != nil {
			t.Errorf("drop fault trigger: %v", err)
		}
	}
	t.Cleanup(drop)
	if _, err := h.srv.DB.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER trade_atomic_fault BEFORE INSERT ON items FOR EACH ROW
		IF NEW.owner_id = %d AND NEW.item_id = %d THEN
			SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected receiver row fault';
		END IF`, ownerID, item.AdenaID)); err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}
	return drop
}

func assertTradeSucceeded(t *testing.T, h *traders) {
	t.Helper()
	for _, who := range h.both() {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 1 {
			t.Fatalf("%s SendTradeDone success = %d, want 1", who.name, got)
		}
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageTradeSuccessful)
	}
}

// assertItemRows compares ownerID's persisted rows, as space-separated
// "templatexcount" pairs in any order, with want.
func assertItemRows(t *testing.T, h *traders, ownerID int32, want string) {
	t.Helper()
	rows, err := h.srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items of %d: %v", ownerID, err)
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, fmt.Sprintf("%dx%d", row.TemplateID, row.Count))
	}
	wantPairs := strings.Fields(want)
	slices.Sort(got)
	slices.Sort(wantPairs)
	if !slices.Equal(got, wantPairs) {
		t.Fatalf("persisted rows of %d = %v, want %v", ownerID, got, wantPairs)
	}
}
