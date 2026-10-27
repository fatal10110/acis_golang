package summon

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

var _ skilltarget.Actor = (*Actor)(nil)

// Summons hold no ground point, summon or Olympiad state of their own, and
// the NPC and door facts never apply to them. Their corpse facts are in
// death.go; their duel, party, clan and alliance standing is their owner's,
// below.
func (a *Actor) CanSeePoint(int, int, int) bool    { return true }
func (a *Actor) GroundTarget() (x, y, z int)       { return 0, 0, 0 }
func (a *Actor) Summon() (skilltarget.Actor, bool) { return nil, false }
func (a *Actor) OlympiadMode() bool                { return false }
func (a *Actor) MageClass() bool                   { return false }
func (a *Actor) OlympiadStarted() bool             { return false }
func (a *Actor) ClanGroups() []string              { return nil }
func (a *Actor) Folk() bool                        { return false }
func (a *Actor) FolkOrGuard() bool                 { return false }
func (a *Actor) MonsterKind() bool                 { return false }
func (a *Actor) Holy() bool                        { return false }
func (a *Actor) Unlockable() bool                  { return false }
func (a *Actor) Spoiled() bool                     { return false }
func (a *Actor) Seeded() bool                      { return false }

// Undead reports whether a is a servitor of the UNDEAD race; a pet never
// is.
func (a *Actor) Undead() bool { return a.undead }

// DuelID is the duel a's owner is in, 0 for none.
func (a *Actor) DuelID() int32 { return max(a.ownerDuelID(), 0) }

// DuelTeam is the team colour a's owner shows, none without an owner.
func (a *Actor) DuelTeam() int {
	if owner := a.currentOwner(); owner != nil {
		return owner.DuelTeam()
	}
	return 0
}

// social returns a's owner, whose social standing a reads as its own; ok
// is false without an owner.
func (a *Actor) social() (Owner, bool) {
	owner := a.currentOwner()
	return owner, owner != nil
}

// IsInParty reports whether a's owner is in a party.
func (a *Actor) IsInParty() bool {
	owner, ok := a.social()
	return ok && owner.IsInParty()
}

// PartyContains reports whether a's owner's party holds the player acting
// through other.
func (a *Actor) PartyContains(other skilltarget.Actor) bool { return a.IsInSameParty(other) }

// IsInSameParty reports whether the player acting through other is in a's
// owner's party.
func (a *Actor) IsInSameParty(other skilltarget.Actor) bool {
	owner, ok := a.social()
	return ok && owner.IsInSameParty(other)
}

// IsInSameClan reports whether the player acting through other belongs to
// a's owner's clan.
func (a *Actor) IsInSameClan(other skilltarget.Actor) bool {
	owner, ok := a.social()
	return ok && owner.IsInSameClan(other)
}

// IsInSameAlly reports whether the player acting through other belongs to
// a's owner's alliance.
func (a *Actor) IsInSameAlly(other skilltarget.Actor) bool {
	owner, ok := a.social()
	return ok && owner.IsInSameAlly(other)
}

// HasClan reports whether a's owner belongs to a clan.
func (a *Actor) HasClan() bool {
	owner, ok := a.social()
	return ok && owner.HasClan()
}

// CanCastOnPlayable judges a's own skill cast on target, a playable: an
// offensive one by the owner's social standing and a's own zones, a
// beneficial one as its owner would. a's own intention acts on target, so
// target is the main target. A summon with no owner's standing to read is
// refused nothing.
func (a *Actor) CanCastOnPlayable(target skilltarget.Actor, skill *modelskill.Definition, ctrl, offensive, _ bool) bool {
	owner, ok := a.social()
	if !ok {
		return true
	}
	if offensive {
		return owner.OffensiveCastAllowed(a, target, skill, ctrl, true)
	}
	return owner.CanCastOnPlayable(target, skill, ctrl, false, true)
}

// AttackableBy reports whether attacker may attack a: a living summon is
// attackable by anyone but itself, its owner included, unless the playable
// attackability rules refuse a playable attacker (attackable.PlayableRefuses).
// A siege guard may attack it unless its owner's clan owns or defends the
// siege in progress where a stands.
func (a *Actor) AttackableBy(attacker skilltarget.Actor) bool {
	if a.Dead() || sameObject(attacker, a) {
		return false
	}
	other, ok := attacker.(attackable.Combatant)
	if !ok {
		return true
	}
	if other.SiegeGuard() {
		owner, ok := a.currentOwner().(siegeDefender)
		if !ok {
			return true
		}
		x, y, z := a.Position()
		return !owner.DefendsSiegeAt(x, y, z)
	}
	return !attackable.PlayableRefuses(a, other)
}

// siegeDefender is an owner whose clan may own or defend a siege.
type siegeDefender interface {
	DefendsSiegeAt(x, y, z int) bool
}

// ProtectionBlessing reports whether a carries a Blessing of Protection of
// its own.
func (a *Actor) ProtectionBlessing() bool {
	return a.effects.IsAffected(effect.FlagProtectionBlessing)
}

// AttackableWithoutForceBy reports whether caster may attack a without a
// forced attack: never by its owner or the owner's own summon; otherwise as
// the owner's duel, party, command channel, clan and alliance rules decide
// with a's own arena membership; otherwise when a and caster both stand
// inside a PvP zone (each its own membership), or while the owner has karma
// or a PvP flag.
func (a *Actor) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	if owner := a.currentOwner(); caster == nil || owner == nil || actingPlayerID(caster) == owner.ObjectID() {
		return false
	}
	if owner, ok := a.social(); ok {
		if allowed, decided := owner.SocialWithoutForce(a, caster); decided {
			return allowed
		}
	}
	if a.InPvPZone() && inPvPZone(caster) {
		return true
	}
	return a.Karma() > 0 || a.PvPFlagState() != task.PvPFlagNone
}

// inPvPZone reports whether c stands inside a PvP zone; an actor without
// zone membership (an NPC or door) never does.
func inPvPZone(c skilltarget.Actor) bool {
	member, ok := c.(attackable.PvPZoneMember)
	return ok && member.InPvPZone()
}

// actingPlayerID is the object id of the player acting through c: c itself,
// or the owner of a summon.
func actingPlayerID(c skilltarget.Actor) int32 {
	if c.Kind() == actor.KindSummon {
		if owner, ok := c.Owner(); ok && owner != nil {
			return owner.ObjectID()
		}
	}
	return c.ObjectID()
}

// A summon is no chest, beast or manor monster, and its resurrection offer,
// siege state and festival standing are its owner's, read through the
// owner.
func (a *Actor) Chest() bool                              { return false }
func (a *Actor) Interacted() bool                         { return false }
func (a *Actor) FeedableBeast() bool                      { return false }
func (a *Actor) Seedable() bool                           { return false }
func (a *Actor) SpawnLocation() (location.Location, bool) { return location.Location{}, false }
func (a *Actor) ReviveOffer() (pending, forPet bool)      { return false, false }
func (a *Actor) SiegeState() int32                        { return 0 }
func (a *Actor) FestivalParticipant() bool                { return false }

// A summon lists no attackers and answers no skill aggro; its reaction to
// an attack is NotifyAttacked.
func (a *Actor) RecordAttacker(attackable.Combatant)                       {}
func (a *Actor) SkillAttacked(attackable.Combatant, modelskill.Definition) {}

// SkillLevels is empty: a summon's chance procs are its effects' triggers
// only.
func (a *Actor) SkillLevels() player.SkillLevels { return nil }
