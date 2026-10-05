package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// attackWeaponRefusalMessage is the system message a player's attack refused
// by its active weapon sends (PlayerAttack.canAttack).
func attackWeaponRefusalMessage(reason event.AttackWeaponRefusal) int {
	switch reason {
	case event.AttackRefusedFishingRod:
		return serverpackets.SystemMessageCannotAttackWithFishingPole
	case event.AttackRefusedNoArrows:
		return serverpackets.SystemMessageNotEnoughArrows
	default:
		return serverpackets.SystemMessageNotEnoughMP
	}
}
