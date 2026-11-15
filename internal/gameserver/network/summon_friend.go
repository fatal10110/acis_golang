package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sendSummonRefusal tells recipient why a summon-friend skill stopped; a
// silent refusal sends nothing.
func sendSummonRefusal(send frameSender, recipient *livePlayer, r player.SummonRefusal) {
	if frame, ok := summonRefusalFrame(r); ok {
		send(recipient, frame)
	}
}

// summonRefusalFrame is the system message telling r, false for an allowed
// summon or a silent refusal.
func summonRefusalFrame(r player.SummonRefusal) (wire.Frame, bool) {
	switch r.Reason {
	case player.SummonRefusedLocation:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotSummonFromLocation), true
	case player.SummonRefusedSelf:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseOnYourself), true
	case player.SummonRefusedDead:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeadCannotBeSummoned, r.Name), true
	case player.SummonRefusedOperating:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1OperatingCannotBeSummoned, r.Name), true
	case player.SummonRefusedCombat:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned, r.Name), true
	case player.SummonRefusedOlympiad:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotSummonOlympiadPlayers), true
	case player.SummonRefusedBlocked:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetInSummonBlockingArea), true
	case player.SummonRefusedBlockingArea:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1InSummonBlockingArea, r.Name), true
	case player.SummonRefusedPending:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AlreadySummoned, r.Name), true
	case player.SummonRefusedItem:
		return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1RequiredForSummoning, int32(r.ItemID)), true
	}
	return wire.Frame{}, false
}
