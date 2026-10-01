package privatestore

import (
	"math"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Trader is one side of a store deal.
type Trader struct {
	Inv *itemcontainer.Inventory
	// Bound reports an item tied where it is that may not change hands
	// whatever its own state: a summoned or ridden pet's collar, the
	// selected enchant scroll. A nil Bound binds nothing. It must not take
	// an inventory lock: the deal calls it holding both.
	Bound func(objectID int32) bool
	// Casting keeps the trader's augmented items where they are.
	Casting bool
}

// PurchaseRow is one row a buyer asks a sell store for.
type PurchaseRow struct {
	ObjectID int32
	Count    int
	Price    int
}

// SaleRow is one row a seller offers a buy store.
type SaleRow struct {
	ObjectID int32
	ItemID   int32
	Enchant  int
	Count    int
	Price    int
}

// DealStatus is how a store deal ended.
type DealStatus uint8

const (
	// DealRefused changed nothing and owes no message.
	DealRefused DealStatus = iota
	// DealDone moved the items and the adena.
	DealDone
	// DealNotEnoughAdena refused a deal the paying side cannot afford.
	DealNotEnoughAdena
	// DealWeightExceeded refused a purchase the buyer cannot carry.
	DealWeightExceeded
	// DealSlotsFull refused a purchase the buyer has no slots for.
	DealSlotsFull
)

// Moved is one row a deal handed over: the item, how many units and its
// enchant.
type Moved struct {
	ItemID    int32
	Count     int
	Enchant   int
	Stackable bool
}

// Deal is the outcome of a store deal.
type Deal struct {
	Status DealStatus
	// Rows lists what changed hands, in request order.
	Rows []Moved
	// Persist carries the item writes the moves made.
	Persist []invops.Persist
	// Emptied reports that the deal sold out the store's list, which closed
	// the store.
	Emptied bool
}

// Buy sells rows of the sell list to buyer, who pays owner in adena.
//
// The store must be selling, as a sell or a package sell store. Every row
// must name a listed item at its listed price, for no more units
// than are still listed, and the owner must still hold it free to trade. A
// package sale must take every listed row whole. The buyer must afford the
// total and carry its weight; each stack it does not hold takes a slot, a
// stackable row always counting one. All of it is checked against both
// inventories as the moves find them, so the deal moves everything or
// nothing. A deal that would pay more than the largest int32 is refused.
func (s *Store) Buy(svc *invops.Service, owner, buyer Trader, rows []PurchaseRow) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner.Inv == nil || buyer.Inv == nil || owner.Inv == buyer.Inv || len(rows) == 0 {
		return Deal{}, nil
	}
	// The store must still be selling as the deal starts: a quit or a
	// return to set-up racing the buyer's request ends the deal here, so
	// the package rule and the sold-out close act on the type checked.
	operate := s.OperateType()
	if operate != OperateSell && operate != OperatePackageSell {
		return Deal{}, nil
	}
	if operate == OperatePackageSell && len(s.sell) > len(rows) {
		return Deal{}, nil
	}
	templates := owner.Inv.Templates()
	var (
		total, weight int64
		slots         int
		moves         = make([]invops.Move, 0, len(rows))
		moved         = make([]Moved, 0, len(rows))
	)
	asked := make(map[int32]int, len(rows))
	for _, row := range rows {
		if row.Count < 1 {
			return Deal{}, nil
		}
		listed, ok := s.sellRowLocked(row.ObjectID, row.Price)
		if !ok {
			return Deal{}, nil
		}
		tmpl, ok := templates.Get(listed.TemplateID)
		if !ok {
			return Deal{}, nil
		}
		// No more units than the row still offers: the owner priced only
		// those.
		asked[row.ObjectID] += row.Count
		if asked[row.ObjectID] > listed.Count {
			return Deal{}, nil
		}
		if math.MaxInt32/row.Count < row.Price {
			return Deal{}, nil
		}
		total += int64(row.Count) * int64(row.Price)
		if total > math.MaxInt32 {
			return Deal{}, nil
		}
		weight += int64(row.Count) * int64(tmpl.Weight)
		if tmpl.Stackable {
			slots++
		} else {
			slots += row.Count
		}
		moves = append(moves, invops.Move{ObjectID: row.ObjectID, Count: row.Count})
		moved = append(moved, Moved{ItemID: listed.TemplateID, Count: row.Count, Enchant: listed.Enchant, Stackable: tmpl.Stackable})
	}
	if operate == OperatePackageSell {
		for _, listed := range s.sell {
			if asked[listed.ObjectID] != listed.Count {
				return Deal{}, nil
			}
		}
	}

	pay, adenaID := adenaMove(buyer.Inv, total)
	deal := Deal{Status: DealRefused}
	res, ok, err := svc.Exchange(owner.Inv, buyer.Inv, moves, pay, func(held, paying itemcontainer.Held) bool {
		for _, row := range rows {
			inst := held.ItemByObjectID(row.ObjectID)
			if !manipulable(held, owner, inst, asked[row.ObjectID]) {
				return false
			}
		}
		if adenaHeld(paying, adenaID) < total {
			deal.Status = DealNotEnoughAdena
			return false
		}
		if weight > math.MaxInt32 || !paying.ValidateWeight(int(weight)) {
			deal.Status = DealWeightExceeded
			return false
		}
		if !paying.ValidateCapacity(slots) {
			deal.Status = DealSlotsFull
			return false
		}
		return true
	})
	// A move failing after the check approved still reports the moves it
	// made, which have to be written.
	deal.Persist = res.Persist
	if err != nil || !ok {
		return deal, err
	}
	for _, row := range rows {
		s.takeSoldLocked(row.ObjectID, row.Count)
	}
	deal.Status, deal.Rows = DealDone, moved
	deal.Emptied = len(s.sell) == 0 && s.operate.CompareAndSwap(int32(operate), int32(OperateNone))
	return deal, nil
}

