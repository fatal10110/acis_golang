package effect

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func spoilStart(e *Effect) bool {
	caster := e.Effector
	if caster == nil {
		return false
	}
	target, ok := asNPC(e.Effected)
	if !ok || target.Dead() {
		return false
	}
	player, casterIsPlayer := asPlayer(caster)
	pool := target.SpoilPool()
	if pool == nil || pool.IsSpoiled() {
		if casterIsPlayer && pool != nil {
			player.NotifySpoilAlready()
		}
		return false
	}

	penalty := casterIsPlayer && player.WeaponGradePenalty()
	rate := formulas.MagicSuccessRate(target.Level(), caster.Level(), e.Skill.MagicLevel, e.Skill.LevelDepend, penalty)
	if formulas.MagicSucceeds(rate, rnd.Get(spoilRoll)) {
		// Another spoiler can mark the pool between the check above and
		// here; losing that race is the already-spoiled branch.
		if !pool.Mark(caster.ObjectID()) {
			if casterIsPlayer {
				player.NotifySpoilAlready()
			}
			return false
		}
		if casterIsPlayer {
			player.NotifySpoilSuccess()
		}
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
	target, ok := asNPC(e.Effected)
	if !ok || !target.MonsterKind() {
		return false
	}
	candidate, ok := target.RandomNearbyMonster(distrustRadius)
	if !ok {
		return true
	}

	level := 0
	if e.Effector != nil {
		level = e.Effector.Level()
	}
	aggro := float64((5 + rnd.Get(5)) * level)
	target.AddDamageHate(candidate, 0, aggro)
	return true
}

// growStart ports EffectGrow.onStart(): rejects a target that isn't
// Npc-shaped (a player target never grows), otherwise scales its collision
// radius by growRadiusScale and refreshes its visible state.
func growStart(e *Effect) bool {
	target, ok := asNPC(e.Effected)
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
	target, ok := asNPC(e.Effected)
	if !ok {
		return
	}
	target.ResetCollisionRadius()
	stopAbnormalEffect(e.Effected, 0x010000)
}

// recoveryStart lowers the target's death-penalty debuff level by one,
// rejecting a target that isn't player-shaped. Reapplying the debuff skill
// at its new level and refreshing the client's status window are the live
// character's own concern, not this effect's.
func recoveryStart(e *Effect) bool {
	target, ok := asPlayer(e.Effected)
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
	target, ok := asNPC(e.Effected)
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
// reference effect's own 2D-only distance check (EffectConfusion.java:43,
// wo.distance2D(getEffected()) <= 1000), the radius search this port
// reuses (world.State.ForEachKnownInPlainRadius, unwidened by collision
// radius) filters in 3D — a documented simplification, not a byte-exact
// match, since no 2D-only radius query exists in this port yet.
func confusionStart(e *Effect) bool {
	if isPlayer(e.Effected) {
		return true
	}
	e.Effected.StopMove()
	refresh(e.Effected)

	target, ok := asNPC(e.Effected)
	if !ok {
		return true
	}
	candidate, ok := target.RandomNearbyCombatant(confusionRadius)
	if !ok {
		return true
	}
	target.AddAttackDesire(candidate, math.MaxInt32)
	return true
}

func confusionExit(e *Effect) {
	refresh(e.Effected)
	if isPlayer(e.Effected) {
		return
	}
	if target, ok := asNPC(e.Effected); ok {
		target.StopMostHatedTarget()
	}
}

func betrayStart(e *Effect) bool {
	summon, ok := asSummon(e.Effected)
	if !ok {
		return false
	}
	owner, ok := summon.OwnerObject()
	if !ok {
		return false
	}
	summon.TryToAttack(owner)
	return true
}

func betrayExit(e *Effect) {
	summon, ok := asSummon(e.Effected)
	if !ok {
		return
	}
	if owner, ok := summon.OwnerObject(); ok {
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
	if player, ok := asPlayer(e.Effected); ok {
		if player.Standing() {
			return false
		}
		if player.HPFull() {
			player.NotifyRelaxDeactivatedHPFull(e)
			return false
		}
	}
	return manaDrainTick(e)
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
	if player, ok := asPlayer(e.Effected); ok && player.Standing() {
		return false
	}
	return manaDrainTick(e)
}

func fakeDeathStart(e *Effect) bool {
	if player, ok := asPlayer(e.Effected); ok {
		player.StartFakeDeath()
	}
	refresh(e.Effected)
	return true
}

// fakeDeathAction drains MP each tick, reusing the same lack-MP handling
// as the other mana-drain ticks in this file.
func fakeDeathAction(e *Effect) bool {
	return manaDrainTick(e)
}

// fakeDeathExit stands the target back up and starts its recent-fake-death
// grace period, during which hostile NPC AI won't retarget it.
func fakeDeathExit(e *Effect) {
	if player, ok := asPlayer(e.Effected); ok {
		player.MarkRecentFakeDeath()
		player.StopFakeDeath()
	}
	refresh(e.Effected)
}

// sit seats a player target; other kinds have no sitting stance.
func sit(effected Actor) {
	if player, ok := asPlayer(effected); ok {
		player.Sit()
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
