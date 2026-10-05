// Package recipe models static crafting recipe data loaded at boot.
package recipe

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Ingredient is an item id and quantity pair used by a recipe.
type Ingredient struct {
	ItemID int32
	Count  int
}

// Recipe is one static crafting recipe row.
type Recipe struct {
	Materials   []Ingredient
	Product     Ingredient
	ID          int
	Level       int
	ItemID      int32
	Alias       string
	SuccessRate int
	MPCost      int
	Dwarven     bool
}

// ParseIngredients parses a ";"-separated list of item-count pairs.
func ParseIngredients(raw string) ([]Ingredient, error) {
	parts := strings.Split(raw, ";")
	out := make([]Ingredient, len(parts))
	for i, part := range parts {
		ingredient, err := ParseIngredient(part)
		if err != nil {
			return nil, err
		}
		out[i] = ingredient
	}
	return out, nil
}

// ParseIngredient parses one "item-count" pair.
func ParseIngredient(raw string) (Ingredient, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return Ingredient{}, fmt.Errorf("want item-count")
	}
	itemID, err := commons.ParseInt(parts[0], 32)
	if err != nil {
		return Ingredient{}, err
	}
	count, err := commons.Atoi(parts[1])
	if err != nil {
		return Ingredient{}, err
	}
	return Ingredient{ItemID: int32(itemID), Count: count}, nil
}

// Table stores recipes keyed by recipe id and by recipe item id.
type Table struct {
	byID     map[int]Recipe
	byItemID map[int32]Recipe
}

// NewTable builds a recipe lookup table.
func NewTable(recipes []Recipe) *Table {
	t := &Table{
		byID:     make(map[int]Recipe, len(recipes)),
		byItemID: make(map[int32]Recipe, len(recipes)),
	}
	for _, r := range recipes {
		t.byID[r.ID] = r
		t.byItemID[r.ItemID] = r
	}
	return t
}

// Len returns the number of recipes keyed by recipe id.
func (t *Table) Len() int {
	return len(t.byID)
}

// Find returns the recipe with id. A nil table holds none.
func (t *Table) Find(id int) (Recipe, bool) {
	if t == nil {
		return Recipe{}, false
	}
	r, ok := t.byID[id]
	return r, ok
}

// FindByItemID returns the recipe attached to recipe item id. A nil table
// holds none.
func (t *Table) FindByItemID(itemID int32) (Recipe, bool) {
	if t == nil {
		return Recipe{}, false
	}
	r, ok := t.byItemID[itemID]
	return r, ok
}