// Sell sells rows from seller's inventory into the buy list; owner pays in
// adena.
//
// The store must be buying. Every row must name a wanted item at its wanted
// enchant and price, for no more units than are still wanted, and the seller must hold that item at
// that enchant free to trade. The owner must afford the total. Both
// inventories are checked as the moves find them, so the deal moves
// everything or nothing. A deal that would pay more than the largest int32
// is refused.
func (s *Store) Sell(svc *invops.Service, owner, seller Trader, rows []SaleRow) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner.Inv == nil || seller.Inv == nil || owner.Inv == seller.Inv || len(rows) == 0 {
		return Deal{}, nil
	}
	// The store must still be buying as the deal starts; see Buy.
	if s.OperateType() != OperateBuy {
		return Deal{}, nil
	}
	templates := owner.Inv.Templates()
	var total int64
	moves := make([]invops.Move, 0, len(rows))
	moved := make([]Moved, 0, len(rows))
	wanted := make([]int, len(rows))
	asked := make(map[int]int, len(rows))
	perItem := make(map[int32]int, len(rows))
	for i, row := range rows {
		if row.Count < 1 {
			return Deal{}, nil
		}
		tmpl, ok := templates.Get(row.ItemID)
		if !ok {
			return Deal{}, nil
		}
		// The row must match what the owner listed, enchant included, and
		// may not fill more than the owner still wants.
		idx := s.buyRowLocked(row.ItemID, row.Enchant, row.Price)
		if idx < 0 {
			return Deal{}, nil
		}
		asked[idx] += row.Count
		if asked[idx] > s.buy[idx].Quantity {
			return Deal{}, nil
		}
		if math.MaxInt32/row.Count < row.Price {
			return Deal{}, nil
		}
		total += int64(row.Count) * int64(row.Price)
		if total > math.MaxInt32 {
			return Deal{}, nil
		}
		wanted[i] = idx
		perItem[row.ObjectID] += row.Count
		moves = append(moves, invops.Move{ObjectID: row.ObjectID, Count: row.Count})
		moved = append(moved, Moved{ItemID: row.ItemID, Count: row.Count, Enchant: row.Enchant, Stackable: tmpl.Stackable})
	}

	pay, adenaID := adenaMove(owner.Inv, total)
	deal := Deal{Status: DealRefused}
	res, ok, err := svc.Exchange(owner.Inv, seller.Inv, pay, moves, func(paying, held itemcontainer.Held) bool {
		for _, row := range rows {
			inst := held.ItemByObjectID(row.ObjectID)
			if !manipulable(held, seller, inst, perItem[row.ObjectID]) {
				return false
			}
			st := inst.Snapshot()
			if st.TemplateID != row.ItemID || st.EnchantLevel != row.Enchant {
				return false
			}
		}
		if adenaHeld(paying, adenaID) < total {
			deal.Status = DealNotEnoughAdena
			return false
		}
		return true
	})
	deal.Persist = res.Persist
	if err != nil || !ok {
		return deal, err
	}
	for i, row := range rows {
		s.buy[wanted[i]].Quantity -= row.Count
	}
	kept := s.buy[:0]
	for _, row := range s.buy {
		if row.Quantity > 0 {
			kept = append(kept, row)
		}
	}
	s.buy = kept
	deal.Status, deal.Rows = DealDone, moved
	deal.Emptied = len(s.buy) == 0 && s.operate.CompareAndSwap(int32(OperateBuy), int32(OperateNone))
	return deal, nil
}

