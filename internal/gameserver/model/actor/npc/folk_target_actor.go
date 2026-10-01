package npc

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Folk)(nil)

// Folk and FolkOrGuard mark the civilian NPC for target resolution: its
// aura skills reach nearby playables, and as a single offensive target it
// accepts only a CTRL-pressed damage skill.
func (f *Folk) Folk() bool        { return true }
func (f *Folk) FolkOrGuard() bool { return true }

// Dead reports false: no damage takes a civilian NPC below 1 HP.
func (f *Folk) Dead() bool { return false }

// AttackableBy reports whether caster may affect this NPC offensively: any
// other creature may, with a forced attack. AttackableWithoutForceBy is
// always false.
func (f *Folk) AttackableBy(caster skilltarget.Actor) bool {
	return caster != nil && caster.ObjectID() != f.ObjectID()
}
func (f *Folk) AttackableWithoutForceBy(skilltarget.Actor) bool { return false }

// InPeaceZone reports whether the NPC stands in a peace zone.
func (f *Folk) InPeaceZone() bool { return f.inPeace }

// Undead reports the template's undead race.
func (f *Folk) Undead() bool { return f.Instance.Template.Race == RaceUndead }

// ClanGroups are the template's clan tags.
func (f *Folk) ClanGroups() []string { return f.Instance.Template.Clans }

// CanSeeTarget reports whether target is in the NPC's line of sight, the
// sight its casts are checked against.
func (f *Folk) CanSeeTarget(target skilltarget.Actor) bool { return f.canSee(target) }

// A civilian NPC casts only at creatures: it holds no ground point, summon,
// party, clan, duel, Olympiad, corpse, spoil or seed state, and is not a
// monster, artifact, chest or pet.
func (f *Folk) CanSeePoint(int, int, int) bool             { return true }
func (f *Folk) EffectRangeInPeaceZone(_, _, _, _ int) bool { return false }
func (f *Folk) GroundTarget() (x, y, z int)                { return 0, 0, 0 }
func (f *Folk) MonsterKind() bool                          { return false }
func (f *Folk) Holy() bool                                 { return false }
func (f *Folk) Unlockable() bool                           { return false }
func (f *Folk) IsPet() bool                                { return false }
func (f *Folk) HasCorpse() bool                            { return false }
func (f *Folk) CorpseDeadline() (time.Time, bool)          { return time.Time{}, false }
func (f *Folk) CorpseTime() time.Duration                  { return 0 }
func (f *Folk) Spoiled() bool                              { return false }
func (f *Folk) Seeded() bool                               { return false }
func (f *Folk) Summon() (skilltarget.Actor, bool)          { return nil, false }
func (f *Folk) OlympiadMode() bool                         { return false }
func (f *Folk) OlympiadStarted() bool                      { return false }
func (f *Folk) IsInParty() bool                            { return false }
func (f *Folk) PartyContains(skilltarget.Actor) bool       { return false }
func (f *Folk) IsInSameParty(skilltarget.Actor) bool       { return false }
func (f *Folk) IsInSameClan(skilltarget.Actor) bool        { return false }
func (f *Folk) IsInSameAlly(skilltarget.Actor) bool        { return false }
func (f *Folk) HasClan() bool                              { return false }
func (f *Folk) DuelID() int32                              { return 0 }
func (f *Folk) DuelTeam() int                              { return 0 }
func (f *Folk) MageClass() bool                            { return false }
func (f *Folk) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}
