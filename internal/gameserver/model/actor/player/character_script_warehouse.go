package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// AttachWarehouse sets wh as c's private warehouse. Call it once, before c
// is shared.
func (c *Character) AttachWarehouse(wh *itemcontainer.Container) {
	c.warehouse = wh
}

// WarehouseSize returns how many items c's private warehouse holds, 0
// before one is attached.
func (c *Character) WarehouseSize() int {
	if c.warehouse == nil {
		return 0
	}
	return c.warehouse.Size()
}

// OpenWarehouseWithdraw makes c's private warehouse the one c deposits into
// and withdraws from, and opens its withdrawal window on c's screen. Call
// it on c's own queue.
func (c *Character) OpenWarehouseWithdraw() {
	c.emit(event.WarehouseWithdrawOpened{})
}
