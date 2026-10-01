// Package exchange runs the multisell rules: preparing a list for the
// player an NPC shows it to, and trading one of its entries. It decides and
// mutates; the caller sends the prepared list's pages and turns the
// returned notices into client packets.
package exchange

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

const (
	// clanReputationID is the ingredient id standing for clan reputation
	// points.
	clanReputationID = 65336
	// maxAmount is the most units one exchange may ask for.
	maxAmount = 9999
	// newbieGuideID is the adventurers' guide whose inventory-only lists
	// also take back the trainee and traveler weapons.
	newbieGuideID = 31760
)

// Service applies the multisell rules against the loaded lists.
type Service struct {
	lists          *multisell.Table
	keepMaintained bool
	nextID         func() (int32, error)
}

// NewService returns a Service over lists. keepMaintained is the
// BlacksmithUseRecipes switch inverted: when set, an ingredient marked
// maintainIngredient is only checked for, once, and never taken. nextID
// allocates the products' object ids.
func NewService(lists *multisell.Table, keepMaintained bool, nextID func() (int32, error)) *Service {
	return &Service{lists: lists, keepMaintained: keepMaintained, nextID: nextID}
}

// Open prepares list name for c talking to the NPC npcID. It returns nil
// when no list has that name or the NPC may not open it. An inventory-only
// list holds, for each unworn armor or weapon c may part with, the entries
// taking it.
func (s *Service) Open(c *player.Character, name string, npcID int, inventoryOnly bool) *multisell.List {
	if s.lists == nil {
		return nil
	}
	tmpl, ok := s.lists.Get(commons.LegacyStringHash(name))
	if !ok || !tmpl.NPCAllowed(int32(npcID)) {
		return nil
	}
	if !inventoryOnly {
		return tmpl.Prepare()
	}
	return tmpl.PrepareFor(offered(c.Inventory(), tmpl.MaintainEnchantment, npcID == newbieGuideID))
}

// offered lists the items of inv an inventory-only list is matched
// against, in inventory order: each unworn armor or weapon that may change
// hands — tradable or sellable, augmented on a list that maintains
// enchantment, or a trainee or traveler weapon at the adventurers' guide.
// A non-stackable item is listed once per item id, or, when maintain is
// set, once per item id, enchant level and augmentation.
func offered(inv *itemcontainer.Inventory, maintain, newbieWeapons bool) []multisell.Held {
	if inv == nil {
		return nil
	}
	var (
		out    []multisell.Held
		listed []item.InstanceState
	)
	for _, inst := range inv.Items() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || (tmpl.Kind != item.KindArmor && tmpl.Kind != item.KindWeapon) {
			continue
		}
		st := inst.Snapshot()
		if !tmpl.Stackable && seen(listed, st, maintain) {
			continue
		}
		if st.Equipped() {
			continue
		}
		parting := (maintain && st.Augmentation != nil) ||
			(newbieWeapons && tmpl.Kind == item.KindWeapon && st.TemplateID >= 7816 && st.TemplateID <= 7831) ||
			inst.Sellable(tmpl) || inst.Tradable(tmpl)
		if !parting {
			continue
		}
		listed = append(listed, st)
		out = append(out, multisell.Held{ItemID: st.TemplateID, EnchantLevel: st.EnchantLevel})
	}
	return out
}

func seen(listed []item.InstanceState, st item.InstanceState, maintain bool) bool {
	for _, l := range listed {
		if l.TemplateID != st.TemplateID {
			continue
		}
		if !maintain || (l.EnchantLevel == st.EnchantLevel && sameAugmentation(l.Augmentation, st.Augmentation)) {
			return true
		}
	}
	return false
}

func sameAugmentation(a, b *item.Augmentation) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Notices an exchange reports, in the order they happen.
type (
	// WeightExceeded refuses products the player cannot carry.
	WeightExceeded struct{}
	// SlotsFull refuses products the inventory has no room for.
	SlotsFull struct{}
	// QuantityExceeded refuses an exchange whose totals overflow.
	QuantityExceeded struct{}
	// NotClanMember refuses a clan reputation price to a clanless player.
	NotClanMember struct{}
	// ClanReputationTooLow refuses a clan reputation price the clan cannot
	// pay.
	ClanReputationTooLow struct{}
	// NotEnoughItems refuses an exchange the player lacks ingredients for,
	// or ends one whose ingredient could not be taken.
	NotEnoughItems struct{}
	// Consumed names an ingredient the exchange took.
	Consumed struct {
		ItemID int32
		Count  int
	}
	// Earned names a product the exchange handed over.
	Earned struct {
		ItemID int32
		Count  int
	}
	// EnchantedAcquired names a single enchanted product a list
	// maintaining enchantment handed over.
	EnchantedAcquired struct {
		ItemID  int32
		Enchant int
	}
	// Traded closes a completed exchange.
	Traded struct{}
)

// Choice is one MultiSellChoose request.
type Choice struct {
	ListID  int32
	EntryID int32
	Amount  int32
}

