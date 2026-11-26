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
// replaced by a chat ban. The online check and ban are one step on the
// target's queue, and gm is told what that step did (#5129).
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
		done := name + " is chat banned" + forMinutes(minutes)
		l.reportOfflinePunishment(gm, name, player.PunishChat, timer, nil, done, func(target *livePlayer) {
			l.banChat(gm, target, int32(timer/player.PunishmentMillis(1)), done)
		})
		return
	}
	banned := target.CharacterName() + " is chat banned" + forMinutes(minutes)
	onPlayerOr(gm, target, func() { l.banChat(gm, target, minutes, banned) }, func() {
		// Leaving: its punishment no longer changes, and the ban is
		// stored after its logout saves for the next login to resume,
		// checked against the stored punishment, not the one it left
		// with, which a punishment stored meanwhile no longer matches
		// (#5502).
		l.replaceStoredPunishment(target.ObjectID(), player.PunishNone, player.PunishChat, player.PunishmentMillis(minutes), func(stored player.Punishment) {
			if stored != player.PunishNone {
				sendText(gm, alreadyPunished(target.CharacterName(), stored))
				return
			}
			sendText(gm, banned)
		})
	})
}

// banChat bans target's chat for minutes unless it is already punished,
// checked and changed in one step on target's queue, and tells gm done or
// the punishment that kept it (#5129).
func (l *GameClientLink) banChat(gm, target *livePlayer, minutes int32, done string) {
	if l.punishIf(target, player.PunishNone, player.PunishChat, minutes) == player.PunishmentKept {
		kind, _ := target.Punishment()
		sendText(gm, alreadyPunished(target.CharacterName(), kind))
		return
	}
	sendText(gm, done)
}

// alreadyPunished is the answer to a chat ban of name, which serves kind.
func alreadyPunished(name string, kind player.Punishment) string {
	return name + " is already " + kind.Description() + ", and can't receive another punishment."
}

// adminUnbanChat answers //unban chat: an online target's chat ban is
// lifted; the offline character named name has its stored punishment, of
// whatever kind, cleared. The online check and lift are one step on the
// target's queue, so a jail term that landed meanwhile stays (#5129).
func (l *GameClientLink) adminUnbanChat(gm, target *livePlayer, name string) {
	if target == nil {
		done := name + "'s chat ban has been lifted."
		l.reportOfflinePunishment(gm, name, player.PunishNone, 0, nil, done, func(target *livePlayer) {
			l.liftChatBan(gm, target, done)
		})
		return
	}
	lifted := target.CharacterName() + "'s chat ban has been lifted."
	onPlayerOr(gm, target, func() { l.liftChatBan(gm, target, lifted) }, func() {
		// Leaving: its punishment no longer changes, and the lift is
		// stored after its logout saves, only over a stored chat ban: a
		// jail term stored meanwhile stays (#5502).
		l.replaceStoredPunishment(target.ObjectID(), player.PunishChat, player.PunishNone, 0, func(stored player.Punishment) {
			if stored != player.PunishChat {
				sendText(gm, notChatBanned(target.CharacterName()))
				return
			}
			sendText(gm, lifted)
		})
	})
}

// liftChatBan lifts target's chat ban, checked and changed in one step on
// target's queue, and tells gm done or that there was none to lift (#5129).
func (l *GameClientLink) liftChatBan(gm, target *livePlayer, done string) {
	if l.punishIf(target, player.PunishChat, player.PunishNone, 0) == player.PunishmentKept {
		sendText(gm, notChatBanned(target.CharacterName()))
		return
	}
	sendText(gm, done)
}

// notChatBanned is the answer to a chat unban of name, which serves no chat
// ban.
func notChatBanned(name string) string {
	return name + " isn't currently chat banned."
}

// adminJail answers //jail [name [minutes]]: the named player, else the
// selected one, is jailed for minutes, without end by default; an offline
// character named name is stored jailed, in the jail.
//
// Minutes that do not parse answer nothing on purpose; a chat command
// leaves no client action pending.
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
			if minutes, ok = parseInt32(args[1]); !ok {
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
		timer, done := player.PunishmentMillis(minutes), name+" has been jailed"+forMinutes(minutes)
		l.reportOfflinePunishment(gm, name, player.PunishJail, timer, &at, done, func(target *livePlayer) {
			l.punish(target, player.PunishJail, int32(timer/player.PunishmentMillis(1)))
			sendText(gm, done)
		})
		return
	}
	l.punishTarget(gm, target, player.PunishJail, minutes)
	sendText(gm, target.CharacterName()+" is jailed"+forMinutes(minutes))
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
		done := name + " has been unjailed."
		l.reportOfflinePunishment(gm, name, player.PunishNone, 0, &at, done, func(target *livePlayer) {
			l.punish(target, player.PunishNone, 0)
			sendText(gm, done)
		})
		return
	}
	l.punishTarget(gm, target, player.PunishNone, 0)
	sendText(gm, target.CharacterName()+" has been unjailed.")
}

// reportOfflinePunishment stores kind and timer as the punishment of the
// offline character named name, moving it to at when at is set, and tells
// gm done, or that no character has that name, once the name is looked up
// off gm's queue (adminByName). A failed lookup is logged and answers
// nothing. A session of the character found by its id, or one a
// selection under way ends with, gets online instead, on its queue, which
// applies the punishment the stored row would have given it and tells gm
// what it did (#5502).
func (l *GameClientLink) reportOfflinePunishment(gm *livePlayer, name string, kind player.Punishment, timer int64, at *location.Location, done string, online func(*livePlayer)) {
	l.adminByName(gm, name, online, func(objectID int32) {
		l.storePunishmentAt(objectID, kind, timer, at)
		sendText(gm, done)
	}, func(found bool) {
		if !found {
			sendText(gm, "This Player isn't found.")
		}
	})
}
