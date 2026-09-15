package door

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Object)(nil)

// A door only answers attackability and unlocking; it never casts, moves,
// dies into a corpse or belongs to any social group, so every method below is
// the neutral answer.
func (o *Object) CanSeeTarget(skilltarget.Actor) bool            { return true }
func (o *Object) CanSeePoint(int, int, int) bool                 { return true }
func (o *Object) EffectRangeInPeaceZone(int, int, int, int) bool { return false }
func (o *Object) InPeaceZone() bool                              { return false }
func (o *Object) GroundTarget() (x, y, z int)                    { return 0, 0, 0 }
func (o *Object) Summon() (skilltarget.Actor, bool)              { return nil, false }
func (o *Object) Owner() (attackable.Combatant, bool)            { return nil, false }
func (o *Object) OlympiadMode() bool                             { return false }
func (o *Object) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}
func (o *Object) IsInParty() bool                      { return false }
func (o *Object) PartyContains(skilltarget.Actor) bool { return false }
func (o *Object) IsInSameParty(skilltarget.Actor) bool { return false }
func (o *Object) IsInSameClan(skilltarget.Actor) bool  { return false }
func (o *Object) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (o *Object) HasClan() bool                        { return false }
func (o *Object) DuelID() int32                        { return 0 }
func (o *Object) DuelTeam() int                        { return 0 }
func (o *Object) MageClass() bool                      { return false }
func (o *Object) OlympiadStarted() bool                { return false }
func (o *Object) ClanGroups() []string                 { return nil }
func (o *Object) Folk() bool                           { return false }
func (o *Object) FolkOrGuard() bool                    { return false }
func (o *Object) MonsterKind() bool                    { return false }
func (o *Object) Undead() bool                         { return false }
func (o *Object) Holy() bool                           { return false }
func (o *Object) IsPet() bool                          { return false }
func (o *Object) HasCorpse() bool                      { return false }
func (o *Object) CorpseDeadline() (time.Time, bool)    { return time.Time{}, false }
func (o *Object) CorpseTime() time.Duration            { return 0 }
func (o *Object) Spoiled() bool                        { return false }
func (o *Object) Seeded() bool                         { return false }
