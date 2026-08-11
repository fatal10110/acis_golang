package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonActor adapts a live summon to the timed cast controller.
type SummonActor struct{ Summon *summon.Actor }

func (a SummonActor) AttackSpeed(magic bool) int {
	if a.Summon == nil {
		return 1
	}
	if magic {
		return int(a.Summon.MAtkSpd())
	}
	return int(a.Summon.PhysicalAttackSpeed())
}
func (SummonActor) ReuseRate(bool) float64 { return 1 }
func (a SummonActor) MP() int {
	if a.Summon == nil {
		return 0
	}
	return int(a.Summon.MPValue())
}
func (a SummonActor) HP() int {
	if a.Summon == nil {
		return 0
	}
	return int(a.Summon.HP())
}
func (SummonActor) MPInitialCost(d modelskill.Definition) int { return d.MPInitialConsume }
func (SummonActor) MPCost(d modelskill.Definition) int        { return d.MPConsume }
func (a SummonActor) ReduceMP(n int) {
	if a.Summon != nil {
		a.Summon.ReduceMP(float64(n))
	}
}
func (a SummonActor) ReduceHP(n int) {
	if a.Summon != nil {
		a.Summon.ReduceHP(float64(n), nil, modelskill.Definition{})
	}
}
func (a SummonActor) SkillDisabled(k int32) bool { return a.Summon != nil && a.Summon.SkillDisabled(k) }
func (a SummonActor) DisableSkill(k int32, d time.Duration) {
	if a.Summon != nil {
		a.Summon.DisableSkill(k, d)
	}
}
func (a SummonActor) AddSkillReuse(r modelskill.Ref, k int32, d time.Duration) {
	if a.Summon != nil {
		a.Summon.AddSkillReuse(r, k, d)
	}
}
func (SummonActor) MagicMuted() bool          { return false }
func (SummonActor) PhysicalMuted() bool       { return false }
func (a SummonActor) SpiritshotCharged() bool { return a.Summon != nil && a.Summon.SpiritshotCharged() }
func (a SummonActor) BlessedSpiritshotCharged() bool {
	return a.Summon != nil && a.Summon.BlessedSpiritshotCharged()
}
func (SummonActor) SkillMastery(modelskill.Definition) bool { return false }
func (SummonActor) ItemCount(int) int                       { return 0 }
func (SummonActor) ConsumeItem(int, int) bool               { return false }
