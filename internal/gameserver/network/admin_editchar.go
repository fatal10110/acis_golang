package network

import (
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminDebug answers //debug [name]: gm selects the named online player,
// else its selected player, else itself, and is shown its character page.
func (l *GameClientLink) adminDebug(gm *livePlayer, line string) {
	l.showCharInfo(gm, l.adminArgPlayer(gm, line))
}

// adminArgPlayer resolves the player a //debug or //party_info acts on: the
// online player its first argument names, else gm's selected player, else
// gm itself.
func (l *GameClientLink) adminArgPlayer(gm *livePlayer, line string) *livePlayer {
	if args := handleradmin.Args(line); len(args) > 0 {
		return l.adminNamedPlayer(gm, args[0], true)
	}
	return adminTargetPlayer(gm, true)
}

// adminInfo answers //info on gm's selection, gm itself without one. A
// player is shown its character page; the pages of the other kinds of
// object are not ported yet (#3325): they log the gap and release the
// client.
func (l *GameClientLink) adminInfo(gm *livePlayer, _ string) {
	target, ok := adminSelectedPlayer(gm)
	if !ok {
		l.log.Warn().Msg("admin: //info on a non-player target not implemented yet (#3325)")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.showCharInfo(gm, target)
}

// showCharInfo selects target for gm, then opens target's character page
// on gm, built on target's queue.
func (l *GameClientLink) showCharInfo(gm, target *livePlayer) {
	l.selectLiveTarget(gm, target)
	onPlayer(gm, target, func() { sendFilledHTML(gm, 0, l.charInfoPage(target), 0) })
}

// charInfoPage is target's character page: who it is, its clan and party,
// its experience, intentions and position, and where it plays from.
func (l *GameClientLink) charInfoPage(target *livePlayer) string {
	clanName := "N/A"
	if l.clans != nil {
		if cl, ok := l.clans.ClanOf(target.Character); ok {
			clanName = `<a action="bypass -h admin_pledge info ` + cl.Name() + `">` + cl.Name() + `</a>`
		}
	}
	partyText := "N/A"
	if l.parties != nil {
		if view, ok := l.parties.View(target.ObjectID()); ok {
			partyText = `<a action="bypass -h admin_party_info ` + target.Name + `">` + strconv.Itoa(len(view.Members)) + ` members</a>`
		}
	}
	ip := target.remoteIP
	if target.SessionDetached() {
		ip = "Disconnected"
	}
	x, y, z := target.Position()
	current, next := target.intentionNames()
	page := l.adminHTML("charinfo.htm")
	for _, r := range [...][2]string{
		{"%name%", target.Name},
		{"%objid%", strconv.Itoa(int(target.ObjectID()))},
		{"%clan%", clanName},
		{"%party%", partyText},
		{"%baseclass%", player.ClassName(target.BaseClassID)},
		{"%xp%", strconv.FormatInt(target.ProgressionValues().Exp, 10)},
		{"%curai%", current},
		{"%nextai%", next},
		{"%loc%", strconv.Itoa(x) + ", " + strconv.Itoa(y) + ", " + strconv.Itoa(z) + ", " + strconv.Itoa(target.Heading())},
		{"%account%", target.AccountName()},
		{"%ip%", ip},
	} {
		page = strings.ReplaceAll(page, r[0], r[1])
	}
	return page
}

// adminPartyInfo answers //party_info [name] for the player adminArgPlayer
// resolves: gm is shown its party's members, its leader highlighted, or
// told it has none.
func (l *GameClientLink) adminPartyInfo(gm *livePlayer, line string) {
	target := l.adminArgPlayer(gm, line)
	var members []*livePlayer
	var leader int32
	if l.parties != nil {
		if view, ok := l.parties.View(target.ObjectID()); ok {
			members, leader = view.Members, view.Leader.ObjectID()
		}
	}
	if members == nil {
		sendText(gm, target.Name+" isn't in a party.")
		return
	}
	var b strings.Builder
	for _, m := range members {
		label := m.Name + " (" + strconv.Itoa(m.Level()) + ")"
		if m.ObjectID() == leader {
			label = `<font color="LEVEL">` + label + `</font>`
		}
		b.WriteString(`<tr><td width=150><a action="bypass -h admin_debug ` + m.Name + `">` + label +
			`</a></td><td width=120 align=right>` + player.ClassName(m.ClassID()) + `</td></tr>`)
	}
	page := l.adminHTML("partyinfo.htm")
	page = strings.ReplaceAll(page, "%name%", target.Name)
	page = strings.ReplaceAll(page, "%party%", b.String())
	sendFilledHTML(gm, 0, page, 0)
}

// adminRemove answers //remove clan_penalty|death_penalty|skill_reuse for
// gm's selected player, gm itself without one:
//
//   - clan_penalty join|create lifts the clan join or creation penalty;
//   - death_penalty lifts the death penalty;
//   - skill_reuse ends every skill reuse delay.
func (l *GameClientLink) adminRemove(gm *livePlayer, line string) {
	const usage = "Usage: //remove <clan_penalty|death_penalty|skill_reuse>"
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, usage)
		return
	}
	target := adminTargetPlayer(gm, true)
	switch args[0] {
	case "clan_penalty":
		if len(args) < 2 {
			sendText(gm, "Usage: //remove clan_penalty join|create")
			return
		}
		param := args[1]
		onPlayer(gm, target, func() {
			switch {
			case strings.Contains(param, "create"):
				target.SetClanCreateExpiryTime(0)
			case strings.Contains(param, "join"):
				target.SetClanJoinExpiryTime(0)
			}
			l.storeClanPenalties(target)
			sendText(gm, "Clan penalty is successfully removed for "+target.Name+".")
		})
	case "death_penalty":
		onPlayer(gm, target, func() {
			target.LiftDeathPenalty()
			if target != gm {
				sendText(gm, target.Name+"'s Death Penalty has been lifted.")
			}
		})
	case "skill_reuse":
		onPlayer(gm, target, func() {
			target.ClearSkillReuses()
			target.ClearDisabledSkills()
			now := target.Now()
			target.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(target.SkillReuseTimers(now), now)))
			sendText(gm, target.Name+"'s skills reuse timers are now cleaned.")
		})
	default:
		sendText(gm, usage)
	}
}
