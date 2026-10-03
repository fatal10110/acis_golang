package creature

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// duellist is a player as a hit on someone else's creature reads its duel.
type duellist interface {
	DuelID() int32
	InterruptDuel()
}

// actingDuellist returns the player acting through attacker: attacker
// itself, or a summon's owner.
func actingDuellist(attacker attackable.Combatant) (duellist, bool) {
	if attacker == nil {
		return nil, false
	}
	if attacker.Kind() == actor.KindSummon {
		owner, ok := attacker.Owner()
		if !ok || owner == nil {
			return nil, false
		}
		attacker = owner
	}
	if attacker.Kind() != actor.KindPlayer {
		return nil, false
	}
	d, ok := attacker.(duellist)
	return d, ok
}

// InterruptDuelOnNPCHit interrupts the duel of the player acting through
// attacker, if it is in one: hitting an NPC, whatever the damage, takes a
// duellist out of its duel.
func InterruptDuelOnNPCHit(attacker attackable.Combatant) {
	if d, ok := actingDuellist(attacker); ok && d.DuelID() > 0 {
		d.InterruptDuel()
	}
}

// InterruptDuelOnSummonHit interrupts the duel of the player acting through
// attacker when it hits a summon whose owner, ownerDuelID's, is not in its
// duel: whatever the damage, a duellist may only hit the summons of its own
// duel.
func InterruptDuelOnSummonHit(attacker attackable.Combatant, ownerDuelID int32) {
	if d, ok := actingDuellist(attacker); ok && d.DuelID() != ownerDuelID {
		d.InterruptDuel()
	}
}
