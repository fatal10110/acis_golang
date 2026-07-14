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
// string into its product rows: semicolon-separated groups, each a comma-
// separated list of item id/quantity pairs followed by a trailing percent
// chance (e.g. "57,10,20.5;1234,1,5678,2,79.5"). A malformed group is
// skipped rather than failing the whole skill, matching the reference
// parser's per-group error tolerance.
func ParseExtractableItems(raw string) []ExtractableProduct {
	if raw == "" {
		return nil
	}

	var products []ExtractableProduct
	for _, group := range strings.Split(raw, ";") {
		product, ok := parseExtractableGroup(group)
		if !ok {
			continue
		}
		products = append(products, product)
	}
	return products
}

func parseExtractableGroup(group string) (ExtractableProduct, bool) {
	fields := strings.Split(group, ",")
	// A group is n item/quantity pairs followed by one chance value: an
	// odd count of at least 3 fields.
	if len(fields) < 3 || len(fields)%2 == 0 {
		return ExtractableProduct{}, false
	}

	pairFields := len(fields) - 1
	items := make([]ExtractableItem, 0, pairFields/2)
	for i := 0; i < pairFields; i += 2 {
		itemID, err := strconv.ParseInt(fields[i], 10, 32)
		if err != nil {
			return ExtractableProduct{}, false
		}
		quantity, err := strconv.Atoi(fields[i+1])
		if err != nil {
			return ExtractableProduct{}, false
		}
		items = append(items, ExtractableItem{ItemID: int32(itemID), Quantity: quantity})
	}

	chance, err := strconv.ParseFloat(fields[pairFields], 64)
	if err != nil {
		return ExtractableProduct{}, false
	}

	return ExtractableProduct{Items: items, Chance: chance}, true
}
