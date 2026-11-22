package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// InBossZone reports whether the character currently stands in a boss
// zone.
func (c *Character) InBossZone() bool {
	z := c.zonesNow()
	return z != nil && z.InBossZone()
}

// Riding reports whether the character rides a strider.
func (c *Character) Riding() bool {
	return c.MountType() == MountTypeStrider
}

// Recall sends the character to dest, the town, castle or clan hall a
// recall skill names, resolved where the character stands. It may be called
// from any queue: the recall runs on the character's own queue once the
// calling task is done.
func (c *Character) Recall(dest modelskill.RecallType) {
	c.emit(event.RecallRequested{Destination: dest})
}

// RecallOnQueue is Recall for a caller running on the character's own
// queue: the recall runs inside the call.
func (c *Character) RecallOnQueue(dest modelskill.RecallType) {
	c.emit(event.RecallRequested{Destination: dest, OwnQueue: true})
}
