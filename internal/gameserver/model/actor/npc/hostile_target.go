package npc

import (
	"slices"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Aggressive reports whether this NPC attacks nearby targets on sight,
// independent of any hate already built against it. A Monster-family NPC is
// aggressive when its template has an aggro range; a FestivalMonster or
// FriendlyMonster always is; a Guard or SiegeGuard never is.
func (h *Hostile) Aggressive() bool {
	switch hostileKind(h.Instance) {
	case "FestivalMonster", "FriendlyMonster":
		return true
	case "Guard", "SiegeGuard":
		return false
	}
	return h.Instance.Template.AggroRange > 0
}

// AutoAttackTargetValid reports whether target is a legal automatic-combat
// target for h at the given max range: a candidate this NPC's AI may keep
// attacking or select from its hate list, not a player-issued attack
// request.
//
// Excluded unconditionally: a nil target and an already-dead target. A
// candidate already the FinalTarget of a queued, non-moving ATTACK Desire
// is excluded too when that already starts or maintains an offensive
// follow (the queued-desire follow gate):
// this NPC is already committed to closing on it, so re-validating it
// against the rules below is redundant. A non-NPC target must also be
// within rangeVal and, unless this NPC is raid-related or its template can
// see through concealment, not be silently moving.
//
// Guard and FriendlyMonster kinds then attack a karma-positive target
// purely on line of sight; a Guard also attacks an aggressive Monster-family
// NPC in sight when AIConfig.GuardAttackAggroMob is set. Every other kind
// excludes another NPC target unless this NPC is confused, in which case it
// attacks purely on line of sight; otherwise it excludes a target standing
// in a peace zone when AIConfig.MobAggroInPeaceZone is off, and any target
// at all when this NPC is neither aggressive nor allowPeaceful. A surviving
// candidate must still be within line of sight.
//
// This is the default NPC targeting rule. Door exclusion
// needs no explicit check: door.Object doesn't implement
// attackable.Combatant, so a door can never be passed as target here. A
// non-NPC target still within its post-fake-death grace period is excluded
// too (the recent-fake-death check), and so is an invisible player or the
// summon of one. Not modeled: the remaining Player-only sub-checks
// (allied-Varka/allied-Ketra exclusion, rift-room memo). The follow gate's
// distance decision reuses move.Controller.MaybeStartOffensiveFollow, which
// reads the current intention's move-to-target flag, not the queued hold
// desire's.
func (h *Hostile) AutoAttackTargetValid(target attackable.Combatant, rangeVal int, allowPeaceful bool) bool {
	if target == nil || target.AlikeDead() {
		return false
	}

	if _, ok := h.brain.Desires().NonMovingAttack(target); ok {
		following, err := h.brain.MaybeStartOffensiveFollow(target)
		if err != nil {
			h.log.Debug().Err(err).Int32("object_id", h.ObjectID()).Msg("npc: offensive follow broadcast")
		}
		if following {
			return false
		}
	}

	// A civilian NPC is never an automatic target: it is no attackable NPC
	// a confused one may turn on, nor a karma-holding playable.
	if _, ok := target.(*Folk); ok {
		return false
	}
	_, targetIsNPC := target.(*Hostile)
	if !targetIsNPC {
		graceTarget := target
		if owner, ok := target.Owner(); ok {
			graceTarget = owner
		}
		if graceTarget.RecentFakeDeath() {
			return false
		}
	}
	if !targetIsNPC && !h.inRangeAndUnconcealed(target, rangeVal) {
		return false
	}
	// An invisible player, or its summon, is never an automatic target.
	if !targetIsNPC && attackable.HiddenActingPlayer(target) {
		return false
	}

	cfg := h.aiSettings()
	switch hostileKind(h.Instance) {
	case "Guard":
		if target.Karma() > 0 {
			return h.CanSee(target)
		}
		if monster, ok := target.(*Hostile); ok && cfg.GuardAttackAggroMob && monster.MonsterKind() {
			return monster.Aggressive() && h.CanSee(monster)
		}
		return false
	case "FriendlyMonster":
		return target.Karma() > 0 && h.CanSee(target)
	}

	if targetIsNPC {
		return h.Confused() && h.CanSee(target)
	}

	if !cfg.MobAggroInPeaceZone && target.InPeaceZone() {
		return false
	}

	return (allowPeaceful || h.Aggressive()) && h.CanSee(target)
}

// inRangeAndUnconcealed applies the range and silent-move gates the
// targeting rule reserves for non-NPC targets.
func (h *Hostile) inRangeAndUnconcealed(target attackable.Combatant, rangeVal int) bool {
	if rangeVal < 0 {
		return false
	}
	tx, ty, tz := target.Position()
	sx, sy, sz := h.Position()
	dx := int64(sx) - int64(tx)
	dy := int64(sy) - int64(ty)
	dz := int64(sz) - int64(tz)
	if dx*dx+dy*dy+dz*dz >= int64(rangeVal)*int64(rangeVal) {
		return false
	}

	if h.RaidRelated() || h.Instance.Template.CanSeeThrough {
		return true
	}
	return !target.SilentMoving()
}

// siegeGuardAutoAttackTargetValid is the dedicated one-argument auto-attack
// rule a SiegeGuard kind uses in place of AutoAttackTargetValid above,
// reachable only through the one-argument reconsider-target path below.
// The three-argument RandomizeHate path (target, range, allowPeaceful)
// keeps calling AutoAttackTargetValid unchanged for SiegeGuard, same as
// every other kind.
//
// Rejects a target with no acting player (an NPC target), an alike-dead or
// invisible acting player, and an acting player silently moving beyond 250
// units; otherwise requires siege attackability (target attackable by this
// guard) and line of sight. The target's acting player is resolved once and
// the alike-dead/invisible/silent-moving/distance gates are checked against
// that acting player, not against target directly — for a Summon/Pet target
// this is the owning player, matching AutoAttackTargetValid's own Owner()
// resolution above; only the closing attackability and line-of-sight checks
// use the raw target. Not modeled: the clan/siege-side DEFENDER/OWNER
// exclusion inside a playable's siege-guard attackability branch is not
// wired to the castle sieges yet (#3375), so the attackability check here
// falls through to whatever general AttackableBy the target exposes.
func (h *Hostile) siegeGuardAutoAttackTargetValid(target attackable.Combatant) bool {
	if target == nil {
		return false
	}

	if target.Kind() == actor.KindNPC {
		return false
	}

	actingPlayer := target
	if target.Kind() == actor.KindSummon {
		actingPlayer, _ = target.Owner()
	}
	if actingPlayer == nil || actingPlayer.AlikeDead() {
		return false
	}

	if attackable.HiddenActingPlayer(actingPlayer) {
		return false
	}

	if actingPlayer.SilentMoving() && !h.withinDistance(actingPlayer, 250) {
		return false
	}

	rules, ok := target.(skilltarget.Actor)
	if !ok || !rules.AttackableBy(h) {
		return false
	}

	return h.CanSee(target)
}

// ReconsiderTarget is in-range target reconsideration, used
// when this NPC can no longer act on its current target (e.g. an
// immobilize state): first tries to pick a replacement from its own hate
// list (see ai.Attackable.ReconsiderTarget / attackable.ThreatTable.
// ReconsiderTarget), gated by the auto-attack rule (AutoAttackTargetValid,
// or the siege guard's own rule) — this NPC's template aggro range,
// allowPeaceful false — plus rangeVal as an extra distance filter applied
// only when rangeVal > 0 (0 disables it). If the hate list yields nothing and this
// NPC isn't a SiegeGuard and is aggressive, it falls back to scanning known
// creatures within its template aggro range for the first (lowest
// ObjectID, for a reproducible pick under Go's unordered world scan)
// auto-attack-valid candidate, granting it 1 hate to simulate an
// aggro-range entrance. Reports the new target and whether one was found.
//
// Nothing in the specified server actually calls target reconsideration,
// although its documentation describes the immobilize use case (verified:
// zero call sites); this ships as the same available, unwired API — see
// acis_golang#977.
func (h *Hostile) ReconsiderTarget(rangeVal int) (attackable.Combatant, bool) {
	valid := func(target attackable.Combatant) bool {
		if h.SiegeGuard() {
			return h.siegeGuardAutoAttackTargetValid(target)
		}
		return h.AutoAttackTargetValid(target, h.Instance.Template.AggroRange, false)
	}
	inRange := func(target attackable.Combatant) bool {
		if rangeVal <= 0 {
			return true
		}
		return h.withinDistance(target, rangeVal)
	}

	if chosen, ok := h.brain.ReconsiderTarget(inRange, valid); ok {
		return chosen, true
	}

	if h.SiegeGuard() || !h.Aggressive() || h.world == nil {
		return nil, false
	}

	var candidates []attackable.Combatant
	h.world.ForEachKnownInRadius(h, h.Instance.Template.AggroRange, func(obj world.Tracked) {
		other, ok := obj.(attackable.Combatant)
		if !ok {
			return
		}
		if rangeVal > 0 && !h.withinDistance(other, rangeVal) {
			return
		}
		if !valid(other) {
			return
		}
		candidates = append(candidates, other)
	})
	if len(candidates) == 0 {
		return nil, false
	}
	slices.SortFunc(candidates, func(a, b attackable.Combatant) int {
		return int(a.ObjectID() - b.ObjectID())
	})
	chosen := candidates[0]
	h.brain.AddDamageHate(chosen, 0, 1)
	return chosen, true
}

// withinDistance reports whether target sits within rangeVal 3D units of h.
func (h *Hostile) withinDistance(target attackable.Combatant, rangeVal int) bool {
	if rangeVal < 0 {
		return false
	}
	tx, ty, tz := target.Position()
	sx, sy, sz := h.Position()
	dx := int64(sx) - int64(tx)
	dy := int64(sy) - int64(ty)
	dz := int64(sz) - int64(tz)
	return dx*dx+dy*dy+dz*dz < int64(rangeVal)*int64(rangeVal)
}
