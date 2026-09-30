package item

import (
	"cmp"
	"slices"
	"sync"
)

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
	items     []SpoilItem // one entry per item id, in the order first added
}

// SpoilItem is one pooled item id and its amount.
type SpoilItem struct {
	ItemID int32
	Count  int32
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
	for i := range p.items {
		if p.items[i].ItemID == itemID {
			p.items[i].Count += quantity
			return
		}
	}
	p.items = append(p.items, SpoilItem{ItemID: itemID, Count: quantity})
}

// Sweepable reports whether the pool holds anything left to harvest.
func (p *SpoilPool) Sweepable() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items) > 0
}

// Sweep drains and returns the pooled items, clearing the spoil state. A
// second call returns nothing.
//
// The items come in hash-bucket order, the order a sweep has always named
// them in: by the bucket an item id falls in within a table of 16 buckets,
// doubled while the pool fills more than three quarters of it, and in the
// order they were added within one bucket.
func (p *SpoilPool) Sweep() []SpoilItem {
	p.mu.Lock()
	items := p.items
	p.spoilerID = 0
	p.items = nil
	p.mu.Unlock()

	buckets := uint32(16)
	for uint32(len(items)) > buckets/4*3 {
		buckets *= 2
	}
	bucket := func(id int32) uint32 {
		h := uint32(id)
		return (h ^ h>>16) & (buckets - 1)
	}
	slices.SortStableFunc(items, func(a, b SpoilItem) int {
		return cmp.Compare(bucket(a.ItemID), bucket(b.ItemID))
	})
	return items
}

// Reset clears the spoil marker and any pooled items.
func (p *SpoilPool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.spoilerID = 0
	p.items = nil
}
