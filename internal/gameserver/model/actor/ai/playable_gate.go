package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// gatePlayer is the acting-player state the playable attack gate reads.
type gatePlayer interface {
	attackable.Combatant
	ProtectionBlessing() bool
	CursedWeaponEquipped() bool
}

// refusesPlayableTarget reports whether a playable acting for attacker's
// player must refuse target as a new attack intention, answered with
// TARGET_IS_INCORRECT. It applies to playable targets only, between the
// acting players of both sides:
//
//   - outside a PvP zone (the target's own membership), a karma player 10 or
//     more levels above a Blessing of Protection holder may not attack it,
//     nor may a blessed player attack a karma player 10 or more levels
//     above it;
//   - a player level 20 or below and a cursed-weapon holder may not attack
//     each other.
func refusesPlayableTarget(attacker, target attackable.Combatant) bool {
	if attacker == nil || target == nil || !target.Kind().Playable() {
		return false
	}
	actorPlayer, ok := actingGatePlayer(attacker)
	if !ok {
		return false
	}
	targetPlayer, ok := actingGatePlayer(target)
	if !ok {
		return false
	}
	if !inPvPZone(target) {
		if targetPlayer.ProtectionBlessing() && actorPlayer.Level()-targetPlayer.Level() >= 10 && actorPlayer.Karma() > 0 {
			return true
		}
		if actorPlayer.ProtectionBlessing() && targetPlayer.Level()-actorPlayer.Level() >= 10 && targetPlayer.Karma() > 0 {
			return true
		}
	}
	if targetPlayer.CursedWeaponEquipped() && actorPlayer.Level() <= 20 {
		return true
	}
	return actorPlayer.CursedWeaponEquipped() && targetPlayer.Level() <= 20
}

// actingGatePlayer returns the player acting through c: its owner for a
// summon, c itself otherwise.
func actingGatePlayer(c attackable.Combatant) (gatePlayer, bool) {
	if owner, ok := c.Owner(); ok && owner != nil {
		c = owner
	}
	player, ok := c.(gatePlayer)
	return player, ok
}

// inPvPZone reports whether c stands inside a PvP zone; a combatant without
// zone membership never does.
func inPvPZone(c attackable.Combatant) bool {
	member, ok := c.(interface{ InPvPZone() bool })
	return ok && member.InPvPZone()
}
