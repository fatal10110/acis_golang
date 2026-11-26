package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// lowHPRatio is the share of maximum HP below which an HP write raises
// tutorialLowHP.
const lowHPRatio = 0.3

// writeHPLocked is the one place c's current HP is written, except the HP
// clear of a character restored dead (MarkDead). It marks the write for
// lowHPNotice, which SettleRegen runs after vitalsMu is released. A writer
// that reports its own status reaches it after that report. AddHP, SetHP and
// ReduceCurrentHP leave it to their caller's status report (BroadcastStatus),
// so the event follows the StatusUpdate, as a reported HP write raises it.
// The caller holds vitalsMu.
func (c *Character) writeHPLocked(hp float64) {
	c.curHP = hp
	c.hpWritten.Store(true)
}

// lowHPNotice raises tutorialLowHP once per marked HP write that left c
// below lowHPRatio of its maximum HP. It runs after vitalsMu is released,
// right after the writer's own status report when it sends one.
func (c *Character) lowHPNotice() {
	if !c.hpWritten.Swap(false) || c.tutorial == nil {
		return
	}
	if res := c.ResourceValues(); res.CurrentHP/res.MaxHP < lowHPRatio {
		c.NotifyTutorial(TutorialClientEvent(tutorialLowHP))
	}
}

// NotifyRestoredHP raises tutorialLowHP when hp, the HP c's selection
// restored, is below lowHPRatio of c's maximum HP, dead or not. Restoring
// the saved HP measures it against the maximum the restored equipment and
// effects make, so it runs once both apply. Its caller keeps the tutorial's
// answer from the client; the journal writes stay.
func (c *Character) NotifyRestoredHP(hp float64) {
	if c.tutorial == nil {
		return
	}
	if maxHP := c.ResourceValues().MaxHP; min(hp, maxHP)/maxHP < lowHPRatio {
		c.NotifyTutorial(TutorialClientEvent(tutorialLowHP))
	}
}

// DieFromDamage runs Die for HP damage that emptied c's HP, then hands c's
// tutorial quest the death event. A competitor of an Olympiad match only
// falls (fallInOlympiad): no death sequence and no tutorial event. It
// reports whether this call killed c.
func (c *Character) DieFromDamage(killer attackable.Combatant) bool {
	if c.OlympiadMode() {
		return c.fallInOlympiad()
	}
	if !c.Die(killer) {
		return false
	}
	c.NotifyTutorial(TutorialClientEvent(tutorialDied))
	return true
}
