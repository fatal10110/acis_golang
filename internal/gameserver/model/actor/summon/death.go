package summon

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var _ task.SummonDecayActor = (*Actor)(nil)

// petCorpseTime is how long a dead pet waits for a revive before its corpse
// decays and the pet is lost.
const petCorpseTime = 1200 * time.Second

// die runs a's death sequence once drainHP has marked it dead; drainHP lets
// exactly one caller through. The summon stops moving, attacking and
// casting, drops its target, loses the effects that do not last through
// death, and republishes its status. Observers then see it die
// (event.Died) and it goes idle; killer gets its karma for the kill. A
// summon whose Phoenix Blessing survived the death offers its owner its
// resurrection, and last the owner is told (event.DeathSettled).
//
// A dead pet stops eating and a dead servitor's lifetime stops (see TickPet
// and TickServitor). The owner's side then schedules the corpse's decay
// (DecayDelay, Decay), and last a pet pays its death penalty.
func (a *Actor) die(killer attackable.Combatant) {
	if a.brain != nil {
		a.brain.AbortAll()
	}
	a.SetTarget(nil)
	for range a.EffectList().StopOnDeath() {
		a.UpdateAbnormalEffect()
	}
	a.UpdateStatus()
	a.emit(event.Died{})
	a.idle()
	if a.owner != nil {
		a.owner.AwardSummonKillKarma(killer)
		if a.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
			a.owner.OfferSummonRevive()
		}
	}
	a.emit(event.DeathSettled{})
	// A pet killed inside a PvP zone keeps its experience, unless that zone
	// is a siege battlefield.
	if a.isPet && a.owner != nil && (!a.InPvPZone() || a.InSiegeZone()) {
		a.applyDeathPenalty()
	}
}

// applyDeathPenalty takes a dead pet's death penalty off its experience:
// (6.5 - 0.07 * level) percent of the experience its current level spans.
// The experience it had is kept for a resurrection to give back (see
// ReviveRestoringExp), even when the loss is skipped: a loss that would
// take the experience below zero is not taken. Dropping
// under the current level's threshold takes the level down with it, which
// refreshes the owner's pet window and the collar's enchant; nothing else is
// sent until the pet's next status refresh.
//
// Its duel exemption is not applied: duels are not ported (#215).
func (a *Actor) applyDeathPenalty() {
	a.statusMu.Lock()
	lost := petDeathPenalty(a.level, a.expForLevelLocked(a.level), a.expForLevelLocked(a.level+1))
	a.expBeforeDeath = a.exp
	if a.exp-lost < 0 {
		a.statusMu.Unlock()
		return
	}
	a.exp -= lost
	leveled := a.lowerLevelToExpLocked()
	a.statusMu.Unlock()
	if leveled {
		a.SyncControlItemEnchant()
	}
}

// petDeathPenalty is the experience a pet of level loses on death, given the
// experience its level and the next one start at.
func petDeathPenalty(level int, levelExp, nextLevelExp int64) int64 {
	percentLost := -0.07*float64(level) + 6.5
	// Rounds half up, as the reference's Math.round does.
	return int64(math.Floor(float64(nextLevelExp-levelExp)*percentLost/100 + 0.5))
}

// expForLevelLocked is the experience level starts at in a's growth table,
// or 0 for a level the table has no row for. statusMu must be held.
func (a *Actor) expForLevelLocked(level int) int64 {
	if a.growth == nil {
		return 0
	}
	return a.growth.Levels[level].MaxExp
}

// lowerLevelToExpLocked drops a's level until its experience reaches the
// level's threshold, applies that level's growth row, and reports whether the
// level changed. statusMu must be held.
func (a *Actor) lowerLevelToExpLocked() bool {
	if a.growth == nil {
		return false
	}
	level := a.level
	for level > 1 && a.exp < a.expForLevelLocked(level) {
		level--
	}
	if level == a.level {
		return false
	}
	a.level = level
	a.refreshGrowthLocked()
	return true
}

// DecayDelay is how long a's corpse stays in the world before it decays: 20
// minutes for a pet, its npc template's corpse time for a servitor.
func (a *Actor) DecayDelay() time.Duration {
	if a.isPet {
		return petCorpseTime
	}
	return a.corpseTime
}

// Decay removes a's corpse once its decay deadline has passed, and reports
// whether this call removed it. A corpse that is no longer its owner's
// summon is left alone; one its owner left behind still decays (see
// OwnerStillLinked). A pet's corpse takes the pet with it: after it has left
// the world its owner loses the collar and the pet's saved state
// (event.PetCorpseDecayed), whether or not the owner is online. The respawn
// hook is never used: summons do not respawn.
//
// A revived pet cancels its decay, and one revived after its decay fell due
// is left alone here; a pet this decay has claimed can no longer be revived
// (claimCorpse). A servitor a player revived keeps its decay (see
// ReviveRestoringExp) and leaves the world at its deadline, alive.
func (a *Actor) Decay(state *world.State, _ func()) bool {
	if !a.OwnerStillLinked() || (a.isPet && !a.claimCorpse()) {
		return false
	}
	if !a.despawn(state) {
		return false
	}
	if a.isPet {
		a.emit(event.PetCorpseDecayed{})
	}
	return true
}

// claimCorpse marks a dead pet's corpse as decaying and reports whether this
// call did; a living pet, or one already claimed, is not. A revive checks
// the mark under the same lock, so a pet is either revived or decays, never
// both.
func (a *Actor) claimCorpse() bool {
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if !a.dead || a.decayed {
		return false
	}
	a.decayed = true
	return true
}

// SetCorpseDeadline records when a's corpse decays: the deadline its decay
// entry was scheduled with. Corpse skills measure the corpse's age from it.
func (a *Actor) SetCorpseDeadline(deadline time.Time) {
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.dead {
		a.corpseDeadline = deadline
	}
}

// HasCorpse reports whether a lies dead with a pending decay.
func (a *Actor) HasCorpse() bool {
	_, ok := a.CorpseDeadline()
	return ok
}

// CorpseDeadline returns when a's corpse decays, if it lies dead with one
// pending.
func (a *Actor) CorpseDeadline() (time.Time, bool) {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	if !a.dead || a.corpseDeadline.IsZero() {
		return time.Time{}, false
	}
	return a.corpseDeadline, true
}

// CorpseTime is a's npc template corpse time: how long a servitor's corpse
// lasts. Corpse skills treat a corpse past half of it as too old.
func (a *Actor) CorpseTime() time.Duration { return a.corpseTime }
