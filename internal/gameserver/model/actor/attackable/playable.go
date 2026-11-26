package attackable

// Playable is the state a player or summon exposes to the playable
// attackability rules. Every value is the playable's own: a summon's level
// and Blessing of Protection are its own, its karma is its owner's.
type Playable interface {
	Combatant
	ProtectionBlessing() bool
	PvPZoneMember
}

// ProtectionBlessingLevelGap is the level lead from which a player with karma
// and a Blessing of Protection holder below it may not fight each other.
const ProtectionBlessingLevelGap = 10

// CursedWeaponNewbieLevel is the highest level at which a player and a
// cursed-weapon holder may not fight each other.
const CursedWeaponNewbieLevel = 20

// BlessingSide is one side of the Blessing of Protection rule.
type BlessingSide interface {
	ProtectionBlessing() bool
	Level() int
	Karma() int
}

// BlessingShields reports whether holder's Blessing of Protection keeps it
// and other apart: holder carries it while other has karma and leads it by
// ProtectionBlessingLevelGap levels or more.
func BlessingShields(holder, other BlessingSide) bool {
	return holder.ProtectionBlessing() && other.Level()-holder.Level() >= ProtectionBlessingLevelGap && other.Karma() > 0
}

// CursedWeaponNewbie reports whether level keeps a player and a
// cursed-weapon holder apart.
func CursedWeaponNewbie(level int) bool {
	return level <= CursedWeaponNewbieLevel
}

// PvPZoneMember is an actor that tracks whether it stands inside a PvP zone:
// a player or a summon. NPCs, doors, and other non-playables track no zone
// membership, so an assertion to it legitimately fails for them and they
// never count as standing inside a PvP zone.
type PvPZoneMember interface {
	InPvPZone() bool
}

// ArenaMember is a playable whose PvP and siege zone membership tell
// whether it stands in an arena.
type ArenaMember interface {
	PvPZoneMember
	InSiegeZone() bool
}

// InArena reports whether a stands in an arena: inside a PvP zone that is
// not a siege zone. An actor without zone membership never does.
func InArena(a any) bool {
	m, ok := a.(ArenaMember)
	return ok && m.InPvPZone() && !m.InSiegeZone()
}

// OlympiadCompetitor is a player's place in an Olympiad match.
type OlympiadCompetitor interface {
	OlympiadStarted() bool
	OlympiadGameID() int
}

// SameActiveOlympiadMatch reports whether a and b, the players acting
// through two playables, fight each other in the same started Olympiad
// match.
func SameActiveOlympiadMatch(a, b any) bool {
	x, ok := a.(OlympiadCompetitor)
	if !ok {
		return false
	}
	y, ok := b.(OlympiadCompetitor)
	return ok && x.OlympiadStarted() && y.OlympiadStarted() && x.OlympiadGameID() == y.OlympiadGameID()
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
	if BlessingShields(target, other) || BlessingShields(other, target) {
		return true
	}
	if targetOK && targetPlayer.CursedWeaponEquipped() && CursedWeaponNewbie(other.Level()) {
		return true
	}
	attackerPlayer, ok := actingPlayerOf(other)
	return ok && attackerPlayer.CursedWeaponEquipped() && CursedWeaponNewbie(target.Level())
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
