package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// NotifyHPRestored sends the HP-restored system message: number-only for a
// self restore, healer name plus amount when another actor applied it.
func (c *Character) NotifyHPRestored(healerName string, amount int, byOther bool) {
	c.emit(event.Restored{Resource: event.ResourceHP, HealerName: healerName, Amount: amount, ByOther: byOther})
}

// NotifyMPRestored sends the MP-restored system message: number-only for a
// self restore, healer name plus amount when another actor applied it.
func (c *Character) NotifyMPRestored(healerName string, amount int, byOther bool) {
	c.emit(event.Restored{Resource: event.ResourceMP, HealerName: healerName, Amount: amount, ByOther: byOther})
}

func (c *Character) NotifyCPRestored(healerName string, amount int, byOther bool) {
	c.emit(event.Restored{Resource: event.ResourceCP, HealerName: healerName, Amount: amount, ByOther: byOther})
}

// NotifyEffectRemovedDueLackHP sends this player's SKILL_REMOVED_DUE_LACK_HP
// system message. e is unused: the reference message carries no skill-name
// parameter (EffectDamOverTime.java:32-36).
func (c *Character) NotifyEffectRemovedDueLackHP(*effect.Effect) {
	c.emit(event.EffectRemovedLackHP{})
}

// NotifyEffectRemovedDueLackMP sends this player's SKILL_REMOVED_DUE_LACK_MP
// system message. e is unused: the reference message carries no skill-name
// parameter (EffectManaDamOverTime.java:29-33).
func (c *Character) NotifyEffectRemovedDueLackMP(*effect.Effect) {
	c.emit(event.EffectRemovedLackMP{})
}

// NotifyRelaxDeactivatedHPFull sends SKILL_DEACTIVATED_HP_FULL.
func (c *Character) NotifyRelaxDeactivatedHPFull(*effect.Effect) {
	c.emit(event.RelaxHPFull{})
}

func (c *Character) NotifySpoilAlready() {
	c.emit(event.SpoilResult{Already: true})
}

func (c *Character) NotifySpoilSuccess() {
	c.emit(event.SpoilResult{})
}

// NotifyOverHit sends the OVER_HIT system message for a valid overhit kill.
func (c *Character) NotifyOverHit() {
	c.emit(event.OverHit{})
}

// NotifyEffectWornOff sends S1_HAS_WORN_OFF for an effect that ran its full
// course.
func (c *Character) NotifyEffectWornOff(skillID modelskill.ID, level int) {
	c.emit(event.EffectEnded{Reason: event.EffectWornOff, SkillID: skillID, Level: level})
}

// NotifyEffectDisappeared sends EFFECT_S1_DISAPPEARED for an effect removed
// before it ran its full course.
func (c *Character) NotifyEffectDisappeared(skillID modelskill.ID, level int) {
	c.emit(event.EffectEnded{Reason: event.EffectDisappeared, SkillID: skillID, Level: level})
}

// NotifyEffectAborted sends S1_HAS_BEEN_ABORTED for a toggle skill turned
// off.
func (c *Character) NotifyEffectAborted(skillID modelskill.ID, level int) {
	c.emit(event.EffectEnded{Reason: event.EffectAborted, SkillID: skillID, Level: level})
}

// NotifyAttackFailed sends ATTACK_FAILED for a half-damage magic resist.
func (c *Character) NotifyAttackFailed() {
	c.emit(event.AttackFailed{})
}

// NotifyResistedSkill sends S1_RESISTED_YOUR_S2 naming the target and skill.
func (c *Character) NotifyResistedSkill(targetName string, skillID modelskill.ID, level int) {
	c.emit(event.SkillResisted{TargetName: targetName, SkillID: skillID, Level: level})
}

// NotifyResistedMagic sends RESISTED_S1_MAGIC naming the attacker.
func (c *Character) NotifyResistedMagic(attackerName string) {
	c.emit(event.MagicResisted{AttackerName: attackerName})
}
