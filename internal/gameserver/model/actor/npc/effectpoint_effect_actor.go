package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var _ effect.Actor = (*EffectPoint)(nil)

// An EffectPoint only hosts its own signet effect: it has no vitals, stance,
// abnormal state or client icons, so every effect.Actor method below is the
// neutral value an effect reading it must tolerate.

func (ep *EffectPoint) Level() int                                 { return 0 }
func (ep *EffectPoint) CharacterName() string                      { return "" }
func (ep *EffectPoint) RaidRelated() bool                          { return false }
func (ep *EffectPoint) CancelVulnerability(string) float64         { return 1 }
func (ep *EffectPoint) UpdateAbnormalEffect()                      {}
func (ep *EffectPoint) StartAbnormalEffect(int)                    {}
func (ep *EffectPoint) StopAbnormalEffect(int)                     {}
func (ep *EffectPoint) StopEffects(effect.Type)                    {}
func (ep *EffectPoint) StopSkillEffectsByID(modelskill.ID)         {}
func (ep *EffectPoint) AddChanceTrigger(*effect.Effect)            {}
func (ep *EffectPoint) RemoveChanceTrigger(*effect.Effect)         {}
func (ep *EffectPoint) HP() float64                                { return 0 }
func (ep *EffectPoint) ReduceHPByDOT(float64, effect.Actor, bool)  {}
func (ep *EffectPoint) MPValue() float64                           { return 0 }
func (ep *EffectPoint) ReduceMP(float64) float64                   { return 0 }
func (ep *EffectPoint) CanBeHealed() bool                          { return false }
func (ep *EffectPoint) AddHP(float64) float64                      { return 0 }
func (ep *EffectPoint) AddMP(float64) float64                      { return 0 }
func (ep *EffectPoint) HealProficiency() float64                   { return 0 }
func (ep *EffectPoint) HealEffectiveness() float64                 { return 100 }
func (ep *EffectPoint) RechargeMP(base float64) float64            { return base }
func (ep *EffectPoint) AbortAll(bool)                              {}
func (ep *EffectPoint) StopMove()                                  {}
func (ep *EffectPoint) TryToIdle()                                 {}
func (ep *EffectPoint) ClearTarget()                               {}
func (ep *EffectPoint) StopAttack()                                {}
func (ep *EffectPoint) SetImmobilized(bool) bool                   { return false }
func (ep *EffectPoint) SetInvul(bool) bool                         { return false }
func (ep *EffectPoint) Afraid() bool                               { return false }
func (ep *EffectPoint) FearImmune() bool                           { return false }
func (ep *EffectPoint) FleeFrom(effect.Actor, int) bool            { return false }
func (ep *EffectPoint) BluffExempt() bool                          { return false }
func (ep *EffectPoint) FlyTo(location.Location, modelskill.Flight) {}
func (ep *EffectPoint) SetXYZ(int, int, int)                       {}
func (ep *EffectPoint) BroadcastPosition()                         {}
func (ep *EffectPoint) UpdateEffectIcons()                         {}
func (ep *EffectPoint) NotifyEffectWornOff(modelskill.ID, int)     {}
func (ep *EffectPoint) NotifyEffectDisappeared(modelskill.ID, int) {}
func (ep *EffectPoint) NotifyEffectAborted(modelskill.ID, int)     {}
func (ep *EffectPoint) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}
