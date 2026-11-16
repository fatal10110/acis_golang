package clan

import (
	"runtime"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanWarehouseDepositLosesToDissolution has a member's deposit pass
// the membership check and wait for its item operation while the leader
// dissolves the clan with no delay. The dissolution destroys the clan
// warehouse first, so the deposit finds it gone: the member keeps its sword
// and its adena (no fee), and no CLANWH row is left under the dissolved
// clan's id.
func TestClanWarehouseDepositLosesToDissolution(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.DissolveDays = 0
	w := bootClanWarehouse(t, nil, map[int32]int32{item.AdenaID: whAdena, swordID: 1},
		gameservertest.WithClanConfig(cfg), dissolutionPages(t), gameservertest.WithRealPool())
	w.raiseLevel(t)
	w.talkToMaster(t)
	w.talkToKeeper(t, w.member, w.memberID)
	w.keeperCommand(t, w.member, "DepositC")

	// Holding the recruit's item operation parks its deposit right after
	// the gate, before it touches the warehouse.
	ops := w.srv.ItemInstances
	release := ops.BeginOperation(w.memberID)
	t.Cleanup(release)
	sword := w.memberItems[swordID]
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{sword, 1}))
	for ops.ParkedOperations() == 0 {
		runtime.Gosched()
	}

	// The leader's page still offers the dissolution. Every frame is read
	// as it comes: a drain would wait for the parked deposit too.
	w.leader.Send(encodeBypass("npc_" + itoa(w.master.ObjectID()) + "_dissolve_clan"))
	for {
		if ids := messages(t, [][]byte{w.leader.Read()}); len(ids) == 1 && ids[0] == msgClanHasDispersed {
			break
		}
	}
	release()
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	if s, a := w.held(t, w.memberID, swordID), w.held(t, w.memberID, item.AdenaID); s != 1 || a != whAdena {
		t.Fatalf("recruit holds sword %d adena %d after the dissolution, want 1 and %d", s, a, whAdena)
	}
	if counts, _ := w.clanRows(t); len(counts) != 0 {
		t.Fatalf("dissolved clan's warehouse rows = %v, want none", counts)
	}
	for _, r := range w.savedRows(t, w.memberID) {
		if r.ObjectID == sword && r.Location != item.LocationInventory {
			t.Fatalf("recruit's sword row at %v, want INVENTORY", r.Location)
		}
	}
}
