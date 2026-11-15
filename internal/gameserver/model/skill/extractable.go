package skill

import (
	"strconv"
	"strings"
)

// ExtractableItem is one item id/quantity pair inside an ExtractableProduct.
type ExtractableItem struct {
	ItemID   int32
	Quantity int
}

// ExtractableProduct is one random reward set a "capsule" skill can grant:
// every item/quantity pair granted together, plus the percent chance (e.g.
// 50.0 for 50%) this set is picked over the others.
type ExtractableProduct struct {
	Items  []ExtractableItem
	Chance float64
}

// ParseExtractableItems structures a Definition's raw ExtractableItems
// string into its product rows: semicolon-separated groups of comma-
// separated fields, every field but the last read in id/quantity pairs and
// the last read as the row's percent chance (e.g.
// "57,10,20.5;1234,1,5678,2,79.5"). With an even field count the last field
// is both the last pair's quantity and the chance; a single field is a row
// of no items. Trailing empty groups and fields are ignored. A group that
// does not parse is skipped rather than failing the whole skill.
func ParseExtractableItems(raw string) []ExtractableProduct {
	if raw == "" {
		return nil
	}

	var products []ExtractableProduct
	for _, group := range splitDroppingTrailingEmpty(raw, ";") {
		product, ok := parseExtractableGroup(group)
		if !ok {
			continue
		}
		products = append(products, product)
	}
	return products
}

func parseExtractableGroup(group string) (ExtractableProduct, bool) {
	fields := splitDroppingTrailingEmpty(group, ",")
	if len(fields) == 0 {
		return ExtractableProduct{}, false
	}

	last := len(fields) - 1
	items := make([]ExtractableItem, 0, last/2)
	for i := 0; i < last; i += 2 {
		itemID, err := strconv.ParseInt(fields[i], 10, 32)
		if err != nil {
			return ExtractableProduct{}, false
		}
		// A quantity past the 32-bit range fails its group.
		quantity, err := strconv.ParseInt(fields[i+1], 10, 32)
		if err != nil {
			return ExtractableProduct{}, false
		}
		items = append(items, ExtractableItem{ItemID: int32(itemID), Quantity: int(quantity)})
	}

	chance, err := strconv.ParseFloat(fields[last], 64)
	if err != nil {
		return ExtractableProduct{}, false
	}

	return ExtractableProduct{Items: items, Chance: chance}, true
}

// PickExtractableProduct returns the row roll, a draw in [0, 100000),
// lands on: each row in turn takes chance * 1000 (truncated) off the roll,
// and the first to take it below zero is picked. ok is false when no row
// does.
func PickExtractableProduct(products []ExtractableProduct, roll int) (product ExtractableProduct, ok bool) {
	for _, p := range products {
		roll -= int(p.Chance * 1000)
		if roll < 0 {
			return p, true
		}
	}
	return ExtractableProduct{}, false
}

// splitDroppingTrailingEmpty splits s around sep and drops the empty parts
// at its end; a whole empty s is one empty part.
func splitDroppingTrailingEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	if s == "" {
		return parts
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