// Outcome is what one exchange request did.
type Outcome struct {
	// Notices are the messages to send, in order.
	Notices []any
	// Forget drops the player's open list: every later choice from it is
	// refused until a list is opened again.
	Forget bool
}

// Choose trades amount units of entry entryID of list, the list c was last
// shown, for c. npcID is the civilian NPC c last selected, or zero, and
// reachable whether c can interact with it. A choice that does not match
// the open list, the NPC or c's reach, or asks for more than one unit of
// an entry with a non-stackable product, forgets the list without a word.
func (s *Service) Choose(c *player.Character, list *multisell.List, npcID int, reachable bool, ch Choice) Outcome {
	if ch.Amount < 1 || ch.Amount > maxAmount || list == nil || list.ID != ch.ListID ||
		ch.EntryID < 1 || int(ch.EntryID) > len(list.Entries) {
		return Outcome{Forget: true}
	}
	if (npcID != 0 && !list.NPCAllowed(int32(npcID))) || (npcID == 0 && list.NPCOnly()) || (npcID != 0 && !reachable) {
		return Outcome{Forget: true}
	}
	inv := c.Inventory()
	entry := list.Entries[ch.EntryID-1]
	amount := int(ch.Amount)
	if inv == nil || (!entry.Stackable() && amount > 1) {
		return Outcome{Forget: true}
	}

	slots, weight := 0, 0
	for _, p := range entry.Products {
		if p.ItemID < 0 {
			continue
		}
		if !p.Stackable() {
			slots += p.Count * amount
		} else if inv.ItemByTemplateID(p.ItemID) == nil {
			slots++
		}
		weight += p.Count * amount * int(p.Weight())
	}
	if !inv.ValidateWeight(weight) {
		return Outcome{Notices: []any{WeightExceeded{}}}
	}
	if !inv.ValidateCapacity(slots) {
		return Outcome{Notices: []any{SlotsFull{}}}
	}

	needs, ok := mergeIngredients(entry.Ingredients)
	if !ok {
		return Outcome{Notices: []any{QuantityExceeded{}}}
	}
	for _, e := range needs {
		if e.Count > math.MaxInt32/amount {
			return Outcome{Notices: []any{QuantityExceeded{}}}
		}
		if e.ItemID == clanReputationID {
			if c.ClanID == 0 {
				return Outcome{Notices: []any{NotClanMember{}}}
			}
			// ponytail: clan reputation (#149). Without the clan system
			// neither the leader nor the clan's score is known, so the
			// price is never payable.
			return Outcome{Notices: []any{ClanReputationTooLow{}}}
		}
		enchant := -1
		if list.MaintainEnchantment {
			enchant = e.EnchantLevel
		}
		if inv.ItemCount(e.ItemID, enchant, false) < s.taken(e, amount) {
			return Outcome{Notices: []any{NotEnoughItems{}}}
		}
	}

	var out Outcome
	augmentations, ok := s.takeIngredients(inv, list, entry, amount, &out)
	if !ok {
		out.Forget = true
		return out
	}
	s.giveProducts(inv, list, entry, amount, augmentations, &out)
	out.Notices = append(out.Notices, Traded{})
	return out
}

// taken is how many units of e an exchange of amount units takes, or, for
// an ingredient kept by the exchange, how many it must find.
func (s *Service) taken(e multisell.Ingredient, amount int) int {
	if s.keepMaintained && e.MaintainIngredient {
		return e.Count
	}
	return e.Count * amount
}

// mergeIngredients adds up the ingredients sharing an item id and enchant
// level, at the place of the first. A merged ingredient reads at enchant
// level 0 from then on. ok is false when a sum would pass the largest
// int32.
func mergeIngredients(ingredients []multisell.Ingredient) ([]multisell.Ingredient, bool) {
	out := make([]multisell.Ingredient, 0, len(ingredients))
	for _, e := range ingredients {
		merged := false
		for i := len(out) - 1; i >= 0; i-- {
			if out[i].ItemID != e.ItemID || out[i].EnchantLevel != e.EnchantLevel {
				continue
			}
			if out[i].Count > math.MaxInt32-e.Count {
				return nil, false
			}
			out[i].Count += e.Count
			out[i].EnchantLevel = 0
			merged = true
			break
		}
		if !merged {
			out = append(out, e)
		}
	}
	return out, true
}

