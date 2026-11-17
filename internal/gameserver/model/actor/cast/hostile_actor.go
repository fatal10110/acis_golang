package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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

// ReuseRate reports the caster's reuse-delay multiplier for a magic or
// physical skill, its live mReuse or pReuse stat.
func (a HostileActor) ReuseRate(magic bool) float64 {
	if a.Hostile == nil {
		return 1
	}
	return a.Hostile.CalcStat(reuseStat(magic), 1)
}

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
// rate to def's raw cost, a rule shared by every creature type rather than
// overridden per Player/Npc.
// Unlike PlayerActor, there is no dance or song surcharge: no NPC skill list
// carries a dance or song skill (ids 264-277, 304-311, 349, 363-366), so the
// per-active-dance cost is never reached.
func (a HostileActor) MPInitialCost(def *modelskill.Definition) int {
	return a.scaleMP(def, def.MPInitialConsume)
}

func (a HostileActor) MPCost(def *modelskill.Definition) int {
	return a.scaleMP(def, def.MPConsume)
}

func (a HostileActor) scaleMP(def *modelskill.Definition, mp int) int {
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
		a.Hostile.ConsumeHP(float64(n))
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

// MagicMuted reports an active effect that blocks the NPC's magic skills.
func (a HostileActor) MagicMuted() bool {
	return a.Hostile != nil && a.Hostile.EffectList().IsAffected(effect.FlagMuted)
}

// PhysicalMuted reports an active effect that blocks the NPC's physical
// skills.
func (a HostileActor) PhysicalMuted() bool {
	return a.Hostile != nil && a.Hostile.EffectList().IsAffected(effect.FlagPhysicalMuted)
}

func (a HostileActor) SpiritshotCharged() bool {
	return a.Hostile != nil && a.Hostile.SpiritshotCharged()
}

func (a HostileActor) BlessedSpiritshotCharged() bool {
	return a.Hostile != nil && a.Hostile.BlessedSpiritshotCharged()
}
func (HostileActor) SkillMastery(*modelskill.Definition) bool { return false }
func (HostileActor) ItemCount(int) int                        { return 0 }
func (HostileActor) ConsumeItem(int, int) bool                { return false }

// HostileActor casters hold no cubics, ground signet, skill lock or charges.
func (HostileActor) ExitSignetGround()             {}
func (HostileActor) AllSkillsDisabled() bool       { return false }
func (HostileActor) EnableAllSkills()              {}
func (HostileActor) GroundTargetUnset() bool       { return false }
func (HostileActor) IncreaseCharges(int, int) bool { return false }
func (HostileActor) DecreaseCharges(int) bool      { return false }

// HeldItemTypeMask is the caster's weapon and shield item-type bits.
func (a HostileActor) HeldItemTypeMask() int32 {
	if a.Hostile == nil {
		return 0
	}
	return a.Hostile.HeldItemTypeMask()
}

// npcCaster gives a hostile NPC's casts the NPC finish timing.
func (HostileActor) npcCaster() {}
