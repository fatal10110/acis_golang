// Package buylist models static NPC buylist data loaded at boot.
package buylist

// Product is one item offered by a buylist.
type Product struct {
	BuyListID          int
	ItemID             int32
	Price              int
	RestockDelayMillis int64
	MaxCount           int
}

// NewProduct builds a Product of buylist buyListID. restockDelayMinutes is
// converted to milliseconds; a maxCount of -1 means unlimited stock.
func NewProduct(buyListID int, itemID int32, price int, restockDelayMinutes int64, maxCount int) Product {
	return Product{
		BuyListID: buyListID, ItemID: itemID, Price: price,
		RestockDelayMillis: restockDelayMinutes * 60000, MaxCount: maxCount,
	}
}

// LimitedStock reports whether this product uses a restock counter.
func (p Product) LimitedStock() bool {
	return p.MaxCount > -1
}

// SiegeGuardTicket reports whether the product is one of the item ids,
// 3960 to 4026, whose price scales by the siege guards price rate.
func (p Product) SiegeGuardTicket() bool {
	return p.ItemID >= 3960 && p.ItemID <= 4026
}

// List is one NPC buylist and its products.
type List struct {
	ID       int
	NPCID    int
	Products []Product
}

// NewList builds a List from its id, npc id and products. A product whose
// item id repeats an earlier one replaces it in place: the list keeps the
// first position and the last definition.
func NewList(id, npcID int, products []Product) List {
	return List{ID: id, NPCID: npcID, Products: dedupeProducts(products)}
}

// dedupeProducts keeps one product per item id, at the position its id
// first appears, holding its last definition.
func dedupeProducts(products []Product) []Product {
	at := make(map[int32]int, len(products))
	out := products[:0:0]
	for _, p := range products {
		if i, ok := at[p.ItemID]; ok {
			out[i] = p
			continue
		}
		at[p.ItemID] = len(out)
		out = append(out, p)
	}
	return out
}

// AllowsNPC reports whether npcID can use this list.
func (l List) AllowsNPC(npcID int) bool {
	return l.NPCID == npcID
}

// FindProduct returns the product with itemID.
func (l List) FindProduct(itemID int32) (Product, bool) {
	for _, p := range l.Products {
		if p.ItemID == itemID {
			return p, true
		}
	}
	return Product{}, false
}

// Table stores buylists keyed by list id.
type Table struct {
	byID map[int]List
}

// NewTable builds a buylist lookup table.
func NewTable(lists []List) *Table {
	t := &Table{byID: make(map[int]List, len(lists))}
	for _, l := range lists {
		t.byID[l.ID] = l
	}
	return t
}

// Len returns the number of buylists keyed by list id.
func (t *Table) Len() int {
	return len(t.byID)
}

// ProductCount returns the total number of products across all lists.
func (t *Table) ProductCount() int {
	n := 0
	for _, l := range t.byID {
		n += len(l.Products)
	}
	return n
}

// Find returns the buylist with id.
func (t *Table) Find(id int) (List, bool) {
	l, ok := t.byID[id]
	return l, ok
}

// Each calls fn for every buylist, in no particular order.
func (t *Table) Each(fn func(List)) {
	for _, l := range t.byID {
		fn(l)
	}
}
