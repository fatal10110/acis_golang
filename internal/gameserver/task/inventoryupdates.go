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
// mu guards entries. Inventories keep their own update queue and weight
// state under their own lock.
type InventoryUpdates struct {
	mu      sync.RWMutex
	entries map[*itemcontainer.Inventory]InventoryUpdateOwner
}

// NewInventoryUpdates returns an empty inventory update task.
func NewInventoryUpdates() *InventoryUpdates {
	return &InventoryUpdates{entries: make(map[*itemcontainer.Inventory]InventoryUpdateOwner)}
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
	u.entries[inv] = owner
	u.mu.Unlock()
}

// Contains reports whether inv is currently waiting for a tick.
func (u *InventoryUpdates) Contains(inv *itemcontainer.Inventory) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	_, ok := u.entries[inv]
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
	entries := make([]inventoryUpdateEntry, 0, len(u.entries))
	for inv, owner := range u.entries {
		entries = append(entries, inventoryUpdateEntry{inventory: inv, owner: owner})
	}
	return entries
}

func (u *InventoryUpdates) remove(inv *itemcontainer.Inventory) {
	u.mu.Lock()
	delete(u.entries, inv)
	u.mu.Unlock()
}
