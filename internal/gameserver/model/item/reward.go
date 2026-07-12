package item

// RollKillReward rolls every category in categories for one kill and
// splits the results by how a killer receives them:
//
//   - A spoil category only rolls once the monster already carries a spoil
//     marker (pool.IsSpoiled), and its results merge into pool rather than
//     items.
//   - A herb category's results are split per SplitHerbDrop and returned in
//     herbs.
//   - Every other category (currency, normal item) merges into items,
//     keyed by item ID.
//
// levelMultiplier and raid are forwarded to every category's Roll exactly
// as DropCategory.Roll expects; rates resolves the per-kind drop-rate
// multiplier. pool may be nil, in which case spoil categories are skipped
// entirely (equivalent to an unspoiled monster).
func RollKillReward(categories []DropCategory, pool *SpoilPool, levelMultiplier float64, raid bool, rates Rates, autoLootHerbs bool) (items map[int32]int32, herbs []HerbPickup) {
	for _, cat := range categories {
		if cat.Kind == DropSpoil && (pool == nil || !pool.IsSpoiled()) {
			continue
		}

		rolled := cat.Roll(levelMultiplier, rates.Resolve(cat.Kind, raid))
		if len(rolled) == 0 {
			continue
		}

		switch cat.Kind {
		case DropSpoil:
			for id, qty := range rolled {
				pool.Add(id, qty)
			}
		case DropHerb:
			for id, qty := range rolled {
				herbs = append(herbs, SplitHerbDrop(id, qty, autoLootHerbs)...)
			}
		default:
			if items == nil {
				items = make(map[int32]int32, len(rolled))
			}
			for id, qty := range rolled {
				items[id] += qty
			}
		}
	}
	return items, herbs
}
