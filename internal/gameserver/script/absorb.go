package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// AbsorbInfo is what a monster records of one player charging a soul
// crystal on it.
type AbsorbInfo = npc.AbsorbInfo

// Monster reports whether the NPC is a monster: a hostile NPC of the
// monster family (monsters, chests, raid and grand bosses, festival
// monsters, feedable beasts), not a guard.
func (n *NPC) Monster() bool {
	h, ok := n.combatant().(*npc.Hostile)
	return ok && h.MonsterKind()
}

// Dead reports whether the NPC is dead; a handle on nothing reports true.
func (n *NPC) Dead() bool {
	c := n.combatant()
	return c == nil || c.Dead()
}

// Level returns the NPC's level; a handle on nothing panics.
func (n *NPC) Level() int32 {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return int32(o.Level())
	case *npc.Folk:
		return int32(o.Instance.Template.Level)
	}
	panic("script: the level of a handle on nothing")
}

// AddAbsorber records that p used the soul crystal crystalObjectID on the
// monster. Anything but a monster records nothing.
func (n *NPC) AddAbsorber(p *Player, crystalObjectID int32) {
	if h, ok := n.combatant().(*npc.Hostile); ok && h.MonsterKind() {
		h.AddAbsorber(p.ObjectID(), crystalObjectID)
	}
}

// Absorber returns what the monster records of p's soul crystal charge,
// false when it records nothing.
func (n *NPC) Absorber(p *Player) (AbsorbInfo, bool) {
	h, ok := n.combatant().(*npc.Hostile)
	if !ok {
		return AbsorbInfo{}, false
	}
	return h.Absorber(p.ObjectID())
}
