package network

import (
	"math"
	"strconv"
	"time"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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
// disconnected, and so is an online one taken from a game master's level to
// one that is not (#5122).
//
// Any other number of arguments answers nothing on purpose; a chat
// command leaves no client action pending.
func (l *GameClientLink) adminSetAccess(gm *livePlayer, args []string) {
	const usage = "Usage: //set access <level> | <name> <level>"
	switch len(args) {
	case 1:
		level, ok := parseInt32(args[0])
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
			sendText(gm, target.CharacterName()+"'s access level is now set to "+strconv.Itoa(int(level))+".")
		})
	case 2:
		name := args[0]
		level, ok := parseInt32(args[1])
		if !ok {
			sendText(gm, usage)
			return
		}
		if target, ok := l.livePlayerByName(name); ok {
			l.changeAccessLevel(gm, target, int(level), func() {
				sendText(gm, target.CharacterName()+"'s access level is now set to "+strconv.Itoa(int(level))+".")
			})
			return
		}
		// An offline level must fit the stored column, and a write that
		// fails is answered (#5129).
		if !storableAccessLevel(int(level)) {
			sendText(gm, "Access level "+strconv.Itoa(int(level))+" is out of range; "+name+"'s access level is unaltered.")
			return
		}
		l.adminByName(gm, name, func(target *livePlayer) {
			l.moveToAccessLevel(gm, target, int(level), nil)
		}, func(objectID int32) {
			l.storeOfflineAccessLevel(objectID, int(level), func() {
				sendText(gm, name+"'s access level could not be stored; it is unaltered.")
			})
		}, func(found bool) {
			if !found {
				sendText(gm, name+"couldn't be found - its access level is unaltered.")
				return
			}
			sendText(gm, name+"'s access level is now set to "+strconv.Itoa(int(level))+".")
		})
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

// changeAccessLevel moves online target to level on target's queue, runs
// report, then disconnects target as moveToAccessLevel does. A target
// already leaving the world gets level stored from gm's goroutine instead.
func (l *GameClientLink) changeAccessLevel(gm, target *livePlayer, level int, report func()) {
	onPlayerOr(gm, target, func() {
		l.moveToAccessLevel(gm, target, level, report)
	}, func() {
		l.storeLeavingAccessLevel(target, level)
		report()
	})
}

// moveToAccessLevel moves live, on live's queue, to level and runs report,
// when given. It then disconnects live when level bans it, or when live
// loses a game master's status (#5122), gm being told so: its logout save
// stores the level, and its next session starts without the hiding,
// invulnerability, movement modes and speed the game master had on.
func (l *GameClientLink) moveToAccessLevel(gm, live *livePlayer, level int, report func()) {
	wasGM := live.IsGM()
	l.setAccessLevel(live, level)
	if report != nil {
		report()
	}
	switch {
	case level < 0:
		live.kickClient()
	case wasGM && !live.IsGM():
		// Sent before the kick: gm may be live itself.
		sendText(gm, live.CharacterName()+" lost GM status and was disconnected.")
		live.kickClient()
	}
}

// gmOffRestore is the access level a //gmoff window gives back when it
// ends, and the timer that ends it.
type gmOffRestore struct {
	level int
	timer *sim.Timer
}

// dropGMOff ends p's //gmoff window, if one is open, without its restore.
// It runs on p's queue.
func (p *livePlayer) dropGMOff() {
	if p.gmOff == nil {
		return
	}
	if p.gmOff.timer != nil {
		p.gmOff.timer.Stop()
	}
	p.gmOff = nil
}

// gmOffLeaveState gives st, the logout save of live leaving inside a //gmoff
// window, the access level and title the window's restore would have given
// back, so the character never stays at the user level (#5123). It runs on
// live's queue.
func (l *GameClientLink) gmOffLeaveState(live *livePlayer, st *player.SaveState) {
	restore := live.gmOff
	if restore == nil {
		return
	}
	live.dropGMOff()
	access := l.resolveAccessLevel(live.Character, restore.level)
	st.AccessLevel = access.Level
	if restore.level > 0 {
		st.Title = trimTitle(access.Name)
	}
}

// adminGMOff answers //gmoff [minutes]: gm drops to the user level and gets
// its level back once the minutes, one by default, have passed. Any other
// access-level change in the window wins over the restore, and a gm leaving
// in the window is stored at the level it gets back (#5123).
func (l *GameClientLink) adminGMOff(gm *livePlayer, line string) {
	minutes := int32(1)
	if args := handleradmin.Args(line); len(args) > 0 {
		if v, ok := parseInt32(args[0]); ok {
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
	// gm's queue closes when it leaves the game, which drops the timer; its
	// logout save stores the level instead (gmOffLeaveState).
	restore := &gmOffRestore{level: previous}
	gm.gmOff = restore
	timer := gm.after(delay, func() {
		if gm.gmOff != restore {
			return
		}
		gm.gmOff = nil
		l.setAccessLevel(gm, previous)
		sendText(gm, "Your previous access level has been rehabilitated.")
	})
	if gm.gmOff == restore {
		restore.timer = timer
	}
}

// adminBan answers //ban account|chat|player [name [minutes]]. Without a
// name the selected player is banned; with one, the online player so named,
// else the offline account or character. A chat ban lasts the minutes
// given, without end by default; see adminBanChat.
//
// An unknown kind answers nothing on purpose; a chat command leaves no
// client action pending.
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
			if minutes, ok = parseInt32(args[2]); !ok {
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
			if l.sendAccountAccessLevel(name, accountBannedLevel) != nil {
				sendText(gm, "Ban request for account "+name+" could not be sent.")
				return
			}
			sendText(gm, "Ban request sent for account "+name+".")
			return
		}
		// The ban goes out from gm's goroutine: target's queue is closed once
		// it starts leaving the world, and the ban must not depend on it. A
		// ban not sent neither kicks nor reports a ban (#5127).
		if l.sendAccountAccessLevel(target.AccountName(), accountBannedLevel) != nil {
			sendText(gm, "Ban request for account "+target.AccountName()+" could not be sent.")
			return
		}
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
// An unknown kind answers nothing on purpose; a chat command leaves no
// client action pending.
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
			sendText(gm, target.CharacterName()+" account isn't actually banned.")
			return
		}
		if l.sendAccountAccessLevel(name, accountUnbannedLevel) != nil {
			sendText(gm, "Unban request for account "+orNull(name)+" could not be sent.")
			return
		}
		sendText(gm, "Unban request sent for account "+orNull(name)+".")
	case "chat":
		l.adminUnbanChat(gm, target, name)
	case "player":
		if target != nil {
			sendText(gm, target.CharacterName()+" player isn't actually banned.")
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
		onPlayerOr(gm, target, func() {
			l.setAccessLevel(target, level)
			target.kickClient()
		}, func() {
			l.storeLeavingAccessLevel(target, level)
		})
		sendText(gm, target.CharacterName()+" has been banned.")
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
	// A session found by the character's id rather than by name, or one a
	// selection under way ends with, takes a ban as an online target does;
	// it is not banned, so an unban leaves it as it is.
	l.adminByName(gm, name, func(target *livePlayer) {
		if level < 0 {
			l.moveToAccessLevel(gm, target, level, nil)
		}
	}, func(objectID int32) {
		l.storeOfflineAccessLevel(objectID, level, nil)
	}, func(found bool) {
		if !found {
			sendText(gm, "This Player isn't found, or the AccessLevel was unaltered.")
			return
		}
		sendText(gm, name+" now has an access level of "+strconv.Itoa(level)+".")
	})
}

// orNull is s as an admin message prints it, where a missing argument
// is null.
func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}