// sellRowLocked finds the sell row for objectID at price.
func (s *Store) sellRowLocked(objectID int32, price int) (SellItem, bool) {
	for _, row := range s.sell {
		if row.ObjectID == objectID && row.Price == price {
			return row, true
		}
	}
	return SellItem{}, false
}

// buyRowLocked returns the index of the buy row for templateID at enchant
// and price, or -1.
func (s *Store) buyRowLocked(templateID int32, enchant, price int) int {
	for i, row := range s.buy {
		if row.TemplateID == templateID && row.Enchant == enchant && row.Price == price {
			return i
		}
	}
	return -1
}

// takeSoldLocked takes count sold units off objectID's sell row, dropping
// the row once nothing of its listed quantity is left.
func (s *Store) takeSoldLocked(objectID int32, count int) {
	for i := range s.sell {
		if s.sell[i].ObjectID != objectID {
			continue
		}
		s.sell[i].Count -= count
		s.sell[i].Quantity -= count
		if s.sell[i].Quantity <= 0 {
			s.sell = append(s.sell[:i], s.sell[i+1:]...)
		}
		return
	}
}

// adenaMove is the single move paying total adena out of inv, or none when
// nothing is owed. It also returns the adena stack's object id, 0 when inv
// holds none.
func adenaMove(inv *itemcontainer.Inventory, total int64) ([]invops.Move, int32) {
	inst := inv.ItemByTemplateID(item.AdenaID)
	if inst == nil {
		return nil, 0
	}
	if total <= 0 {
		return nil, inst.ObjectID
	}
	return []invops.Move{{ObjectID: inst.ObjectID, Count: int(total)}}, inst.ObjectID
}

// adenaHeld is how much adena held still holds in the stack adenaID names.
func adenaHeld(held itemcontainer.Held, adenaID int32) int64 {
	if adenaID == 0 {
		return 0
	}
	inst := held.ItemByObjectID(adenaID)
	if inst == nil {
		return 0
	}
	st := inst.Snapshot()
	if st.TemplateID != item.AdenaID {
		return 0
	}
	return int64(st.Count)
}

// manipulable reports whether count units of inst may leave trader's held
// inventory: inst is still held, not worn, not bound and not augmented while
// trader casts, the count is within what it holds and at most one for an
// item that does not stack, and the item is tradable.
func manipulable(held itemcontainer.Held, trader Trader, inst *item.Instance, count int) bool {
	if inst == nil {
		return false
	}
	st := inst.Snapshot()
	tmpl, ok := held.Templates().Get(st.TemplateID)
	if !ok || st.Equipped() {
		return false
	}
	if count < 1 || count > st.Count || (count > 1 && !tmpl.Stackable) {
		return false
	}
	if trader.Bound != nil && trader.Bound(st.ObjectID) {
		return false
	}
	if trader.Casting && st.Augmentation != nil {
		return false
	}
	return inst.Tradable(tmpl)
}
