package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HostileActor adapts a live hostile NPC to the timed cast controller,
// mirroring SummonActor's role for a summon.
type HostileActor struct{ Hostile *npc.Hostile }

func (a HostileActor) AttackSpeed(magic bool) int {
	if a.Hostile == nil {
		return 1
	}
	if magic {
		return a.Hostile.MagicAttackSpeed()
	}
	return a.Hostile.AttackSpeed()
}
func (HostileActor) ReuseRate(bool) float64 { return 1 }
func (a HostileActor) MP() int {
	if a.Hostile == nil {
		return 0
	}
	return int(a.Hostile.MPValue())
}
func (a HostileActor) HP() int {
	if a.Hostile == nil {
		return 0
	}
	return int(a.Hostile.HP())
}

// MPInitialCost and MPCost apply the caster's magical/physical MP-consume
// rate to def's raw cost, matching CreatureStatus.getMpInitialConsume/
// getMpConsume (CreatureStatus.java:698-737), which are defined on the base
// class shared by every creature type rather than overridden per Player/Npc.
// Unlike PlayerActor, there is no Dance/song surcharge: Bard/Warsmith dance
// skills are never cast by a hostile NPC caster.
func (a HostileActor) MPInitialCost(def modelskill.Definition) int {
	return a.scaleMP(def, def.MPInitialConsume)
}
func (a HostileActor) MPCost(def modelskill.Definition) int {
	return a.scaleMP(def, def.MPConsume)
}
func (a HostileActor) scaleMP(def modelskill.Definition, mp int) int {
	if a.Hostile == nil {
		return mp
	}
	rate := stat.PhysicalMpConsumeRate
	if def.Magic {
		rate = stat.MagicalMpConsumeRate
	}
	return int(a.Hostile.CalcStat(rate, float64(mp)))
}
func (a HostileActor) ReduceMP(n int) {
	if a.Hostile != nil {
		a.Hostile.ReduceMP(float64(n))
	}
}
func (a HostileActor) ReduceHP(n int) {
	if a.Hostile != nil {
		a.Hostile.ReduceHP(float64(n), nil, modelskill.Definition{})
	}
}
func (a HostileActor) SkillDisabled(k int32) bool {
	return a.Hostile != nil && a.Hostile.SkillDisabled(k)
}
func (a HostileActor) DisableSkill(k int32, d time.Duration) {
	if a.Hostile != nil {
		a.Hostile.DisableSkill(k, d)
	}
}
func (a HostileActor) AddSkillReuse(r modelskill.Ref, k int32, d time.Duration) {
	if a.Hostile != nil {
		a.Hostile.AddSkillReuse(r, k, d)
	}
}
func (HostileActor) MagicMuted() bool    { return false }
func (HostileActor) PhysicalMuted() bool { return false }
func (a HostileActor) SpiritshotCharged() bool {
	return a.Hostile != nil && a.Hostile.SpiritshotCharged()
}
func (a HostileActor) BlessedSpiritshotCharged() bool {
	return a.Hostile != nil && a.Hostile.BlessedSpiritshotCharged()
}
func (HostileActor) SkillMastery(modelskill.Definition) bool { return false }
func (HostileActor) ItemCount(int) int                       { return 0 }
func (HostileActor) ConsumeItem(int, int) bool               { return false }
