package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// actingPlayer returns the creature c acts for: a summon's owner, or c
// itself.
func actingPlayer(c attackable.Combatant) attackable.Combatant {
	if owner, ok := c.Owner(); ok && owner != nil {
		return owner
	}
	return c
}

// canKeepAttacking reports whether a playable attacker swings again at target
// once a swing ends with nothing queued. Any non-playable target is kept. A
// playable target is kept only while its acting player has karma, while it
// fights the attacker's acting player in the same started Olympiad match or
// in the same duel, while both sides stand inside a PvP zone (each its own
// membership), or when the attacker is a betrayed summon.
func canKeepAttacking(attacker, target attackable.Combatant) bool {
	if target == nil {
		return false
	}
	if !target.Kind().Playable() {
		return true
	}
	acting := actingPlayer(target)
	if acting.Karma() > 0 {
		return true
	}
	if attackable.SameActiveOlympiadMatch(actingPlayer(attacker), acting) {
		return true
	}
	if a, ok := actingPlayer(attacker).(duel.Standing); ok {
		if t, ok := acting.(duel.Standing); ok && duel.SameActive(a, t) {
			return true
		}
	}
	if inPvPZone(attacker) && inPvPZone(target) {
		return true
	}
	return betrayed(attacker)
}

// betrayed reports whether c is a summon turned on its owner.
func betrayed(c attackable.Combatant) bool {
	summon, ok := c.(betrayer)
	return ok && summon.Betrayed()
}

// betrayer is a summon that tells whether it turned on its owner.
type betrayer interface {
	Betrayed() bool
}
