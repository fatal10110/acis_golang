package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) handleSummonActionUse(live *livePlayer, req clientpackets.RequestActionUse) bool {
	command, ok := summonCommandForActionID(req.ActionID)
	if !ok || l.world == nil {
		return false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return true
	}
	actor, ok := obj.(*summon.Actor)
	if !ok {
		return true
	}
	summonType := actor.SummonType()
	objectID := actor.ObjectID()
	result := actor.ApplyCommand(summon.CommandContext{Command: command, World: l.world})
	if id, ok := systemMessageForSummonFeedback(result.Feedback); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(id))
	}
	if result.Outcome == summon.OutcomeApplied && (command == summon.CommandReturnPet || command == summon.CommandUnsummonServitor) {
		live.SendFrame(serverpackets.FramePetDelete(summonType, objectID))
	}
	return true
}

func summonCommandForActionID(actionID int32) (summon.Command, bool) {
	switch actionID {
	case 15, 21:
		return summon.CommandToggleFollow, true
	case 16, 22:
		return summon.CommandAttack, true
	case 17, 23:
		return summon.CommandStop, true
	case 19:
		return summon.CommandReturnPet, true
	case 52:
		return summon.CommandUnsummonServitor, true
	case 53, 54:
		return summon.CommandMoveToTarget, true
	default:
		return 0, false
	}
}

func systemMessageForSummonFeedback(feedback summon.Feedback) (int, bool) {
	switch feedback {
	case summon.FeedbackPetRefusingOrder:
		return serverpackets.SystemMessagePetRefusingOrder, true
	case summon.FeedbackDeadPetCannotBeReturned:
		return serverpackets.SystemMessageDeadPetCannotBeReturned, true
	case summon.FeedbackPetCannotBeSentBackDuringBattle:
		return serverpackets.SystemMessagePetCannotSentBackDuringBattle, true
	case summon.FeedbackCannotRestoreHungryPet:
		return serverpackets.SystemMessageYouCannotRestoreHungryPets, true
	case summon.FeedbackPetTooHighToControl:
		return serverpackets.SystemMessagePetTooHighToControl, true
	default:
		return 0, false
	}
}
