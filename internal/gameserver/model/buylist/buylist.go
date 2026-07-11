// Package buylist models static NPC buylist data loaded at boot.
package buylist

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Product is one item offered by a buylist.
type Product struct {
	BuyListID          int
	ItemID             int32
	Price              int
	RestockDelayMillis int64
	MaxCount           int
}

// NewProduct builds a Product from one folded <product> element.
func NewProduct(buyListID int, set *commons.StatSet) (Product, error) {
	idf := commons.NewFields(set, fmt.Sprintf("buylist %d product", buyListID))
	itemID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return Product{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("buylist %d product %d", buyListID, itemID))
	price := f.IntDefault("price", 0)
	restockDelay := f.Int64Default("restockDelay", -1)
	maxCount := f.IntDefault("count", -1)
	if err := f.Err(); err != nil {
		return Product{}, err
	}
	return Product{
		BuyListID: buyListID, ItemID: itemID, Price: price,
		RestockDelayMillis: restockDelay * 60000, MaxCount: maxCount,
	}, nil
}

// LimitedStock reports whether this product uses a restock counter.
func (p Product) LimitedStock() bool {
	return p.MaxCount > -1
}

// List is one NPC buylist and its products.
type List struct {
	ID       int
	NPCID    int
	Products []Product
}

// NewList builds a List from one folded <buyList> element and its products.
func NewList(set *commons.StatSet, products []Product) (List, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return List{}, fmt.Errorf("buylist: %w", err)
	}
	npcID, err := set.GetInt("npcId")
	if err != nil {
		return List{}, fmt.Errorf("buylist %d: %w", id, err)
	}
	return List{ID: id, NPCID: npcID, Products: products}, nil
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
