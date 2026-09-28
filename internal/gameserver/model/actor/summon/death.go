package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
// (event.Died) and it goes idle; killer gets its karma for the kill, and
// last the owner is told (event.DeathSettled).
//
// A dead pet stops eating and a dead servitor's lifetime stops (see TickPet
// and TickServitor). The owner's side then schedules the corpse's decay
// (DecayDelay, Decay). The pet's death penalty is not applied yet (#2439). A
// Phoenix Blessing does not offer its revive yet (#2620).
func (a *Actor) die(killer attackable.Combatant) {
	if a.brain != nil {
		a.brain.AbortAll()
	}
	a.SetTarget(nil)
	a.EffectList().StopOnDeath()
	a.UpdateStatus()
	a.emit(event.Died{})
	a.idle()
	if a.owner != nil {
		a.owner.AwardSummonKillKarma(killer)
	}
	a.emit(event.DeathSettled{})
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
// summon is left alone. A pet's corpse takes the pet with it: after it has
// left the world its owner loses the collar and the pet's saved state
// (event.PetCorpseDecayed). The respawn hook is never used: summons do not
// respawn.
func (a *Actor) Decay(state *world.State, _ func()) bool {
	if !a.Dead() || !a.OwnerStillLinked() {
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
