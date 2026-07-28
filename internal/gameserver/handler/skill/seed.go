package skill

type seedHandler struct{}

func (seedHandler) Types() []string { return []string{"SEED"} }

// Use ports L2SkillSeed.useSkill(): a target that already carries a live
// seed effect from this exact skill grows its power in place instead of
// getting a second instance, and every active seed effect on that target
// (not just the one that grew) has its duration timer restarted. A target
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
			list.RescheduleSeeds()
			continue
		}

		applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
	}
}
