package npc

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

var _ skilltarget.Actor = (*Folk)(nil)

// Folk and FolkOrGuard mark the civilian NPC for target resolution: its
// aura skills reach nearby playables, and as a single offensive target it
// accepts only a CTRL-pressed damage skill. A control tower takes any
// offensive skill as a single target.
func (f *Folk) Folk() bool        { return true }
func (f *Folk) FolkOrGuard() bool { return !f.Tower() }

// AttackableBy reports whether caster may affect this NPC offensively: any
// other creature may while the NPC lives, with a forced attack, unless the
// NPC is a holy artifact; a control tower follows its own rule
// (towerAttackableBy).
func (f *Folk) AttackableBy(caster skilltarget.Actor) bool {
	if caster == nil || caster.ObjectID() == f.ObjectID() || f.AlikeDead() || f.HolyThing() {
		return false
	}
	return !f.Tower() || f.towerAttackableBy(caster)
}

// AttackableWithoutForceBy reports whether caster may attack this NPC with
// no forced attack: a control tower it may attack at all; never any other.
func (f *Folk) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	return f.Tower() && f.AttackableBy(caster)
}

// InPeaceZone reports whether the NPC's zones hold it in peace.
func (f *Folk) InPeaceZone() bool { return f.zones.has(zone.FlagPeace) }

// Undead reports the template's undead race.
func (f *Folk) Undead() bool { return f.Instance.Template.Race == RaceUndead }

// ClanGroups are the template's clan tags.
func (f *Folk) ClanGroups() []string { return f.Instance.Template.Clans }

// CanSeeTarget reports whether target is in the NPC's line of sight, the
// sight its casts are checked against.
func (f *Folk) CanSeeTarget(target skilltarget.Actor) bool { return f.canSee(target) }

// Holy reports a holy artifact, the target of a HOLY skill.
func (f *Folk) Holy() bool { return f.HolyThing() }

// A civilian NPC casts only at creatures: it holds no ground point, summon,
// party, clan, duel, Olympiad, spoil or seed state, and is not a monster,
// chest or pet.
func (f *Folk) CanSeePoint(int, int, int) bool             { return true }
func (f *Folk) EffectRangeInPeaceZone(_, _, _, _ int) bool { return false }
func (f *Folk) GroundTarget() (x, y, z int)                { return 0, 0, 0 }
func (f *Folk) MonsterKind() bool                          { return false }
func (f *Folk) Unlockable() bool                           { return false }
func (f *Folk) Box() bool                                  { return false }
func (f *Folk) Interacted() bool                           { return false }
func (f *Folk) ClaimInteraction() bool                     { return false }

// A civilian NPC keeps no aggro list: it holds no hate against anyone and
// records none.
func (f *Folk) AggroHate(attackable.Combatant) float64               { return 0 }
func (f *Folk) AddDamageHate(attackable.Combatant, float64, float64) {}

func (f *Folk) IsPet() bool                          { return false }
func (f *Folk) Spoiled() bool                        { return false }
func (f *Folk) Seeded() bool                         { return false }
func (f *Folk) Summon() (skilltarget.Actor, bool)    { return nil, false }
func (f *Folk) OlympiadMode() bool                   { return false }
func (f *Folk) OlympiadStarted() bool                { return false }
func (f *Folk) IsInParty() bool                      { return false }
func (f *Folk) PartyContains(skilltarget.Actor) bool { return false }
func (f *Folk) IsInSameParty(skilltarget.Actor) bool { return false }
func (f *Folk) IsInSameClan(skilltarget.Actor) bool  { return false }
func (f *Folk) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (f *Folk) HasClan() bool                        { return false }
func (f *Folk) DuelID() int32                        { return 0 }
func (f *Folk) DuelTeam() int                        { return 0 }
func (f *Folk) MageClass() bool                      { return false }
func (f *Folk) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool, bool) bool {
	return true
}

// A folk NPC is no chest, beast or manor monster, and holds no
// resurrection, siege or festival standing.
func (f *Folk) Chest() bool                              { return false }
func (f *Folk) FeedableBeast() bool                      { return false }
func (f *Folk) Seedable() bool                           { return false }
func (f *Folk) SpawnLocation() (location.Location, bool) { return location.Location{}, false }
func (f *Folk) ReviveOffer() (pending, forPet bool)      { return false, false }
func (f *Folk) InSiegeZone() bool                        { return false }
func (f *Folk) SiegeState() int32                        { return 0 }
func (f *Folk) FestivalParticipant() bool                { return false }

// A folk NPC keeps no attacker list.
func (f *Folk) RecordAttacker(attackable.Combatant) {}

// SkillLevels is empty: an NPC's chance procs are its effects' triggers
// only.
func (f *Folk) SkillLevels() player.SkillLevels { return nil }
