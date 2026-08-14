package effect

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func spoilStart(e *Effect) bool {
	caster, ok := e.Effector.(spoilCaster)
	if !ok {
		return false
	}
	target, ok := e.Effected.(spoilTarget)
	if !ok || target.Dead() {
		return false
	}
	pool := target.SpoilPool()
	if pool == nil || pool.IsSpoiled() {
		return false
	}

	penalty := false
	if p, ok := e.Effector.(weaponGradePenalized); ok {
		penalty = p.WeaponGradePenalty()
	}

	rate := formulas.MagicSuccessRate(target.Level(), caster.Level(), e.Skill.MagicLevel, e.Skill.LevelDepend, penalty)
	if formulas.MagicSucceeds(rate, rnd.Get(spoilRoll)) {
		pool.Mark(caster.ObjectID())
	}
	return true
}

// distrustRadius is the search radius, in game units, a distrust effect
// scans for a redirect candidate.
const distrustRadius = 600

// distrustStart turns a Monster-family target's aggression toward a random
// other Monster-family actor found nearby (excluding lootable chests),
// with an aggro amount scaled by the caster's level. A target with no
// nearby candidate, or no radius-search capability at all, still reports
// success.
func distrustStart(e *Effect) bool {
	target, ok := e.Effected.(hateRaiser)
	if !ok {
		return false
	}
	if m, ok := e.Effected.(monsterKindTarget); !ok || !m.MonsterKind() {
		return false
	}
	finder, ok := e.Effected.(nearbyMonsterFinder)
	if !ok {
		return true
	}
	candidate, ok := finder.RandomNearbyMonster(distrustRadius)
	if !ok {
		return true
	}

	level := 0
	if lv, ok := e.Effector.(levelTarget); ok {
		level = lv.Level()
	}
	aggro := float64((5 + rnd.Get(5)) * level)
	target.AddDamageHate(candidate, 0, aggro)
	return true
}

// growTarget is implemented by an Npc-shaped actor whose collision radius
// can be overridden at runtime and later restored to its template value.

func growStart(e *Effect) bool {
	target, ok := e.Effected.(growTarget)
	if !ok {
		return false
	}
	target.SetCollisionRadius(target.CollisionRadius() * growRadiusScale)
	startAbnormalEffect(e.Effected, 0x010000)
	return true
}

// growExit ports EffectGrow.onExit(): restores the target's runtime
// collision-radius override to its template value.
func growExit(e *Effect) {
	target, ok := e.Effected.(growTarget)
	if !ok {
		return
	}
	target.ResetCollisionRadius()
	stopAbnormalEffect(e.Effected, 0x010000)
}

// recoveryTarget is implemented by a player-shaped actor that tracks a
// death-penalty debuff level.

func recoveryStart(e *Effect) bool {
	target, ok := e.Effected.(recoveryTarget)
	if !ok {
		return false
	}
	target.ReduceDeathPenaltyLevel()
	return true
}

// randomizeHateStart ports EffectRandomizeHate.onStart(): rejects a target
// that isn't an Attackable-shaped actor, otherwise delegates the swap to
// its threat table.
func randomizeHateStart(e *Effect) bool {
	target, ok := e.Effected.(hateRandomizer)
	if !ok {
		return false
	}
	target.RandomizeHate()
	return true
}

// confusionRadius is the search radius, in game units, a confusion
// effect scans for a redirect candidate.
const confusionRadius = 1000

// confusionStart aborts a non-player target's current move, then redirects
// its aggression toward a random nearby attackable actor found within
// confusionRadius. A player target is left untouched entirely. Unlike the
// reference effect's own 2D-only distance check, the radius search this
// port reuses (world.State.ForEachKnownInRadius) filters in 3D — a
// documented simplification, not a byte-exact match, since no 2D-only
// radius query exists in this port yet.
func confusionStart(e *Effect) bool {
	if isPlayer(e.Effected) {
		return true
	}
	if target, ok := e.Effected.(moveStopper); ok {
		target.StopMove()
	}
	refresh(e.Effected)

	finder, ok := e.Effected.(nearbyCombatTarget)
	if !ok {
		return true
	}
	candidate, ok := finder.RandomNearbyCombatant(confusionRadius)
	if !ok {
		return true
	}
	if target, ok := e.Effected.(hateRaiser); ok {
		target.AddDamageHate(candidate, 0, math.MaxInt32)
	}
	return true
}

func confusionExit(e *Effect) {
	refresh(e.Effected)
	if isPlayer(e.Effected) {
		return
	}
	if target, ok := e.Effected.(mostHatedResetter); ok {
		target.StopMostHatedTarget()
	}
}

