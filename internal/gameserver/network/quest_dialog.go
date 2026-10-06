package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// showDialogPage opens the dialog page a script or the quest dialog sent
// live: the datapack page file as an HTML window takes it, or the given
// page with the page limit applied, with the NPC's object id filled in when
// there is an NPC; its links become the ones live may send back, and the
// client is released.
func (l *GameClientLink) showDialogPage(live *livePlayer, e event.DialogPageShown) {
	page := serverpackets.NpcHtmlBody(e.HTML)
	if e.File != "" {
		if l.html != nil {
			page = l.setPage(e.File)
		} else {
			page = "<html><body>My html is missing:<br>" + e.File + "</body></html>"
		}
	}
	if e.ObjectID != 0 {
		page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(e.ObjectID)))
	}
	sendFilledHTML(live, e.ObjectID, page, 0)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// talkThroughScripts is the scripts' part of live's talk to the NPC n in
// reach: n becomes live's last quest NPC, and a script holding n's first
// talk answers instead of n. It reports whether one did.
func (l *GameClientLink) talkThroughScripts(live *livePlayer, n attackable.Combatant) bool {
	if l.scripts == nil {
		return false
	}
	return l.scripts.Interact(script.PlayerOf(live), script.NPCOf(n))
}

// questWindow answers a "Quest [name]" dialog command live sent the NPC n:
// the command's text after "Quest", trimmed, names the quest, and an empty
// name opens n's quest window.
func (l *GameClientLink) questWindow(live *livePlayer, n attackable.Combatant, command string) {
	if l.scripts == nil {
		return
	}
	name := strings.TrimFunc(strings.TrimPrefix(command, "Quest"), javaSpace)
	l.scripts.QuestWindow(script.PlayerOf(live), script.NPCOf(n), name)
}

// bypassQuest handles "Quest <quest> [event]", a quest event link of the
// last page sent: the event goes to the named quest through the NPC live
// last talked to about quests, which the engine checks is still in reach
// and talks through that quest. A command not on the last page, or one the
// engine refuses, is dropped silently, as specified. A player trading or
// running a private store is refused with ActionFailed: the event may
// take or give items, which a trade or store in progress must not see
// change.
func (l *GameClientLink) bypassQuest(live *livePlayer, command string) {
	if !live.bypasses.allows(command) {
		return
	}
	if !l.playerCanAttemptInteract(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if l.scripts == nil {
		return
	}
	name, event, _ := strings.Cut(strings.TrimFunc(command[len("Quest "):], javaSpace), " ")
	l.scripts.QuestEvent(script.PlayerOf(live), l.lastQuestNPC(live), name, event)
}

// lastQuestNPC returns the NPC live last talked to about quests, nil when
// that object is no NPC in the world.
func (l *GameClientLink) lastQuestNPC(live *livePlayer) *script.NPC {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Object(live.LastQuestNPC())
	if !ok {
		return nil
	}
	n, ok := obj.(attackable.Combatant)
	if !ok {
		return nil
	}
	return script.NPCOf(n)
}
