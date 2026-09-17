package target

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
)

// neutralActor supplies neutral values for every target.Actor, effect.Actor and
// attackable.Combatant method except the world placement ones: embed it next
// to world.Presence in a test double and override only what the test
// exercises. The neutral actor is not attackable, sees everything, and holds
// no social, corpse or NPC-kind facts.
type neutralActor struct {
	effecttest.Actor
}

func (neutralActor) AttackableBy(Actor) bool                                          { return false }
func (neutralActor) AttackableWithoutForceBy(Actor) bool                              { return false }
func (neutralActor) CanSeeTarget(Actor) bool                                          { return true }
func (neutralActor) GroundTarget() (x, y, z int)                                      { return 0, 0, 0 }
func (neutralActor) CanSeePoint(int, int, int) bool                                   { return true }
func (neutralActor) Folk() bool                                                       { return false }
func (neutralActor) FolkOrGuard() bool                                                { return false }
func (neutralActor) MonsterKind() bool                                                { return false }
func (neutralActor) Undead() bool                                                     { return false }
func (neutralActor) Holy() bool                                                       { return false }
func (neutralActor) Unlockable() bool                                                 { return false }
func (neutralActor) IsPet() bool                                                      { return false }
func (neutralActor) HasCorpse() bool                                                  { return false }
func (neutralActor) CorpseDeadline() (time.Time, bool)                                { return time.Time{}, false }
func (neutralActor) CorpseTime() time.Duration                                        { return 0 }
func (neutralActor) Spoiled() bool                                                    { return false }
func (neutralActor) Seeded() bool                                                     { return false }
func (neutralActor) Summon() (Actor, bool)                                            { return nil, false }
func (neutralActor) CanCastOnPlayable(Actor, *modelskill.Definition, bool, bool) bool { return true }
func (neutralActor) OlympiadMode() bool                                               { return false }
func (neutralActor) OlympiadStarted() bool                                            { return false }
func (neutralActor) IsInParty() bool                                                  { return false }
func (neutralActor) PartyContains(Actor) bool                                         { return false }
func (neutralActor) IsInSameParty(Actor) bool                                         { return false }
func (neutralActor) IsInSameClan(Actor) bool                                          { return false }
func (neutralActor) IsInSameAlly(Actor) bool                                          { return false }
func (neutralActor) HasClan() bool                                                    { return false }
func (neutralActor) DuelID() int32                                                    { return 0 }
func (neutralActor) DuelTeam() int                                                    { return 0 }
func (neutralActor) MageClass() bool                                                  { return false }
func (neutralActor) ClanGroups() []string                                             { return nil }
