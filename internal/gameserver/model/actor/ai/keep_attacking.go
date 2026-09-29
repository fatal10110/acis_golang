package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// canKeepAttacking reports whether a playable attacker swings again at target
// once a swing ends with nothing queued. Any non-playable target is kept. A
// playable target is kept only while its acting player has karma, while both
// sides stand inside a PvP zone (each its own membership), or when the
// attacker is a betrayed summon.
//
// A target in the same active Olympiad match or the same active duel is also
// kept; neither system exists yet (#216, #215), so that branch never applies.
func canKeepAttacking(attacker, target attackable.Combatant) bool {
	if target == nil {
		return false
	}
	if !target.Kind().Playable() {
		return true
	}
	acting := target
	if owner, ok := target.Owner(); ok && owner != nil {
		acting = owner
	}
	if acting.Karma() > 0 {
		return true
	}
	if inPvPZone(attacker) && inPvPZone(target) {
		return true
	}
	return betrayed(attacker)
}

// betrayed reports whether c is a summon turned on its owner.
func betrayed(c attackable.Combatant) bool {
	summon, ok := c.(interface{ Betrayed() bool })
	return ok && summon.Betrayed()
}
