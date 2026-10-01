package inventory

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// SaleRow is one object/count row a player offers a merchant.
type SaleRow struct {
	ObjectID int32
	Count    int
}

// SaleResult is what a sale to a merchant did.
type SaleResult struct {
	// Result carries the paperdoll changes of a worn item the sale took.
	Result
	// Adena is what the merchant paid.
	Adena int
}

// SellableItems lists the items of inv a merchant's sell window offers, in
// inventory order: every sellable item not worn, except the control item of
// the pet summonedControl names.
func (s *Service) SellableItems(inv *itemcontainer.Inventory, summonedControl func(objectID int32) bool) []*item.Instance {
	if inv == nil {
		return nil
	}
	var out []*item.Instance
	for _, inst := range inv.Items() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || inst.Equipped() || !inst.Sellable(tmpl) {
			continue
		}
		if summonedControl != nil && summonedControl(inst.ObjectID) {
			continue
		}
		out = append(out, inst)
	}
	return out
}

// Sell sells rows from inv to a merchant, who pays half of each item's
// reference price per unit in adena. Rows are taken in order, each against
// what the rows before it left. A row is skipped when inv does not hold its
// item, its count is below 1, above the held count or above 1 for an item
// that does not stack, when bound reports the item tied where it is (a pet
// out, the selected enchant scroll), or when the item is not sellable. A
// worn item a row takes whole comes off first.
//
// When a row's payment, or the running total, would exceed the largest
// int32, the whole sale is refused: it reports false and nothing changes,
// the rows before the overflowing one included. Every row is checked before
// the first is taken, so a refused sale never takes items it does not pay
// for. Adena paid on top of a full stack caps it at the largest int32.
func (s *Service) Sell(inv *itemcontainer.Inventory, rows []SaleRow, bound func(objectID int32) bool) (SaleResult, bool, error) {
	if inv == nil {
		return SaleResult{}, false, nil
	}
	type sale struct {
		inst  *item.Instance
		count int
		price int64
	}
	var (
		sales []sale
		total int64
	)
	left := make(map[int32]int, len(rows))
	for _, row := range rows {
		inst := inv.ItemByObjectID(row.ObjectID)
		if inst == nil {
			continue
		}
		held, seen := left[row.ObjectID]
		if !seen {
			held = inst.CountValue()
		}
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || held < 1 || row.Count < 1 || row.Count > held || (row.Count > 1 && !tmpl.Stackable) {
			continue
		}
		if bound != nil && bound(row.ObjectID) {
			continue
		}
		if !inst.Sellable(tmpl) {
			continue
		}
		price := int64(tmpl.ReferencePrice / 2)
		pay := price * int64(row.Count)
		total += pay
		if pay > math.MaxInt32 || total > math.MaxInt32 {
			return SaleResult{}, false, nil
		}
		left[row.ObjectID] = held - row.Count
		sales = append(sales, sale{inst: inst, count: row.Count, price: pay})
	}

	adenaID := int32(0)
	if total > 0 {
		id, ok, err := s.nextID()
		if err != nil {
			return SaleResult{}, false, err
		}
		if !ok {
			return SaleResult{}, false, errNoIDAllocator
		}
		adenaID = id
	}

	var res SaleResult
	for _, sl := range sales {
		st := sl.inst.Snapshot()
		wasEquipped := st.Equipped() && st.Count <= sl.count
		changed := unequipConsumed(inv, sl.inst, wasEquipped)
		res.Changed = append(res.Changed, changed...)
		res.EquipmentChanged = res.EquipmentChanged || len(changed) > 0
		// A row something else took first since it was checked is not paid.
		if inv.DestroyItem(sl.inst, sl.count) == nil {
			continue
		}
		res.Adena += int(sl.price)
	}
	if res.Adena > 0 {
		inv.AddNew(item.AdenaID, res.Adena, adenaID)
	}
	return res, true, nil
}
