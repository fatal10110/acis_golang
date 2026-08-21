package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// wireWeightNotifier mirrors character_flow.go's attachLivePlayer weight
// wiring — PcInventory.updateWeight() sends StatusUpdate(CUR_LOAD) to the
// owner on every changed total weight, then refreshes the weight penalty —
// for tests that build a *livePlayer directly instead of through the full
// login flow.
func wireWeightNotifier(live *livePlayer) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inv.SetWeightNotifier(func() {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusCurrentLoad, Value: inv.TotalWeight()},
		}))
		live.RefreshWeightPenalty()
	})
}

// TestWeightNotifierSendsStatusUpdateOnWeightChange is the regression test
// for issue #1137: PcInventory.updateWeight() (PcInventory.java:101-113)
// sends StatusUpdate(CUR_LOAD) to the item owner on every weight change, not
// only when the change happens to cross a weight-penalty band. A weight
// change that stays inside the current band must still refresh the client's
// load gauge.
func TestWeightNotifierSendsStatusUpdateOnWeightChange(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID: 1, Kind: item.KindEtcItem, Weight: 10, Stackable: true, EtcItem: &item.EtcItemDetail{},
	}})
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	wireWeightNotifier(live)

	testsupport.ResetCapture(capture)
	live.Inventory().AddNew(1, 5, 100)
	if !live.Inventory().UpdateWeight() {
		t.Fatal("UpdateWeight() = false, want true for a nonzero weight change")
	}

	testsupport.AssertOpcodeSequence(t, capture.Frames(), serverpackets.OpcodeStatusUpdate)
	assertStatusAttrs(t, capture.Frames()[0], live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
}

// TestWeightNotifierSkipsUnchangedWeight pins Inventory.updateWeight's
// "calculated value is identical, don't send any update" early return
// (Inventory.java:168-170): recomputing an unchanged total must not refresh
// the load gauge again.
func TestWeightNotifierSkipsUnchangedWeight(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID: 1, Kind: item.KindEtcItem, Weight: 10, Stackable: true, EtcItem: &item.EtcItemDetail{},
	}})
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	wireWeightNotifier(live)

	live.Inventory().AddNew(1, 5, 100)
	live.Inventory().UpdateWeight()
	testsupport.ResetCapture(capture)

	if live.Inventory().UpdateWeight() {
		t.Fatal("UpdateWeight() = true, want false when total weight has not changed")
	}
	if len(capture.Frames()) != 0 {
		t.Fatalf("frames = %d, want none for an unchanged weight recompute", len(capture.Frames()))
	}
}
