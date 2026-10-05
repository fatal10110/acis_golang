// Package armorset models static armor set data loaded at boot.
package armorset

import "github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"

// Set is one armor set template.
type Set struct {
	Name          string
	Chest         int32
	Legs          int32
	Head          int32
	Gloves        int32
	Feet          int32
	SkillID       int32
	Shield        int32
	ShieldSkillID int32
	Enchant6Skill int32
}

// PieceIDs returns chest, legs, head, gloves and feet item ids in paperdoll order.
func (s Set) PieceIDs() [5]int32 {
	return [5]int32{s.Chest, s.Legs, s.Head, s.Gloves, s.Feet}
}

// Table stores armor sets keyed by chest item id.
type Table struct {
	byChest map[int32]Set
}

// NewTable builds an armor set lookup table.
func NewTable(sets []Set) *Table {
	t := &Table{byChest: make(map[int32]Set, len(sets))}
	for _, s := range sets {
		t.byChest[s.Chest] = s
	}
	return t
}

// Len returns the number of armor sets keyed by chest item id.
func (t *Table) Len() int {
	return len(t.byChest)
}

// FindByChest returns the armor set whose chest piece is chestID.
func (t *Table) FindByChest(chestID int32) (Set, bool) {
	if t == nil {
		return Set{}, false
	}
	s, ok := t.byChest[chestID]
	return s, ok
}

// Worn returns the set whose chest piece doll wears.
func (t *Table) Worn(doll *itemcontainer.Inventory) (Set, bool) {
	if t == nil || doll == nil {
		return Set{}, false
	}
	chest := doll.ItemAt(itemcontainer.Chest)
	if chest == nil {
		return Set{}, false
	}
	return t.FindByChest(chest.TemplateID)
}

// pieceSlots are the paperdoll positions of PieceIDs, in the same order.
var pieceSlots = [5]int{itemcontainer.Chest, itemcontainer.Legs, itemcontainer.Head, itemcontainer.Gloves, itemcontainer.Feet}

// ContainsItem reports whether itemID is the set's piece for paperdoll
// position slot. Any position other than the five set pieces' holds none.
func (s Set) ContainsItem(slot int, itemID int32) bool {
	for i, pos := range pieceSlots {
		if pos == slot {
			return s.PieceIDs()[i] == itemID
		}
	}
	return false
}

// ContainsAll reports whether doll wears every non-chest piece the set
// names; a piece the set leaves at 0 accepts whatever that position holds.
// The chest is not checked: the set is looked up by the worn chest.
func (s Set) ContainsAll(doll *itemcontainer.Inventory) bool {
	return s.wears(doll, 0)
}

// Enchant6Level is the enchant level from which a worn set's pieces grant
// its +6 skill.
const Enchant6Level = 6

// Enchanted6 reports whether doll wears the set's chest at Enchant6Level or
// higher and every other piece the set names at Enchant6Level or higher.
func (s Set) Enchanted6(doll *itemcontainer.Inventory) bool {
	chest := doll.ItemAt(itemcontainer.Chest)
	if chest == nil || chest.Snapshot().EnchantLevel < Enchant6Level {
		return false
	}
	return s.wears(doll, Enchant6Level)
}

// wears reports whether every non-chest piece the set names is worn at
// minEnchant or higher.
func (s Set) wears(doll *itemcontainer.Inventory, minEnchant int) bool {
	ids := s.PieceIDs()
	for i := 1; i < len(ids); i++ {
		if ids[i] == 0 {
			continue
		}
		inst := doll.ItemAt(pieceSlots[i])
		if inst == nil || inst.TemplateID != ids[i] || inst.Snapshot().EnchantLevel < minEnchant {
			return false
		}
	}
	return true
}

// WearsShield reports whether doll holds the set's shield in the left hand.
func (s Set) WearsShield(doll *itemcontainer.Inventory) bool {
	inst := doll.ItemAt(itemcontainer.LHand)
	return inst != nil && inst.TemplateID == s.Shield
}

// IsShield reports whether itemID is the set's shield; a set without one
// has none.
func (s Set) IsShield(itemID int32) bool {
	return s.Shield != 0 && s.Shield == itemID
}
