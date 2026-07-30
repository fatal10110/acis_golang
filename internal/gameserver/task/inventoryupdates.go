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
}

// InventoryUpdates batches pending inventory update packets.
//
// mu guards order and owners. Inventories keep their own update queue and
// weight state under their own lock. order tracks registration order
// (oldest first) so a tick with several newly-registered inventories — a
// give-to-pet touching both the player's and the pet's — sends them in a
// deterministic sequence, matching the reference manager's list-based
// visitation instead of Go map iteration order.
type InventoryUpdates struct {
	mu     sync.RWMutex
	order  []*itemcontainer.Inventory
	owners map[*itemcontainer.Inventory]InventoryUpdateOwner
}

// NewInventoryUpdates returns an empty inventory update task.
func NewInventoryUpdates() *InventoryUpdates {
	return &InventoryUpdates{owners: make(map[*itemcontainer.Inventory]InventoryUpdateOwner)}
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
	for _, entry := range entries {
		if !entry.inventory.HasUpdates() {
			u.remove(entry.inventory)
			continue
		}
		if !entry.owner.Visible() && !entry.owner.Teleporting() {
			u.remove(entry.inventory)
			continue
		}

		updates := entry.inventory.DrainUpdates()
		if len(updates) == 0 {
			u.remove(entry.inventory)
			continue
		}
		entry.owner.SendInventoryUpdate(updates)
		entry.inventory.UpdateWeight()
	}
}

func (u *InventoryUpdates) snapshot() []inventoryUpdateEntry {
	u.mu.RLock()
	defer u.mu.RUnlock()
	entries := make([]inventoryUpdateEntry, 0, len(u.order))
	for _, inv := range u.order {
		entries = append(entries, inventoryUpdateEntry{inventory: inv, owner: u.owners[inv]})
	}
	return entries
}

func (u *InventoryUpdates) remove(inv *itemcontainer.Inventory) {
	u.mu.Lock()
	delete(u.owners, inv)
	for i, other := range u.order {
		if other == inv {
			u.order = append(u.order[:i], u.order[i+1:]...)
			break
		}
	}
	u.mu.Unlock()
}
