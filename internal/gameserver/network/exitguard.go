package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// exitBlock enumerates the reasons the restart and logout handlers refuse to
// end a play session, in the order they are checked.
type exitBlock int

const (
	exitAllowed exitBlock = iota
	exitBlockEnchant
	exitBlockNoRestartZone
	exitBlockAttackStance
)

// exitBlockReason reports why a live player may not restart or log out right
// now. An active enchant selection refuses silently; every later reason
// carries its own system message.
//
// The reference also refuses while the character's subclass lock is held and
// while an initialized festival of darkness holds the player; neither system
// is ported yet, so both conditions are unreachable here.
func (l *GameClientLink) exitBlockReason(live *livePlayer) exitBlock {
	if l.enchantStateStore().Active(live.ObjectID()) != 0 {
		return exitBlockEnchant
	}
	if live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagNoRestart) {
		return exitBlockNoRestartZone
	}
	if l.attackStance != nil && l.attackStance.InAttackStance(live) {
		return exitBlockAttackStance
	}
	return exitAllowed
}

// refuseExit answers a refused restart or logout with the reason's system
// message followed by the flow's rejection frame, leaving the session in
// game.
func (l *GameClientLink) refuseExit(session *Session, live *livePlayer, block exitBlock, restart bool) {
	if msgID, ok := block.systemMessage(restart); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(msgID))
	}
	if restart {
		session.SendFrame(serverpackets.FrameRestartResponse(false))
		return
	}
	session.SendFrame(serverpackets.FrameActionFailed())
}

func (b exitBlock) systemMessage(restart bool) (int, bool) {
	switch b {
	case exitBlockNoRestartZone:
		if restart {
			return serverpackets.SystemMessageNoRestartHere, true
		}
		return serverpackets.SystemMessageNoLogoutHere, true
	case exitBlockAttackStance:
		if restart {
			return serverpackets.SystemMessageCannotRestartWhileFighting, true
		}
		return serverpackets.SystemMessageCannotLogoutWhileFighting, true
	default:
		return 0, false
	}
}
