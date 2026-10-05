package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// petitionSystemName is who the game masters hear petition news from.
const petitionSystemName = "Petition System"

// petitionPresence answers the petition manager from the world.
type petitionPresence struct{ l *GameClientLink }

func (p petitionPresence) Online(objectID int32) bool {
	live, ok := p.l.livePlayerByID(objectID)
	return ok && !live.detached()
}

func (p petitionPresence) GM(objectID int32) bool {
	live, ok := p.l.livePlayerByID(objectID)
	return ok && !live.detached() && live.accessLevel().IsGM
}

func petitionPerson(live *livePlayer) petition.Person {
	return petition.Person{ID: live.ObjectID(), Name: live.Name}
}

// deliverPetition sends each notice to its recipient still in the world,
// in order.
func (l *GameClientLink) deliverPetition(notices []petition.Notice) {
	for _, n := range notices {
		to, ok := l.livePlayerByID(n.To)
		if !ok {
			continue
		}
		to.SendFrame(petitionFrame(n))
	}
}

// petitionFrame is the packet n stands for.
func petitionFrame(n petition.Notice) wire.Frame {
	switch n.Kind {
	case petition.Say:
		return framePetitionSay(n.Say)
	case petition.VoteWindow:
		return serverpackets.FramePetitionVote()
	case petition.Participating:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ParticipatePetition, n.Name)
	case petition.LeftChat:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1LeftPetitionChat, n.Name)
	case petition.FailedAdding:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageFailedAddingS1ToPetition, n.Name)
	case petition.ConsultationReceived:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ReceivedConsultationRequest, n.Name)
	case petition.ReceivedCode:
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessagePetitionS1ReceivedCodeIsS2, n.Name, n.Number)
	case petition.ApplicationAccepted:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionAppAccepted)
	case petition.UnderWay:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessagePetitionWithS1UnderWay, n.Name)
	case petition.EndedWith:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessagePetitionEndedWithS1, n.Name)
	case petition.ReceiptCancelled:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageReceiptNoS1Canceled, n.Number)
	default: // petition.ProvideFeedback
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageThisEndThePetitionPleaseProvideFeedback)
	}
}

func framePetitionSay(msg petition.Message) wire.Frame {
	return serverpackets.FrameCreatureSay(msg.ObjectID, int32(msg.Channel), msg.Name, msg.Text)
}

// tellGMsOfPetition sends every game master on the GM list, hidden ones
// too, a petition news line about petitioner.
func (l *GameClientLink) tellGMsOfPetition(petitioner int32, text string) {
	gms := l.gms.Entries(true)
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameCreatureSay(petitioner, int32(chat.HeroVoice), petitionSystemName, text)
	}, func(send func(frameReceiver)) {
		for _, gm := range gms {
			send(gm.Player)
		}
	})
}

// requestPetition sends live's petition to the game masters. It is refused
// while no game master is listed, when petitioning is off, while live has
// a petition active, when the server or live has as many as allowed, and
// for a text over 255 characters.
func (l *GameClientLink) requestPetition(live *livePlayer, req clientpackets.RequestPetition) {
	if !l.gms.Online(false) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoGMProvidingServiceNow))
		live.SendFrame(serverpackets.FramePlaySound("systemmsg_e.702"))
		return
	}
	s, err := l.petitions.Submit(petitionPerson(live), req.Type, req.Content)
	switch s.Result {
	case petition.SubmitDisabled:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageGameClientUnableToConnectToPetitionServer))
	case petition.SubmitAlreadyActive:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyOneActivePetitionAtTime))
	case petition.SubmitServerFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionSystemCurrentUnavailable))
	case petition.SubmitPlayerLimit:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageWeHaveReceivedS1PetitionsToday, int32(s.PlayerCount)))
	case petition.SubmitTooLong:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionMaxChars255))
	case petition.SubmitBadType, petition.SubmitNoID:
		// A type the client cannot name is a malformed request; the
		// reference answers it with nothing, but the petition window
		// still waits on an answer.
		l.log.Warn().Err(err).Int32("object_id", live.ObjectID()).Int32("type", req.Type).Msg("petition refused")
		live.SendFrame(serverpackets.FrameActionFailed())
	case petition.Submitted:
		if s.NotifyGMs {
			l.tellGMsOfPetition(live.ObjectID(), live.Name+" has submitted a new petition.")
		}
		maxPerPlayer := l.petitions.Config().MaxPerPlayer
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessagePetitionAcceptedRecentNoS1, s.ID))
		live.SendFrame(serverpackets.FrameSystemMessageTwoNumbers(serverpackets.SystemMessageSubmittedYourS1ThPetitionS2Left, int32(s.PlayerCount), int32(maxPerPlayer-s.PlayerCount)))
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1PetitionOnWaitingList, int32(s.ServerCount)))
	}
}

// requestPetitionCancel cancels live's pending petition. A petition being
// answered cannot be cancelled by its petitioner; a game master answering
// one closes it instead, and any other responder leaves its chat.
func (l *GameClientLink) requestPetitionCancel(live *livePlayer) {
	c, notices := l.petitions.Cancel(petitionPerson(live), live.accessLevel().IsGM, petitionPresence{l})
	l.deliverPetition(notices)
	switch c.Result {
	case petition.CancelUnderProcess:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionUnderProcess))
	case petition.CancelNotSubmitted:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionNotSubmitted))
	case petition.CancelDone:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessagePetitionCanceledSubmitS1MoreToday, int32(c.Remaining)))
		if c.NotifyGMs {
			l.tellGMsOfPetition(live.ObjectID(), live.Name+" has canceled a pending petition.")
		}
	}
}

// petitionVote records live's rating of its closed petition. Without a
// closed petition awaiting it, or with a rate the client cannot name, it
// is dropped without an answer as in the reference: the client closes its
// feedback window when it sends the vote and waits on nothing.
func (l *GameClientLink) petitionVote(live *livePlayer, req clientpackets.PetitionVote) {
	l.petitions.Vote(live.ObjectID(), req.Rate, strings.TrimFunc(req.Feedback, javaSpace))
}

// chatPetition says live's line in the chat of the petition it sent or
// answers.
func (l *GameClientLink) chatPetition(_ *Client, live *livePlayer, line chat.Line) {
	notices, ok := l.petitions.Say(petitionPerson(live), live.accessLevel().IsGM, line.Text, petitionPresence{l})
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotInPetitionChat))
		return
	}
	l.deliverPetition(notices)
}

// enterWorldPetition shows a player entering the world the chat of the
// petition it sent or answers that is still active.
func (l *GameClientLink) enterWorldPetition(client *Client, live *livePlayer) {
	for _, msg := range l.petitions.ActiveLog(live.ObjectID()) {
		client.Session.SendFrame(framePetitionSay(msg))
	}
}
