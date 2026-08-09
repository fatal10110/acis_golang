package skill

type seedHandler struct{}

func (seedHandler) Types() []string { return []string{"SEED"} }

// Use ports L2SkillSeed.useSkill(): a target that already carries a live
// seed effect from this exact skill grows its power in place instead of
// getting a second instance. L2SkillSeed.useSkill() also loops every live
// EffectSeed on the target through AbstractEffect.rescheduleEffect(), but
// that call is a same-deadline no-op: startEffectTask()'s initialDelay is
// derived from the effect's fixed construction-time _periodStartTime, which
// rescheduleEffect() never mutates (AbstractEffect.java:264-270, 186-206,
// 138-141), so it reproduces the original expiry instead of granting a
// fresh period — recasting must not extend a live seed's duration. A target
// with no existing seed effect from this skill gets a fresh one applied
// normally.
func (seedHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(effectListTarget)
		if !ok {
			continue
		}
		list := target.EffectList()
		if list == nil {
			continue
		}

		if e := firstEffectByID(list, cast.Skill.ID); e != nil {
			e.IncreasePower()
			continue
		}

		applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
	}
}
