package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// SetKarma sets c's karma to karma, floored at 0. A change tells c its new
// total, refreshes its UserInfo and resends its relations; setting the
// karma it already has does nothing.
func (c *Character) SetKarma(karma int) {
	c.progressionMu.Lock()
	if c.KarmaPoints == karma {
		c.progressionMu.Unlock()
		return
	}
	c.KarmaPoints = max(0, karma)
	karma = c.KarmaPoints
	c.progressionMu.Unlock()

	c.notifyKarmaChanged(karma)
	c.UpdateUserInfo()
	c.BroadcastRelations()
}

// SetRecommendationsHave sets how many recommendations c holds, clamped to
// [0, 255].
func (c *Character) SetRecommendationsHave(have int) {
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	r.have = min(max(have, 0), maxRecommendationsHave)
}

// LiftDeathPenalty drops c's death-penalty level to 0, emitting
// DeathPenaltyChanged when there was one to lift.
func (c *Character) LiftDeathPenalty() {
	c.stateMu.Lock()
	old := c.deathPenaltyLevel
	if old <= 0 {
		c.stateMu.Unlock()
		return
	}
	c.deathPenaltyLevel = 0
	c.stateMu.Unlock()

	c.emit(event.DeathPenaltyChanged{Old: old, New: 0})
}
