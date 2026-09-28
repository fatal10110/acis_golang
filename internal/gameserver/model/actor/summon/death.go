package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// die runs a's death sequence once drainHP has marked it dead; drainHP lets
// exactly one caller through. The summon stops moving, attacking and
// casting, drops its target, loses the effects that do not last through
// death, and republishes its status. Observers then see it die
// (event.Died) and it goes idle; killer gets its karma for the kill, and
// last the owner is told (event.DeathSettled).
//
// A dead pet stops eating and a dead servitor's lifetime stops (see TickPet
// and TickServitor). The corpse stays in the world: summon corpse decay and
// the pet's death penalty belong to the decay lifecycle (#2439). A Phoenix
// Blessing does not offer its revive yet (#2620).
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
