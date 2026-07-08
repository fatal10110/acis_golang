// Package recipe models static crafting recipe data loaded at boot.
package recipe

import (
	"fmt"
	"strconv"
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

// New builds a Recipe from one folded <recipe> element.
func New(set *commons.StatSet) (Recipe, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return Recipe{}, fmt.Errorf("recipe: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("recipe %d: %w", id, err) }

	rawMaterials, err := set.GetString("material")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	materials, err := parseIngredients(rawMaterials)
	if err != nil {
		return Recipe{}, wrap(fmt.Errorf("material %q: %w", rawMaterials, err))
	}
	rawProduct, err := set.GetString("product")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	product, err := parseIngredient(rawProduct)
	if err != nil {
		return Recipe{}, wrap(fmt.Errorf("product %q: %w", rawProduct, err))
	}
	itemID, err := set.GetInt32("itemId")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	level, err := set.GetInt("level")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	mpCost, err := set.GetInt("mpConsume")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	successRate, err := set.GetInt("successRate")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	dwarven, err := set.GetBool("isDwarven")
	if err != nil {
		return Recipe{}, wrap(err)
	}
	alias, err := set.GetString("alias")
	if err != nil {
		return Recipe{}, wrap(err)
	}

	return Recipe{
		Materials: materials, Product: product, ID: id, Level: level, ItemID: itemID,
		Alias: alias, SuccessRate: successRate, MPCost: mpCost, Dwarven: dwarven,
	}, nil
}

func parseIngredients(raw string) ([]Ingredient, error) {
	parts := strings.Split(raw, ";")
	out := make([]Ingredient, len(parts))
	for i, part := range parts {
		ingredient, err := parseIngredient(part)
		if err != nil {
			return nil, err
		}
		out[i] = ingredient
	}
	return out, nil
}

func parseIngredient(raw string) (Ingredient, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return Ingredient{}, fmt.Errorf("want item-count")
	}
	itemID, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return Ingredient{}, err
	}
	count, err := strconv.Atoi(parts[1])
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

// Find returns the recipe with id.
func (t *Table) Find(id int) (Recipe, bool) {
	r, ok := t.byID[id]
	return r, ok
}

// FindByItemID returns the recipe attached to recipe item id.
func (t *Table) FindByItemID(itemID int32) (Recipe, bool) {
	r, ok := t.byItemID[itemID]
	return r, ok
}
