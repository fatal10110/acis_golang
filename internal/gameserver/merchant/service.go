package merchant

import (
	"errors"
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Config holds the server.properties shop settings.
type Config struct {
	// SiegeGuardsPriceRate scales the price of the siege guard tickets
	// (RateSiegeGuardsPrice).
	SiegeGuardsPriceRate float64
	// AllowWear lets a merchant open its try-on window (AllowWear).
	AllowWear bool
	// WearDelay is how long tried-on items stay shown (WearDelay).
	WearDelay time.Duration
	// WearPrice is the adena one tried-on item costs (WearPrice).
	WearPrice int
}

// DefaultConfig is the shipped server.properties shop settings.
func DefaultConfig() Config {
	return Config{SiegeGuardsPriceRate: 1, AllowWear: true, WearDelay: 5 * time.Second, WearPrice: 10}
}

// IDs allocates the object id of each item a purchase creates.
type IDs interface {
	NextID() (int32, error)
}

// Service answers the merchant windows from the buylists and their stock.
// A nil *Service has no buylists.
type Service struct {
	lists *buylist.Table
	stock *Stock
	ids   IDs
	cfg   Config
}

// NewService builds a Service over lists and their stock.
func NewService(lists *buylist.Table, stock *Stock, ids IDs, cfg Config) *Service {
	return &Service{lists: lists, stock: stock, ids: ids, cfg: cfg}
}

// Config returns the shop settings.
func (s *Service) Config() Config {
	if s == nil {
		return DefaultConfig()
	}
	return s.cfg
}

// List returns buylist id.
func (s *Service) List(id int) (buylist.List, bool) {
	if s == nil || s.lists == nil {
		return buylist.List{}, false
	}
	return s.lists.Find(id)
}

// Count returns p's current stock count, 0 for an unlimited product.
func (s *Service) Count(p buylist.Product) int {
	if s == nil || s.stock == nil {
		return 0
	}
	return s.stock.Count(p)
}

// BuyRow is one item and count of a purchase.
type BuyRow struct {
	ItemID int32
	Count  int32
}

// BuyOutcome is how a purchase ended.
type BuyOutcome int

const (
	// BuyDropped refuses the purchase with no answer: an item not on the
	// list, an unpriced item for a player who is not a GM, a count over the
	// remaining stock, or a price overflowing the adena range.
	BuyDropped BuyOutcome = iota
	// BuyQuantityExceeded asks for more than one of a non-stackable item.
	BuyQuantityExceeded
	// BuyWeightExceeded would carry the buyer over its weight limit.
	BuyWeightExceeded
	// BuySlotsFull would take more inventory slots than the buyer has free.
	BuySlotsFull
	// BuyNotEnoughAdena costs more adena than the buyer holds.
	BuyNotEnoughAdena
	// BuyDone paid for the items and put them in the inventory.
	BuyDone
)

// Purchase is the result of Buy.
type Purchase struct {
	Outcome BuyOutcome
	// Subtotal is the adena paid, tax included.
	Subtotal int64
}

// Buy sells rows of list to the owner of inv at taxRate on top of the list
// prices; gm lets a GM take unpriced items. The rows are all checked
// before anything changes: each item must be on the list, a non-stackable
// one bought singly, a limited one within its remaining count, and the
// total within the adena range, the buyer's weight limit, free slots and
// adena. Then the adena is taken and each item created; a limited item
// whose count ran out in the meantime is not created, its price already
// paid.
func (s *Service) Buy(inv *itemcontainer.Inventory, list buylist.List, rows []BuyRow, taxRate float64, gm bool) (Purchase, error) {
	var subtotal, slots, weight int64
	for _, row := range rows {
		p, ok := list.FindProduct(row.ItemID)
		if !ok {
			return Purchase{}, nil
		}
		tmpl, ok := inv.Templates().Get(row.ItemID)
		if !ok {
			return Purchase{}, nil
		}
		if !tmpl.Stackable && row.Count > 1 {
			return Purchase{Outcome: BuyQuantityExceeded}, nil
		}
		price := int32(p.Price)
		if p.SiegeGuardTicket() {
			price = commons.JavaInt(float64(price) * s.cfg.SiegeGuardsPriceRate)
		}
		if price < 0 || (price == 0 && !gm) {
			return Purchase{}, nil
		}
		if p.LimitedStock() && int(row.Count) > s.Count(p) {
			return Purchase{}, nil
		}
		if math.MaxInt32/row.Count < price {
			return Purchase{}, nil
		}
		// The tax applies to the unit price, before the count; the 32-bit
		// products wrap as int32 arithmetic does.
		price = commons.JavaInt(float64(price) * (1 + taxRate))
		subtotal += int64(row.Count * price)
		if subtotal > math.MaxInt32 {
			return Purchase{}, nil
		}
		weight += int64(row.Count * tmpl.Weight)
		switch {
		case !tmpl.Stackable:
			slots += int64(row.Count)
		case inv.ItemByTemplateID(row.ItemID) == nil:
			slots++
		}
	}
	if weight > math.MaxInt32 || weight < 0 || !inv.ValidateWeight(int(weight)) {
		return Purchase{Outcome: BuyWeightExceeded}, nil
	}
	if slots > math.MaxInt32 || slots < 0 || !inv.ValidateCapacity(int(slots)) {
		return Purchase{Outcome: BuySlotsFull}, nil
	}
	if subtotal < 0 || !reduceAdena(inv, subtotal) {
		return Purchase{Outcome: BuyNotEnoughAdena}, nil
	}

	var errs []error
	for _, row := range rows {
		p, _ := list.FindProduct(row.ItemID)
		if p.LimitedStock() && (s.stock == nil || !s.stock.Decrease(p, int(row.Count))) {
			continue
		}
		id, err := s.ids.NextID()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		inv.AddNew(row.ItemID, int(row.Count), id)
	}
	return Purchase{Outcome: BuyDone, Subtotal: subtotal}, errors.Join(errs...)
}

// TryOnOutcome is how a try-on ended.
type TryOnOutcome int

const (
	// TryOnDropped refuses the try-on with no answer: an item not on the
	// list, or a total price overflowing the adena range.
	TryOnDropped TryOnOutcome = iota
	// TryOnSameSlot names two items for one paperdoll slot.
	TryOnSameSlot
	// TryOnNegativePrice totals a negative price, as a negative WearPrice
	// does: refused as not enough adena without checking the adena.
	TryOnNegativePrice
	// TryOnNotEnoughAdena costs more adena than the player holds.
	TryOnNotEnoughAdena
	// TryOnDone paid for the try-on.
	TryOnDone
)

// TryOn is the result of TryOn.
type TryOn struct {
	Outcome TryOnOutcome
	// Paid is the adena taken.
	Paid int
	// Items holds the item id tried on at each paperdoll position, 0 where
	// none is.
	Items [item.PaperdollSlots]int32
	// Shown reports whether any item went on.
	Shown bool
}

// TryOn charges the owner of inv WearPrice for each item of itemIDs that
// goes on a paperdoll slot, to show those items worn for a while. Every
// item must be on list; one no slot takes is skipped and costs nothing,
// and two taking one slot refuse the try-on.
func (s *Service) TryOn(inv *itemcontainer.Inventory, list buylist.List, itemIDs []int32) TryOn {
	var res TryOn
	var taken [item.PaperdollSlots]bool
	var total int64
	for _, id := range itemIDs {
		if _, ok := list.FindProduct(id); !ok {
			return TryOn{}
		}
		tmpl, ok := inv.Templates().Get(id)
		if !ok {
			continue
		}
		slot, ok := tmpl.Slot.PaperdollIndex()
		if !ok {
			continue
		}
		if taken[slot] {
			return TryOn{Outcome: TryOnSameSlot}
		}
		taken[slot] = true
		res.Items[slot] = id
		res.Shown = true
		total += int64(s.cfg.WearPrice)
		if total > math.MaxInt32 {
			return TryOn{}
		}
	}
	if total < 0 {
		return TryOn{Outcome: TryOnNegativePrice}
	}
	if !reduceAdena(inv, total) {
		return TryOn{Outcome: TryOnNotEnoughAdena}
	}
	res.Outcome, res.Paid = TryOnDone, int(total)
	return res
}

// reduceAdena takes count adena from inv, reporting false when inv holds
// less. Nothing is taken for a count of 0.
func reduceAdena(inv *itemcontainer.Inventory, count int64) bool {
	if count > int64(inv.Adena()) {
		return false
	}
	if count > 0 {
		return inv.DestroyByTemplateID(item.AdenaID, int(count)) != nil
	}
	return true
}
