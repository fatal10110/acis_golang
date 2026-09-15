// Package effecttest provides a neutral effect.Actor for tests.
package effecttest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Actor supplies neutral values for every effect.Actor and
// attackable.Combatant method except the world placement ones: embed it next
// to world.Presence in a test double and override only what the test
// exercises. The neutral actor has no effect list, no vitals, full heal
// effectiveness, and ignores every control and flight request.
type Actor struct {
	attackabletest.Combatant
}

func (Actor) EffectList() *effect.List                   { return nil }
func (Actor) CancelVulnerability(string) float64         { return 1 }
func (Actor) UpdateAbnormalEffect()                      {}
func (Actor) StartAbnormalEffect(int)                    {}
func (Actor) StopAbnormalEffect(int)                     {}
func (Actor) StopEffects(effect.Type)                    {}
func (Actor) StopSkillEffectsByID(modelskill.ID)         {}
func (Actor) AddChanceTrigger(*effect.Effect)            {}
func (Actor) RemoveChanceTrigger(*effect.Effect)         {}
func (Actor) HP() float64                                { return 0 }
func (Actor) ReduceHPByDOT(float64, effect.Actor, bool)  {}
func (Actor) MPValue() float64                           { return 0 }
func (Actor) ReduceMP(float64) float64                   { return 0 }
func (Actor) CanBeHealed() bool                          { return false }
func (Actor) AddHP(float64) float64                      { return 0 }
func (Actor) AddMP(float64) float64                      { return 0 }
func (Actor) HealProficiency() float64                   { return 0 }
func (Actor) HealEffectiveness() float64                 { return 100 }
func (Actor) RechargeMP(base float64) float64            { return base }
func (Actor) AbortAll(bool)                              {}
func (Actor) StopMove()                                  {}
func (Actor) TryToIdle()                                 {}
func (Actor) ClearTarget()                               {}
func (Actor) StopAttack()                                {}
func (Actor) SetImmobilized(bool) bool                   { return false }
func (Actor) SetInvul(bool) bool                         { return false }
func (Actor) Afraid() bool                               { return false }
func (Actor) FearImmune() bool                           { return false }
func (Actor) FleeFrom(effect.Actor, int) bool            { return false }
func (Actor) BluffExempt() bool                          { return false }
func (Actor) FlyTo(location.Location, modelskill.Flight) {}
func (Actor) SetXYZ(int, int, int)                       {}
func (Actor) BroadcastPosition()                         {}
func (Actor) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}
