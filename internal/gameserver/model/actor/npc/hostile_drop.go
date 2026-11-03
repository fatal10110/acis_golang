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

// RewardsSource gives a hostile NPC its death rewards each time it needs
// them, so an NPC whose rewards hold nothing of their own keeps none while
// it lives.
type RewardsSource interface {
	RewardsFor(h *Hostile) Rewards
}

// fixedRewards is the source of rewards an NPC was attached with.
type fixedRewards struct{ r Rewards }

func (f fixedRewards) RewardsFor(*Hostile) Rewards { return f.r }

// deathRewards returns h's death rewards, nil for an NPC attached with
// none.
func (h *Hostile) deathRewards() Rewards {
	if h.rewards == nil {
		return nil
	}
	return h.rewards.RewardsFor(h)
}

// DropItem drops count units of itemID on the ground around the NPC,
// reserved for to while to acts for a player. It panics on an NPC attached
// with no Rewards: a script bound to an NPC must find every operation it
// uses there.
func (h *Hostile) DropItem(to attackable.Combatant, itemID int32, count int) {
	rewards := h.deathRewards()
	if rewards == nil {
		panic("npc: DropItem on a hostile attached with no Rewards")
	}
	rewards.DropItem(to, itemID, count)
}
