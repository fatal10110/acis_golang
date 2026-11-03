package merchant

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/rs/zerolog"
)

// reloadingIDs allocates item ids and, on its first allocation, starts a
// reload of svc to lists. It returns only once that reload has finished or
// is waiting for the purchase in progress, so the reload gets every chance
// to land between the purchase's first created item and its next sale.
type reloadingIDs struct {
	svc   *Service
	lists *buylist.Table
	next  int32
	done  chan error
}

func (r *reloadingIDs) NextID() (int32, error) {
	r.next++
	if r.done != nil {
		return r.next, nil
	}
	r.done = make(chan error, 1)
	go func() { r.done <- r.svc.Reload(context.Background(), r.lists) }()
	for {
		select {
		case err := <-r.done:
			r.done <- err
			return r.next, nil
		default:
		}
		if !r.svc.reloadMu.TryRLock() {
			// The reload is waiting for the purchase.
			return r.next, nil
		}
		r.svc.reloadMu.RUnlock()
		runtime.Gosched()
	}
}

// A reload changing a limited product while a purchase of it is between
// its stock check and its sale does not cost the buyer the item: the
// reload waits for the purchase, which sells from the stock it checked.
// A purchase starting after the reload with the replaced list is refused
// before it pays.
func TestBuyRacingReloadDeliversCheckedItems(t *testing.T) {
	t.Parallel()
	const unlimitedID, limitedID = 18, 17
	templates := item.NewTable([]*item.Template{
		{ID: unlimitedID, Kind: item.KindEtcItem, Stackable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: limitedID, Kind: item.KindEtcItem, Stackable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
	})
	unlimited := buylist.Product{BuyListID: 1, ItemID: unlimitedID, MaxCount: -1, RestockDelayMillis: -60000}
	old := buylist.Product{BuyListID: 1, ItemID: limitedID, MaxCount: 5, RestockDelayMillis: 3600000}
	fresh := old
	fresh.MaxCount = 6
	table := func(p buylist.Product) *buylist.Table {
		return buylist.NewTable([]buylist.List{{ID: 1, NPCID: 30001, Products: []buylist.Product{unlimited, p}}})
	}
	oldLists := table(old)
	stock := NewStock(oldLists, nil, nil, time.Now, zerolog.Nop())
	ids := &reloadingIDs{lists: table(fresh)}
	svc := NewService(oldLists, stock, ids, DefaultConfig())
	ids.svc = svc
	list, _ := svc.List(1)
	inv := itemcontainer.NewPlayerInventory(1, templates)
	rows := []BuyRow{{ItemID: unlimitedID, Count: 1}, {ItemID: limitedID, Count: 1}}

	// The unlimited item is created before the limited one is sold; its id
	// allocation starts the reload.
	res, err := svc.Buy(inv, list, rows, 0, true)
	if err != nil || res.Outcome != BuyDone {
		t.Fatalf("purchase = %+v, %v; want done", res, err)
	}
	if err := <-ids.done; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int32{unlimitedID, limitedID} {
		if it := inv.ItemByTemplateID(id); it == nil || it.Count != 1 {
			t.Fatalf("item %d after the purchase = %v, want 1", id, it)
		}
	}
	if got := svc.Count(fresh); got != fresh.MaxCount {
		t.Fatalf("reloaded product count = %d, want %d", got, fresh.MaxCount)
	}

	// The list read before the reload is now stale.
	res, err = svc.Buy(inv, list, rows, 0, true)
	if err != nil || res.Outcome != BuyDropped {
		t.Fatalf("stale-list purchase = %+v, %v; want dropped", res, err)
	}
	if it := inv.ItemByTemplateID(unlimitedID); it.Count != 1 {
		t.Fatalf("stale-list purchase created %d unlimited items, want none", it.Count-1)
	}
}
