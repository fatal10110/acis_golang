package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// itemDropper is the part of an NPC's death rewards that drops a given item
// on the ground around the NPC.
type itemDropper interface {
	DropItem(to attackable.Combatant, itemID int32, count int)
}

// DropItem drops count units of itemID on the ground around the NPC,
// reserved for to while to acts for a player. An NPC whose rewards cannot
// drop items drops nothing.
func (h *Hostile) DropItem(to attackable.Combatant, itemID int32, count int) {
	if d, ok := h.rewards.(itemDropper); ok {
		d.DropItem(to, itemID, count)
	}
}
