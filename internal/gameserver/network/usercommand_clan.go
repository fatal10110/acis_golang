package network

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// clanPenaltyPage is the /clanpenalty window; %content% takes its rows.
const clanPenaltyPage = "data/html/clan_penalty.htm"

// siegeStatusPage is the /siegestatus window: %kills% and %deaths% take the
// clan's siege counts, %content% a row per member in the world.
const siegeStatusPage = "data/html/siege_status.htm"

// The /siegestatus member statuses: on the battlefield of the siege, or
// not.
const (
	siegeStatusInside  = "Castle Siege in Progress"
	siegeStatusOutside = "Outside Castle Siege Zone"
)

// clanPenaltyNone is the one row of a report with no penalty.
const clanPenaltyNone = "<tr><td width=170>No penalty is imposed.</td><td width=100 align=center></td></tr>"

// clanPenaltyLabels names each penalty report line.
var clanPenaltyLabels = map[clan.PenaltyKind]string{
	clan.PenaltyJoinClan:         "Unable to join a clan.",
	clan.PenaltyCreateClan:       "Unable to create a clan.",
	clan.PenaltyInviteMember:     "Unable to invite a clan member.",
	clan.PenaltyJoinAlliance:     "Unable to join an alliance.",
	clan.PenaltyInviteAllyMember: "Unable to invite a new alliance member.",
	clan.PenaltyCreateAlliance:   "Unable to create an alliance.",
	clan.PenaltyDissolving:       "The request to dissolve the clan is currently being processed.  (Restrictions are now going to be imposed on the use of clan functions.)",
}

// warListMessages are, per war listing command, the kind it lists, its
// header, and what it says when the listing is empty.
var warListMessages = map[int32]struct {
	kind          clan.WarListKind
	header, empty int
}{
	userCommandAttackList:      {clan.WarsDeclared, serverpackets.SystemMessageClansYouDeclaredWarOn, serverpackets.SystemMessageYouArentInClanWars},
	userCommandUnderAttackList: {clan.WarsAgainst, serverpackets.SystemMessageClansThatHaveDeclaredWarOnYou, serverpackets.SystemMessageNoClanWarsVsYou},
	userCommandWarList:         {clan.WarsMutual, serverpackets.SystemMessageWarList, serverpackets.SystemMessageNotInvolvedInWar},
}

// userCommandClanWarsList (/attacklist, /underattacklist, /warlist) lists
// the clans at war with live's clan, by direction. A clanless player is
// refused.
//
// The first clan of a non-empty listing is never named: the header goes
// out for it, then the entries from the second on, then the footer.
func (l *GameClientLink) userCommandClanWarsList(live *livePlayer, id int32) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	msgs := warListMessages[id]
	entries := l.clanService().WarListing(cl, msgs.kind)
	if len(entries) == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(msgs.empty))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msgs.header))
	for _, e := range entries[1:] {
		if e.AllyID > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1S2Alliance,
				serverpackets.TextParam(e.Name), serverpackets.TextParam(e.AllyName)))
			continue
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1NoAllianceExists, e.Name))
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListFooter))
}

// userCommandClanPenalty (/clanpenalty) opens the window listing the
// penalties live and its clan are under, each with its expiry date.
func (l *GameClientLink) userCommandClanPenalty(live *livePlayer, _ int32) {
	var rows strings.Builder
	for _, p := range l.clanService().PenaltyReport(live.Character, time.Now().UnixMilli(), l.sieges) {
		if p.Kind == clan.PenaltyNoDissolve {
			rows.WriteString("<tr><td width=170>Unable to dissolve a clan.</td><td></td></tr>")
			continue
		}
		fmt.Fprintf(&rows, "<tr><td width=170>%s</td><td width=100 align=center>%s</td></tr>",
			clanPenaltyLabels[p.Kind], time.UnixMilli(p.Expiry).Format("2006-01-02"))
	}
	content := rows.String()
	if content == "" {
		content = clanPenaltyNone
	}
	sendFilledHTML(live, 0, strings.ReplaceAll(l.setPage(clanPenaltyPage), "%content%", content), 0)
}

// userCommandSiegeStatus (/siegestatus) shows a noble clan leader its
// members' places in the siege its clan fights: the first siege in
// progress, in castle order, its clan attacks, defends or owns opens the
// window with the clan's siege kills and deaths and a row per member in
// the world, telling whether it stands on that siege's battlefield. Anyone
// but a noble clan leader is refused, and a clan fighting no siege is
// told the window only opens during one.
func (l *GameClientLink) userCommandSiegeStatus(live *livePlayer, _ int32) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || !cl.IsLeader(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderCanIssueCommands))
		return
	}
	if !live.IsNoble() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyNoblesseLeaderCanViewSiegeStatusWindow))
		return
	}
	for _, s := range l.sieges.All() {
		if !s.InProgress() || !s.CheckSides(cl.ID(), siege.SideAttacker, siege.SideDefender, siege.SideOwner) {
			continue
		}
		field := s.Battlefield()
		var rows strings.Builder
		for _, member := range l.onlineClanMembers(cl, 0) {
			status := siegeStatusOutside
			if field != nil && member.zoneActor != nil && field.Inside(member.zoneActor) {
				status = siegeStatusInside
			}
			fmt.Fprintf(&rows, "<tr><td width=170>%s</td><td width=100>%s</td></tr>", member.CharacterName(), status)
		}
		page := l.setPage(siegeStatusPage)
		page = strings.ReplaceAll(page, "%kills%", strconv.Itoa(l.sieges.Kills(cl.ID())))
		page = strings.ReplaceAll(page, "%deaths%", strconv.Itoa(l.sieges.Deaths(cl.ID())))
		page = strings.ReplaceAll(page, "%content%", rows.String())
		sendFilledHTML(live, 0, page, 0)
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyDuringSiege))
}
