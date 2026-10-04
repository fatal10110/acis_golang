package network

import (
	"math"
	"strconv"
	"time"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Account access levels the ban commands send the login server.
const (
	accountBannedLevel   = -100
	accountUnbannedLevel = 0
)

// Character access levels the ban commands give.
const (
	characterBannedLevel   = -1
	characterUnbannedLevel = 0
)

// adminSetAccess answers //set access <level> for the selected object (gm
// itself without one), and //set access <name> <level> for the named
// character, online or not. A character set below 0 is banned and
// disconnected.
//
// Any other number of arguments answers nothing, as the reference does; a
// chat command leaves no client action pending.
func (l *GameClientLink) adminSetAccess(gm *livePlayer, args []string) {
	const usage = "Usage: //set access <level> | <name> <level>"
	switch len(args) {
	case 1:
		level, ok := parseJavaInt(args[0])
		if !ok {
			sendText(gm, usage)
			return
		}
		target, ok := adminSelectedPlayer(gm)
		if !ok {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		l.changeAccessLevel(gm, target, int(level), func() {
			sendText(gm, target.Name+"'s access level is now set to "+strconv.Itoa(int(level))+".")
		})
	case 2:
		name := args[0]
		level, ok := parseJavaInt(args[1])
		if !ok {
			sendText(gm, usage)
			return
		}
		if target, ok := l.livePlayerByName(name); ok {
			l.changeAccessLevel(gm, target, int(level), func() {
				sendText(gm, target.Name+"'s access level is now set to "+strconv.Itoa(int(level))+".")
			})
			return
		}
		found, ok := l.storeAccessLevelByName(name, int(level))
		switch {
		case !ok:
		case !found:
			sendText(gm, name+"couldn't be found - its access level is unaltered.")
		default:
			sendText(gm, name+"'s access level is now set to "+strconv.Itoa(int(level))+".")
		}
	}
}

// adminSelectedPlayer resolves the object //set acts on: gm's selection, gm
// itself without one. ok is false when the selection is not a player.
func adminSelectedPlayer(gm *livePlayer) (*livePlayer, bool) {
	switch target := gm.Target().(type) {
	case nil:
		return gm, true
	case *livePlayer:
		return target, true
	default:
		return nil, false
	}
}

// changeAccessLevel moves online target to level on target's queue,
// disconnects it when level bans it, then runs report. A target already
// leaving the world gets level stored from gm's goroutine instead.
func (l *GameClientLink) changeAccessLevel(gm, target *livePlayer, level int, report func()) {
	if !onPlayer(gm, target, func() {
		l.setAccessLevel(target, level)
		if level < 0 {
			target.kickClient()
		}
		report()
	}) {
		l.storeLeavingAccessLevel(target, level)
		report()
	}
}

// adminGMOff answers //gmoff [minutes]: gm drops to the user level and gets
// its level back once the minutes, one by default, have passed, unless it
// has left the game by then.
func (l *GameClientLink) adminGMOff(gm *livePlayer, line string) {
	minutes := int32(1)
	if args := handleradmin.Args(line); len(args) > 0 {
		if v, ok := parseJavaInt(args[0]); ok {
			minutes = v
		} else {
			sendText(gm, "Invalid timer set for //gm ; default time is used.")
		}
	}
	previous := gm.accessLevel().Level
	l.setAccessLevel(gm, 0)
	sendText(gm, "You no longer have GM status, but will be rehabilitated after "+strconv.Itoa(int(minutes))+" minutes.")
	delay := time.Duration(math.MaxInt64)
	if int64(minutes) < int64(delay/time.Minute) {
		delay = time.Duration(minutes) * time.Minute
	}
	// gm's queue closes when it leaves the game, which drops the timer.
	gm.after(delay, func() {
		l.setAccessLevel(gm, previous)
		sendText(gm, "Your previous access level has been rehabilitated.")
	})
}

// adminBan answers //ban account|chat|player [name [minutes]]. Without a
// name the selected player is banned; with one, the online player so named,
// else the offline account or character. A chat ban lasts the minutes
// given, without end by default; see adminBanChat.
//
// An unknown kind answers nothing, as the reference does; a chat command
// leaves no client action pending.
func (l *GameClientLink) adminBan(gm *livePlayer, line string) {
	const usage = "Usage : //ban account|chat|player [name [time]]"
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, usage)
		return
	}
	kind := args[0]
	var (
		name    string
		target  *livePlayer
		minutes int32 = -1
	)
	if len(args) > 1 {
		name = args[1]
		target, _ = l.livePlayerByName(name)
		if len(args) > 2 {
			var ok bool
			if minutes, ok = parseJavaInt(args[2]); !ok {
				sendText(gm, usage)
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
	switch kind {
	case "account":
		if target == nil {
			if name == "" {
				sendText(gm, "Usage: //ban account [name].")
				return
			}
			l.sendAccountAccessLevel(name, accountBannedLevel)
			sendText(gm, "Ban request sent for account "+name+".")
			return
		}
		// The ban goes out from gm's goroutine: target's queue is closed once
		// it starts leaving the world, and the ban must not depend on it.
		l.sendAccountAccessLevel(target.AccountName(), accountBannedLevel)
		onPlayer(gm, target, target.kickClient)
		sendText(gm, target.AccountName()+" account is banned.")
	case "chat":
		l.adminBanChat(gm, target, name, minutes)
	case "player":
		l.changeCharAccessLevel(gm, target, name, characterBannedLevel)
	}
}

// adminUnban answers //unban account|chat|player name: an offline account
// or character gets its access back; an online one is not banned. A chat
// unban lifts the named player's chat ban; see adminUnbanChat.
//
// An unknown kind answers nothing, as the reference does; a chat command
// leaves no client action pending.
func (l *GameClientLink) adminUnban(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, "Usage : //unban account|chat|player name")
		return
	}
	kind := args[0]
	var (
		name   string
		target *livePlayer
	)
	if len(args) > 1 {
		name = args[1]
		target, _ = l.livePlayerByName(name)
	}
	if target == gm {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseOnYourself))
		return
	}
	switch kind {
	case "account":
		if target != nil {
			sendText(gm, target.Name+" account isn't actually banned.")
			return
		}
		l.sendAccountAccessLevel(name, accountUnbannedLevel)
		sendText(gm, "Unban request sent for account "+javaString(name)+".")
	case "chat":
		l.adminUnbanChat(gm, target, name)
	case "player":
		if target != nil {
			sendText(gm, target.Name+" player isn't actually banned.")
			return
		}
		l.changeCharAccessLevel(gm, nil, name, characterUnbannedLevel)
	}
}

// changeCharAccessLevel bans (level -1) or unbans (level 0) a character:
// online target is moved to level and disconnected; otherwise the stored
// level of the character named name changes.
func (l *GameClientLink) changeCharAccessLevel(gm, target *livePlayer, name string, level int) {
	if target != nil {
		if !onPlayer(gm, target, func() {
			l.setAccessLevel(target, level)
			target.kickClient()
		}) {
			l.storeLeavingAccessLevel(target, level)
		}
		sendText(gm, target.Name+" has been banned.")
		return
	}
	if name == "" {
		if level == characterUnbannedLevel {
			sendText(gm, "Usage: //unban player [name].")
		} else {
			sendText(gm, "Usage: //ban player [name].")
		}
		return
	}
	found, ok := l.storeAccessLevelByName(name, level)
	switch {
	case !ok:
	case !found:
		sendText(gm, "This Player isn't found, or the AccessLevel was unaltered.")
	default:
		sendText(gm, name+" now has an access level of "+strconv.Itoa(level)+".")
	}
}

// javaString is s as string concatenation prints it in the reference, where
// a missing argument is null.
func javaString(s string) string {
	if s == "" {
		return "null"
	}
	return s
}
