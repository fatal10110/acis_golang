package inventory

import (
	"context"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// ClanWarehouses holds each clan's warehouse, restored from its rows the
// first time it is used and shared by every member's session and every
// server task from then on. mu guards byClan. Each entry's restoring
// serializes its read, so two users opening it at once read it once, and
// its mu guards the warehouse itself: a lookup of a restored warehouse
// never waits on a read. Item moves in and out of a warehouse are
// serialized by the item operation that names the clan id
// (task.ItemInstances.BeginOperation), not by any one member's queue. The
// zero value is ready to use.
type ClanWarehouses struct {
	mu     sync.Mutex
	byClan map[int32]*clanWarehouse
}

type clanWarehouse struct {
	restoring sync.Mutex
	mu        sync.Mutex
	wh        *itemcontainer.Container
}

func (e *clanWarehouse) warehouse() *itemcontainer.Container {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.wh
}

func (b *ClanWarehouses) entry(clanID int32) *clanWarehouse {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byClan == nil {
		b.byClan = map[int32]*clanWarehouse{}
	}
	e := b.byClan[clanID]
	if e == nil {
		e = &clanWarehouse{}
		b.byClan[clanID] = e
	}
	return e
}

// Restore returns the warehouse of the clan clanID, built by load under
// ctx when no one has used it yet. Concurrent callers for one clan load it
// once. A failed load is not kept: the next use loads again.
func (b *ClanWarehouses) Restore(ctx context.Context, clanID int32, load func(context.Context) (*itemcontainer.Container, error)) (*itemcontainer.Container, error) {
	e := b.entry(clanID)
	e.restoring.Lock()
	defer e.restoring.Unlock()
	if wh := e.warehouse(); wh != nil {
		return wh, nil
	}
	wh, err := load(ctx)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.wh = wh
	e.mu.Unlock()
	return wh, nil
}

// Forget drops the warehouse of the clan clanID, dissolved.
func (b *ClanWarehouses) Forget(clanID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.byClan, clanID)
}

// Holds reports whether store is still the warehouse of the clan clanID:
// a dissolution forgets it inside the item operation that destroys its
// items.
func (b *ClanWarehouses) Holds(clanID int32, store Store) bool {
	wh := b.Restored(clanID)
	return wh != nil && Store(wh) == store
}

// Restored returns the warehouse of the clan clanID when it is in memory,
// nil when no one has used it yet or its clan was dissolved. It never
// loads it.
func (b *ClanWarehouses) Restored(clanID int32) *itemcontainer.Container {
	b.mu.Lock()
	e := b.byClan[clanID]
	b.mu.Unlock()
	if e == nil {
		return nil
	}
	return e.warehouse()
}
