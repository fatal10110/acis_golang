package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// FolkActor adapts a civilian NPC to the timed cast controller, as
// HostileActor does a hostile one.
type FolkActor struct{ Folk *npc.Folk }

func (a FolkActor) AttackSpeed(magic bool) int {
	if magic {
		return a.Folk.MagicAttackSpeed()
	}
	return a.Folk.AttackSpeed()
}
func (FolkActor) ReuseRate(bool) float64 { return 1 }
func (a FolkActor) MP() int              { return int(a.Folk.MPValue()) }
func (a FolkActor) HP() int              { return int(a.Folk.HP()) }

// MPInitialCost and MPCost apply the caster's magical or physical MP
// consume rate to def's raw cost.
func (a FolkActor) MPInitialCost(def modelskill.Definition) int {
	return a.scaleMP(def, def.MPInitialConsume)
}

func (a FolkActor) MPCost(def modelskill.Definition) int {
	return a.scaleMP(def, def.MPConsume)
}

func (a FolkActor) scaleMP(def modelskill.Definition, mp int) int {
	rate := stat.PhysicalMpConsumeRate
	if def.Magic {
		rate = stat.MagicalMpConsumeRate
	}
	return int(a.Folk.CalcStat(rate, float64(mp)))
}

func (a FolkActor) ReduceMP(n int) { a.Folk.ReduceMP(float64(n)) }
func (a FolkActor) ReduceHP(n int) { a.Folk.ConsumeHP(float64(n)) }

func (a FolkActor) SkillDisabled(k int32) bool { return a.Folk.SkillDisabled(k) }
func (a FolkActor) DisableSkill(k int32, d time.Duration) {
	a.Folk.DisableSkill(k, d)
}

func (a FolkActor) AddSkillReuse(r modelskill.Ref, k int32, d time.Duration) {
	a.Folk.AddSkillReuse(r, k, d)
}

// MagicMuted and PhysicalMuted report an active effect that blocks the
// NPC's magic or physical skills.
func (a FolkActor) MagicMuted() bool { return a.Folk.EffectList().IsAffected(effect.FlagMuted) }

func (a FolkActor) PhysicalMuted() bool {
	return a.Folk.EffectList().IsAffected(effect.FlagPhysicalMuted)
}

// A civilian NPC charges no shots, has no skill mastery, carries no items,
// cubics, ground signet, skill lock or charges.
func (FolkActor) SpiritshotCharged() bool                 { return false }
func (FolkActor) BlessedSpiritshotCharged() bool          { return false }
func (FolkActor) SkillMastery(modelskill.Definition) bool { return false }
func (FolkActor) ItemCount(int) int                       { return 0 }
func (FolkActor) ConsumeItem(int, int) bool               { return false }
func (FolkActor) ExitSignetGround()                       {}
func (FolkActor) AllSkillsDisabled() bool                 { return false }
func (FolkActor) EnableAllSkills()                        {}
func (FolkActor) GroundTargetUnset() bool                 { return false }
func (FolkActor) IncreaseCharges(int, int) bool           { return false }
func (FolkActor) DecreaseCharges(int) bool                { return false }

// HeldItemTypeMask is the NPC's weapon and shield item-type bits.
func (a FolkActor) HeldItemTypeMask() int32 { return a.Folk.HeldItemTypeMask() }

var (
	_ npc.FolkCastAI = (*AIController)(nil)
	_ AICaster       = (*npc.Folk)(nil)
	_ SkillCaster    = (*npc.Folk)(nil)
	_ Actor          = FolkActor{}
)
