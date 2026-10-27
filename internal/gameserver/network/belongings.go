package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// BelongingsConfig holds the collaborators of what characters own and carry:
// the item, shortcut, henna and recipe book rows, the henna templates, the
// items lying on the ground, and the tasks that report and persist item
// changes the server makes on its own.
type BelongingsConfig struct {
	Items       itemStore
	Shortcuts   shortcutStore
	Hennas      hennaStore
	HennaTable  *henna.Table
	RecipeBooks recipeBookStore
	GroundItems groundItemDropper
	// InventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	InventoryUpdates *task.InventoryUpdates
	// ItemInstances lazily persists item rows whose live state changed.
	ItemInstances *task.ItemInstances
}

// belongings is what characters own and carry: their item, shortcut, henna
// and recipe book rows, the henna templates, the items on the ground, and
// the tasks behind inventory changes the server makes on its own.
// NewGameClientLink builds it whole and nothing in it is replaced
// afterwards, so every client goroutine reads it without a lock.
type belongings struct {
	items       itemStore
	shortcuts   shortcutStore
	hennas      hennaStore
	hennaTable  *henna.Table
	recipeBooks recipeBookStore
	groundItems groundItemDropper
	// inventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	inventoryUpdates *task.InventoryUpdates
	// itemInstances lazily persists item rows whose live state changed,
	// so a mutation made outside a client request still reaches the
	// items table.
	itemInstances *task.ItemInstances
}

// newBelongings builds the belongings from cfg.
func newBelongings(cfg BelongingsConfig) belongings {
	return belongings{
		items:            cfg.Items,
		shortcuts:        cfg.Shortcuts,
		hennas:           cfg.Hennas,
		hennaTable:       cfg.HennaTable,
		recipeBooks:      cfg.RecipeBooks,
		groundItems:      cfg.GroundItems,
		inventoryUpdates: cfg.InventoryUpdates,
		itemInstances:    cfg.ItemInstances,
	}
}
