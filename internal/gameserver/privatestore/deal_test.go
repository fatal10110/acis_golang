package privatestore

import (
	"sync"
	"sync/atomic"
	"testing"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const (
	potionID int32 = 40
	swordID  int32 = 50
)

type atomicIDs struct{ next atomic.Int32 }

func (ids *atomicIDs) NextID() (int32, error) { return ids.next.Add(1), nil }

func dealTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: potionID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, Destroyable: true, Weight: 5, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: swordID, Kind: item.KindWeapon, Slot: item.SlotRHand, Tradable: true, Destroyable: true, Weight: 100, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
	})
}

// dealWorld hands out object ids and inventories for one test.
type dealWorld struct {
	templates *item.Table
	ids       *atomicIDs
	svc       *invops.Service
	owners    int32
}

func newDealWorld() *dealWorld {
	ids := &atomicIDs{}
	ids.next.Store(10_000)
	return &dealWorld{templates: dealTemplates(), ids: ids, svc: invops.NewService(ids)}
}

// inventory returns a fresh player inventory holding each template/count
// pair of stacks.
func (w *dealWorld) inventory(t *testing.T, stacks ...[2]int32) *itemcontainer.Inventory {
	t.Helper()
	w.owners++
	inv := itemcontainer.NewPlayerInventory(w.owners, w.templates)
	for _, s := range stacks {
		if s[0] == swordID {
			for range s[1] {
				id, _ := w.ids.NextID()
				if inv.AddNew(swordID, 1, id) == nil {
					t.Fatalf("add sword")
				}
			}
			continue
		}
		id, _ := w.ids.NextID()
		if inv.AddNew(s[0], int(s[1]), id) == nil {
			t.Fatalf("add %d x%d", s[0], s[1])
		}
	}
	inv.DrainUpdates()
	return inv
}

func held(inv *itemcontainer.Inventory, templateID int32) int {
	total := 0
	for _, inst := range inv.Items() {
		if st := inst.Snapshot(); st.TemplateID == templateID {
			total += st.Count
		}
	}
	return total
}

// race runs fn on n goroutines released together.
func race(n int, fn func(i int)) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fn(i)
		}()
	}
	close(start)
	wg.Wait()
}

// TestConcurrentBuysNeverOversell races many buyers on the last units of
// one sell row: the units sold never exceed the units listed, every unit
// sold is paid for at the listed price, and the row leaves the list once
// sold out, which closes the store.
func TestConcurrentBuysNeverOversell(t *testing.T) {
	const (
		listed  = 10
		price   = 7
		perBuy  = 3
		buyers  = 16
		wallets = 1000
	)
	for range 20 {
		w := newDealWorld()
		ownerInv := w.inventory(t, [2]int32{potionID, 50})
		stack := ownerInv.ItemByTemplateID(potionID)
		var s Store
		if got := s.FillSell(ownerInv, []SellRow{{ObjectID: stack.ObjectID, Count: listed, Price: price}}, false, 0); got != FillOK {
			t.Fatalf("FillSell = %d", got)
		}
		s.SetOperateType(OperateSell)
		buyerInvs := make([]*itemcontainer.Inventory, buyers)
		for i := range buyerInvs {
			buyerInvs[i] = w.inventory(t, [2]int32{item.AdenaID, wallets})
		}

		var done atomic.Int32
		race(buyers, func(i int) {
			deal, err := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInvs[i]}, []PurchaseRow{{ObjectID: stack.ObjectID, Count: perBuy, Price: price}})
			if err != nil {
				t.Errorf("Buy: %v", err)
			}
			if deal.Status == DealDone {
				done.Add(1)
			}
		})

		sold := 0
		for _, inv := range buyerInvs {
			got := held(inv, potionID)
			sold += got
			if paid := wallets - held(inv, item.AdenaID); paid != got*price {
				t.Fatalf("buyer paid %d for %d units, want %d", paid, got, got*price)
			}
		}
		if want := listed / perBuy * perBuy; sold != want || int(done.Load()) != listed/perBuy {
			t.Fatalf("sold %d units in %d deals, want %d in %d", sold, done.Load(), want, listed/perBuy)
		}
		if got := held(ownerInv, potionID); got != 50-sold {
			t.Fatalf("owner potions = %d, want %d", got, 50-sold)
		}
		if got := held(ownerInv, item.AdenaID); got != sold*price {
			t.Fatalf("owner adena = %d, want %d", got, sold*price)
		}
		rows, _ := s.SellList()
		if len(rows) != 1 || rows[0].Count != listed-sold {
			t.Fatalf("sell list after the race = %+v, want %d units left", rows, listed-sold)
		}
	}
}

