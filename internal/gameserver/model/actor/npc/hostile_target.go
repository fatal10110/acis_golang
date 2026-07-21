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
// Excluded unconditionally: a nil target, another NPC (this codebase's
// combat AI never auto-attacks another NPC by default), and an
// already-dead target. Also excluded: a target outside rangeVal, a
// silently-moving target (unless this NPC's template can see through
// concealment), and — unless allowPeaceful is set — a target standing in
// a peace zone, or this NPC not being aggressive. A surviving candidate
// must still be within line of sight.
//
// This ports the reference server's default targeting rule only. Door
// exclusion needs no explicit check: door.Object doesn't implement
// attackable.Combatant, so a door can never be passed as target here. Not
// modeled: the Player-only sub-checks (appearance invisibility,
// allied-race exclusion, rift-room memo, recent-fake-death grace period),
// the confused-actor special case, and the Guard/FriendlyMonster
// karma-gated branches — every Hostile uses this same general rule
// regardless of its own instance kind.
func (h *Hostile) AutoAttackTargetValid(target attackable.Combatant, rangeVal int, allowPeaceful bool) bool {
	if target == nil || target.AlikeDead() {
		return false
	}
	if _, isNpcTarget := target.(*Hostile); isNpcTarget {
		return false
	}

	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	tx, ty, tz := other.Position()
	sx, sy, sz := h.Position()
	if !location.In3DRange(sx, sy, sz, tx, ty, tz, rangeVal) {
		return false
	}

	if !h.Instance.Template.CanSeeThrough {
		if sm, ok := target.(interface{ SilentMoving() bool }); ok && sm.SilentMoving() {
			return false
		}
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
