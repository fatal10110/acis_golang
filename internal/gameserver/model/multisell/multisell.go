// Package multisell models loaded multisell lists and their ingredients.
package multisell

import (
	"errors"
	"fmt"

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
	idf := commons.NewFields(set, "multisell ingredient")
	itemID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return Ingredient{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("multisell ingredient %d", itemID))
	in := Ingredient{
		ItemID:             itemID,
		Count:              f.Int("count"),
		TaxIngredient:      f.BoolDefault("isTaxIngredient", false),
		MaintainIngredient: f.BoolDefault("maintainIngredient", false),
	}
	if err := f.Err(); err != nil {
		return Ingredient{}, err
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

// PageSize is how many entries one MultiSellList page carries.
const PageSize = 40

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

// prepare returns e as a talker is shown it. Its adena ingredients leave
// their places: the tax ingredients are dropped, as no castle collects tax
// yet, and the rest become one adena ingredient at the end. When enchant is
// set, every armor or weapon ingredient and product takes its level.
//
// ponytail: castle taxes (#239). Under a castle with an owner, a list
// applying taxes adds its tax ingredients at the castle's rate, rounded
// half up, to that adena ingredient, and the exchange pays the castle.
func (e Entry) prepare(enchant *int) Entry {
	out := Entry{Ingredients: make([]Ingredient, 0, len(e.Ingredients)+1), stackable: true}
	adena := 0
	var adenaTemplate *item.Template
	for _, in := range e.Ingredients {
		if in.ItemID == item.AdenaID {
			adenaTemplate = in.template
			if !in.TaxIngredient {
				adena += in.Count
			}
			continue
		}
		out.Ingredients = append(out.Ingredients, in.prepared(enchant))
	}
	if adena > 0 {
		out.Ingredients = append(out.Ingredients, Ingredient{ItemID: item.AdenaID, Count: adena, template: adenaTemplate})
	}
	out.Products = make([]Ingredient, 0, len(e.Products))
	for _, p := range e.Products {
		if !p.Stackable() {
			out.stackable = false
		}
		out.Products = append(out.Products, p.prepared(enchant))
	}
	return out
}

// prepared is a copy of i at no enchant level, or at enchant's when set
// and i is an armor or weapon.
func (i Ingredient) prepared(enchant *int) Ingredient {
	i.EnchantLevel = 0
	if enchant != nil && i.ArmorOrWeapon() {
		i.EnchantLevel = *enchant
	}
	return i
}

// List is one loaded multisell list keyed by its filename hash.
type List struct {
	ID                  int32
	ApplyTaxes          bool
	MaintainEnchantment bool
	Entries             []Entry
	NPCIDs              []int32
}

// Held is one inventory item an inventory-only list is matched against.
type Held struct {
	ItemID       int32
	EnchantLevel int
}

// Prepare returns the list as a talker is shown it.
func (l *List) Prepare() *List {
	out := l.preparedHeader()
	out.Entries = make([]Entry, 0, len(l.Entries))
	for _, e := range l.Entries {
		out.Entries = append(out.Entries, e.prepare(nil))
	}
	return out
}

// PrepareFor returns the inventory-only form of the list: for each held
// item in order, every entry taking that item as an ingredient, in list
// order. On a list that maintains enchantment, those entries' armor and
// weapon ingredients and products take the held item's level.
func (l *List) PrepareFor(held []Held) *List {
	out := l.preparedHeader()
	for _, h := range held {
		var enchant *int
		if l.MaintainEnchantment {
			enchant = &h.EnchantLevel
		}
		for _, e := range l.Entries {
			for _, in := range e.Ingredients {
				if in.ItemID == h.ItemID {
					out.Entries = append(out.Entries, e.prepare(enchant))
					break
				}
			}
		}
	}
	return out
}

// preparedHeader is the list's header as prepared: it applies no taxes.
func (l *List) preparedHeader() *List {
	return &List{ID: l.ID, MaintainEnchantment: l.MaintainEnchantment, NPCIDs: l.NPCIDs}
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
// at boot and replaced in place by //reload multisell.
type Table struct {
	*commons.Lookup[int32, *List]
}

// NewTable returns a Table backed by lists. An empty slice is an error: a
// multisell table with no lists is not useful data.
func NewTable(lists []*List) (*Table, error) {
	if len(lists) == 0 {
		return nil, errors.New("multisell: table has no lists")
	}
	return &Table{commons.NewLookup(lists, func(l *List) int32 { return l.ID })}, nil
}

// Replace swaps t's lists for from's, at once for every holder of t:
// MultisellData.reload. A list already prepared for a player stays as it
// was prepared.
func (t *Table) Replace(from *Table) {
	t.Swap(from.Lookup)
}

// Count returns the number of lists loaded.
func (t *Table) Count() int {
	return t.Len()
}
