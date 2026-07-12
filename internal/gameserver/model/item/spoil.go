package item

// SpoilPool holds one monster's spoil state across a single life: whether a
// player has marked it for spoil, and — once marked — the pool of items its
// spoil-kind drop categories roll into. A sweep skill drains the pool once;
// Reset clears everything back to the unspoiled state, as happens on
// respawn.
type SpoilPool struct {
	spoilerID int32
	items     map[int32]int32
}

// IsSpoiled reports whether a player has successfully marked the monster
// for spoil.
func (p *SpoilPool) IsSpoiled() bool {
	return p.spoilerID != 0
}

// IsSpoiler reports whether spoilerID is the player that marked the
// monster for spoil.
func (p *SpoilPool) IsSpoiler(spoilerID int32) bool {
	return p.spoilerID != 0 && p.spoilerID == spoilerID
}

// Mark records spoilerID as the player entitled to sweep this monster. The
// caller is responsible for only marking an unspoiled pool (IsSpoiled
// false); a spoil skill's success roll happens elsewhere.
func (p *SpoilPool) Mark(spoilerID int32) {
	p.spoilerID = spoilerID
}

// Add merges quantity into itemID's pooled amount, as rolled by a spoil
// drop category.
func (p *SpoilPool) Add(itemID, quantity int32) {
	if p.items == nil {
		p.items = make(map[int32]int32, 1)
	}
	p.items[itemID] += quantity
}

// Sweepable reports whether the pool holds anything left to harvest.
func (p *SpoilPool) Sweepable() bool {
	return len(p.items) > 0
}

// Sweep drains and returns the pooled items, leaving the pool empty. A
// second call returns nothing, matching the one-time harvest a sweep skill
// performs; the spoil marker itself is untouched (the monster stays
// spoiled, just with nothing left to sweep).
func (p *SpoilPool) Sweep() map[int32]int32 {
	items := p.items
	p.items = nil
	return items
}

// Reset clears the spoil marker and any pooled items.
func (p *SpoilPool) Reset() {
	p.spoilerID = 0
	p.items = nil
}
