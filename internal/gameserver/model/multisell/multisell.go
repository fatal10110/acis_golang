// Package multisell models loaded multisell lists and their ingredients.
package multisell

import (
	"errors"
	"fmt"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Ingredient is one item consumed or produced by a multisell entry.
type Ingredient struct {
	ItemID             int32
	Count              int
	EnchantLevel       int
	TaxIngredient      bool
	MaintainIngredient bool

	template *item.Template
}

// NewIngredient builds an Ingredient from one <ingredient> or <production>
// element's attributes. id and count are required. If items is non-nil and
// contains ItemID, the matching template is attached for stackability and
// weight queries; otherwise those queries fall back to the same defaults the
// source behavior uses for unknown items.
func NewIngredient(set *commons.StatSet, items *item.Table) (Ingredient, error) {
	itemID, err := set.GetInt32("id")
	if err != nil {
		return Ingredient{}, fmt.Errorf("multisell ingredient: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("multisell ingredient %d: %w", itemID, err) }

	count, err := set.GetInt("count")
	if err != nil {
		return Ingredient{}, wrap(err)
	}
	enchantLevel, err := set.GetIntDefault("enchantLevel", 0)
	if err != nil {
		return Ingredient{}, wrap(err)
	}

	in := Ingredient{
		ItemID:             itemID,
		Count:              count,
		EnchantLevel:       enchantLevel,
		TaxIngredient:      set.GetBoolDefault("isTaxIngredient", false),
		MaintainIngredient: set.GetBoolDefault("maintainIngredient", false),
	}
	if items != nil && itemID > 0 {
		in.template, _ = items.Get(itemID)
	}
	return in, nil
}

// Template returns the resolved item template, if one was attached at load
// time.
func (i Ingredient) Template() *item.Template {
	return i.template
}

// Stackable reports whether the ingredient's item stacks. Unknown items are
// treated as stackable, matching the source behavior's null-template path.
func (i Ingredient) Stackable() bool {
	return i.template == nil || i.template.Stackable
}

// ArmorOrWeapon reports whether the ingredient resolves to an armor or weapon
// template.
func (i Ingredient) ArmorOrWeapon() bool {
	if i.template == nil {
		return false
	}
	return i.template.Kind == item.KindArmor || i.template.Kind == item.KindWeapon
}

// Weight returns the per-unit item weight, or zero when the template is not
// known.
func (i Ingredient) Weight() int32 {
	if i.template == nil {
		return 0
	}
	return i.template.Weight
}

// Entry is one multisell exchange option: ordered ingredients consumed and
// ordered products produced.
type Entry struct {
	Ingredients []Ingredient
	Products    []Ingredient
	stackable   bool
}

// NewEntry builds an Entry from its already-parsed ingredients and products.
func NewEntry(ingredients, products []Ingredient) Entry {
	stackable := true
	for _, product := range products {
		if !product.Stackable() {
			stackable = false
			break
		}
	}
	return Entry{Ingredients: ingredients, Products: products, stackable: stackable}
}

// Stackable reports whether every product item in the entry stacks.
func (e Entry) Stackable() bool {
	return e.stackable
}

// TaxAmount is not populated by the M3 loader slice yet.
func (e Entry) TaxAmount() int {
	return 0
}

// List is one loaded multisell list keyed by its filename hash.
type List struct {
	ID                  int32
	ApplyTaxes          bool
	MaintainEnchantment bool
	Entries             []Entry
	NPCIDs              []int32
}

// NPCAllowed reports whether npcID may open the list.
func (l *List) NPCAllowed(npcID int32) bool {
	if len(l.NPCIDs) == 0 {
		return true
	}
	for _, allowed := range l.NPCIDs {
		if allowed == npcID {
			return true
		}
	}
	return false
}

// NPCOnly reports whether the list is restricted to explicit NPC ids.
func (l *List) NPCOnly() bool {
	return len(l.NPCIDs) > 0
}

// Table is an in-memory lookup of multisell lists keyed by list id, built
// once at boot and read for the remainder of the process lifetime.
type Table struct {
	lists map[int32]*List
}

// NewTable returns a Table backed by lists. An empty slice is an error: a
// multisell table with no lists is not useful data.
func NewTable(lists []*List) (*Table, error) {
	if len(lists) == 0 {
		return nil, errors.New("multisell: table has no lists")
	}
	t := &Table{lists: make(map[int32]*List, len(lists))}
	for _, list := range lists {
		t.lists[list.ID] = list
	}
	return t, nil
}

// Get returns the list with id, or false if none was loaded.
func (t *Table) Get(id int32) (*List, bool) {
	list, ok := t.lists[id]
	return list, ok
}

// Count returns the number of lists loaded.
func (t *Table) Count() int {
	return len(t.lists)
}

// All returns every loaded list, ordered ascending by id.
func (t *Table) All() []*List {
	lists := make([]*List, 0, len(t.lists))
	for _, list := range t.lists {
		lists = append(lists, list)
	}
	sort.Slice(lists, func(i, j int) bool { return lists[i].ID < lists[j].ID })
	return lists
}
