package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SetInBossZone records the live zone engine's current boss-zone
// membership (zone.FlagBoss).
func (c *Character) SetInBossZone(inside bool) {
	c.insideBossZone.Store(inside)
}

// InBossZone reports whether the character currently stands in a boss
// zone.
func (c *Character) InBossZone() bool {
	return c.insideBossZone.Load()
}

// Riding reports whether the character rides a strider.
func (c *Character) Riding() bool {
	return c.MountType() == MountTypeStrider
}

// Recall sends the character to dest, the town, castle or clan hall a
// recall skill names, resolved where the character stands.
func (c *Character) Recall(dest modelskill.RecallType) {
	c.emit(event.RecallRequested{Destination: dest})
}
