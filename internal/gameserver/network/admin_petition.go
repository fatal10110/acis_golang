package network

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// petitionListLimit is how many petitions one page of the list shows.
const petitionListLimit = 7

const (
	petitionUsage          = "Usage: //petition [join|reject|reset|show|unfollow|view]"
	petitionUnfollowButton = `<td><button value="Unfollow" action="bypass -h admin_petition unfollow" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td>`
	petitionButtons        = `<center><img src="L2UI.SquareGray" width=280 height=1><br><table width=130><tr><td><button value="Join" action="bypass -h admin_petition join %id%" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td><td><button value="Reject" action="bypass -h admin_petition reject %id%" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td></tr></table></center>`
	petitionFeedback       = `<center><img src="L2UI.SquareGray" width=280 height=1><br><table width=280><tr><td>Rate: %rate%</td></tr><tr><td>Feedback: %feedback%</td></tr></table></center>`
)

// adminPetition answers //petition: the petition list, page 1 or the page
// named, after one of the actions join, reject, reset, show and unfollow
// on a petition; view opens one petition instead.
func (l *GameClientLink) adminPetition(gm *livePlayer, line string) {
	args := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' })[1:]
	page := int32(1)
	if len(args) > 0 {
		if isDigits(args[0]) {
			var ok bool
			if page, ok = parseJavaInt(args[0]); !ok {
				// A page number past the int range opens nothing.
				return
			}
		} else if !l.adminPetitionAction(gm, args[0], args[1:]) {
			return
		}
	}
	l.showPetitions(gm, int(page))
}

// adminPetitionAction runs //petition's action on gm's behalf. It reports
// whether the petition list follows.
func (l *GameClientLink) adminPetitionAction(gm *livePlayer, action string, args []string) bool {
	id := func() (int32, bool) {
		if len(args) == 0 {
			return 0, false
		}
		return parseJavaInt(args[0])
	}
	pres := petitionPresence{l}
	switch action {
	case "join", "reject", "show", "view":
		petitionID, ok := id()
		if !ok {
			sendText(gm, petitionUsage)
			return true
		}
		switch action {
		case "join":
			ok, notices := l.petitions.Join(petitionPerson(gm), petitionID, false, pres)
			l.deliverPetition(notices)
			if !ok {
				gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotUnderPetitionConsultation))
			}
		case "reject":
			ok, notices := l.petitions.Reject(petitionPerson(gm), petitionID, pres)
			l.deliverPetition(notices)
			if !ok {
				gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedCancelPetitionTryLater))
			}
		case "show":
			for _, msg := range l.petitions.Log(petitionID) {
				gm.SendFrame(framePetitionSay(msg))
			}
		case "view":
			l.showPetition(gm, petitionID)
			return false
		}
	case "reset":
		if !l.petitions.Reset() {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetitionUnderProcess))
			return false
		}
	case "unfollow":
		l.deliverPetition(l.petitions.Unfollow(petitionPerson(gm), pres))
	default:
		sendText(gm, petitionUsage)
	}
	return true
}

// adminPetitionChat answers //add_peti_chat: gm's target joins the chat of
// the petition gm answers.
func (l *GameClientLink) adminPetitionChat(gm *livePlayer, _ string) {
	target := adminTargetPlayer(gm, false)
	if target == nil || target.detached() {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClientNotLoggedOntoGameServer))
		return
	}
	result, notices := l.petitions.AddToChat(petitionPerson(gm), petitionPerson(target), petitionPresence{l})
	if result != petition.Added {
		gm.SendFrame(serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessagePetitionAddingS1FailedErrorNumberS2, target.Name, int32(result)))
		return
	}
	l.deliverPetition(notices)
}

// adminForcePetition answers //force_peti: gm opens a petition for its
// target and answers it.
func (l *GameClientLink) adminForcePetition(gm *livePlayer, _ string) {
	target := adminTargetPlayer(gm, false)
	if target == nil || target.detached() {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClientNotLoggedOntoGameServer))
		return
	}
	result, id, err := l.petitions.Force(petitionPerson(gm), petitionPerson(target))
	switch result {
	case petition.ForceSelf:
		gm.SendFrame(serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessagePetitionFailedForS1ErrorNumberS2, target.Name, 1))
		return
	case petition.ForceAlreadySubmitted:
		gm.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessagePetitionFailedS1AlreadySubmitted, target.Name))
		return
	case petition.ForceNoID:
		l.log.Error().Err(err).Int32("object_id", target.ObjectID()).Msg("admin: //force_peti")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.tellGMsOfPetition(target.ObjectID(), target.Name+" has submitted a new petition.")
	ok, notices := l.petitions.Join(petitionPerson(gm), id, true, petitionPresence{l})
	l.deliverPetition(notices)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotUnderPetitionConsultation))
	}
}

