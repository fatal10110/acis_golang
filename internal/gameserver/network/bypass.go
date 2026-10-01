package network

import (
	"strconv"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// bypassWhitelist holds the bypass commands of the last validated HTML
// page a player was sent. A validated npc_ or Quest command must be one of
// them: equal to a link's command, or starting with the part before the
// '$' of a link that takes an edit-box value.
type bypassWhitelist struct {
	mu     sync.Mutex
	exact  []string
	prefix []string
}

// record replaces the whitelist with the bypass links of html, the page
// exactly as sent. A link reads "bypass [-h ]<command>" inside double
// quotes; the three characters after "bypass " are skipped when they begin
// with "-h". An empty prefix would admit every command and is not kept.
func (w *bypassWhitelist) record(html string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.exact, w.prefix = w.exact[:0], w.prefix[:0]
	const marker = `"bypass `
	for i := 0; i < len(html); {
		start := strings.Index(html[i:], marker)
		if start < 0 {
			break
		}
		start += i
		end := strings.IndexByte(html[start+1:], '"')
		if end < 0 {
			break
		}
		end += start + 1
		start += len(marker)
		if strings.HasPrefix(html[start:], "-h") {
			start += 3
		}
		i = end + 1
		if start > end {
			continue
		}
		if dollar := strings.IndexByte(html[start:end], '$'); dollar >= 0 {
			if p := strings.TrimFunc(html[start:start+dollar], javaSpace); p != "" {
				w.prefix = append(w.prefix, p)
			}
			continue
		}
		w.exact = append(w.exact, strings.TrimFunc(html[start:end], javaSpace))
	}
}

// allows reports whether command is one of the recorded bypasses.
func (w *bypassWhitelist) allows(command string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, bp := range w.exact {
		if bp == command {
			return true
		}
	}
	for _, bp := range w.prefix {
		if strings.HasPrefix(command, bp) {
			return true
		}
	}
	return false
}

// javaSpace is the set trimmed around a bypass command: every character up
// to and including the space.
func javaSpace(r rune) bool { return r <= ' ' }

// sendValidatedHTML opens an HTML window on live and makes its bypass links
// the ones live may send back.
func sendValidatedHTML(live *livePlayer, objectID int32, html string, itemID int32) {
	live.bypasses.record(serverpackets.NpcHtmlBody(html))
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(objectID, html, itemID))
}

// bypassRoute sends the bypass commands starting with one of prefixes to
// handle.
type bypassRoute struct {
	prefixes []string
	handle   func(l *GameClientLink, live *livePlayer, command string)
}

// bypassRoutes are the bypass command families, tried in order.
var bypassRoutes = []bypassRoute{
	{[]string{"admin_"}, unportedBypass("admin command handlers (#161)")},
	{[]string{"player_help "}, (*GameClientLink).bypassPlayerHelp},
	{[]string{"npc_"}, (*GameClientLink).bypassNpc},
	{[]string{"manor_menu_select?"}, unportedBypass("manor (#240)")},
	{[]string{"bbs_", "_bbs", "_friend", "_mail", "_block"}, unportedBypass("community board (#162)")},
	{[]string{"Quest "}, (*GameClientLink).bypassQuest},
	{[]string{"_match", "_diary"}, unportedBypass("hero records (#220)")},
	{[]string{"arenachange"}, unportedBypass("olympiad observation (#219)")},
}

// requestBypassToServer routes a clicked HTML link to its command family.
// A command no family claims is dropped, as the reference drops it.
func (l *GameClientLink) requestBypassToServer(live *livePlayer, req clientpackets.RequestBypassToServer) {
	if live == nil || req.Command == "" {
		return
	}
	for _, route := range bypassRoutes {
		for _, prefix := range route.prefixes {
			if strings.HasPrefix(req.Command, prefix) {
				route.handle(l, live, req.Command)
				return
			}
		}
	}
}

