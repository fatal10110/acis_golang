package trade

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTradeSettlesWhileTheItemPersisterFlushes settles a trade on the real
// pool while another goroutine keeps flushing the item persister and running
// the inventory-update tick. The settlement's check reads only what it was
// handed before the exchange locked both inventories, and the persister never
// waits on a container, so the trade, the flushes and the ticks all finish,
// and the rows end as the trade left them.
func TestTradeSettlesWhileTheItemPersisterFlushes(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithRealPool())
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, potionID, 3)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, potions, 2)

	stop, flushed := make(chan struct{}), make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				flushed <- nil
				return
			default:
			}
			if err := h.srv.ItemInstances.Save(context.Background()); err != nil {
				flushed <- err
				return
			}
			h.srv.InventoryUpdates.Tick()
		}
	}()
	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	close(stop)
	select {
	case err := <-flushed:
		if err != nil {
			t.Fatalf("item flush: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the item flush did not finish: lock-order deadlock between the trade's exchange and the persister")
	}

	h.srv.Settle(t)
	h.srv.FlushItems(t)
	assertItemRows(t, h, h.firstID, "57x60 20x2")
	assertItemRows(t, h, h.secondID, "57x40 20x1")
}
