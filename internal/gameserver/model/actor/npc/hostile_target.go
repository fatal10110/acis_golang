package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
// attackable.Combatant, so a door can never be passed as target here. Not
// modeled: the Player-only sub-checks (appearance invisibility,
// allied-Varka/allied-Ketra exclusion, rift-room memo — recent-fake-death
// grace period is tracked separately by issue #898), Guard's aggressive-
// Monster branch (gated by a config flag that ships disabled by default,
// and needs npc AI config plumbing that doesn't exist yet), and the
// peace-zone aggro config flag (allowPeaceful is a caller-supplied
// parameter here rather than the reference's own config-driven default).
func (h *Hostile) AutoAttackTargetValid(target attackable.Combatant, rangeVal int, allowPeaceful bool) bool {
	if target == nil || target.AlikeDead() {
		return false
	}

	_, targetIsNPC := target.(*Hostile)
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
