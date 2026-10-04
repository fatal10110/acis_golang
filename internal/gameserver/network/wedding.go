package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// weddingSpouse is a live player as the wedding manager reads it; ids
// numbers the adena a refund hands back.
type weddingSpouse struct {
	*livePlayer
	ids func() (int32, error)
}

// spouse is live as the wedding manager reads it.
func (l *GameClientLink) spouse(live *livePlayer) weddingSpouse {
	return weddingSpouse{livePlayer: live, ids: l.nextObjectID}
}

// Pay takes count adena from the player and names the amount spent; it
// takes and says nothing when the player holds less.
func (s weddingSpouse) Pay(count int) bool {
	if count <= 0 {
		return true
	}
	inv := s.Inventory()
	if inv == nil || inv.DestroyByTemplateID(item.AdenaID, count) == nil {
		return false
	}
	s.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(count)))
	return true
}

// Refund gives back count adena Pay took, named as earned.
func (s weddingSpouse) Refund(count int) {
	s.AddEarnedItem(item.AdenaID, count, s.ids)
}

// Female reports a female character.
func (s weddingSpouse) Female() bool { return s.Sex == player.SexFemale }

// Adena is the adena the player holds.
func (s weddingSpouse) Adena() int {
	inv := s.Inventory()
	if inv == nil {
		return 0
	}
	return inv.Adena()
}

// sendWeddingPage opens wedding page path of manager f on live, filled in.
// No ActionFailed follows it.
func (l *GameClientLink) sendWeddingPage(live *livePlayer, f *npc.Folk, path string) {
	sendFilledHTML(live, f.ObjectID(), l.wedding.Page(l.setPage(path), f.ObjectID()), 0)
}

// weddingGreeting is wedding manager f's answer to live's interact: the
// married menu, the pending request page or the request form.
func (l *GameClientLink) weddingGreeting(live *livePlayer, f *npc.Folk) {
	l.sendWeddingPage(live, f, l.wedding.Greeting(l.spouse(live)))
}

// weddingBypass runs command on wedding manager f for live: AskWedding
// <name> asks the named player online to marry, Divorce dissolves live's
// couple and GoToLove takes live to its spouse. Any other command answers
// nothing.
func (l *GameClientLink) weddingBypass(live *livePlayer, f *npc.Folk, command string) {
	switch {
	case strings.HasPrefix(command, "AskWedding"):
		l.askWedding(live, f, command)
	case strings.HasPrefix(command, "Divorce"):
		l.divorce(live)
	case strings.HasPrefix(command, "GoToLove"):
		l.goToLove(live)
	}
}

// askWedding has live ask the player named by command's second word to
// marry: the refusal page, or the request dialog shown to that player.
func (l *GameClientLink) askWedding(live *livePlayer, f *npc.Folk, command string) {
	words := strings.FieldsFunc(command, tokenizerSpace)
	if len(words) < 2 {
		l.sendWeddingPage(live, f, wedding.PageNotFound)
		return
	}
	partner, ok := l.livePlayerByName(words[1])
	if !ok {
		l.sendWeddingPage(live, f, wedding.PageNotFound)
		return
	}
	friends := l.relations.AreFriends(live.ObjectID(), partner.ObjectID())
	if page := l.wedding.Ask(l.spouse(live), l.spouse(partner), friends); page != "" {
		l.sendWeddingPage(live, f, page)
		return
	}
	partner.SendFrame(serverpackets.FrameConfirmDlgEngageRequest(wedding.RequestText(live.Name)))
}

// tokenizerSpace is the default delimiter set of a command's words: space,
// tab, newline, carriage return and form feed.
func tokenizerSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// divorce dissolves live's couple; each spouse online is told so, the one
// who asked first.
func (l *GameClientLink) divorce(live *livePlayer) {
	couple, ok := l.wedding.Divorce(live.ObjectID())
	if !ok {
		return
	}
	for _, id := range []int32{couple.RequesterID, couple.PartnerID} {
		if spouse, online := l.livePlayerByID(id); online {
			sendText(spouse, wedding.NoticeDivorced)
		}
	}
}

// goToLove teleports live next to its spouse, unless live is not married,
// the spouse is offline or the spouse's state refuses it.
func (l *GameClientLink) goToLove(live *livePlayer) {
	partnerID := l.wedding.PartnerID(live.ObjectID())
	if partnerID == 0 {
		sendText(live, wedding.NoticePartnerNotFound)
		return
	}
	partner, ok := l.livePlayerByID(partnerID)
	if !ok {
		sendText(live, wedding.NoticePartnerOffline)
		return
	}
	if notice := wedding.TeleportRefusal(partner.Character); notice != "" {
		sendText(live, notice)
		return
	}
	x, y, z := partner.Position()
	live.Character.TeleportTo(x, y, z, 20)
}

// engageAnswer is live's answer to a marriage request; answer 1 accepts.
func (l *GameClientLink) engageAnswer(live *livePlayer, answer int32) {
	online := func(id int32) (wedding.Spouse, bool) {
		requester, ok := l.livePlayerByID(id)
		if !ok {
			return nil, false
		}
		return l.spouse(requester), true
	}
	out := l.wedding.Answer(l.spouse(live), answer == 1, online)
	if out.Outcome == wedding.AnswerIgnored {
		return
	}
	requester := out.Requester.(weddingSpouse).livePlayer
	switch out.Outcome {
	case wedding.AnswerDeclined:
		sendText(live, wedding.NoticeDeclinedByYou)
		sendText(requester, wedding.NoticeDeclinedByPartner)
	case wedding.AnswerMarried:
		l.celebrateWedding(requester, live)
	case wedding.AnswerUnpaid:
		if out.RequesterShort {
			requester.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		}
		if out.PartnerShort {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		}
	case wedding.AnswerIgnored, wedding.AnswerVoid:
	}
}

// celebrateWedding finishes the wedding of requester and partner, who
// have each paid the price: each is congratulated, each shows the wedding march then
// the fireworks to everyone watching, and every player online hears of it.
func (l *GameClientLink) celebrateWedding(requester, partner *livePlayer) {
	sendText(requester, wedding.MarriedNotice(partner.Name))
	sendText(partner, wedding.MarriedNotice(requester.Name))
	for _, skillID := range []int32{wedding.MarchSkillID, wedding.FireworksSkillID} {
		for _, spouse := range []*livePlayer{requester, partner} {
			l.broadcastLiveFrame(spouse, func() wire.Frame {
				self := serverpackets.SkillCastObject{ObjectID: spouse.ObjectID(), Location: spouse.CurrentLocation()}
				return serverpackets.FrameMagicSkillUse(self, self, skillID, 1, 1, 0, false)
			})
		}
	}
	announceToOnline(l.world, wedding.MarriedAnnouncement(requester.Name, partner.Name), false)
}