// unportedBypass answers a command family whose system is not in place
// yet: the gap is logged and the client released.
func unportedBypass(system string) func(*GameClientLink, *livePlayer, string) {
	return func(l *GameClientLink, live *livePlayer, command string) {
		l.log.Debug().Str("command", command).Str("system", system).Msg("bypass: command family not modeled")
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

func (l *GameClientLink) bypassPlayerHelp(live *livePlayer, command string) {
	l.sendPlayerHelp(live, strings.TrimPrefix(command, "player_help "))
}

// bypassQuest handles a validated "Quest <quest> [event]" link. A command
// not on the last page is dropped silently, as the reference drops it.
// ponytail: quest events need the quest engine; the command is logged and
// the client released until #130 routes it to the quest's event handler.
func (l *GameClientLink) bypassQuest(live *livePlayer, command string) {
	if !live.bypasses.allows(command) {
		return
	}
	unportedBypass("quest events (#130)")(l, live, command)
}

// bypassNpc handles npc_<objectId>_<command>: a command on the last page
// sent reaches the named NPC when the player can interact with it, and
// the client is then released with ActionFailed. A command not on that
// page, or an object id that does not parse, is dropped silently, as the
// reference drops it; the link click leaves no client action pending.
func (l *GameClientLink) bypassNpc(live *livePlayer, command string) {
	if !live.bypasses.allows(command) {
		return
	}
	end := -1
	if len(command) > 5 {
		if i := strings.IndexByte(command[5:], '_'); i >= 0 {
			end = i + 5
		}
	}
	idText := command[4:]
	if end > 0 {
		idText = command[4:end]
	}
	id, err := strconv.ParseInt(idText, 10, 32)
	if err != nil {
		return
	}
	if end > 0 && l.world != nil {
		if obj, ok := l.world.Object(int32(id)); ok {
			switch target := obj.(type) {
			case *npc.Folk:
				if l.playerCanDoInteract(live, target) && !l.folkBypass(live, target, command[end+1:]) {
					return
				}
			default:
				l.log.Debug().Int32("object_id", obj.ObjectID()).Str("command", command).Msg("bypass: dialog of a non-civilian NPC not modeled")
			}
		}
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// folkBypass runs command on f for live. It reports false when the command
// aborted, so that nothing more is sent.
func (l *GameClientLink) folkBypass(live *livePlayer, f *npc.Folk, command string) bool {
	rules := l.playerConfig.chatRules()
	rules.AllowWear = l.merchant.Config().AllowWear
	talker := npc.Talker{Karma: live.Karma(), Level: live.Level(), LowLevelNewbie: live.LowLevelNewbie()}
	reply := f.Bypass(l.html, rules, talker, command)
	if reply.LeadingActionFailed {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	if reply.CancelEnchant {
		l.cancelActiveEnchant(live)
	}
	switch reply.Outcome {
	case npc.BypassChatWindow:
		sendValidatedHTML(live, f.ObjectID(), reply.HTML, 0)
		live.SendFrame(serverpackets.FrameActionFailed())
	case npc.BypassPage:
		sendValidatedHTML(live, f.ObjectID(), reply.HTML, 0)
	case npc.BypassSellList:
		l.sendSellList(live, f, reply.HTML)
	case npc.BypassBuyList:
		l.showBuyWindow(live, f, reply.ListID)
	case npc.BypassWearList:
		l.showWearWindow(live, f, reply.ListID)
	case npc.BypassHennaDraw:
		l.sendHennaEquipList(live)
	case npc.BypassHennaRemoveList:
		l.openHennaRemoveList(live)
	case npc.BypassMultisell:
		l.openMultisell(live, f, reply.Multisell, reply.InventoryOnly)
	case npc.BypassAborted:
		return false
	case npc.BypassUnported:
		l.log.Debug().Int("npc_id", f.NpcID()).Str("type", f.Instance.Template.Type).Str("command", command).Msg("bypass: npc dialog command not modeled")
	case npc.BypassRefused:
	}
	return true
}
