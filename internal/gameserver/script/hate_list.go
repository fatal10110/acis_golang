package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// HateList is a hostile NPC's hate list: the creatures its spells are
// aimed at, each with an amount of hate. It is separate from the hate the
// NPC's attacks follow (MostHated, StopHate). A behavior reaches it
// through NPC.HateList.
type HateList struct{ h *npc.Hostile }

// HateList returns the NPC's hate list. Only a hostile NPC keeps one; any
// other NPC panics.
func (n *NPC) HateList() HateList { return HateList{h: n.hostile("read the hate list")} }

// Add adds amount to the hate of c, entering c at amount when it is not
// listed yet. Nil adds nothing, and so does one siege guard's hate of
// another.
func (l HateList) Add(c Creature, amount float64) {
	l.h.AI().Hates().Add(combatantOf(c), amount)
}

// Clear empties the list. It touches no desire.
func (l HateList) Clear() { l.h.AI().Hates().Clear() }

// Empty reports whether the list holds no creature.
func (l HateList) Empty() bool { return l.h.AI().Hates().IsEmpty() }

// MostHated returns the listed creature with the most hate, nil when the
// list is empty or the NPC is dead.
func (l HateList) MostHated() Creature {
	e, ok := l.h.AI().Hates().MostHated()
	if !ok {
		return nil
	}
	return creatureOf(e.Attacker)
}

// Refresh drops the creatures that are dead or play dead, and the ones the
// NPC no longer knows or sees. It touches no desire.
func (l HateList) Refresh() { l.h.AI().Hates().Refresh(l.h.Knows) }

// RemoveIfOutOfRange drops the creatures standing distance or farther from
// the NPC, in 3D, and the cast desires aimed at them.
func (l HateList) RemoveIfOutOfRange(distance int) { l.h.AI().RemoveHatesOutOfRange(distance) }

// Muted reports whether an effect keeps the NPC from casting its magic
// skills. Only a hostile NPC is asked; any other NPC panics.
func (n *NPC) Muted() bool {
	return n.hostile("check a mute").EffectList().IsAffected(effect.FlagMuted)
}
