package summon

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Revive stands a dead summon back up and reports whether it did; a call on
// a living summon is a no-op. A Phoenix Blessing restores full HP and MP and
// is used up; otherwise HP comes back to the configured respawn share of max
// HP. Observers see the new status and the revive.
//
// A pet also closes its owner's pending resurrection offer, cancels its
// corpse's decay and goes idle; its feeding and its regeneration resume, as
// they skip only a dead pet, and a baby pet starts healing its owner again.
// A servitor keeps its decay: at the deadline it leaves the world, alive
// (Decay).
//
// A corpse its owner left behind revives too, on the queue of its own the
// corpse moved to (AdoptCorpseQueue), and stays where it stood up, since its
// owner is not in the world to follow. A pet is handed to its owner's next
// session alive (RelinkOwner). A servitor stays nobody's: it has given up
// its owner's summon slot (LeaveWithOwner), is never handed back, and its
// owner's next session sees it as any other summon (ShownAsOwnedBy). It
// leaves the world at its decay, or, once a non-player's resurrection has
// cancelled that (ResurrectOutright), only when it dies again and decays.
func (a *Actor) Revive() bool {
	return a.revive()
}

// ReviveRestoringExp is a resurrection's revive at power percent: a dead pet
// first gets back that share of the experience its last death penalty took,
// rounded half up, then revives as Revive does. A servitor has no
// experience of its own, so power changes nothing for it.
func (a *Actor) ReviveRestoringExp(power float64) bool {
	if !a.Dead() {
		return false
	}
	if a.isPet {
		a.restoreExp(power)
	}
	return a.revive()
}

// CancelDecay drops a's pending corpse decay, if any.
func (a *Actor) CancelDecay() {
	a.emit(event.DecayCanceled{})
}

// ResurrectOutright is a non-player caster's resurrection at power percent.
// It runs on a's own queue, as every other command on a does. The summon
// drops its pending decay first, so a revived servitor stays in the world,
// as does a living servitor a player revived earlier; a dead one then
// revives as ReviveRestoringExp does. That includes a corpse whose owner is
// offline (Revive).
func (a *Actor) ResurrectOutright(power float64) {
	// A closed queue refuses the job, which is what the job would do too:
	// a summon's queue closes only once the summon has left the world. (A
	// queue the summon moved off closes too, but Post hands the job on to
	// the one it moved to.)
	a.Post(func() { a.resurrectOutright(power) })
}

func (a *Actor) resurrectOutright(power float64) bool {
	a.CancelDecay()
	return a.ReviveRestoringExp(power)
}

// restoreExp gives a pet back percent of the experience its last death
// penalty took, once: the recorded pre-death experience is then cleared.
// Climbing back over a level threshold raises the level, which refreshes
// the owner's pet window and the collar's enchant, and plays the level-up
// animation; nothing else is sent and HP and MP stay as they are, since the
// pet is still dead.
func (a *Actor) restoreExp(percent float64) {
	a.statusMu.Lock()
	if a.expBeforeDeath <= 0 {
		a.statusMu.Unlock()
		return
	}
	if restored := petRestoredExp(a.expBeforeDeath, a.exp, percent); a.exp+restored >= 0 {
		a.exp += restored
	}
	a.expBeforeDeath = 0
	leveled := a.refreshGrowthLocked()
	a.statusMu.Unlock()
	if leveled {
		a.SyncControlItemEnchant()
		a.emit(event.SocialAction{ID: socialActionLevelUp})
	}
}

// petRestoredExp is the experience a resurrection at percent gives back to a
// pet that had before and now has current.
func petRestoredExp(before, current int64, percent float64) int64 {
	// Rounds half up.
	return int64(math.Floor(float64(before-current)*percent/100 + 0.5))
}

// revive is Revive's state transition and its follow-ups. HP (and MP for a
// Phoenix Blessing) is set in the same step that clears the dead flag, so no
// hit lands on a living summon at 0 HP. Max HP counts in whole points, as the
// client sees it.
func (a *Actor) revive() bool {
	blessed := a.EffectList().IsAffected(effect.FlagPhoenixBlessing)
	maxHP, maxMP := math.Floor(a.MaxHPValue()), math.Floor(a.MaxMPValue())
	a.vitals.mu.Lock()
	if !a.dead || a.decayed {
		a.vitals.mu.Unlock()
		return false
	}
	a.dead = false
	a.corpseDeadline = time.Time{}
	if blessed {
		a.vitals.hp, a.vitals.mp = maxHP, maxMP
	} else {
		a.vitals.hp = min(maxHP*a.respawnRestoreHP, maxHP)
	}
	a.vitals.mu.Unlock()

	if owner := a.currentOwner(); a.isPet && owner != nil {
		owner.ClearReviveOffer()
	}
	if blessed {
		a.EffectList().StopByType(effect.TypePhoenixBless)
		a.UpdateAbnormalEffect()
	}
	a.UpdateStatus()
	a.emit(event.Revived{})
	if a.isPet {
		a.CancelDecay()
		a.TryToIdle()
		a.startBabyHeal()
	}
	return true
}