// showPetitions opens page of the petition list on gm. The petition gm
// answers is listed without a link, and an unfollow button leaves it. A
// page number below 1 opens nothing while there are petitions.
func (l *GameClientLink) showPetitions(gm *livePlayer, page int) {
	answering, isAnswering := l.petitions.InProcess(gm.ObjectID())
	list := l.petitions.List()
	pg, ok := handleradmin.Paginate(len(list), page, petitionListLimit)
	if !ok {
		l.log.Warn().Int("page", page).Msg("admin: //petition page out of range")
		return
	}
	var b strings.Builder
	for i, p := range list[pg.Start:pg.End] {
		name, status := p.PetitionerName, "4"
		if petitioner, ok := l.livePlayerByID(p.Petitioner); ok && !petitioner.detached() {
			name, status = petitioner.Name, "1"
		}
		read := "QuestWndInfoIcon_5"
		if !p.Unread {
			read = "party_styleicon1_2"
		}
		if i%2 == 0 {
			b.WriteString("<table width=280 height=40 bgcolor=000000>")
		} else {
			b.WriteString("<table width=280 height=40>")
		}
		fmt.Fprintf(&b, `<tr><td width=20 align=center><img src="L2UI_CH3.msnicon%s" width=12 height=16><img src="L2UI_CH3.%s" width=11 height=16></td>`, status, read)
		details := fmt.Sprintf(`<br1><font color=B09878>Type:</font> %s <font color=B09878>State:</font> %s</td>`, p.Type, p.State)
		if isAnswering && answering == p.ID {
			fmt.Fprintf(&b, "<td width=260>#%d by %s%s", p.ID, name, details)
		} else {
			fmt.Fprintf(&b, `<td width=260><a action="bypass -h admin_petition view %d">#%d by %s</a>%s`, p.ID, p.ID, name, details)
		}
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 41)
	pg.Links(&b, "bypass admin_petition %page%")
	unfollow := ""
	if isAnswering {
		unfollow = petitionUnfollowButton
	}
	html := l.adminHTML("petitions.htm")
	html = strings.ReplaceAll(html, "%unfollow%", unfollow)
	html = strings.ReplaceAll(html, "%content%", b.String())
	sendFilledHTML(gm, 0, html, 0)
}

// showPetition opens petition id on gm and marks it read. A closed
// petition shows the petitioner's rating, an active one the join and
// reject buttons.
func (l *GameClientLink) showPetition(gm *livePlayer, id int32) {
	if !gm.accessLevel().IsGM {
		return
	}
	d, ok := l.petitions.Read(id)
	if !ok {
		return
	}
	status := "offline"
	if petitioner, ok := l.livePlayerByID(d.Petitioner); ok && !petitioner.detached() {
		status = "online"
	}
	html := l.adminHTML("petition.htm")
	for _, r := range []struct{ key, value string }{
		{"%submitDate%", time.UnixMilli(d.SubmitDate).Format("02-01-2006 15:04")},
		{"%petitionerName%", d.PetitionerName},
		{"%petitionerStatus%", status},
		{"%type%", d.Type.String()},
		{"%state%", d.State.String()},
		{"%responders%", d.Responders},
		{"%content%", commons.StripLinkWords(commons.HTMLValue(d.Content))},
	} {
		html = strings.ReplaceAll(html, r.key, r.value)
	}
	switch d.State {
	case petition.Pending, petition.Accepted:
		html = strings.ReplaceAll(html, "%buttonsOrFeedback%", petitionButtons)
	case petition.Closed:
		html = strings.ReplaceAll(html, "%buttonsOrFeedback%", petitionFeedback)
		html = strings.ReplaceAll(html, "%rate%", d.Rate.Desc())
		html = strings.ReplaceAll(html, "%feedback%", commons.StripLinkWords(commons.HTMLValue(d.Feedback)))
	default:
		html = strings.ReplaceAll(html, "%buttonsOrFeedback%", "")
	}
	html = strings.ReplaceAll(html, "%id%", strconv.Itoa(int(id)))
	sendFilledHTML(gm, 0, html, 0)
}

// adminHTML returns admin panel page name as set, before its placeholders
// are filled: the page limited, or the missing-page notice when there is
// none.
func (l *GameClientLink) adminHTML(name string) string {
	return l.setPage("data/html/admin/" + name)
}
