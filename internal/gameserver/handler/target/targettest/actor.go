// Package targettest provides a neutral target.Actor for tests.
package targettest

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
)

// Actor supplies neutral values for every target.Actor, effect.Actor and
// attackable.Combatant method except the world placement ones: embed it next
// to world.Presence in a test double and override only what the test
// exercises. The neutral actor is not attackable, sees everything, and holds
// no social, corpse or NPC-kind facts.
type Actor struct {
	effecttest.Actor
}

func (Actor) AttackableBy(target.Actor) bool                                          { return false }
func (Actor) AttackableWithoutForceBy(target.Actor) bool                              { return false }
func (Actor) CanSeeTarget(target.Actor) bool                                          { return true }
func (Actor) GroundTarget() (x, y, z int)                                             { return 0, 0, 0 }
func (Actor) CanSeePoint(int, int, int) bool                                          { return true }
func (Actor) Folk() bool                                                              { return false }
func (Actor) FolkOrGuard() bool                                                       { return false }
func (Actor) MonsterKind() bool                                                       { return false }
func (Actor) Undead() bool                                                            { return false }
func (Actor) Holy() bool                                                              { return false }
func (Actor) Unlockable() bool                                                        { return false }
func (Actor) IsPet() bool                                                             { return false }
func (Actor) HasCorpse() bool                                                         { return false }
func (Actor) CorpseDeadline() (time.Time, bool)                                       { return time.Time{}, false }
func (Actor) CorpseTime() time.Duration                                               { return 0 }
func (Actor) Spoiled() bool                                                           { return false }
func (Actor) Seeded() bool                                                            { return false }
func (Actor) Summon() (target.Actor, bool)                                            { return nil, false }
func (Actor) CanCastOnPlayable(target.Actor, *modelskill.Definition, bool, bool) bool { return true }
func (Actor) OlympiadMode() bool                                                      { return false }
func (Actor) OlympiadStarted() bool                                                   { return false }
func (Actor) IsInParty() bool                                                         { return false }
func (Actor) PartyContains(target.Actor) bool                                         { return false }
func (Actor) IsInSameParty(target.Actor) bool                                         { return false }
func (Actor) IsInSameClan(target.Actor) bool                                          { return false }
func (Actor) IsInSameAlly(target.Actor) bool                                          { return false }
func (Actor) HasClan() bool                                                           { return false }
func (Actor) DuelID() int32                                                           { return 0 }
func (Actor) DuelTeam() int                                                           { return 0 }
func (Actor) MageClass() bool                                                         { return false }
func (Actor) ClanGroups() []string                                                    { return nil }
