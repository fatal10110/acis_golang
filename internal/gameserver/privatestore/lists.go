package privatestore

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Holdings is what the list rules read from the owner's inventory.
type Holdings interface {
	ItemByObjectID(objectID int32) *item.Instance
	ItemByTemplateID(templateID int32) *item.Instance
	Templates() *item.Table
}

// SellRow is one row a player asks to put on a sell list.
type SellRow struct {
	ObjectID int32
	Count    int
	Price    int
}

// BuyRow is one row a player asks to put on a buy list.
type BuyRow struct {
	TemplateID int32
	Enchant    int
	Count      int
	Price      int
}

// FillStatus is how filling a store list ended.
type FillStatus uint8

const (
	// FillOK means every row went on the list.
	FillOK FillStatus = iota
	// FillExceeded means a row could not go on the list, or the list's
	// total ran past the largest int32; the rows before it stay listed.
	FillExceeded
	// FillPriceAboveAdena means a buy list would cost more than the owner
	// holds.
	FillPriceAboveAdena
)

// ClearSell empties the sell list and its package flag. Its title stays.
func (s *Store) ClearSell() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sell = nil
	s.packaged = false
}

// ClearBuy empties the buy list. Its title stays.
func (s *Store) ClearBuy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buy = nil
}

// CanPassSell reports whether every row names an item inv holds at least
// that many units of, with a count of at least 1 and no negative price.
func CanPassSell(inv Holdings, rows []SellRow) bool {
	for _, row := range rows {
		if row.Count < 1 || row.Price < 0 {
			return false
		}
		inst := inv.ItemByObjectID(row.ObjectID)
		if inst == nil || inst.CountValue() < row.Count {
			return false
		}
	}
	return true
}

// CanPassBuy reports whether every row has a count of at least 1, no
// negative price, and names an item inv already holds, its first instance
// at the row's enchant.
func CanPassBuy(inv Holdings, rows []BuyRow) bool {
	for _, row := range rows {
		if row.Count < 1 || row.Price < 0 {
			return false
		}
		inst := inv.ItemByTemplateID(row.TemplateID)
		if inst == nil || inst.Snapshot().EnchantLevel != row.Enchant {
			return false
		}
	}
	return true
}

// FillSell lists rows on the emptied sell list, packaged or not, for an
// owner holding adena. A row naming an item that is not tradable, is a
// quest item, is worn, or is short of the units asked fails the fill, as
// does a row or a running total (the owner's adena plus every row's price)
// past the largest int32; the rows listed before the failing one stay.
// A row naming an item already listed adds its count to that row.
func (s *Store) FillSell(inv Holdings, rows []SellRow, packaged bool, adena int) FillStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packaged = packaged
	total := int64(adena)
	for _, row := range rows {
		if !s.addSellLocked(inv, row) {
			return FillExceeded
		}
		total += int64(row.Count) * int64(row.Price)
		if total > math.MaxInt32 {
			return FillExceeded
		}
	}
	return FillOK
}

func (s *Store) addSellLocked(inv Holdings, row SellRow) bool {
	inst := inv.ItemByObjectID(row.ObjectID)
	if inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Tradable(tmpl) || inst.QuestItem(tmpl) {
		return false
	}
	st := inst.Snapshot()
	// A worn item never reaches the owner's manage window, and a store
	// cannot hand one over: it would leave the paperdoll holding it.
	if st.Equipped() {
		return false
	}
	if row.Count <= 0 || row.Count > st.Count || (!tmpl.Stackable && row.Count > 1) {
		return false
	}
	if math.MaxInt32/row.Count < row.Price {
		return false
	}
	for i := range s.sell {
		if s.sell[i].ObjectID != row.ObjectID {
			continue
		}
		count := s.sell[i].Count + row.Count
		if st.Count < count {
			return false
		}
		s.sell[i].Count = count
		return true
	}
	s.sell = append(s.sell, SellItem{
		ObjectID:   st.ObjectID,
		TemplateID: st.TemplateID,
		Enchant:    st.EnchantLevel,
		Count:      row.Count,
		Quantity:   row.Count,
		Price:      row.Price,
	})
	return true
}

// FillBuy lists rows on the emptied buy list for an owner holding adena. A
// row naming an unknown, untradable or quest item, more than one unit of
// an item that does not stack, or a price past the largest int32 fails the
// fill, as does a running total past it; the rows listed before stay. A
// list costing more than adena reports FillPriceAboveAdena with every row
// listed.
func (s *Store) FillBuy(templates *item.Table, rows []BuyRow, adena int) FillStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, row := range rows {
		tmpl, ok := templates.Get(row.TemplateID)
		if !ok || !tmpl.Tradable || (tmpl.EtcItem != nil && tmpl.EtcItem.IsQuestItem()) {
			return FillExceeded
		}
		if (!tmpl.Stackable && row.Count > 1) || math.MaxInt32/row.Count < row.Price {
			return FillExceeded
		}
		s.buy = append(s.buy, BuyItem{TemplateID: row.TemplateID, Enchant: row.Enchant, Quantity: row.Count, Price: row.Price})
		total += int64(row.Count) * int64(row.Price)
		if total > math.MaxInt32 {
			return FillExceeded
		}
	}
	if total > int64(adena) {
		return FillPriceAboveAdena
	}
	return FillOK
}

// RefreshSell drops every sell row whose item inv no longer holds, holds
// worn, or that has no units left, and caps the others at what inv holds.
// It returns the refreshed list and its package flag.
func (s *Store) RefreshSell(inv Holdings) ([]SellItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.sell[:0]
	for _, row := range s.sell {
		inst := inv.ItemByObjectID(row.ObjectID)
		if inst == nil || row.Count < 1 {
			continue
		}
		st := inst.Snapshot()
		if st.Equipped() {
			continue
		}
		row.Count = min(row.Count, st.Count)
		kept = append(kept, row)
	}
	s.sell = kept
	return append([]SellItem(nil), s.sell...), s.packaged
}

// RefreshBuy drops every buy row whose item inv no longer holds any of and
// returns the refreshed list.
func (s *Store) RefreshBuy(inv Holdings) []BuyItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.buy[:0]
	for _, row := range s.buy {
		if inv.ItemByTemplateID(row.TemplateID) != nil {
			kept = append(kept, row)
		}
	}
	s.buy = kept
	return append([]BuyItem(nil), s.buy...)
}
