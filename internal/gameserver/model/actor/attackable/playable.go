package attackable

// Playable is the state a player or summon exposes to the playable
// attackability rules. Every value is the playable's own: a summon's level
// and Blessing of Protection are its own, its karma is its owner's.
type Playable interface {
	Combatant
	ProtectionBlessing() bool
	PvPZoneMember
}

// PvPZoneMember is an actor that tracks whether it stands inside a PvP zone:
// a player or a summon. NPCs, doors, and other non-playables track no zone
// membership, so an assertion to it legitimately fails for them and they
// never count as standing inside a PvP zone.
type PvPZoneMember interface {
	InPvPZone() bool
}

// actingPlayer is the acting-player state the playable attackability rules
// read through a playable: the player itself, or a summon's owner.
type actingPlayer interface {
	OlympiadMode() bool
	OlympiadStarted() bool
	CursedWeaponEquipped() bool
}

// PlayableRefuses reports whether target, a playable, may not be attacked
// by attacker, another playable. Nothing is refused to a non-playable
// attacker. For a playable attacker, in order:
//
//   - refused while target's acting player is in an Olympiad match that has
//     not started;
//   - allowed while target stands inside a PvP zone;
//   - refused when either side carries Blessing of Protection and the other
//     side has karma and is 10 or more levels above it;
//   - refused when either side's acting player wields a cursed weapon and the
//     other side is level 20 or below.
func PlayableRefuses(target Playable, attacker Combatant) bool {
	if target == nil || attacker == nil || !attacker.Kind().Playable() {
		return false
	}
	other, ok := attacker.(Playable)
	if !ok {
		return false
	}
	targetPlayer, targetOK := actingPlayerOf(target)
	if targetOK && targetPlayer.OlympiadMode() && !targetPlayer.OlympiadStarted() {
		return true
	}
	if target.InPvPZone() {
		return false
	}
	if target.ProtectionBlessing() && other.Level()-target.Level() >= 10 && other.Karma() > 0 {
		return true
	}
	if other.ProtectionBlessing() && target.Level()-other.Level() >= 10 && target.Karma() > 0 {
		return true
	}
	if targetOK && targetPlayer.CursedWeaponEquipped() && other.Level() <= 20 {
		return true
	}
	attackerPlayer, ok := actingPlayerOf(other)
	return ok && attackerPlayer.CursedWeaponEquipped() && target.Level() <= 20
}

// actingPlayerOf returns the player acting through c: its owner for a
// summon, c itself otherwise.
func actingPlayerOf(c Combatant) (actingPlayer, bool) {
	if owner, ok := c.Owner(); ok && owner != nil {
		c = owner
	}
	player, ok := c.(actingPlayer)
	return player, ok
}
