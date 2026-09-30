package summon

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

var _ skilltarget.Actor = (*Actor)(nil)

// Summons hold no ground point, summon, party, clan, duel or Olympiad state
// of their own, and the NPC and door facts never apply to them. Their corpse
// facts are in death.go.
func (a *Actor) CanSeePoint(int, int, int) bool    { return true }
func (a *Actor) GroundTarget() (x, y, z int)       { return 0, 0, 0 }
func (a *Actor) Summon() (skilltarget.Actor, bool) { return nil, false }
func (a *Actor) OlympiadMode() bool                { return false }
func (a *Actor) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}
func (a *Actor) IsInParty() bool                      { return false }
func (a *Actor) PartyContains(skilltarget.Actor) bool { return false }
func (a *Actor) IsInSameParty(skilltarget.Actor) bool { return false }
func (a *Actor) IsInSameClan(skilltarget.Actor) bool  { return false }
func (a *Actor) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (a *Actor) HasClan() bool                        { return false }
func (a *Actor) DuelID() int32                        { return 0 }
func (a *Actor) DuelTeam() int                        { return 0 }
func (a *Actor) MageClass() bool                      { return false }
func (a *Actor) OlympiadStarted() bool                { return false }
func (a *Actor) ClanGroups() []string                 { return nil }
func (a *Actor) Folk() bool                           { return false }
func (a *Actor) FolkOrGuard() bool                    { return false }
func (a *Actor) MonsterKind() bool                    { return false }
func (a *Actor) Undead() bool                         { return false }
func (a *Actor) Holy() bool                           { return false }
func (a *Actor) Unlockable() bool                     { return false }
func (a *Actor) Spoiled() bool                        { return false }
func (a *Actor) Seeded() bool                         { return false }

// AttackableBy reports whether attacker may attack a: a living summon is
// attackable by anyone but itself, its owner included, unless the playable
// attackability rules refuse a playable attacker (attackable.PlayableRefuses).
func (a *Actor) AttackableBy(attacker skilltarget.Actor) bool {
	if a.Dead() || sameObject(attacker, a) {
		return false
	}
	other, ok := attacker.(attackable.Combatant)
	return !ok || !attackable.PlayableRefuses(a, other)
}

// ProtectionBlessing reports whether a carries a Blessing of Protection of
// its own.
func (a *Actor) ProtectionBlessing() bool {
	return a.effects.IsAffected(effect.FlagProtectionBlessing)
}

// AttackableWithoutForceBy reports whether caster may attack a without a
// forced attack: never by its owner or the owner's own summon, otherwise
// when a and caster both stand inside a PvP zone (each its own membership),
// or while the owner has karma or a PvP flag.
//
// The Olympiad, duel, arena, party, clan, alliance and siege-side rules are
// not applied: that state is not tracked yet, the same as for a player
// target.
func (a *Actor) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	if owner := a.currentOwner(); caster == nil || owner == nil || actingPlayerID(caster) == owner.ObjectID() {
		return false
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
