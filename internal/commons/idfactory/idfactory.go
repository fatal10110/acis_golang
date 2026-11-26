package idfactory

import (
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// FirstObjectID and LastObjectID bound the range of ids Allocator hands out.
const (
	FirstObjectID = 0x10000000
	LastObjectID  = 0x7FFFFFFF
)

// ErrIDSpaceExhausted is returned by NextID when every id in range is in use.
var ErrIDSpaceExhausted = errors.New("idfactory: id space exhausted")

// Allocator hands out unique object ids, reusing ids released back to it.
//
// An id released mid-session only becomes available again once allocation
// naturally reaches it (ids are handed out in increasing order and the
// search cursor never moves backward) or the Allocator is rebuilt via New,
// which reclaims every id its scan no longer reports. This trades
// perfect same-session reuse for O(1) amortized allocation.
//
// mu guards used and next.
type Allocator struct {
	mu   sync.Mutex
	used map[int32]struct{}
	next int32

	first, last int32 // id range; always FirstObjectID/LastObjectID outside tests
	log         zerolog.Logger
}

// New returns an Allocator seeded with every object id scan reports as
// already stored, ready to hand out ids that don't collide with existing
// rows. scan calls mark once per stored id; its error fails the build
// rather than leaving a partial id set.
func New(log zerolog.Logger, scan func(mark func(id int64)) error) (*Allocator, error) {
	a := &Allocator{
		used:  make(map[int32]struct{}),
		first: FirstObjectID,
		last:  LastObjectID,
		log:   log,
	}
	if err := scan(a.markUsed); err != nil {
		return nil, err
	}

	a.next = a.first
	log.Info().Int("used_object_ids", len(a.used)).Msg("idfactory: initialized")
	return a, nil
}

// markUsed records a stored id as in use; ids below the range never came
// from this allocator and are skipped.
func (a *Allocator) markUsed(id int64) {
	if id < int64(a.first) {
		a.log.Warn().Int64("object_id", id).Int32("minimum_id", a.first).Msg("idfactory: skipping object id below minimum")
		return
	}
	a.used[int32(id)] = struct{}{}
}

// NextID returns the next available object id and marks it used, or
// ErrIDSpaceExhausted if every id in range is already in use.
func (a *Allocator) NextID() (int32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id, err := a.nextFreeFrom(a.next)
	if err != nil {
		return 0, err
	}
	a.used[id] = struct{}{}
	a.next = id + 1
	return id, nil
}

// ReleaseID returns id to the pool so a later NextID call can hand it out
// again. Ids below FirstObjectID never came from this allocator; releasing
// one is logged and ignored rather than corrupting allocator state.
func (a *Allocator) ReleaseID(id int32) {
	if id < a.first {
		a.log.Warn().Int32("object_id", id).Int32("minimum_id", a.first).Msg("idfactory: ignored invalid object id release")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, id)
}

// nextFreeFrom returns the first id >= from that isn't marked used, or
// ErrIDSpaceExhausted if none remains up to last. Callers hold mu.
func (a *Allocator) nextFreeFrom(from int32) (int32, error) {
	for id := from; id <= a.last; id++ {
		if _, used := a.used[id]; !used {
			return id, nil
		}
	}
	return 0, fmt.Errorf("%w: range [%d, %d]", ErrIDSpaceExhausted, a.first, a.last)
}
