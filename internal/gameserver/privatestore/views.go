package privatestore

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// SellCandidate is one inventory item a sell store's manage window offers
// to list, with how many of its units are not listed yet.
type SellCandidate struct {
	Item  item.InstanceState
	Count int
}

// ItemsToSell lists, in inventory order, every item of inv a sell store may
// list and how many of its units sell does not list yet: tradable items
// that are not worn, not quest items, not adena and not hidden (a summoned
// pet's collar, the selected enchant scroll). A stackable item lists its
// count less the first sell row of the same item; any other item is left
// out once a row lists it.
func ItemsToSell(inv *itemcontainer.Inventory, sell []SellItem, hidden func(objectID int32) bool) []SellCandidate {
	if inv == nil {
		return nil
	}
	var out []SellCandidate
	for _, inst := range inv.Items() {
		st := inst.Snapshot()
		tmpl, ok := inv.Templates().Get(st.TemplateID)
		if !ok || st.Equipped() || st.TemplateID == item.AdenaID || inst.QuestItem(tmpl) || !inst.Tradable(tmpl) {
			continue
		}
		if hidden != nil && hidden(st.ObjectID) {
			continue
		}
		count := st.Count
		switch {
		case len(sell) == 0:
		case tmpl.Stackable:
			for _, row := range sell {
				if row.TemplateID == st.TemplateID {
					count = st.Count - row.Count
					break
				}
			}
		default:
			for _, row := range sell {
				if row.ObjectID == st.ObjectID {
					count = 0
					break
				}
			}
		}
		if count > 0 {
			out = append(out, SellCandidate{Item: st, Count: count})
		}
	}
	return out
}

// ItemsToBuy lists, in inventory order, the items of inv a buy store's
// manage window offers to want: one instance per item that does not stack,
// worn ones included, every one that is tradable or sellable and is not a
// quest item, not adena and not hidden.
func ItemsToBuy(inv *itemcontainer.Inventory, hidden func(objectID int32) bool) []item.InstanceState {
	if inv == nil {
		return nil
	}
	var out []item.InstanceState
	for _, inst := range inv.Items() {
		st := inst.Snapshot()
		tmpl, ok := inv.Templates().Get(st.TemplateID)
		if !ok || st.TemplateID == item.AdenaID {
			continue
		}
		if !tmpl.Stackable && listsTemplate(out, st.TemplateID) {
			continue
		}
		if inst.QuestItem(tmpl) || !(inst.Tradable(tmpl) || inst.Sellable(tmpl)) {
			continue
		}
		if hidden != nil && hidden(st.ObjectID) {
			continue
		}
		out = append(out, st)
	}
	return out
}

func listsTemplate(items []item.InstanceState, templateID int32) bool {
	for _, st := range items {
		if st.TemplateID == templateID {
			return true
		}
	}
	return false
}

// BuyOffer is one buy-list row as a seller sees it: the seller's own item
// that can fill it and how many of its units the row takes. A row the
// seller holds nothing for shows no item and a count of 0.
type BuyOffer struct {
	BuyItem
	ObjectID int32
	Count    int
}

// OffersFor shows buy to a seller holding inv: each row takes the seller's
// first unworn instance of the item at the row's enchant, up to the units
// the row still wants.
func OffersFor(buy []BuyItem, inv *itemcontainer.Inventory) []BuyOffer {
	out := make([]BuyOffer, 0, len(buy))
	for _, row := range buy {
		offer := BuyOffer{BuyItem: row}
		if inv != nil {
			for _, inst := range inv.ItemsByTemplateID(row.TemplateID) {
				st := inst.Snapshot()
				if st.EnchantLevel != row.Enchant || st.Equipped() {
					continue
				}
				offer.ObjectID = st.ObjectID
				offer.Count = min(st.Count, row.Quantity)
				break
			}
		}
		out = append(out, offer)
	}
	return out
}
