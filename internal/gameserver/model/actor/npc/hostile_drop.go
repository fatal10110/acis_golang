package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// Rewards is a hostile NPC's death rewards: the payout computed when it
// dies, and the drop of a given item on the ground around it that a script
// asks for while it lives.
type Rewards interface {
	creature.Rewarder
	// DropItem drops count units of itemID on the ground around the NPC,
	// reserved for to while to acts for a player.
	DropItem(to attackable.Combatant, itemID int32, count int)
}

// DropItem drops count units of itemID on the ground around the NPC,
// reserved for to while to acts for a player. It panics on an NPC attached
// with no Rewards: a script bound to an NPC must find every operation it
// uses there.
func (h *Hostile) DropItem(to attackable.Combatant, itemID int32, count int) {
	if h.rewards == nil {
		panic("npc: DropItem on a hostile attached with no Rewards")
	}
	h.rewards.DropItem(to, itemID, count)
}
