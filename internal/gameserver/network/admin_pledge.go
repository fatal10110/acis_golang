package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminPledgeUsage is what gm is told when //pledge names no action.
const adminPledgeUsage = "Usage: //pledge create|dismiss|info|level|rep|transfer"

// adminPledge answers //pledge <action> on the selected player, gm itself
// without one. Every action but create needs that player in a clan, which
// it then acts on:
//
//   - create <name> founds the clan name with the player as its leader,
//     whatever wait a clan it left or dissolved put on it;
//   - dismiss dissolves its clan at once;
//   - info shows gm the clan's view;
//   - level <0-8> sets its clan's level, up or down;
//   - rep <points> adds points, or takes them when negative, from the
//     reputation of a clan of level 5 or more (an action starting with
//     "rep");
//   - transfer makes the player its clan's leader (an action starting
//     with "transfer").
//
// gm is then shown the game menu, except after a refused transfer. The
// command runs on the player's queue, where its clan membership changes.
func (l *GameClientLink) adminPledge(gm *livePlayer, line string) {
	target := adminTargetPlayer(gm, true)
	args := javaTokens(line)[1:]
	onPlayer(gm, target, func() { l.runAdminPledge(gm, target, args) })
}

// runAdminPledge runs //pledge with args on target's queue.
func (l *GameClientLink) runAdminPledge(gm, target *livePlayer, args []string) {
	menu := func() { l.sendAdminFile(gm, "game_menu.htm") }
	if len(args) == 0 {
		sendText(gm, adminPledgeUsage)
		menu()
		return
	}
	action, args := args[0], args[1:]
	if action == "create" {
		l.adminPledgeCreate(gm, target, args)
		menu()
		return
	}
	cl, ok := l.clanService().ClanOf(target.Character)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetMustBeInClan))
		menu()
		return
	}
	switch {
	case action == "dismiss":
		l.destroyClan(cl, target, false)
		sendText(gm, "The clan is now disbanded.")
	case action == "info":
		gm.SendFrame(l.frameGMViewPledgeInfo(cl, target))
	case action == "level":
		l.adminPledgeLevel(gm, cl, args, menu)
		return
	case strings.HasPrefix(action, "rep"):
		l.adminPledgeReputation(gm, target, cl, args)
	case strings.HasPrefix(action, "transfer"):
		l.adminPledgeTransfer(gm, target, cl, menu)
		return
	}
	menu()
}

// adminPledgeCreate founds the clan args names with target as its leader,
// lifting target's wait before founding one and putting it back when the
// founding fails.
func (l *GameClientLink) adminPledgeCreate(gm, target *livePlayer, args []string) {
	if len(args) == 0 {
		sendText(gm, "Invalid string parameter for //pledge create.")
		return
	}
	name := args[0]
	c := target.Character
	expiry := c.ClanCreateExpiryTime()
	c.SetClanCreateExpiryTime(0)
	if l.createClan(target, name) {
		sendText(gm, "Clan "+name+" have been created. Clan leader is "+target.CharacterName()+".")
		return
	}
	c.SetClanCreateExpiryTime(expiry)
	sendText(gm, "There was a problem while creating the clan.")
}

// adminPledgeLevel sets cl to the level args names, then tells gm and runs
// menu once the clan has been shown the change.
func (l *GameClientLink) adminPledgeLevel(gm *livePlayer, cl *clan.Clan, args []string, menu func()) {
	var level int32
	ok := len(args) > 0
	if ok {
		level, ok = parseJavaInt(args[0])
	}
	switch {
	case !ok:
		sendText(gm, "Invalid number parameter for //pledge setlevel.")
	case level < 0 || level >= 9:
		sendText(gm, "This clan level is incorrect. Put a number between 0 and 8.")
	default:
		l.changeClanLevel(cl, int(level), func() {
			sendText(gm, "You have set clan "+cl.Name()+" to level "+strconv.Itoa(int(level)))
			menu()
		})
		return
	}
	menu()
}