func betrayStart(e *Effect) bool {
	summon, ok := e.Effected.(summonOwnerAttacker)
	if !ok {
		return false
	}
	ownerSource, ok := e.Effected.(summonOwnerCombatant)
	if !ok {
		return false
	}
	owner := ownerSource.OwnerCombatant()
	if owner == nil {
		return false
	}
	summon.TryToAttack(owner)
	return true
}

func betrayExit(e *Effect) {
	summon, ok := e.Effected.(summonOwnerAttacker)
	if !ok {
		return
	}
	ownerSource, ok := e.Effected.(summonOwnerCombatant)
	if !ok {
		return
	}
	if owner := ownerSource.OwnerCombatant(); owner != nil {
		summon.TryToFollow(owner)
	}
}

// relaxStart sits the target down; it always reports success.
func relaxStart(e *Effect) bool {
	sit(e.Effected)
	return true
}

// relaxAction drains MP each tick while the target stays seated and its HP
// isn't already full, reusing the same lack-MP handling as the other
// mana-drain ticks in this file. Unlike TypeChameleonRest, this tick has no
// continuous-skill gate: the reference effect never checks one.
func relaxAction(e *Effect) bool {
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	if st, ok := e.Effected.(standingTarget); ok && st.Standing() {
		return false
	}
	if full, ok := e.Effected.(hpFullTarget); ok && full.HPFull() {
		return false
	}
	return manaDrainTick(e, target)
}

// chameleonRestStart sits the target down; it always reports success.
func chameleonRestStart(e *Effect) bool {
	sit(e.Effected)
	return true
}

// chameleonRestAction drains MP each tick while a continuous cast keeps the
// tick alive and the target stays seated, reusing the same lack-MP
// handling as the other mana-drain ticks in this file.
func chameleonRestAction(e *Effect) bool {
	if e.Skill.SkillType != "CONT" {
		return false
	}
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	if st, ok := e.Effected.(standingTarget); ok && st.Standing() {
		return false
	}
	return manaDrainTick(e, target)
}

// manaDrainTick runs one ManaDamageOverTimeTick against target and applies
// its result, shared by relaxAction and chameleonRestAction.
//
// Toggle is forced true regardless of e.Skill.Toggle: the reference Relax
// and ChameleonRest effects check "cost exceeds current MP" unconditionally,
// not only for toggle skills (unlike EffectManaDamOverTime, whose lack-MP
// check really is toggle-gated). Every skill carrying either effect in the
// current datapack happens to be TOGGLE-typed, so reading e.Skill.Toggle
// would produce the same result today — but that's a data coincidence, not
// a contract; force true here so a future non-toggle skill using these
// effects still gets the unconditional check Java requires.

func fakeDeathStart(e *Effect) bool {
	if target, ok := e.Effected.(fakeDeathStanceTarget); ok {
		target.StartFakeDeath()
	} else {
		sit(e.Effected)
	}
	refresh(e.Effected)
	return true
}

// fakeDeathAction drains MP each tick, reusing the same lack-MP handling
// as the other mana-drain ticks in this file.
func fakeDeathAction(e *Effect) bool {
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	return manaDrainTick(e, target)
}

// fakeDeathExit stands the target back up and starts its recent-fake-death
// grace period, during which hostile NPC AI won't retarget it.
func fakeDeathExit(e *Effect) {
	if target, ok := e.Effected.(recentFakeDeathMarker); ok {
		target.MarkRecentFakeDeath()
	}
	if target, ok := e.Effected.(fakeDeathStanceTarget); ok {
		target.StopFakeDeath()
	} else if target, ok := e.Effected.(sitTarget); ok {
		target.SetStanding(true)
	}
	refresh(e.Effected)
}

func sit(effected Participant) {
	if target, ok := effected.(stanceTarget); ok {
		target.Sit()
	} else if target, ok := effected.(sitTarget); ok {
		target.SetStanding(false)
	}
}

// cancelDebuffStart strips a capped selection of a player target's active,
// dispellable debuffs against an independently-rolled per-candidate chance
// (formulas.EffectCancelDebuffSuccessRate), scanning the effect list's
// current snapshot from its most-recently-added entry back to its oldest.
//
// The scan runs up to two full passes over the same snapshot whenever the
// cap isn't reached (or is unlimited, cap 0) in the first pass — including
// re-examining candidates the first pass already removed, since removing
// an already-removed candidate is a safe no-op but still counts against
// the cap exactly as it does in the reference effect. A candidate whose
// owning skill id matches the immediately preceding removal is stripped
// without its own roll. Both quirks reproduce the reference effect's own
// two-pass loop exactly; do not "simplify" this into cancelStart's single
// shuffled pass.
