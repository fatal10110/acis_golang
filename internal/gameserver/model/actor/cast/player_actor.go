package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// PlayerActor adapts a live player character to the cast controller's actor
// contract.
type PlayerActor struct {
	Character *player.Character
}

func (a PlayerActor) AttackSpeed(magic bool) int {
	if a.Character == nil {
		return 1
	}
	if magic {
		return a.Character.MagicAttackSpeed()
	}
	return a.Character.AttackSpeed()
}

func (PlayerActor) ReuseRate(bool) float64 { return 1 }

func (a PlayerActor) MP() int {
	if a.Character == nil {
		return 0
	}
	return a.Character.CurrentMP()
}

func (a PlayerActor) HP() int {
	if a.Character == nil {
		return 0
	}
	return a.Character.CurrentHP()
}

// MPInitialCost is def's up-front MP cost, scaled by the caster's dance/song
// MP-consume rate when def is a dance/song skill (Java's
// CreatureStatus.getMpInitialConsume, Stats.DANCE_MP_CONSUME_RATE).
func (a PlayerActor) MPInitialCost(def modelskill.Definition) int {
	return a.scaleDanceMP(def, def.MPInitialConsume)
}

// MPCost is def's per-cast MP cost. A dance/song skill (def.Dance) pays an
// extra danceCount * def.NextDanceCost surcharge for each dance/song
// already active on the caster, then the whole sum is scaled by the
// caster's dance MP-consume rate — mirroring Java's
// CreatureStatus.getMpConsume: "casting more dances costs more MP".
func (a PlayerActor) MPCost(def modelskill.Definition) int {
	mp := def.MPConsume
	if def.Dance {
		if dc := a.danceCount(); dc > 0 {
			mp += dc * def.NextDanceCost
		}
	}
	return a.scaleDanceMP(def, mp)
}

func (a PlayerActor) danceCount() int {
	if a.Character == nil {
		return 0
	}
	return a.Character.EffectList().DanceCount()
}

func (a PlayerActor) scaleDanceMP(def modelskill.Definition, mp int) int {
	if !def.Dance || a.Character == nil {
		return mp
	}
	return int(a.Character.CalcStat(stat.DanceMpConsumeRate, float64(mp)))
}

func (a PlayerActor) ReduceMP(amount int) {
	if a.Character == nil || amount <= 0 {
		return
	}
	a.Character.ReduceCurrentMP(amount)
}

func (a PlayerActor) ReduceHP(amount int) {
	if a.Character == nil || amount <= 0 {
		return
	}
	a.Character.ReduceCurrentHP(amount)
}

func (a PlayerActor) SkillDisabled(key int32) bool {
	return a.Character != nil && a.Character.SkillDisabled(key)
}

func (a PlayerActor) DisableSkill(key int32, delay time.Duration) {
	if a.Character != nil {
		a.Character.DisableSkill(key, delay)
	}
}

func (a PlayerActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	if a.Character != nil {
		a.Character.AddSkillReuse(ref, key, delay)
	}
}

func (PlayerActor) MagicMuted() bool { return false }

func (PlayerActor) PhysicalMuted() bool { return false }

func (PlayerActor) SpiritshotCharged() bool { return false }

func (PlayerActor) BlessedSpiritshotCharged() bool { return false }

func (PlayerActor) SkillMastery(modelskill.Definition) bool { return false }

func (a PlayerActor) ItemCount(itemID int) int {
	if a.Character == nil || a.Character.Inventory() == nil {
		return 0
	}
	return a.Character.Inventory().ItemCount(int32(itemID), -1, true)
}

func (a PlayerActor) ConsumeItem(itemID, count int) bool {
	if a.Character == nil || a.Character.Inventory() == nil {
		return false
	}
	return a.Character.Inventory().DestroyByTemplateID(int32(itemID), count) != nil
}

// ExitSignetGround drops the first ground-signet effect the character
// carries, ending the signet its cast placed. Only the caster-side signet
// effect is held here; the ones living on the signet's own world actor are
// unreachable from the caster and end with that actor instead.
func (a PlayerActor) ExitSignetGround() {
	if a.Character == nil {
		return
	}
	list := a.Character.EffectList()
	for _, e := range list.All() {
		if e != nil && e.Type == effect.TypeSignetGround {
			list.Remove(e)
			return
		}
	}
}

// CubicListFull reports whether a's character already holds as many active
// cubics as Cubic Mastery allows, satisfying the cubicLister interface
// CanCast's cubic-specific gate checks.
func (a PlayerActor) CubicListFull() bool {
	return a.Character != nil && a.Character.CubicListFull()
}

// AllSkillsDisabled satisfies the allSkillsDisabler interface Controller.Stop
// and AIController.Disabled probe for, matching Java's
// Creature.isAllSkillsDisabled().
func (a PlayerActor) AllSkillsDisabled() bool {
	return a.Character != nil && a.Character.AllSkillsDisabled()
}

// EnableAllSkills is a no-op: it mirrors Java's Creature.enableAllSkills(),
// which clears only the raw Duel-defeat lock. This port doesn't model that
// lock since Duel isn't ported, so there is nothing to clear yet.
func (PlayerActor) EnableAllSkills() {}
