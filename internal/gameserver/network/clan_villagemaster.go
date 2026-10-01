package network

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Pages a village master answers a leader nomination with.
const (
	clanPageNominated        = "data/html/script/feature/Clan/9000-07-success.htm"
	clanPageNominationActive = "data/html/script/feature/Clan/9000-07-in-progress.htm"
	clanPageCancelled        = "data/html/script/feature/Clan/9000-08-success.htm"
	clanPageNoNomination     = "data/html/script/feature/Clan/9000-08-no.htm"
)

// clanLevelUpSkillID is the visual a clan level-up shows around its leader.
const clanLevelUpSkillID = 5103

// mainClanOnlyText answers a leader nomination of a sub-unit member.
const mainClanOnlyText = "Selected member cannot be found in main clan."

// bypassArgs splits a dialog command on single spaces, dropping trailing
// empty words, and returns its first two arguments ("" when missing).
func bypassArgs(command string) (string, string) {
	words := strings.Split(command, " ")
	for len(words) > 0 && words[len(words)-1] == "" {
		words = words[:len(words)-1]
	}
	var first, second string
	if len(words) > 1 {
		first = words[1]
	}
	if len(words) > 2 {
		second = words[2]
	}
	return first, second
}

// villageMasterClan runs one of a village master's clan commands for live.
// A command missing its name argument does nothing.
func (l *GameClientLink) villageMasterClan(live *livePlayer, f *npc.Folk, command string) {
	arg, arg2 := bypassArgs(command)
	verb := strings.ToLower(strings.SplitN(command, " ", 2)[0])
	if l.villageMasterSubunit(live, verb, arg, arg2) {
		return
	}
	switch verb {
	case "create_clan":
		if arg != "" {
			l.createClan(live, arg)
		}
	case "increase_clan_level":
		l.raiseClanLevel(live)
	case "change_clan_leader":
		if arg != "" {
			l.nominateClanLeader(live, f, arg)
		}
	case "cancel_clan_leader_change":
		l.cancelClanLeaderNomination(live, f)
	case "learn_clan_skills":
		l.showPledgeSkillList(live)
	}
}

// createClan founds the clan name with live as its leader.
func (l *GameClientLink) createClan(live *livePlayer, name string) {
	cl, result := l.clanService().Create(live.Character, name, time.Now())
	switch result {
	case clan.Created:
		live.SendFrame(l.framePledgeMemberList(cl))
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanCreated))
	case clan.CreateLevelTooLow:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotMeetCriteriaToCreateClan))
	case clan.CreateAlreadyInClan, clan.CreateFailed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToCreateClan))
	case clan.CreateMustWait:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMustWaitBeforeCreatingClan))
	case clan.CreateNameInvalid:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanNameInvalid))
	case clan.CreateNameLength:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanNameLengthIncorrect))
	case clan.CreateNameTaken:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AlreadyExists, name))
	}
}

// clanLevelPayer pays a clan level's price out of live, reporting each
// payment as the reference's own adena, item and SP removals do.
type clanLevelPayer struct {
	l    *GameClientLink
	live *livePlayer
}

func (p clanLevelPayer) SP() int { return p.live.Character.ProgressionValues().SP }

func (p clanLevelPayer) PayAdena(count int) bool {
	inv := p.live.Inventory()
	if inv == nil || count > inv.Adena() {
		p.live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return false
	}
	if inv.DestroyByTemplateID(item.AdenaID, count) == nil {
		return false
	}
	p.live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(count)))
	return true
}

func (p clanLevelPayer) PayItem(itemID int32, count int) bool {
	inv := p.live.Inventory()
	if inv == nil || inv.ItemCount(itemID, -1, false) < count || inv.DestroyByTemplateID(itemID, count) == nil {
		p.live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return false
	}
	switch {
	case shadowTemplate(p.live, itemID):
		p.live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageRemainingManaIsNow0, itemID))
	case count > 1:
		p.live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageS2S1Disappeared, itemID, int32(count)))
	default:
		p.live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disappeared, itemID))
	}
	return true
}

func (p clanLevelPayer) TakeSP(sp int) {
	c := p.live.Character
	c.RemoveExpAndSp(p.l.levels, p.live.Template(), 0, sp)
}

// raiseClanLevel raises live's clan by one level for its price. A clanless
// player's request does nothing.
func (l *GameClientLink) raiseClanLevel(live *livePlayer) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	for _, n := range l.clanService().RaiseLevel(live.Character, clanLevelPayer{l: l, live: live}, time.Now()) {
		switch n := n.(type) {
		case clan.LevelNotLeader:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		case clan.LevelDissolving:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotRiseLevelWhileDissolving))
		case clan.LevelFailed:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToIncreaseClanLevel))
		case clan.ReputationChanged:
			l.sendReputationChange(cl, n, live)
		case clan.ReputationDeducted:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DeductedFromClanRep, int32(n.Points)))
		case clan.LevelRaised:
			// The leader's siege skills follow the level here once sieges
			// exist (#3150).
			if clan.TellsLeaderAboutReputation(n.Level) {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanCanAccumulateReputation))
			}
			// Queued behind the clan skill changes a reputation price may
			// have posted to each member.
			l.broadcastToClanQueued(cl, live,
				func() wire.Frame { return framePledgeShowInfoUpdate(cl) },
				func() wire.Frame {
					return serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanLevelIncreased)
				})
			// The visual follows those frames for a clan member watching.
			self := skillCastObject(live)
			l.broadcastLiveFrameAfterClanQueued(live, cl, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, clanLevelUpSkillID, 1, 0, 0, false)
			})
		}
	}
}

// nominateClanLeader names the member called name as live's clan's next
// leader.
func (l *GameClientLink) nominateClanLeader(live *livePlayer, f *npc.Folk, name string) {
	switch l.clanService().NominateLeader(live.Character, name) {
	case clan.NominateNotLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
	case clan.NominateUnknown:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DoesNotExist, name))
	case clan.NominateOffline:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvitedUserNotOnline))
	case clan.NominateOutsideMainClan:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, mainClanOnlyText))
	case clan.Nominated:
		l.sendClanPage(live, f, clanPageNominated)
	case clan.NominationPending:
		l.sendClanPage(live, f, clanPageNominationActive)
	}
}

// cancelClanLeaderNomination withdraws live's clan's leader nomination.
func (l *GameClientLink) cancelClanLeaderNomination(live *livePlayer, f *npc.Folk) {
	switch l.clanService().CancelNomination(live.Character) {
	case clan.CancelNotLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
	case clan.NominationCancelled:
		l.sendClanPage(live, f, clanPageCancelled)
	case clan.NoNomination:
		l.sendClanPage(live, f, clanPageNoNomination)
	}
}

// sendClanPage opens the clan dialog page path from f on live.
func (l *GameClientLink) sendClanPage(live *livePlayer, f *npc.Folk, path string) {
	if l.html == nil {
		return
	}
	page, ok := l.html.Get(path)
	if !ok {
		l.log.Warn().Str("path", path).Msg("clan: dialog page missing")
		return
	}
	sendValidatedHTML(live, f.ObjectID(), page, 0)
}