// TestConcurrentBuysSellOutOnce races buyers who each want one unit of a
// row that lists fewer units than there are buyers: exactly the listed
// units sell, and only the deal that emptied the list closes the store.
func TestConcurrentBuysSellOutOnce(t *testing.T) {
	const listed, price, buyers = 5, 20, 12
	w := newDealWorld()
	ownerInv := w.inventory(t, [2]int32{potionID, listed})
	stack := ownerInv.ItemByTemplateID(potionID)
	var s Store
	s.FillSell(ownerInv, []SellRow{{ObjectID: stack.ObjectID, Count: listed, Price: price}}, false, 0)
	s.SetOperateType(OperateSell)
	buyerInvs := make([]*itemcontainer.Inventory, buyers)
	for i := range buyerInvs {
		buyerInvs[i] = w.inventory(t, [2]int32{item.AdenaID, 100})
	}
	var done, emptied atomic.Int32
	race(buyers, func(i int) {
		deal, _ := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInvs[i]}, []PurchaseRow{{ObjectID: stack.ObjectID, Count: 1, Price: price}})
		if deal.Status == DealDone {
			done.Add(1)
		}
		if deal.Emptied {
			emptied.Add(1)
		}
	})
	if done.Load() != listed || emptied.Load() != 1 {
		t.Fatalf("deals done %d emptied %d, want %d and 1", done.Load(), emptied.Load(), listed)
	}
	if s.OperateType() != OperateNone {
		t.Fatalf("operate after selling out = %d, want none", s.OperateType())
	}
	if got := held(ownerInv, item.AdenaID); got != listed*price {
		t.Fatalf("owner adena = %d, want %d", got, listed*price)
	}
}

// TestConcurrentSellsNeverOverfill races many sellers on the last units
// one buy row still wants: the units bought never exceed the units wanted
// and the owner pays exactly units × price.
func TestConcurrentSellsNeverOverfill(t *testing.T) {
	const (
		wanted  = 5
		price   = 11
		perSell = 2
		sellers = 12
		wallet  = 1000
	)
	for range 20 {
		w := newDealWorld()
		ownerInv := w.inventory(t, [2]int32{item.AdenaID, wallet}, [2]int32{potionID, 1})
		var s Store
		if got := s.FillBuy(w.templates, []BuyRow{{TemplateID: potionID, Count: wanted, Price: price}}, wallet); got != FillOK {
			t.Fatalf("FillBuy = %d", got)
		}
		s.SetOperateType(OperateBuy)
		sellerInvs := make([]*itemcontainer.Inventory, sellers)
		for i := range sellerInvs {
			sellerInvs[i] = w.inventory(t, [2]int32{potionID, perSell})
		}
		var done atomic.Int32
		race(sellers, func(i int) {
			stack := sellerInvs[i].ItemByTemplateID(potionID)
			deal, err := s.Sell(w.svc, Trader{Inv: ownerInv}, Trader{Inv: sellerInvs[i]}, []SaleRow{{ObjectID: stack.ObjectID, ItemID: potionID, Count: perSell, Price: price}})
			if err != nil {
				t.Errorf("Sell: %v", err)
			}
			if deal.Status == DealDone {
				done.Add(1)
			}
		})
		bought := 0
		for _, inv := range sellerInvs {
			left := held(inv, potionID)
			bought += perSell - left
			if earned := held(inv, item.AdenaID); earned != (perSell-left)*price {
				t.Fatalf("seller earned %d for %d units, want %d", earned, perSell-left, (perSell-left)*price)
			}
		}
		if want := wanted / perSell * perSell; bought != want || int(done.Load()) != wanted/perSell {
			t.Fatalf("bought %d units in %d deals, want %d in %d", bought, done.Load(), want, wanted/perSell)
		}
		if got := held(ownerInv, potionID); got != 1+bought {
			t.Fatalf("owner potions = %d, want %d", got, 1+bought)
		}
		if got := held(ownerInv, item.AdenaID); got != wallet-bought*price {
			t.Fatalf("owner adena = %d, want %d", got, wallet-bought*price)
		}
		if rows := s.BuyList(); len(rows) != 1 || rows[0].Quantity != wanted-bought {
			t.Fatalf("buy list after the race = %+v, want %d units still wanted", rows, wanted-bought)
		}
	}
}

