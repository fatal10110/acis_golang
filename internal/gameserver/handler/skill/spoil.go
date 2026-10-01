package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type magicCaster interface {
	Actor
	Level() int
}

// weaponGradePenalized optionally reports whether the caster's equipped
// weapon grade is insufficient for the skill being cast (a flat magic-
// resist penalty); a caster without one is treated as unpenalized.
type weaponGradePenalized interface {
	WeaponGradePenalty() bool
}

type spoilHandler struct{ roll func(int) int }

func (spoilHandler) Types() []string { return []string{"SPOIL"} }

// Use marks every live, unspoiled target as spoiled by the caster when the
// magic-resist roll succeeds.
func (h spoilHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(magicCaster)
	if !ok {
		return
	}
	roll := rnd.Get
	if h.roll != nil {
		roll = h.roll
	}
	penalty := false
	if p, ok := cast.Caster.(weaponGradePenalized); ok {
		penalty = p.WeaponGradePenalty()
	}

	for _, obj := range cast.Targets {
		target, ok := asNPC(obj)
		if !ok || target.Dead() {
			continue
		}
		pool := target.SpoilPool()
		if pool == nil || pool.IsSpoiled() {
			if notify, ok := asPlayer(cast.Caster); ok && pool != nil {
				notify.NotifySpoilAlready()
			}
			continue
		}

		rate := formulas.MagicSuccessRate(target.Level(), caster.Level(), cast.Skill.MagicLevel, cast.Skill.LevelDepend, penalty)
		if formulas.MagicSucceeds(rate, roll(10000)) {
			// Another spoiler can mark the pool between the check above
			// and here; losing that race reads as the pool already spoiled.
			marked := pool.Mark(caster.ObjectID())
			if notify, ok := asPlayer(cast.Caster); ok {
				if marked {
					notify.NotifySpoilSuccess()
				} else {
					notify.NotifySpoilAlready()
				}
			}
		} else if notify, ok := asPlayer(cast.Caster); ok {
			notify.NotifyResistedSkill(actorName(target), cast.Skill.ID, 1)
		}
	}
}

type sweepHandler struct{ ids objectIDAllocator }

func (sweepHandler) Types() []string { return []string{"SWEEP"} }

// Use drains every target's spoil pool into a player caster's inventory as
// earned items, then fully clears the pool — including its spoiler marker,
// reset together with it — and applies the
// skill's own self-targeted effects, if any. A caster that is not a player,
// or a handler without ids to create items with, leaves every pool alone.
// Sweeping has no slot check. Items go to the sweeper alone: a partied
// sweeper's items following the party's loot rule is #3157.
func (h sweepHandler) Use(cast Cast) {
	if h.ids == nil || cast.Caster == nil || cast.Caster.Kind() != actor.KindPlayer {
		return
	}
	sweeper, ok := cast.Caster.(earner)
	if !ok {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := asNPC(obj)
		if !ok {
			continue
		}
		pool := target.SpoilPool()
		if pool == nil || !pool.Sweepable() {
			continue
		}

		for _, swept := range pool.Sweep() {
			sweeper.AddEarnedItem(swept.ItemID, int(swept.Count), h.ids.NextID)
		}
	}

	applyCasterSelfEffects(cast, cast.Skill)
}
