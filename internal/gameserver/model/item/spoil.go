package item

import "sync"

// SpoilPool holds one monster's spoil state across a single life: whether a
// player has marked it for spoil, and — once marked — the pool of items its
// spoil-kind drop categories roll into. A sweep skill drains the pool once;
// Reset clears everything back to the unspoiled state, as happens on
// respawn.
//
// Players spoil and sweep from their own queues while the killer's queue
// fills the pool, so every method takes mu.
type SpoilPool struct {
	mu        sync.Mutex
	spoilerID int32
	items     map[int32]int32
}

// IsSpoiled reports whether a player has successfully marked the monster
// for spoil.
func (p *SpoilPool) IsSpoiled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.spoilerID != 0
}

// IsSpoiler reports whether spoilerID is the player that marked the
// monster for spoil.
func (p *SpoilPool) IsSpoiler(spoilerID int32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.spoilerID != 0 && p.spoilerID == spoilerID
}

// Mark records spoilerID as the player entitled to sweep this monster,
// unless another player already marked it. It reports whether spoilerID
// took the mark; a spoil skill's success roll happens elsewhere.
func (p *SpoilPool) Mark(spoilerID int32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.spoilerID != 0 {
		return false
	}
	p.spoilerID = spoilerID
	return true
}

// Add merges quantity into itemID's pooled amount, as rolled by a spoil
// drop category.
func (p *SpoilPool) Add(itemID, quantity int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.items == nil {
		p.items = make(map[int32]int32, 1)
	}
	p.items[itemID] += quantity
}

// Sweepable reports whether the pool holds anything left to harvest.
func (p *SpoilPool) Sweepable() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items) > 0
}

// Sweep drains and returns the pooled items, clearing the spoil state. A
// second call returns nothing.
func (p *SpoilPool) Sweep() map[int32]int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	items := p.items
	p.spoilerID = 0
	p.items = nil
	return items
}

// Reset clears the spoil marker and any pooled items.
func (p *SpoilPool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.spoilerID = 0
	p.items = nil
}