// TestDealsNeedTheStoreOpen pins that a deal only runs against a store
// still open for it: a sell store that went back to set-up or closed sells
// nothing, even a package row, and a buy store likewise buys nothing; the
// owner's set-up state is left alone.
func TestDealsNeedTheStoreOpen(t *testing.T) {
	w := newDealWorld()
	ownerInv := w.inventory(t, [2]int32{potionID, 4}, [2]int32{item.AdenaID, 500}, [2]int32{swordID, 1})
	potion := ownerInv.ItemByTemplateID(potionID)
	sword := ownerInv.ItemByTemplateID(swordID)
	buyerInv := w.inventory(t, [2]int32{item.AdenaID, 500})
	sellerInv := w.inventory(t, [2]int32{potionID, 2})
	for _, op := range []OperateType{OperateNone, OperateSellManage, OperateBuy, OperateManufacture} {
		var s Store
		s.FillSell(ownerInv, []SellRow{{ObjectID: potion.ObjectID, Count: 4, Price: 1}, {ObjectID: sword.ObjectID, Count: 1, Price: 1}}, true, 0)
		s.SetOperateType(op)
		deal, _ := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInv}, []PurchaseRow{{ObjectID: potion.ObjectID, Count: 4, Price: 1}})
		if deal.Status != DealRefused || s.OperateType() != op {
			t.Fatalf("buy from a store in %d: status %d, operate now %d", op, deal.Status, s.OperateType())
		}
	}
	for _, op := range []OperateType{OperateNone, OperateBuyManage, OperateSell} {
		var s Store
		s.FillBuy(w.templates, []BuyRow{{TemplateID: potionID, Count: 2, Price: 1}}, 500)
		s.SetOperateType(op)
		stack := sellerInv.ItemByTemplateID(potionID)
		deal, _ := s.Sell(w.svc, Trader{Inv: ownerInv}, Trader{Inv: sellerInv}, []SaleRow{{ObjectID: stack.ObjectID, ItemID: potionID, Count: 2, Price: 1}})
		if deal.Status != DealRefused || s.OperateType() != op {
			t.Fatalf("sell to a store in %d: status %d, operate now %d", op, deal.Status, s.OperateType())
		}
	}
	if held(buyerInv, potionID) != 0 || held(sellerInv, potionID) != 2 || held(ownerInv, item.AdenaID) != 500 {
		t.Fatal("a refused deal moved items")
	}
}

