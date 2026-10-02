package network

import (
	"strconv"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// offlineChatBanMillis is the timer an offline chat ban given no minutes
// gets: one minute, not a ban without end.
const offlineChatBanMillis = 60000

// forMinutes is how a punishment report ends: " for N minutes." for a timed
// punishment, "." for one without end.
func forMinutes(minutes int32) string {
	if minutes > 0 {
		return " for " + strconv.Itoa(int(minutes)) + " minutes."
	}
	return "."
}

// adminBanChat answers //ban chat: an online target, unless already
// punished, has its chat banned for minutes, without end when not
// positive; the offline character named name has its stored punishment
// replaced by a chat ban.
func (l *GameClientLink) adminBanChat(gm, target *livePlayer, name string, minutes int32) {
	if target == nil {
		if name == "" {
			sendText(gm, "Usage: //ban chat [name duration].")
			return
		}
		timer := player.PunishmentMillis(minutes)
		if timer == 0 {
			timer = offlineChatBanMillis
		}
		l.reportOfflinePunishment(gm, name, player.PunishChat, timer, nil, name+" is chat banned"+forMinutes(minutes))
		return
	}
	if kind, _ := target.Punishment(); kind != player.PunishNone {
		sendText(gm, target.Name+" is already "+kind.Description()+", and can't receive another punishment.")
		return
	}
	l.punishTarget(gm, target, player.PunishChat, minutes)
	sendText(gm, target.Name+" is chat banned"+forMinutes(minutes))
}

// adminUnbanChat answers //unban chat: an online target's chat ban is
// lifted; the offline character named name has its stored punishment, of
// whatever kind, cleared.
func (l *GameClientLink) adminUnbanChat(gm, target *livePlayer, name string) {
	if target == nil {
		l.reportOfflinePunishment(gm, name, player.PunishNone, 0, nil, name+"'s chat ban has been lifted.")
		return
	}
	if !target.ChatBanned() {
		sendText(gm, target.Name+" isn't currently chat banned.")
		return
	}
	l.punishTarget(gm, target, player.PunishNone, 0)
	sendText(gm, target.Name+"'s chat ban has been lifted.")
}

// adminJail answers //jail [name [minutes]]: the named player, else the
// selected one, is jailed for minutes, without end by default; an offline
// character named name is stored jailed, in the jail.
//
// Minutes that do not parse answer nothing, as the reference does; a chat
// command leaves no client action pending.
func (l *GameClientLink) adminJail(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	var (
		name    string
		target  *livePlayer
		minutes int32 = -1
	)
	if len(args) > 0 {
		name = args[0]
		target, _ = l.livePlayerByName(name)
		if len(args) > 1 {
			var ok bool
			if minutes, ok = parseJavaInt(args[1]); !ok {
				l.log.Warn().Str("minutes", args[1]).Msg("admin: //jail minutes not a number")
				return
			}
		}
	} else {
		target = adminTargetPlayer(gm, false)
	}
	if target == gm {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseOnYourself))
		return
	}
	if target == nil {
		at := jailLocation
		l.reportOfflinePunishment(gm, name, player.PunishJail, player.PunishmentMillis(minutes), &at, name+" has been jailed"+forMinutes(minutes))
		return
	}
	l.punishTarget(gm, target, player.PunishJail, minutes)
	sendText(gm, target.Name+" is jailed"+forMinutes(minutes))
}

// adminUnjail answers //unjail [name]: the named player, else the selected
// one, has its punishment lifted, a chat ban as well as a jail term; an
// offline character named name is stored unpunished, in Floran village.
func (l *GameClientLink) adminUnjail(gm *livePlayer, line string) {
	var (
		name   string
		target *livePlayer
	)
	if args := handleradmin.Args(line); len(args) > 0 {
		name = args[0]
		target, _ = l.livePlayerByName(name)
	} else {
		target = adminTargetPlayer(gm, false)
	}
	if target == nil {
		at := jailReleaseLocation
		l.reportOfflinePunishment(gm, name, player.PunishNone, 0, &at, name+" has been unjailed.")
		return
	}
	l.punishTarget(gm, target, player.PunishNone, 0)
	sendText(gm, target.Name+" has been unjailed.")
}

// reportOfflinePunishment stores kind and timer as the punishment of the
// offline character named name, moving it to at when at is set, and tells
// gm done, or that no character has that name. A failed write is logged
// and answers nothing.
func (l *GameClientLink) reportOfflinePunishment(gm *livePlayer, name string, kind player.Punishment, timer int64, at *location.Location, done string) {
	found, ok := l.storePunishmentByName(name, kind, timer, at)
	switch {
	case !ok:
	case !found:
		sendText(gm, "This Player isn't found.")
	default:
		sendText(gm, done)
	}
}
