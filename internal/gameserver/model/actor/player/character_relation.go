package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// BroadcastRelations fires the runtime relation-broadcast hook, if wired.
func (c *Character) BroadcastRelations() {
	c.emit(event.RelationChanged{})
}
