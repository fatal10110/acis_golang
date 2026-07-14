package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type spoilableTarget interface {
	Dead() bool
	Level() int
	SpoilPool() *item.SpoilPool
}

type magicCaster interface {
	ObjectID() int32
	Level() int
}

// weaponGradePenalized optionally reports whether the caster's equipped
// weapon grade is insufficient for the skill being cast (a flat magic-
// resist penalty); a caster without one is treated as unpenalized.
type weaponGradePenalized interface {
	WeaponGradePenalty() bool
}

type spoilHandler struct{}

func (spoilHandler) Types() []string { return []string{"SPOIL"} }

// Use marks every live, unspoiled target as spoiled by the caster when the
// magic-resist roll succeeds.
func (spoilHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(magicCaster)
	if !ok {
		return
	}
	penalty := false
	if p, ok := cast.Caster.(weaponGradePenalized); ok {
		penalty = p.WeaponGradePenalty()
	}

	for _, obj := range cast.Targets {
		target, ok := obj.(spoilableTarget)
		if !ok || target.Dead() {
			continue
		}
		pool := target.SpoilPool()
		if pool == nil || pool.IsSpoiled() {
			continue
		}

		rate := formulas.MagicSuccessRate(target.Level(), caster.Level(), cast.Skill.MagicLevel, cast.Skill.LevelDepend, penalty)
		if formulas.MagicSucceeds(rate, rnd.Get(10000)) {
			pool.Mark(caster.ObjectID())
		}
	}
}

// partyDistributor optionally routes a sweep/harvest reward through the
// caster's party split instead of directly into their own inventory.
type partyDistributor interface {
	InParty() bool
	DistributeItem(itemID, count int32)
}

type sweepHandler struct{}

func (sweepHandler) Types() []string { return []string{"SWEEP"} }

// Use drains every target's spoil pool into the caster's inventory (or
// their party's split), then fully clears the pool — including its spoiler
// marker, matching the reference container's combined reset — and applies
// the skill's own self-targeted effects, if any.
func (sweepHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(spoilableTarget)
		if !ok {
			continue
		}
		pool := target.SpoilPool()
		if pool == nil || !pool.Sweepable() {
			continue
		}

		items := pool.Sweep()
		pool.Reset()

		for itemID, count := range items {
			rewardSweep(cast.Caster, itemID, count)
		}
	}

	applyEffects(cast.Caster, cast.Caster, cast.Skill, cast.Skill.SelfEffects)
}

func rewardSweep(caster any, itemID, count int32) {
	if pd, ok := caster.(partyDistributor); ok && pd.InParty() {
		pd.DistributeItem(itemID, count)
		return
	}
	if e, ok := caster.(earner); ok {
		e.AddEarnedItem(itemID, int(count))
	}
}
