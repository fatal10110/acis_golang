package npc

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// clanMember is an NPC the clan calls reach and make: a hostile or a
// civilian NPC.
type clanMember interface {
	attackable.Combatant
	world.Tracked
	NpcID() int
	Dead() bool
	clanTemplate() *Template
	// seesClanMember reports whether the NPC and other see each other.
	seesClanMember(other attackable.Combatant) bool
}

func (h *Hostile) clanTemplate() *Template                        { return h.Instance.Template }
func (h *Hostile) seesClanMember(other attackable.Combatant) bool { return h.CanSee(other) }
func (f *Folk) clanTemplate() *Template                           { return f.Instance.Template }
func (f *Folk) seesClanMember(other attackable.Combatant) bool    { return f.canSee(other) }

// makesClanCalls reports whether an NPC of template t calls its clan: it
// belongs to a clan and has a clan range.
func (t *Template) makesClanCalls() bool {
	return len(t.Clans) > 0 && t.ClanRange > 0
}

// clanAttackSource is which clan calls an attack makes.
type clanAttackSource struct {
	// self calls the attacked NPC itself first.
	self bool
	// neighbours calls the clan members in range.
	neighbours bool
	// hostileOnly leaves civilian NPCs out of the neighbours.
	hostileOnly bool
}

var (
	// A hit calls the NPC itself, then every NPC of its clan in range.
	clanAttackedByHit = clanAttackSource{self: true, neighbours: true}
	// An aggression effect calls the NPC itself only.
	clanAttackedByAggression = clanAttackSource{self: true}
	// A skill calls the hostile NPCs of the clan in range, not the NPC
	// itself.
	clanAttackedBySkill = clanAttackSource{neighbours: true, hostileOnly: true}
)

// raiseClanAttacked makes caller's clan calls for attacker's attack of
// damage, with sk when a skill did it, as src makes them. It does nothing
// for an NPC that makes no clan calls.
func raiseClanAttacked(hooks ScriptHooks, w *world.State, caller clanMember, attacker attackable.Combatant, damage int32, sk skill.Ref, src clanAttackSource) {
	if hooks == nil || attacker == nil || !caller.clanTemplate().makesClanCalls() {
		return
	}
	if src.self {
		hooks.ClanAttacked(caller, caller, attacker, damage, sk)
	}
	if src.neighbours {
		forEachClanMember(w, caller, src.hostileOnly, func(called clanMember) {
			hooks.ClanAttacked(caller, called, attacker, damage, sk)
		})
	}
}

// raiseClanDied tells caller's clan members in range that killer has just
// killed caller. caller itself is not told.
func raiseClanDied(hooks ScriptHooks, w *world.State, caller clanMember, killer attackable.Combatant) {
	if hooks == nil || !caller.clanTemplate().makesClanCalls() {
		return
	}
	forEachClanMember(w, caller, false, func(called clanMember) {
		hooks.ClanDied(caller, called, killer)
	})
}

// forEachClanMember calls fn for every NPC within caller's clan range (3D,
// inclusive, widened by both collision radii) that answers caller's call:
// alive, sharing one of caller's clans, not ignoring caller's template id,
// and in sight of caller. hostileOnly leaves civilian NPCs out. The NPCs in
// range are collected first; the other gates are checked as each one's
// turn comes, so a hook that changes a later NPC is seen.
func forEachClanMember(w *world.State, caller clanMember, hostileOnly bool, fn func(called clanMember)) {
	if w == nil {
		return
	}
	t := caller.clanTemplate()
	var near []clanMember
	w.ForEachKnownInRadius(caller, t.ClanRange, func(o world.Tracked) {
		switch n := o.(type) {
		case *Hostile:
			near = append(near, n)
		case *Folk:
			if !hostileOnly {
				near = append(near, n)
			}
		}
	})
	for _, called := range near {
		if called.Dead() || called.ObjectID() == caller.ObjectID() {
			continue
		}
		ct := called.clanTemplate()
		if !sharesClan(t.Clans, ct.Clans) || slices.Contains(ct.IgnoredIDs, caller.NpcID()) {
			continue
		}
		if !caller.seesClanMember(called) {
			continue
		}
		fn(called)
	}
}

// sharesClan reports whether a and b have a clan name in common.
func sharesClan(a, b []string) bool {
	for _, name := range b {
		if slices.Contains(a, name) {
			return true
		}
	}
	return false
}
