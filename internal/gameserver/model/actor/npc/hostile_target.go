package npc

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Aggressive reports whether this NPC attacks nearby targets on sight,
// independent of any hate already built against it. Driven by the
// template's aggro range: a template with no configured aggro range never
// initiates combat on its own.
func (h *Hostile) Aggressive() bool {
	return h.Instance.Template.AggroRange > 0
}

// AutoAttackTargetValid reports whether target is a legal automatic-combat
// target for h at the given max range: a candidate this NPC's AI may keep
// attacking or select from its hate list, not a player-issued attack
// request.
//
// Excluded unconditionally: a nil target and an already-dead target. A
// non-NPC target must also be within rangeVal and, unless this NPC is
// raid-related or its template can see through concealment, not be
// silently moving.
//
// Guard and FriendlyMonster kinds then use one rule: attack only a
// karma-positive target, purely on line of sight. Every other kind excludes
// another NPC target unless this NPC is confused, in which case it attacks
// purely on line of sight; otherwise, unless allowPeaceful is set, it
// excludes a target standing in a peace zone or excludes any target at all
// when this NPC isn't aggressive. A surviving candidate must still be
// within line of sight.
//
// This ports the reference server's default targeting rule. Door exclusion
// needs no explicit check: door.Object doesn't implement
// attackable.Combatant, so a door can never be passed as target here. A
// non-NPC target still within its post-fake-death grace period is excluded
// too, matching the reference's recent-fake-death check. Not modeled: the
// remaining Player-only sub-checks (appearance invisibility, allied-Varka/
// allied-Ketra exclusion, rift-room memo), Guard's aggressive-Monster
// branch (gated by a config flag that ships disabled by default, and needs
// npc AI config plumbing that doesn't exist yet), and the peace-zone aggro
// config flag (allowPeaceful is a caller-supplied parameter here rather
// than the reference's own config-driven default).
func (h *Hostile) AutoAttackTargetValid(target attackable.Combatant, rangeVal int, allowPeaceful bool) bool {
	if target == nil || target.AlikeDead() {
		return false
	}

	_, targetIsNPC := target.(*Hostile)
	if !targetIsNPC {
		if recent, ok := target.(interface{ RecentFakeDeath() bool }); ok && recent.RecentFakeDeath() {
			return false
		}
	}
	if !targetIsNPC && !h.inRangeAndUnconcealed(target, rangeVal) {
		return false
	}

	switch hostileKind(h.Instance) {
	case "Guard", "FriendlyMonster":
		return h.karmaTargetVisible(target)
	}

	if targetIsNPC {
		return h.Confused() && h.CanSee(target)
	}

	if !allowPeaceful {
		if pz, ok := target.(interface{ InPeaceZone() bool }); ok && pz.InPeaceZone() {
			return false
		}
		if !h.Aggressive() {
			return false
		}
	}

	return h.CanSee(target)
}

// inRangeAndUnconcealed applies the range and silent-move gates the
// reference rule reserves for non-NPC targets.
func (h *Hostile) inRangeAndUnconcealed(target attackable.Combatant, rangeVal int) bool {
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	tx, ty, tz := other.Position()
	sx, sy, sz := h.Position()
	if !location.In3DRange(sx, sy, sz, tx, ty, tz, rangeVal) {
		return false
	}

	if h.RaidRelated() || h.Instance.Template.CanSeeThrough {
		return true
	}
	sm, ok := target.(interface{ SilentMoving() bool })
	return !ok || !sm.SilentMoving()
}

// karmaTargetVisible reports whether target is a karma-positive actor
// within line of sight — the sole target rule Guard and FriendlyMonster
// kinds use in place of the general rule below.
func (h *Hostile) karmaTargetVisible(target attackable.Combatant) bool {
	pk, ok := target.(interface{ Karma() int })
	return ok && pk.Karma() > 0 && h.CanSee(target)
}

// ReconsiderTarget ports Npc.java's AggroList.reconsiderTarget(range), used
// when this NPC can no longer act on its current target (e.g. an
// immobilize state): first tries to pick a replacement from its own hate
// list (see ai.Attackable.ReconsiderTarget / attackable.ThreatTable.
// ReconsiderTarget), gated by canAutoAttack(target) — this NPC's template
// aggro range, allowPeaceful false — plus rangeVal as an extra distance
// filter applied only when rangeVal > 0 (0 disables it, matching the
// reference's "range > 0" guard). If the hate list yields nothing and this
// NPC isn't a SiegeGuard and is aggressive, it falls back to scanning known
// creatures within its template aggro range for the first (lowest
// ObjectID, for a reproducible pick under Go's unordered world scan)
// canAutoAttack-valid candidate, granting it 1 hate to simulate an
// aggro-range entrance. Reports the new target and whether one was found.
//
// No caller in the Java reference actually invokes reconsiderTarget despite
// its javadoc describing the immobilize use case (verified: zero call
// sites anywhere in aCis_gameserver); this ships as the same available,
// unwired API the reference itself carries — see acis_golang#977.
func (h *Hostile) ReconsiderTarget(rangeVal int) (attackable.Combatant, bool) {
	valid := func(target attackable.Combatant) bool {
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
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	tx, ty, tz := other.Position()
	sx, sy, sz := h.Position()
	return location.In3DRange(sx, sy, sz, tx, ty, tz, rangeVal)
}
