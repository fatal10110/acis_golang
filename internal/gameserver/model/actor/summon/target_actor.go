package summon

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Actor)(nil)

// Summons hold no ground point, summon, party, clan, duel or Olympiad state
// of their own, and the NPC, corpse and door facts never apply to them.
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
func (a *Actor) HasCorpse() bool                      { return false }
func (a *Actor) CorpseDeadline() (time.Time, bool)    { return time.Time{}, false }
func (a *Actor) CorpseTime() time.Duration            { return 0 }
func (a *Actor) Spoiled() bool                        { return false }
func (a *Actor) Seeded() bool                         { return false }

// AttackableBy reports whether caster may affect this summon offensively: any
// living summon may be attacked.
func (a *Actor) AttackableBy(skilltarget.Actor) bool { return !a.AlikeDead() }

// AttackableWithoutForceBy reports false: the owner's karma and PvP flag are
// not consulted yet, so affecting a summon always needs a forced attack.
func (a *Actor) AttackableWithoutForceBy(skilltarget.Actor) bool { return false }
