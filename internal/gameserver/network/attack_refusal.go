package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// attackRefusalMessage is the system message an attack think refused by the
// attack gate sends its player.
func attackRefusalMessage(reason event.AttackRefusal) int {
	switch reason {
	case event.AttackRefusedFishingRod:
		return serverpackets.SystemMessageCannotAttackWithFishingPole
	case event.AttackRefusedNoArrows:
		return serverpackets.SystemMessageNotEnoughArrows
	case event.AttackRefusedCantSeeTarget:
		return serverpackets.SystemMessageCantSeeTarget
	case event.AttackRefusedInPeaceZone:
		return serverpackets.SystemMessageCantAtkPeacezone
	case event.AttackRefusedTargetInPeaceZone:
		return serverpackets.SystemMessageTargetInPeacezone
	default:
		return serverpackets.SystemMessageNotEnoughMP
	}
}
