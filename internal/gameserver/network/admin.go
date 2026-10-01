package network

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminCommand runs one administrator command for gm on gm's queue. line
// is the whole command, its admin_ word first.
type adminCommand func(l *GameClientLink, gm *livePlayer, line string)

// adminCommands maps every ported admin_ command word, matched exactly, to
// its handler. A word the command table defines but no entry here claims is
// a command still to port.
var adminCommands = map[string]adminCommand{
	"admin_admin":        (*GameClientLink).adminMainPage,
	"admin_gmlist":       (*GameClientLink).adminToggleGMList,
	"admin_link":         (*GameClientLink).adminLink,
	"admin_msg":          (*GameClientLink).adminSystemMessage,
	"admin_tele":         (*GameClientLink).adminTeleport,
	"admin_teleport":     (*GameClientLink).adminTeleport,
	"admin_teleportto":   (*GameClientLink).adminTeleport,
	"admin_recall":       (*GameClientLink).adminTeleport,
	"admin_sendhome":     (*GameClientLink).adminTeleport,
	"admin_instant_move": (*GameClientLink).adminTeleport,
	"admin_kick":         (*GameClientLink).adminKick,
	"admin_enchant":      (*GameClientLink).adminEnchant,
	"admin_set":          (*GameClientLink).adminSet,
	"admin_gmoff":        (*GameClientLink).adminGMOff,
	"admin_ban":          (*GameClientLink).adminBan,
	"admin_unban":        (*GameClientLink).adminUnban,
	"admin_atmosphere":   (*GameClientLink).adminAtmosphere,

	// Petitions; see admin_petition.go.
	"admin_petition":      (*GameClientLink).adminPetition,
	"admin_force_peti":    (*GameClientLink).adminForcePetition,
	"admin_add_peti_chat": (*GameClientLink).adminPetitionChat,

	// Announcements; see admin_announce.go.
	"admin_announce": (*GameClientLink).adminAnnounce,
	"admin_ann":      (*GameClientLink).adminAnnounceText,
	"admin_say":      (*GameClientLink).adminAnnounceText,
	"admin_gmchat":   (*GameClientLink).adminGMChat,
}

// adminEntry is one of the two ways a command reaches the server; they
// differ only in the refusal they answer and log.
type adminEntry struct {
	refusal string
	logText string
}

var (
	// adminBypassEntry is a clicked admin_ link.
	adminBypassEntry = adminEntry{
		refusal: "You don't have the access rights to use this command.",
		logText: "tried to use an admin command without proper access level",
	}
	// adminChatEntry is a "//command" typed in chat.
	adminChatEntry = adminEntry{
		refusal: "You don't have the access right to use this command.",
		logText: "tried to use an admin command without access to it",
	}
)

// bypassAdmin runs a clicked admin_ link. Its word is the command up to
// its first space.
func (l *GameClientLink) bypassAdmin(live *livePlayer, command string) {
	word, _, _ := strings.Cut(command, " ")
	l.runAdminCommand(live, adminBypassEntry, word, command, command)
}

// sendBypassBuildCmd runs a "//command" typed in chat: the text is trimmed,
// its first word prefixed with admin_ names the command, and the handler
// gets the whole text so prefixed.
func (l *GameClientLink) sendBypassBuildCmd(live *livePlayer, req clientpackets.SendBypassBuildCmd) {
	if live == nil {
		return
	}
	text := strings.TrimFunc(req.Command, javaSpace)
	word, _, _ := strings.Cut(text, " ")
	l.runAdminCommand(live, adminChatEntry, "admin_"+word, "admin_"+text, text)
}

// runAdminCommand runs the command word names for gm, line being the whole
// command and audited the text the audit log records.
//
// A word no handler claims and the command table does not define is unknown:
// a game master is told so, anyone else gets nothing, and neither leaves a
// client action pending. A known command checks gm's access level first,
// then is audited, then runs.
func (l *GameClientLink) runAdminCommand(gm *livePlayer, entry adminEntry, word, line, audited string) {
	run, ported := adminCommands[word]
	if !ported && !l.admin.Defines(word) {
		if gm.accessLevel().IsGM {
			sendText(gm, "The command "+strings.TrimPrefix(word, "admin_")+" doesn't exist.")
		}
		return
	}
	if !l.admin.HasAccess(word, gm.accessLevel()) {
		if !l.admin.Defines(word) {
			l.log.Warn().Str("command", word).Msg("admin: no rights defined for admin command")
		}
		sendText(gm, entry.refusal)
		l.log.Warn().Str("player", gm.Name).Str("command", word).Msg("admin: " + entry.logText)
		return
	}
	l.gmAudit.Info().Msgf("%s [%d] used '%s' command on: %s", gm.Name, gm.ObjectID(), audited, trackedName(gm.Target()))
	if !ported {
		l.log.Warn().Str("command", word).Msg("admin: command not implemented yet")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	run(l, gm, line)
}

// trackedName names obj for the audit log, "none" without one.
func trackedName(obj world.Tracked) string {
	switch o := obj.(type) {
	case nil:
		return "none"
	case interface{ CharacterName() string }:
		return o.CharacterName()
	default:
		return strconv.Itoa(int(obj.ObjectID()))
	}
}

// sendText sends live a plain-text system message.
func sendText(live *livePlayer, text string) {
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, text))
}

// sendAdminFile opens the admin panel page name, the missing-page notice
// when there is none.
func (l *GameClientLink) sendAdminFile(live *livePlayer, name string) {
	file := "data/html/admin/" + name
	html, ok := l.html.Get(file)
	if !ok {
		html = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", file)
	}
	sendValidatedHTML(live, 0, html, 0)
}

// adminTargetPlayer resolves the player a command acts on: gm's selected
// player, else gm itself when orSelf is set.
func adminTargetPlayer(gm *livePlayer, orSelf bool) *livePlayer {
	if target, ok := gm.Target().(*livePlayer); ok {
		return target
	}
	if orSelf {
		return gm
	}
	return nil
}

// adminNamedPlayer resolves the online player called name, else what
// adminTargetPlayer resolves.
func (l *GameClientLink) adminNamedPlayer(gm *livePlayer, name string, orSelf bool) *livePlayer {
	if p, ok := l.livePlayerByName(name); ok {
		return p
	}
	return adminTargetPlayer(gm, orSelf)
}

// onPlayer runs fn for target on target's queue: at once when target is gm,
// whose queue the command already runs on, else posted there. It reports
// whether fn will run: a target that left the world meanwhile drops it.
func onPlayer(gm, target *livePlayer, fn func()) bool {
	if target == gm {
		fn()
		return true
	}
	return postLive(target, fn)
}
