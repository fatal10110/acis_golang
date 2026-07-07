package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// DropKind distinguishes the rate table a drop category rolls against:
// spoil (sweep-only), currency (adena-like), a normal item drop, or an
// herb pickup.
type DropKind uint8

const (
	DropSpoil DropKind = iota
	DropCurrency
	DropNormal
	DropHerb
)

// String returns the canonical XML spelling for k.
func (k DropKind) String() string {
	switch k {
	case DropSpoil:
		return "SPOIL"
	case DropCurrency:
		return "CURRENCY"
	case DropNormal:
		return "DROP"
	case DropHerb:
		return "HERB"
	default:
		return fmt.Sprintf("DropKind(%d)", uint8(k))
	}
}

// dropKindNames maps a drop category's XML "type" attribute to the DropKind
// it selects.
var dropKindNames = map[string]DropKind{
	"SPOIL":    DropSpoil,
	"CURRENCY": DropCurrency,
	"DROP":     DropNormal,
	"HERB":     DropHerb,
}

// ParseDropKind resolves a drop category's "type" attribute to a DropKind.
// It returns an error for any other value rather than guessing.
func ParseDropKind(s string) (DropKind, error) {
	k, ok := dropKindNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown drop kind %q", s)
	}
	return k, nil
}

// Drop is one entry in a DropCategory: the item and quantity range it can
// yield, and its share of that category's roll.
type Drop struct {
	ItemID   int32
	Min, Max int32
	Chance   float64
}

// NewDrop builds a Drop from set, the folded attributes of one <drop>
// element. itemid, min, max and chance are all required.
func NewDrop(set *commons.StatSet) (Drop, error) {
	itemID, err := set.GetInt32("itemid")
	if err != nil {
		return Drop{}, fmt.Errorf("item: drop: %w", err)
	}
	min, err := set.GetInt32("min")
	if err != nil {
		return Drop{}, fmt.Errorf("item: drop %d: %w", itemID, err)
	}
	max, err := set.GetInt32("max")
	if err != nil {
		return Drop{}, fmt.Errorf("item: drop %d: %w", itemID, err)
	}
	chance, err := set.GetDouble("chance")
	if err != nil {
		return Drop{}, fmt.Errorf("item: drop %d: %w", itemID, err)
	}
	return Drop{ItemID: itemID, Min: min, Max: max, Chance: chance}, nil
}

// DropCategory is one weighted group of possible drops an NPC template
// carries (e.g. one spoil result, one general drop table). Rolling a
// category against the server's drop-rate configuration is combat/loot
// behavior owned by the system that resolves a kill, not this loader's
// concern; this type only holds the data as read from the template.
type DropCategory struct {
	Kind   DropKind
	Chance float64
	Drops  []Drop
}

// NewDropCategory builds a DropCategory from set, the folded attributes of
// one <category> element, and its already-parsed drops. type is required;
// chance defaults to 100 when absent.
func NewDropCategory(set *commons.StatSet, drops []Drop) (DropCategory, error) {
	typeAttr, err := set.GetString("type")
	if err != nil {
		return DropCategory{}, fmt.Errorf("item: drop category: %w", err)
	}
	kind, err := ParseDropKind(typeAttr)
	if err != nil {
		return DropCategory{}, fmt.Errorf("item: drop category: %w", err)
	}
	chance, err := set.GetDoubleDefault("chance", 100.0)
	if err != nil {
		return DropCategory{}, fmt.Errorf("item: drop category: %w", err)
	}
	return DropCategory{Kind: kind, Chance: chance, Drops: drops}, nil
}
