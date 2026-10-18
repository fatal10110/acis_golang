package script

// WarehouseSize returns how many items the player's private warehouse
// holds.
func (p *Player) WarehouseSize() int { return p.character().WarehouseSize() }

// OpenWarehouseWithdraw releases the player's client and opens the
// withdrawal window of its private warehouse, which becomes the one it
// deposits into and withdraws from. Call it from a hook on the player's
// own queue: a talk or a bypass event.
func (p *Player) OpenWarehouseWithdraw() { p.character().OpenWarehouseWithdraw() }
