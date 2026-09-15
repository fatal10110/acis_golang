package effect

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// neutralActor supplies neutral values for every Actor and
// attackable.Combatant method except the world placement ones: embed it next
// to world.Presence in a test double and override only what the test
// exercises. The neutral actor has no effect list, no vitals, full heal
// effectiveness, and ignores every control and flight request.
type neutralActor struct {
	attackabletest.Combatant
}

func (neutralActor) EffectList() *List                          { return nil }
func (neutralActor) CancelVulnerability(string) float64         { return 1 }
func (neutralActor) UpdateAbnormalEffect()                      {}
func (neutralActor) StartAbnormalEffect(int)                    {}
func (neutralActor) StopAbnormalEffect(int)                     {}
func (neutralActor) StopEffects(Type)                           {}
func (neutralActor) StopSkillEffectsByID(modelskill.ID)         {}
func (neutralActor) AddChanceTrigger(*Effect)                   {}
func (neutralActor) RemoveChanceTrigger(*Effect)                {}
func (neutralActor) HP() float64                                { return 0 }
func (neutralActor) ReduceHPByDOT(float64, Actor, bool)         {}
func (neutralActor) MPValue() float64                           { return 0 }
func (neutralActor) ReduceMP(float64) float64                   { return 0 }
func (neutralActor) CanBeHealed() bool                          { return false }
func (neutralActor) AddHP(float64) float64                      { return 0 }
func (neutralActor) AddMP(float64) float64                      { return 0 }
func (neutralActor) HealProficiency() float64                   { return 0 }
func (neutralActor) HealEffectiveness() float64                 { return 100 }
func (neutralActor) RechargeMP(base float64) float64            { return base }
func (neutralActor) AbortAll(bool)                              {}
func (neutralActor) StopMove()                                  {}
func (neutralActor) TryToIdle()                                 {}
func (neutralActor) ClearTarget()                               {}
func (neutralActor) StopAttack()                                {}
func (neutralActor) SetImmobilized(bool) bool                   { return false }
func (neutralActor) SetInvul(bool) bool                         { return false }
func (neutralActor) Afraid() bool                               { return false }
func (neutralActor) FearImmune() bool                           { return false }
func (neutralActor) FleeFrom(Actor, int) bool                   { return false }
func (neutralActor) BluffExempt() bool                          { return false }
func (neutralActor) FlyTo(location.Location, modelskill.Flight) {}
func (neutralActor) SetXYZ(int, int, int)                       {}
func (neutralActor) BroadcastPosition()                         {}
func (neutralActor) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}
