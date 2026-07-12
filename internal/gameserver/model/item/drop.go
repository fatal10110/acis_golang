package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
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

// RandomAmount returns a uniformly random quantity in [Min, Max], inclusive
// of both ends. Malformed data with Max below Min yields Min rather than
// panicking.
func (d Drop) RandomAmount() int32 {
	if d.Max <= d.Min {
		return d.Min
	}
	return int32(rnd.GetRange(int(d.Min), int(d.Max)))
}

// NewDrop builds a Drop from set, the folded attributes of one <drop>
// element. itemid, min, max and chance are all required.
func NewDrop(set *commons.StatSet) (Drop, error) {
	idf := commons.NewFields(set, "item: drop")
	itemID := idf.Int32("itemid")
	if err := idf.Err(); err != nil {
		return Drop{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("item: drop %d", itemID))
	drop := Drop{
		ItemID: itemID,
		Min:    f.Int32("min"),
		Max:    f.Int32("max"),
		Chance: f.Float64("chance"),
	}
	if err := f.Err(); err != nil {
		return Drop{}, err
	}
	return drop, nil
}

// DropCategory is one weighted group of possible drops an NPC template
// carries (e.g. one spoil result, one general drop table). See Roll for how
// a category resolves against a kill.
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
	chance, err := set.GetFloat64Default("chance", 100.0)
	if err != nil {
		return DropCategory{}, fmt.Errorf("item: drop category: %w", err)
	}
	return DropCategory{Kind: kind, Chance: chance, Drops: drops}, nil
}

// maxRollChance is the resolution a chance percentage rolls against: a
// chance is scaled to this range and compared to a uniform draw in
// [0, maxRollChance).
const maxRollChance = 1_000_000

// rollChanceScale converts a chance expressed as a percentage (0-100) into
// the maxRollChance range.
const rollChanceScale = maxRollChance / 100.0

// Roll evaluates this category once per whole unit of rate (a fractional
// remainder still gets one extra attempt, e.g. a rate of 1.5 rolls twice)
// and returns the resulting item quantities keyed by item ID, merging
// duplicates when more than one attempt or drop lands on the same item.
// levelMultiplier scales the category's base chance down for an
// out-leveled kill (1 leaves it unaffected); rate is the drop-rate
// multiplier already resolved for this category's kind and raid/non-raid
// status (see Rates.Resolve). A zero chance, multiplier, or rate never
// drops.
//
// A spoil category evaluates every one of its drops independently on each
// attempt (a spoil pool can accumulate several items at once); any other
// category picks at most one drop per attempt, weighted by each drop's
// share of the category's cumulative chance.
func (c DropCategory) Roll(levelMultiplier, rate float64) map[int32]int32 {
	if c.Chance == 0 || levelMultiplier == 0 || rate == 0 {
		return nil
	}

	var result map[int32]int32
	add := func(itemID, quantity int32) {
		if result == nil {
			result = make(map[int32]int32, 1)
		}
		result[itemID] += quantity
	}

	for i := 0; float64(i) < rate; i++ {
		chance := c.Chance * levelMultiplier * rollChanceScale
		if chance < maxRollChance && float64(rnd.Get(maxRollChance)) >= chance {
			continue
		}

		if c.Kind == DropSpoil {
			for _, d := range c.Drops {
				dropChance := d.Chance * rollChanceScale
				if dropChance >= maxRollChance || float64(rnd.Get(maxRollChance)) < dropChance {
					add(d.ItemID, d.RandomAmount())
				}
			}
			continue
		}

		cumulative := 0.0
		roll := float64(rnd.Get(maxRollChance))
		for _, d := range c.Drops {
			cumulative += d.Chance * rollChanceScale
			if roll < cumulative {
				add(d.ItemID, d.RandomAmount())
				break
			}
		}
	}
	return result
}

// Rates holds the configured drop-rate multipliers, one per DropKind, that
// Roll's rate argument is drawn from. A caller assembles this from server
// configuration.
type Rates struct {
	Spoil    float64
	Currency float64
	Item     float64
	ItemRaid float64
	Herb     float64
}

// Resolve returns the configured rate for kind, using ItemRaid instead of
// Item for a normal item drop when raid is true.
func (r Rates) Resolve(kind DropKind, raid bool) float64 {
	switch kind {
	case DropSpoil:
		return r.Spoil
	case DropCurrency:
		return r.Currency
	case DropNormal:
		if raid {
			return r.ItemRaid
		}
		return r.Item
	case DropHerb:
		return r.Herb
	default:
		return 0
	}
}

// LevelPenaltyMultiplier returns the drop-rate multiplier applied when the
// killer significantly outlevels the dying monster. attackerLevel is the
// highest level among the monster's attackers; monsterLevel is the
// monster's own level. enabled gates whether the rule applies at all (a
// server can turn it off, in which case the multiplier is always 1).
//
// A raid boss tolerates 2 levels of difference before the penalty kicks
// in; any other monster tolerates 5. Beyond that, each extra level cuts
// the multiplier by 18%, floored at 0.1.
func LevelPenaltyMultiplier(attackerLevel, monsterLevel int32, raid, enabled bool) float64 {
	if !enabled {
		return 1
	}

	threshold := int32(5)
	if raid {
		threshold = 2
	}

	diff := attackerLevel - monsterLevel - threshold
	if diff <= 0 {
		return 1
	}
	return max(0.1, 1-0.18*float64(diff))
}
