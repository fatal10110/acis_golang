package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// SetExp moves this pet's experience to exp and its level, up or down, to
// the one exp reaches in its growth table. A level past petRealMaxLevel is
// refused: the pet keeps its level, though not its new experience. A level
// change brings the experience back inside the new level's span, applies the
// level's growth row and resyncs the collar's enchant level; a gain also
// restores vitals, publishes them and plays the level-up animation, as an
// earned level-up does. Unlike a kill reward, the owner is not told the
// experience moved, and no pet rate applies.
func (a *Actor) SetExp(exp int64) {
	if a == nil || !a.isPet {
		return
	}
	a.statusMu.Lock()
	if a.growth == nil || exp == a.exp {
		a.statusMu.Unlock()
		return
	}
	a.exp = exp
	// The level is found from the bottom of the table, a level with no row
	// starting at no experience.
	level := 0
	for l := 1; l <= petMaxLevel; l++ {
		if exp < a.expForLevelLocked(l) {
			break
		}
		level = l
	}
	oldLevel := a.level
	if level == oldLevel || level > petRealMaxLevel {
		a.statusMu.Unlock()
		return
	}
	a.level = level
	if a.exp >= a.expForLevelLocked(level+1) || a.expForLevelLocked(level) > a.exp {
		a.exp = a.expForLevelLocked(level)
	}
	a.refreshGrowthLocked()
	a.statusMu.Unlock()
	if level < oldLevel {
		a.SyncControlItemEnchant()
		return
	}
	a.resetVitals()
	a.SyncControlItemEnchant()
	a.BroadcastStatus()
	a.emit(event.SocialAction{ID: socialActionLevelUp})
}
