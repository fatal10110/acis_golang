package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestWarehouseDepositIgnoredWithoutAllowWarehouse pins server.properties
// AllowWarehouse=False: the keeper still opens the deposit window, but the
// deposit list the client sends is neither read nor answered, so nothing
// moves and no fee is taken. Withdrawals are not gated.
func TestWarehouseDepositIgnoredWithoutAllowWarehouse(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}}, gameservertest.WithWarehouseDisabled())
	sword, potion := w.items[swordID], w.items[potionID]

	requireOpcodes(t, "DepositP", w.command(t, "DepositP"), serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
	w.c.Send(encodeDeposit(whRow{sword, 1}, whRow{potion, 4}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("deposit answered %x, want nothing", opcodes(frames))
	}
	if s, p, a := w.held(t, sword), w.held(t, potion), w.adena(t); s != 1 || p != 10 || a != whAdena {
		t.Fatalf("after the ignored deposit: sword %d potions %d adena %d, want 1, 10 and %d", s, p, a, whAdena)
	}
	if got := rowsAt(saved(t, w.srv, w.player), item.LocationWarehouse); len(got) != 0 {
		t.Fatalf("saved warehouse rows = %v, want none", got)
	}

	// The connection reads on: the keeper still answers.
	requireMessage(t, "WithdrawP of the empty warehouse", w.command(t, "WithdrawP"), serverpackets.SystemMessageNoItemDepositedInWH, true)
}
