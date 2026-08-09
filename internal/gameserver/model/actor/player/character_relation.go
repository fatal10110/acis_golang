package player

// SetRelationBroadcaster records the runtime hook that sends a self-view
// RelationChanged for an owned summon and broadcasts the updated relation
// to every nearby observer, mirroring Player.updatePvPFlag/setKarma's
// shared tail (`if (_summon != null) sendPacket(new RelationChanged(...))`
// followed by `broadcastRelationsChanges()`). UpdatePvPFlag and the karma
// award path call BroadcastRelations after a state change actually takes
// effect.
func (c *Character) SetRelationBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastRelations = broadcast
}

// BroadcastRelations fires the runtime relation-broadcast hook, if wired.
func (c *Character) BroadcastRelations() {
	c.stateMu.RLock()
	broadcast := c.broadcastRelations
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}
