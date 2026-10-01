// Package privatestore runs a player's private stores: the sell and package
// sell lists other players buy from, the buy list they sell into, and the
// manufacture (recipe shop) list they order crafts from. It owns the lists
// and the store's operate state and decides every list and deal rule; the
// caller turns the outcomes into client packets and queues the item writes.
package privatestore

import (
	"sync"
	"sync/atomic"
)

// OperateType is what a player's store is doing. Its value is the operate
// byte UserInfo and CharInfo carry.
type OperateType int32

// Operate types.
const (
	OperateNone              OperateType = 0
	OperateSell              OperateType = 1
	OperateSellManage        OperateType = 2
	OperateBuy               OperateType = 3
	OperateBuyManage         OperateType = 4
	OperateManufacture       OperateType = 5
	OperateManufactureManage OperateType = 6
	OperatePackageSell       OperateType = 8
)

// Operating reports any operate state but none: a store open, or its
// window being set up.
func (t OperateType) Operating() bool { return t != OperateNone }

// InStoreMode reports a store open for business: sell, package sell, buy or
// manufacture.
func (t OperateType) InStoreMode() bool {
	switch t {
	case OperateSell, OperatePackageSell, OperateBuy, OperateManufacture:
		return true
	}
	return false
}

// InManageMode reports a store whose window is being set up.
func (t OperateType) InManageMode() bool {
	switch t {
	case OperateSellManage, OperateBuyManage, OperateManufactureManage:
		return true
	}
	return false
}

// MaxMessageLength is the longest store title or workshop name, in UTF-16
// code units, a player may set.
const MaxMessageLength = 29

// MaxManufactureItems is how many recipes a workshop may offer.
const MaxManufactureItems = 20

// SellItem is one row of a sell list: an item of the owner's inventory and
// what each unit costs.
type SellItem struct {
	ObjectID   int32
	TemplateID int32
	Enchant    int
	// Count is how many units are still offered.
	Count int
	// Quantity is the count the row was listed with, less the units sold.
	// The row leaves the list once it reaches zero.
	Quantity int
	Price    int
}

// BuyItem is one row of a buy list: an item the owner wants, at what
// enchant, how many and what each unit pays.
type BuyItem struct {
	TemplateID int32
	Enchant    int
	// Quantity is how many units are still wanted.
	Quantity int
	Price    int
}

// ManufactureItem is one recipe a workshop crafts and what a craft costs.
type ManufactureItem struct {
	RecipeID int
	Cost     int
	Dwarven  bool
}

// Store is one player's private-store state. The zero value is an empty
// store with nothing open.
//
// operate is read lock-free by every gate that asks whether the player is
// operating; mu guards the lists and titles and is held across a whole deal
// or workshop craft, so a list cannot change between its check and the
// trade it allows. No caller holds an inventory lock while taking mu.
type Store struct {
	operate atomic.Int32

	mu          sync.Mutex
	sellTitle   string
	buyTitle    string
	shopName    string
	packaged    bool
	sell        []SellItem
	buy         []BuyItem
	manufacture []ManufactureItem
	dwarvenShop bool
}

// OperateType returns the store's operate state.
func (s *Store) OperateType() OperateType { return OperateType(s.operate.Load()) }

// SetOperateType sets the store's operate state and reports whether it
// changed.
func (s *Store) SetOperateType(t OperateType) bool {
	return OperateType(s.operate.Swap(int32(t))) != t
}

// SellTitle returns the sell store's title.
func (s *Store) SellTitle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sellTitle
}

// SetSellTitle sets the sell store's title.
func (s *Store) SetSellTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sellTitle = title
}

// BuyTitle returns the buy store's title.
func (s *Store) BuyTitle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buyTitle
}

// SetBuyTitle sets the buy store's title.
func (s *Store) SetBuyTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buyTitle = title
}

// ShopName returns the workshop's name.
func (s *Store) ShopName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shopName
}

// SetShopName sets the workshop's name.
func (s *Store) SetShopName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shopName = name
}

// SellList returns a copy of the sell list and whether it sells as one
// package.
func (s *Store) SellList() ([]SellItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SellItem(nil), s.sell...), s.packaged
}

// BuyList returns a copy of the buy list.
func (s *Store) BuyList() []BuyItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]BuyItem(nil), s.buy...)
}

// ManufactureList returns a copy of the workshop's recipes and whether the
// workshop window was last set up as dwarven.
func (s *Store) ManufactureList() ([]ManufactureItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ManufactureItem(nil), s.manufacture...), s.dwarvenShop
}

// ShowManufacture records which page, dwarven or common, the workshop
// window is set up from, and returns the workshop's recipes on that page
// that keep reports still held.
func (s *Store) ShowManufacture(dwarven bool, keep func(recipeID int) bool) []ManufactureItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dwarvenShop = dwarven
	var out []ManufactureItem
	for _, m := range s.manufacture {
		if m.Dwarven == dwarven && keep(m.RecipeID) {
			out = append(out, m)
		}
	}
	return out
}

// SetManufacture replaces the workshop's recipes.
func (s *Store) SetManufacture(items []ManufactureItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manufacture = append(s.manufacture[:0:0], items...)
}

func (s *Store) manufactureCostLocked(recipeID int) (int, bool) {
	for _, m := range s.manufacture {
		if m.RecipeID == recipeID {
			return m.Cost, true
		}
	}
	return 0, false
}

// Craft runs fn with the workshop held: no other order and no list change
// can land while it runs. cost is what the workshop lists the recipe at.
// A recipe the workshop does not list is not crafted: fn does not run and
// Craft reports false. The reference crafts it for nothing; only a crafted
// order can name one, since the workshop window shows listed recipes only.
func (s *Store) Craft(recipeID int, fn func(cost int)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cost, ok := s.manufactureCostLocked(recipeID)
	if !ok {
		return false
	}
	fn(cost)
	return true
}
