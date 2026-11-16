package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/rs/zerolog"
)

// Rewards is a hostile NPC's death rewards: the payout computed when it
// dies.
type Rewards interface {
	creature.Rewarder
}

// RewardsSource gives a hostile NPC its death rewards each time it needs
// them, so an NPC whose rewards hold nothing of their own keeps none while
// it lives.
type RewardsSource interface {
	RewardsFor(h *Hostile) Rewards
}

// ItemDropper places the items a script drops around a live NPC.
type ItemDropper interface {
	// DropItem drops count units of itemID on the ground around h,
	// reserved for to while to acts for a player.
	DropItem(h *Hostile, to attackable.Combatant, itemID int32, count int)
}

// fixedRewards is the source of rewards an NPC was attached with.
type fixedRewards struct{ r Rewards }

func (f fixedRewards) RewardsFor(*Hostile) Rewards { return f.r }

// deathRewards returns h's death rewards: nil for an NPC attached with
// none, and for any NPC but a Monster-family one, whose kill pays no
// experience, SP, drops or karma loss.
func (h *Hostile) deathRewards() Rewards {
	if h.rewards == nil || !h.MonsterKind() {
		return nil
	}
	return h.rewards.RewardsFor(h)
}

// DropItem drops count units of itemID on the ground around the NPC,
// reserved for to while to acts for a player. An NPC attached with no
// Drops drops nothing.
func (h *Hostile) DropItem(to attackable.Combatant, itemID int32, count int) {
	if h.drops == nil {
		h.logger().Warn().Int("npc_id", h.Instance.Template.ID).Int32("object_id", h.ObjectID()).
			Int32("item_id", itemID).Int("count", count).Msg("npc: script drop on an npc with no drops")
		return
	}
	h.drops.DropItem(h, to, itemID, count)
}

// Logger returns the logger this NPC logs through.
func (h *Hostile) Logger() *zerolog.Logger {
	return h.logger()
}
