package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Stunned reports whether an effect stuns the NPC.
func (f *Folk) Stunned() bool { return f.effects.IsAffected(effect.FlagStunned) }

// GeoPathFailCount reports how many path findings in a row failed the NPC;
// zero for one that cannot move.
func (f *Folk) GeoPathFailCount() int {
	if f.motion == nil {
		return 0
	}
	return f.motion.GeoPathFailCount()
}

// CanAttack reports whether f may swing at target now, as its attack gate
// decides; never for an f with nothing to swing with.
func (f *Folk) CanAttack(target attackable.Combatant) bool {
	a := f.fight.attack
	return a != nil && target != nil && a.CanAttack(target)
}

// RemoveAllAttackDesire zeroes f's hate of every creature of its aggro
// list, drops its attack desires and stops its movement. The creatures stay
// listed with their damage, and f is not sent back to peace.
func (f *Folk) RemoveAllAttackDesire() {
	for _, t := range f.fight.aggro.Snapshot() {
		f.fight.aggro.StopHate(t.Attacker)
	}
	f.cast.desires.RemoveKind(ai.IntentionAttack)
	f.stopMoving()
}
