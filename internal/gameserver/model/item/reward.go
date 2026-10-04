package item

import "slices"

// KillDrop is one reward a kill's roll hands its killer: an item drop of
// Count, or, with Herb set, one herb pickup (see SplitHerbDrop) whose
// AutoLoot says whether the killer takes it on the spot.
type KillDrop struct {
	ItemID, Count  int32
	Herb, AutoLoot bool
}

// RollKillReward rolls every category in categories for one kill and
// returns what the killer receives, in the order the reference hands it
// out: categories in template order, each category's results in the
// reference's per-category order (see dropMerger).
//
//   - A spoil category only rolls once the monster already carries a spoil
//     marker (pool.IsSpoiled), and its results merge into pool rather than
//     the returned drops.
//   - A herb category's results are split per SplitHerbDrop into herb
//     pickups.
//   - Every other category (currency, normal item) returns its results as
//     drops of their own: the same item rolled by two categories stays two
//     drops, each dropped apart.
//
// levelMultiplier and raid are forwarded to every category's roll exactly
// as DropCategory.Roll expects; rates resolves the per-kind drop-rate
// multiplier. pool may be nil, in which case spoil categories are skipped
// entirely (equivalent to an unspoiled monster).
func RollKillReward(categories []DropCategory, pool *SpoilPool, levelMultiplier float64, raid bool, rates Rates, autoLootHerbs bool) []KillDrop {
	var drops []KillDrop
	for _, cat := range categories {
		if cat.Kind == DropSpoil && (pool == nil || !pool.IsSpoiled()) {
			continue
		}

		rolled := cat.rollOrdered(levelMultiplier, rates.Resolve(cat.Kind, raid))
		if len(rolled) == 0 {
			continue
		}

		switch cat.Kind {
		case DropSpoil:
			// The pool keeps the order items are added in, which Sweep
			// reads; add them in the category's drop order.
			for _, d := range cat.Drops {
				if i := slices.IndexFunc(rolled, func(r rolledDrop) bool { return r.ItemID == d.ItemID }); i >= 0 {
					pool.Add(d.ItemID, rolled[i].Count)
					rolled = slices.Delete(rolled, i, i+1)
				}
			}
		case DropHerb:
			for _, d := range rolled {
				for _, p := range SplitHerbDrop(d.ItemID, d.Count, autoLootHerbs) {
					drops = append(drops, KillDrop{ItemID: p.ItemID, Count: p.Amount, Herb: true, AutoLoot: p.AutoLoot})
				}
			}
		default:
			for _, d := range rolled {
				drops = append(drops, KillDrop{ItemID: d.ItemID, Count: d.Count})
			}
		}
	}
	return drops
}
