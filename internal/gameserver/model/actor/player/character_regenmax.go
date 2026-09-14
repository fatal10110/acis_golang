package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// IsPlayer identifies this actor as a player rather than another playable.
func (*Character) IsPlayer() bool { return true }

// SendRegenMax updates this player's client-only heal-over-time regen gauge.
func (c *Character) SendRegenMax(count, period int32, hpRegen float64) {
	c.emit(event.RegenMax{Count: count, Period: period, HPRegen: hpRegen})
}