// changeClanLevel sets cl's level to level, up or down. Its leader, when
// connected, holds the siege skills from level 4 on and loses them below,
// and from level 5 on is told its clan earns reputation; then every member
// in the world gets the clan's header and the level-up notice, and after
// runs. The leader's part runs on its queue.
func (l *GameClientLink) changeClanLevel(cl *clan.Clan, level int, after func()) {
	l.clanService().ChangeLevel(cl, level)
	l.ensureClanForums(cl.ID(), level)
	var leader *livePlayer
	if live, ok := l.livePlayerByID(cl.LeaderID()); ok && !live.clientDetached() {
		leader = live
	}
	l.thenOn(leader, func(live *livePlayer) {
		if level > 3 {
			l.giveSiegeSkills(live)
		} else {
			l.takeSiegeSkills(live)
		}
		if clan.TellsLeaderAboutReputation(level) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanCanAccumulateReputation))
		}
	}, func() {
		l.broadcastToClan(cl, 0,
			func() wire.Frame { return framePledgeShowInfoUpdate(cl) },
			func() wire.Frame {
				return serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanLevelIncreased)
			})
		after()
	})
}

// adminPledgeReputation adds the points args names to the reputation of
// cl, target's clan, and tells gm the new score; a clan below level 5 gets
// none.
func (l *GameClientLink) adminPledgeReputation(gm, target *livePlayer, cl *clan.Clan, args []string) {
	var points int32
	ok := len(args) > 0
	if ok {
		points, ok = parseJavaInt(args[0])
	}
	if !ok {
		sendText(gm, "Invalid number parameter for //pledge rep.")
		return
	}
	if cl.Level() < 5 {
		sendText(gm, "Only clans of level 5 or above may receive reputation points.")
		return
	}
	if change, changed := l.clanService().AddReputation(cl, int(points)); changed {
		l.sendReputationChange(cl, change, target)
	}
	verb, preposition := "removed ", "from "
	if points > 0 {
		verb, preposition = "added ", "to "
	}
	// The amount is the 32-bit absolute value: the lowest int stays
	// negative.
	amount := points
	if amount < 0 {
		amount = -amount
	}
	sendText(gm, "You "+verb+strconv.Itoa(int(amount))+" points "+preposition+cl.Name()+"'s reputation. Their current score is: "+strconv.Itoa(cl.Reputation()))
}

// adminPledgeTransfer makes target the leader of cl, its clan, then tells
// gm and runs menu once the clan has been shown the change. A target
// already leading it, or no longer on its roster, is refused with no menu.
func (l *GameClientLink) adminPledgeTransfer(gm, target *livePlayer, cl *clan.Clan, menu func()) {
	name := target.CharacterName()
	if cl.IsLeader(target.ObjectID()) {
		sendText(gm, name+" is already the Clan leader of "+cl.Name()+".")
		return
	}
	h, ok := l.clanService().SetLeader(cl, target.ObjectID())
	if !ok {
		sendText(gm, name+" can't be set as the new Clan leader of "+cl.Name()+".")
		return
	}
	l.showHandover(h, func() {
		sendText(gm, "You set "+h.Leader.Name+" as the new Clan leader of "+cl.Name()+".")
		menu()
	})
}

// frameGMViewPledgeInfo builds a game master's view of cl, the clan of
// viewed: its header and its whole roster, in the order of the hash table
// the roster is kept in.
func (l *GameClientLink) frameGMViewPledgeInfo(cl *clan.Clan, viewed *livePlayer) wire.Frame {
	info := cl.Info()
	members := cl.MembersInTableOrder()
	rows := make([]serverpackets.PledgeMemberListMember, len(members))
	for i, m := range members {
		rows[i] = l.memberRow(m)
		rows[i].HasSponsor = m.Sponsor != 0
	}
	return serverpackets.FrameGMViewPledgeInfo(viewed.CharacterName(), serverpackets.PledgeMemberList{
		ClanID: info.ID, PledgeName: info.Name, LeaderName: cl.SubunitLeaderName(clan.SubunitMain),
		CrestID: info.CrestID, Level: int32(info.Level), CastleID: info.CastleID, ClanHallID: info.HallID,
		Rank: int32(info.Rank), Reputation: int32(info.Reputation), Dissolving: info.DissolvingExpiry > 0,
		AllyID: info.AllyID, AllyName: info.AllyName, AllyCrestID: info.AllyCrestID, AtWar: info.AtWar,
		Members: rows,
	})
}
