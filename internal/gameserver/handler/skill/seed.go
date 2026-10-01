package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type seedHandler struct{}

func (seedHandler) Types() []string { return []string{"SEED"} }

// Use casts a seed skill: a target that already carries a live seed effect
// from this exact skill grows its power in place instead of getting a
// second instance. Every live seed effect on the target is also
// rescheduled, but that is a same-deadline no-op: the new delay derives
// from the effect's fixed construction-time period start, which rescheduling
// never mutates, so it reproduces the original expiry instead of granting a
// fresh period — recasting must not extend a live seed's duration. A target
// with no existing seed effect from this skill gets a fresh one applied
// normally.
func (seedHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(effect.Actor)
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

		applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects, formulas.ShieldFailed, false)
	}
}
