package task

import (
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// InventoryUpdateTick is the fixed cadence for batched inventory updates.
const InventoryUpdateTick = 333 * time.Millisecond

// InventoryUpdateOwner is the narrow playable surface the inventory update
// task needs.
type InventoryUpdateOwner interface {
	Visible() bool
	Teleporting() bool
	SendInventoryUpdate([]itemcontainer.Update)
}

type inventoryUpdateEntry struct {
	inventory *itemcontainer.Inventory
	owner     InventoryUpdateOwner
	epoch     uint64
}

// InventoryUpdates batches pending inventory update packets.
//
// mu guards order, owners and epoch. Inventories keep their own update
// queue and weight state under their own lock. order tracks registration
// order (oldest first) so a tick with several newly-registered inventories
// — a give-to-pet touching both the player's and the pet's — sends them in
// a deterministic sequence, matching the reference manager's list-based
// visitation instead of Go map iteration order.
//
// epoch closes a check-then-remove race Tick would otherwise have: Add
// bumps an inventory's epoch, and Tick only drops an entry found empty or
// gated out if its epoch is still what the tick observed, so a mutation
// that re-registers the inventory while its tick is being processed keeps
// it — rather than the entry being deleted out from under the queued
// update, orphaning it until some later, unrelated mutation happens to
// re-register the same inventory.
type InventoryUpdates struct {
	mu     sync.RWMutex
	order  []*itemcontainer.Inventory
	owners map[*itemcontainer.Inventory]InventoryUpdateOwner
	epoch  map[*itemcontainer.Inventory]uint64
}

// NewInventoryUpdates returns an empty inventory update task.
func NewInventoryUpdates() *InventoryUpdates {
	return &InventoryUpdates{
		owners: make(map[*itemcontainer.Inventory]InventoryUpdateOwner),
		epoch:  make(map[*itemcontainer.Inventory]uint64),
	}
}

// Start launches the fixed inventory update task.
func (u *InventoryUpdates) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(InventoryUpdateTick, u.Tick, log)
}

// Add registers inv for the next inventory update tick.
func (u *InventoryUpdates) Add(inv *itemcontainer.Inventory, owner InventoryUpdateOwner) {
	if inv == nil || owner == nil {
		return
	}
	u.mu.Lock()
	if _, exists := u.owners[inv]; !exists {
		u.order = append(u.order, inv)
	}
	u.owners[inv] = owner
	u.epoch[inv]++
	u.mu.Unlock()
}

// Contains reports whether inv is currently waiting for a tick.
func (u *InventoryUpdates) Contains(inv *itemcontainer.Inventory) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	_, ok := u.owners[inv]
	return ok
}

// Tick sends one queued inventory update to every visible or teleporting
// owner, then refreshes the inventory weight.
func (u *InventoryUpdates) Tick() {
	entries := u.snapshot()
	done := make(map[*itemcontainer.Inventory]uint64, len(entries))
	for _, entry := range entries {
		if !entry.inventory.HasUpdates() {
			done[entry.inventory] = entry.epoch
			continue
		}
		if !entry.owner.Visible() && !entry.owner.Teleporting() {
			done[entry.inventory] = entry.epoch
			continue
		}

		updates := entry.inventory.DrainUpdates()
		if len(updates) == 0 {
			done[entry.inventory] = entry.epoch
			continue
		}
		entry.owner.SendInventoryUpdate(updates)
		entry.inventory.UpdateWeight()
	}
	if len(done) > 0 {
		u.removeUnchanged(done)
	}
}

func (u *InventoryUpdates) snapshot() []inventoryUpdateEntry {
	u.mu.RLock()
	defer u.mu.RUnlock()
	entries := make([]inventoryUpdateEntry, 0, len(u.order))
	for _, inv := range u.order {
		entries = append(entries, inventoryUpdateEntry{inventory: inv, owner: u.owners[inv], epoch: u.epoch[inv]})
	}
	return entries
}

// removeUnchanged drops every inventory in seen whose epoch hasn't moved
// since the tick observed it, rebuilding order once rather than scanning it
// per removal.
func (u *InventoryUpdates) removeUnchanged(seen map[*itemcontainer.Inventory]uint64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := len(u.order)
	kept := u.order[:0]
	for _, inv := range u.order {
		wantEpoch, marked := seen[inv]
		if marked && u.epoch[inv] == wantEpoch {
			delete(u.owners, inv)
			delete(u.epoch, inv)
			continue
		}
		kept = append(kept, inv)
	}
	// The in-place filter above leaves dropped *Inventory pointers live in
	// the backing array past the new length; clear them so a logged-out
	// player's inventory (and every item.Instance it holds) doesn't stay
	// reachable through order's capacity until it happens to grow back to
	// this size.
	clear(u.order[len(kept):n])
	u.order = kept
}
