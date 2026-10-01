package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// ServitorVanished sends this player's YOUR_SERVITOR_HAS_VANISHED system
// message, as an ERASE skill does.
func (c *Character) ServitorVanished() {
	c.emit(event.ServitorVanished{})
}
