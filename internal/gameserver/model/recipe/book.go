package recipe

import (
	"slices"
	"sync"
)

// Book is one player's recipe book: a dwarven and a common page, each
// holding recipes keyed by recipe id. The zero value is an empty book ready
// for use. mu guards both pages, so any goroutine may read or write it.
type Book struct {
	mu      sync.Mutex
	dwarven bookPage
	common  bookPage
}

// bookPage is one page of a Book. It lists its recipes in the order the
// client receives them: grouped by recipe id modulo the page's table size,
// in insertion order within a group. That is the iteration order of the
// hash table the book is specified against, which starts at 16 buckets,
// doubles once it holds more than three quarters of its size, and also
// doubles, while under 64 buckets, when a ninth recipe lands in one
// bucket. The table never shrinks, not even on removal.
type bookPage struct {
	// entries is in insertion order.
	entries []Recipe
	buckets int
}

const (
	pageInitialBuckets = 16
	pageTreeifyBuckets = 64
	pageBucketCrowd    = 8
)

func (b *Book) page(dwarven bool) *bookPage {
	if dwarven {
		return &b.dwarven
	}
	return &b.common
}

// Recipes returns the dwarven or common page's recipes in client order.
func (b *Book) Recipes(dwarven bool) []Recipe {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.page(dwarven).ordered()
}

// Count returns how many recipes the dwarven or common page holds.
func (b *Book) Count(dwarven bool) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.page(dwarven).entries)
}

// Has reports whether either page holds recipe id.
func (b *Book) Has(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dwarven.index(id) >= 0 || b.common.index(id) >= 0
}

// HasOn reports whether the dwarven or common page holds recipe id.
func (b *Book) HasOn(id int, dwarven bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.page(dwarven).index(id) >= 0
}

// Put writes r on the page its Dwarven flag names. A recipe already on that
// page is replaced in place.
func (b *Book) Put(r Recipe) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.page(r.Dwarven)
	if i := p.index(r.ID); i >= 0 {
		p.entries[i] = r
		return
	}
	p.add(r)
}

// Remove takes recipe id off the dwarven page, or off the common page when
// the dwarven page does not hold it. It reports whether a page held it.
func (b *Book) Remove(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dwarven.remove(id) {
		return true
	}
	return b.common.remove(id)
}

func (p *bookPage) index(id int) int {
	return slices.IndexFunc(p.entries, func(r Recipe) bool { return r.ID == id })
}

func (p *bookPage) bucket(id int) int {
	return id & (p.buckets - 1)
}

func (p *bookPage) add(r Recipe) {
	if p.buckets == 0 {
		p.buckets = pageInitialBuckets
	}
	crowd := 0
	for _, held := range p.entries {
		if p.bucket(held.ID) == p.bucket(r.ID) {
			crowd++
		}
	}
	p.entries = append(p.entries, r)
	if crowd >= pageBucketCrowd && p.buckets < pageTreeifyBuckets {
		p.buckets *= 2
	}
	if len(p.entries) > p.buckets*3/4 {
		p.buckets *= 2
	}
}

func (p *bookPage) remove(id int) bool {
	i := p.index(id)
	if i < 0 {
		return false
	}
	p.entries = slices.Delete(p.entries, i, i+1)
	return true
}

func (p *bookPage) ordered() []Recipe {
	out := slices.Clone(p.entries)
	slices.SortStableFunc(out, func(a, b Recipe) int {
		return p.bucket(a.ID) - p.bucket(b.ID)
	})
	return out
}
