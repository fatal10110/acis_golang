package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// BroadcastRelations reports a PvP flag or karma change observers must see.
func (c *Character) BroadcastRelations() {
	c.emit(event.RelationChanged{})
}
