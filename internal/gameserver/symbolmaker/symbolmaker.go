// Package symbolmaker runs the dye symbol rules: which symbols a player may
// draw, drawing one for adena and dyes, and deleting one for adena with part
// of its dyes handed back. It decides and mutates; the caller turns the
// returned notices into client packets and queues the symbol row writes.
package symbolmaker

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Service applies the symbol rules against the loaded henna table.
type Service struct {
	hennas *henna.Table
	nextID func() (int32, error)
}

// NewService returns a Service over hennas. nextID allocates the object id
// of a dye stack a deletion hands back when none is held.
func NewService(hennas *henna.Table, nextID func() (int32, error)) *Service {
	return &Service{hennas: hennas, nextID: nextID}
}

// Henna returns the loaded symbol with symbolID.
func (s *Service) Henna(symbolID int) (henna.Henna, bool) {
	return s.hennas.Find(symbolID)
}

// Drawable returns, in ascending symbol id order, the symbols c's class may
// use and whose dye c carries at least one item of.
func (s *Service) Drawable(c *player.Character) []henna.Henna {
	inv := c.Inventory()
	if inv == nil {
		return nil
	}
	var out []henna.Henna
	for _, h := range s.hennas.All() {
		if h.UsableByClass(c.ClassID()) && inv.ItemByTemplateID(h.DyeID) != nil {
			out = append(out, h)
		}
	}
	return out
}

// Notices a draw or a deletion reports, in the order they happen.
type (
	// CantDraw refuses a symbol the class may not use, or one whose dye
	// the player holds too little of.
	CantDraw struct{}
	// SymbolsFull refuses a draw while every slot the class has is taken.
	SymbolsFull struct{}
	// NotEnoughAdena refuses a draw or deletion the player cannot pay for.
	NotEnoughAdena struct{}
	// AdenaSpent names the adena a draw took.
	AdenaSpent struct{ Count int }
	// NotEnoughItems reports dyes that could not be taken once the adena
	// was paid.
	NotEnoughItems struct{}
	// DyeConsumed names the dyes a draw took.
	DyeConsumed struct {
		ItemID int32
		Count  int
	}
)

// Drawing is the outcome of one draw request.
type Drawing struct {
	// Notices are the messages to send, in order.
	Notices []any
	// Added reports that the symbol now sits in DBSlot (1-based).
	Added  bool
	Symbol henna.Henna
	DBSlot int
}

// Draw draws symbolID on c: the class must be allowed the symbol, a slot
// must be free and c must carry DrawAmount of its dye. The adena is paid,
// then the dyes are taken, then the symbol takes the first free slot. An
// unknown symbol is dropped without a word. Adena already paid is not given
// back when the dyes can no longer be taken.
func (s *Service) Draw(c *player.Character, symbolID int) Drawing {
	h, ok := s.Henna(symbolID)
	if !ok {
		return Drawing{}
	}
	if !h.UsableByClass(c.ClassID()) {
		return Drawing{Notices: []any{CantDraw{}}}
	}
	if c.HennaEmptySlots() <= 0 {
		return Drawing{Notices: []any{SymbolsFull{}}}
	}
	inv := c.Inventory()
	held := 0
	if inv != nil {
		if inst := inv.ItemByTemplateID(h.DyeID); inst != nil {
			held = inst.CountValue()
		}
	}
	if held < henna.DrawAmount {
		return Drawing{Notices: []any{CantDraw{}}}
	}
	if !payAdena(c, h.DrawPrice) {
		return Drawing{Notices: []any{NotEnoughAdena{}}}
	}
	d := Drawing{Symbol: h}
	if h.DrawPrice > 0 {
		d.Notices = append(d.Notices, AdenaSpent{Count: h.DrawPrice})
	}
	if inv.DestroyByTemplateID(h.DyeID, henna.DrawAmount) == nil {
		d.Notices = append(d.Notices, NotEnoughItems{})
		return d
	}
	d.Notices = append(d.Notices, DyeConsumed{ItemID: h.DyeID, Count: henna.DrawAmount})
	d.DBSlot, d.Added = c.AddHenna(h)
	return d
}

// Deletion is the outcome of one deletion request.
type Deletion struct {
	// NotEnoughAdena refuses the deletion.
	NotEnoughAdena bool
	// Removed reports that Symbol left DBSlot (1-based) and that
	// DyesReturned of its dye were handed back.
	Removed      bool
	Symbol       henna.Henna
	DBSlot       int
	DyesReturned int
}

// Delete removes the symbol symbolID c wears: the removal price is paid,
// the symbol leaves its slot and RemoveAmount of its dye are handed back.
// A symbol c does not wear is dropped without a word.
func (s *Service) Delete(c *player.Character, symbolID int) Deletion {
	h, ok := c.HennaList().BySymbolID(symbolID)
	if !ok {
		return Deletion{}
	}
	price := h.RemovePrice()
	// The price is taken before the symbol leaves, in one step with its
	// check, so adena spent elsewhere in between cannot buy a free removal.
	if !payAdena(c, price) {
		return Deletion{NotEnoughAdena: true}
	}
	slot, removed := c.RemoveHenna(symbolID)
	if !removed {
		if price > 0 {
			c.AddCraftedItem(item.AdenaID, price, s.nextID)
		}
		return Deletion{}
	}
	return Deletion{
		Removed:      true,
		Symbol:       h,
		DBSlot:       slot,
		DyesReturned: c.AddCraftedItem(h.DyeID, henna.RemoveAmount, s.nextID),
	}
}

// payAdena takes price adena from c, reporting false when c holds less. A
// zero price takes nothing.
func payAdena(c *player.Character, price int) bool {
	if price <= 0 {
		return true
	}
	inv := c.Inventory()
	if inv == nil || inv.Adena() < price {
		return false
	}
	return inv.DestroyByTemplateID(item.AdenaID, price) != nil
}