// TestDealDivergences pins the trust gaps the store closes beyond the
// reference: buying more units than a row lists, selling a buy row an
// item at another enchant or more units than it still wants, and buying
// part of a package. Each is refused with nothing moved.
func TestDealDivergences(t *testing.T) {
	t.Run("past the listed count", func(t *testing.T) {
		w := newDealWorld()
		ownerInv := w.inventory(t, [2]int32{potionID, 10})
		stack := ownerInv.ItemByTemplateID(potionID)
		buyerInv := w.inventory(t, [2]int32{item.AdenaID, 100})
		var s Store
		s.FillSell(ownerInv, []SellRow{{ObjectID: stack.ObjectID, Count: 3, Price: 1}}, false, 0)
		s.SetOperateType(OperateSell)
		for _, rows := range [][]PurchaseRow{
			{{ObjectID: stack.ObjectID, Count: 4, Price: 1}},
			{{ObjectID: stack.ObjectID, Count: 2, Price: 1}, {ObjectID: stack.ObjectID, Count: 2, Price: 1}},
		} {
			if deal, _ := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInv}, rows); deal.Status != DealRefused {
				t.Fatalf("rows %+v: status %d", rows, deal.Status)
			}
		}
		if held(buyerInv, potionID) != 0 || held(ownerInv, potionID) != 10 {
			t.Fatal("an over-listed purchase moved items")
		}
	})
	t.Run("enchant and wanted quantity", func(t *testing.T) {
		w := newDealWorld()
		ownerInv := w.inventory(t, [2]int32{item.AdenaID, 1000}, [2]int32{swordID, 1}, [2]int32{potionID, 1})
		sellerInv := w.inventory(t, [2]int32{swordID, 1}, [2]int32{potionID, 5})
		sword := sellerInv.ItemByTemplateID(swordID)
		potion := sellerInv.ItemByTemplateID(potionID)
		var s Store
		s.FillBuy(w.templates, []BuyRow{{TemplateID: swordID, Enchant: 3, Count: 1, Price: 10}, {TemplateID: potionID, Count: 2, Price: 1}}, 1000)
		s.SetOperateType(OperateBuy)
		for name, rows := range map[string][]SaleRow{
			"a +0 sword claimed +3":    {{ObjectID: sword.ObjectID, ItemID: swordID, Enchant: 3, Count: 1, Price: 10}},
			"a +0 sword offered as +0": {{ObjectID: sword.ObjectID, ItemID: swordID, Count: 1, Price: 10}},
			"more than still wanted":   {{ObjectID: potion.ObjectID, ItemID: potionID, Count: 3, Price: 1}},
		} {
			if deal, _ := s.Sell(w.svc, Trader{Inv: ownerInv}, Trader{Inv: sellerInv}, rows); deal.Status != DealRefused {
				t.Fatalf("%s: status %d", name, deal.Status)
			}
		}
		if held(sellerInv, swordID) != 1 || held(sellerInv, potionID) != 5 || held(ownerInv, item.AdenaID) != 1000 {
			t.Fatal("a mismatched sale moved items")
		}
	})
	t.Run("part of a package", func(t *testing.T) {
		w := newDealWorld()
		ownerInv := w.inventory(t, [2]int32{potionID, 4}, [2]int32{swordID, 1})
		potion := ownerInv.ItemByTemplateID(potionID)
		sword := ownerInv.ItemByTemplateID(swordID)
		buyerInv := w.inventory(t, [2]int32{item.AdenaID, 100})
		var s Store
		s.FillSell(ownerInv, []SellRow{{ObjectID: potion.ObjectID, Count: 4, Price: 1}, {ObjectID: sword.ObjectID, Count: 1, Price: 1}}, true, 0)
		s.SetOperateType(OperatePackageSell)
		for _, rows := range [][]PurchaseRow{
			{{ObjectID: potion.ObjectID, Count: 4, Price: 1}},
			{{ObjectID: potion.ObjectID, Count: 3, Price: 1}, {ObjectID: sword.ObjectID, Count: 1, Price: 1}},
		} {
			if deal, _ := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInv}, rows); deal.Status != DealRefused {
				t.Fatalf("rows %+v: status %d", rows, deal.Status)
			}
		}
		deal, _ := s.Buy(w.svc, Trader{Inv: ownerInv}, Trader{Inv: buyerInv}, []PurchaseRow{{ObjectID: potion.ObjectID, Count: 4, Price: 1}, {ObjectID: sword.ObjectID, Count: 1, Price: 1}})
		if deal.Status != DealDone || !deal.Emptied || held(buyerInv, item.AdenaID) != 95 {
			t.Fatalf("whole package: status %d emptied %v buyer adena %d", deal.Status, deal.Emptied, held(buyerInv, item.AdenaID))
		}
	})
}

// TestCraftNeedsAListedRecipe pins that a workshop crafts only what it
// lists, at its listed cost.
func TestCraftNeedsAListedRecipe(t *testing.T) {
	var s Store
	s.SetManufacture([]ManufactureItem{{RecipeID: 1, Cost: 100}})
	ran := -1
	if !s.Craft(1, func(cost int) { ran = cost }) || ran != 100 {
		t.Fatalf("listed recipe: ran with cost %d", ran)
	}
	ran = -1
	if s.Craft(2, func(cost int) { ran = cost }) || ran != -1 {
		t.Fatalf("unlisted recipe crafted with cost %d", ran)
	}
}