// takeIngredients takes entry's ingredients for amount units from inv, in
// entry order: a stack in one piece; non-stackable items one by one, of
// exactly the ingredient's enchant level on a list maintaining enchantment
// (keeping their augmentations, in order, for the products), else each time
// the lowest-enchanted unworn one. It reports false when an ingredient
// could not be found or taken; what was taken before stays taken.
func (s *Service) takeIngredients(inv *itemcontainer.Inventory, list *multisell.List, entry multisell.Entry, amount int, out *Outcome) ([]item.Augmentation, bool) {
	var augmentations []item.Augmentation
	for _, e := range entry.Ingredients {
		if e.ItemID == clanReputationID {
			continue
		}
		first := inv.ItemByTemplateID(e.ItemID)
		if first == nil {
			return nil, false
		}
		if s.keepMaintained && e.MaintainIngredient {
			continue
		}
		count := e.Count * amount
		tmpl, _ := inv.Templates().Get(first.TemplateID)
		switch {
		case tmpl == nil || tmpl.Stackable:
			if !consume(inv, first, count, out) {
				return nil, false
			}
		case list.MaintainEnchantment:
			matching := unworn(inv, e.ItemID, e.EnchantLevel)
			for i := range count {
				if i >= len(matching) {
					return nil, false
				}
				if st := matching[i].Snapshot(); st.Augmentation != nil {
					augmentations = append(augmentations, *st.Augmentation)
				}
				if !consume(inv, matching[i], 1, out) {
					return nil, false
				}
			}
		default:
			for range count {
				pick := lowestEnchanted(unworn(inv, e.ItemID, -1))
				if pick == nil {
					return nil, false
				}
				if !consume(inv, pick, 1, out) {
					return nil, false
				}
			}
		}
	}
	return augmentations, true
}

// unworn lists inv's unworn instances of itemID, of exactly enchant unless
// it is negative, in inventory order.
func unworn(inv *itemcontainer.Inventory, itemID int32, enchant int) []*item.Instance {
	var out []*item.Instance
	for _, inst := range inv.ItemsByTemplateID(itemID) {
		st := inst.Snapshot()
		if st.Equipped() || (enchant >= 0 && st.EnchantLevel != enchant) {
			continue
		}
		out = append(out, inst)
	}
	return out
}

// lowestEnchanted picks the first of items unless it is enchanted: then
// the first one enchanted lower than every one before it, stopping at the
// first unenchanted one.
func lowestEnchanted(items []*item.Instance) *item.Instance {
	if len(items) == 0 {
		return nil
	}
	pick, level := items[0], items[0].Snapshot().EnchantLevel
	if level == 0 {
		return pick
	}
	for _, inst := range items {
		if l := inst.Snapshot().EnchantLevel; l < level {
			pick, level = inst, l
			if level == 0 {
				break
			}
		}
	}
	return pick
}

// consume destroys count units of inst and names them. It reports false,
// with NotEnoughItems, when inv no longer holds that many. A worn item is
// never consumed: the ingredient check counts unworn items only, a stack
// included, and the non-stackable picks skip worn ones.
func consume(inv *itemcontainer.Inventory, inst *item.Instance, count int, out *Outcome) bool {
	st := inst.Snapshot()
	if inv.ItemByObjectID(inst.ObjectID) != inst || st.Count < count || st.Equipped() {
		out.Notices = append(out.Notices, NotEnoughItems{})
		return false
	}
	if inv.DestroyItem(inst, count) == nil {
		out.Notices = append(out.Notices, NotEnoughItems{})
		return false
	}
	out.Notices = append(out.Notices, Consumed{ItemID: st.TemplateID, Count: count})
	return true
}

// giveProducts hands over entry's products for amount units: a stackable
// in one stack, anything else one item at a time, which on a list
// maintaining enchantment takes the product's enchant level and, by its
// place among the units, the taken ingredients' augmentations. Each product
// is named once.
func (s *Service) giveProducts(inv *itemcontainer.Inventory, list *multisell.List, entry multisell.Entry, amount int, augmentations []item.Augmentation, out *Outcome) {
	for _, p := range entry.Products {
		if p.ItemID == clanReputationID {
			// ponytail: clan reputation (#149). The points would go to the
			// player's clan, which the clan system does not hold yet; no
			// shipped list pays them.
			continue
		}
		count := p.Count * amount
		if p.Stackable() {
			s.add(inv, p.ItemID, count)
		} else {
			for i := range count {
				inst := s.add(inv, p.ItemID, 1)
				if inst == nil || !list.MaintainEnchantment {
					continue
				}
				if i < len(augmentations) {
					inst.SetAugmentation(&augmentations[i])
				}
				inv.SetEnchantLevel(inst, p.EnchantLevel)
			}
		}
		switch {
		case count > 1:
			out.Notices = append(out.Notices, Earned{ItemID: p.ItemID, Count: count})
		case list.MaintainEnchantment && p.EnchantLevel > 0:
			out.Notices = append(out.Notices, EnchantedAcquired{ItemID: p.ItemID, Enchant: p.EnchantLevel})
		default:
			out.Notices = append(out.Notices, Earned{ItemID: p.ItemID, Count: count})
		}
	}
}

// add creates count units of itemID in inv as one new instance, joining a
// held stack of a stackable item. It returns nil when itemID has no
// template or no object id could be allocated.
func (s *Service) add(inv *itemcontainer.Inventory, itemID int32, count int) *item.Instance {
	if s.nextID == nil {
		return nil
	}
	if _, ok := inv.Templates().Get(itemID); !ok {
		return nil
	}
	id, err := s.nextID()
	if err != nil {
		return nil
	}
	return inv.AddNew(itemID, count, id)
}
