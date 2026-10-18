package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// showNPCChat opens on live the chat window of the NPC objectID, as a
// script asked: the window the NPC opens when talked to with no script
// answering. An NPC no longer in the world shows nothing.
func (l *GameClientLink) showNPCChat(live *livePlayer, objectID int32) {
	if l.world == nil {
		return
	}
	obj, ok := l.world.Object(objectID)
	if !ok {
		return
	}
	switch n := obj.(type) {
	case *npc.Folk:
		l.openFolkChat(live, n)
	case *npc.Hostile:
		l.openHostileChat(live, n, 0)
	}
}

// openFolkChat opens f's chat window on live, as its talk does when no
// script answers and f shows no observer or siege window.
func (l *GameClientLink) openFolkChat(live *livePlayer, f *npc.Folk) {
	html, outcome := f.ChatWindow(setPages{l.html}, l.chatRules(), live.Karma(), folkChatState{l: l, live: live, f: f})
	switch outcome {
	case npc.ChatWedding:
		l.weddingGreeting(live, f)
	case npc.ChatUnported:
		l.log.Debug().Int("npc_id", f.NpcID()).Str("type", f.Instance.Template.Type).Msg("npc: chat window not modeled")
	default:
		sendFolkChat(live, f, html, outcome)
	}
}

// openHostileChat opens h's chat page val on live, then ActionFailed.
func (l *GameClientLink) openHostileChat(live *livePlayer, h *npc.Hostile, val int) {
	file := h.ChatPage(val, func(file string) bool {
		if l.html == nil {
			return false
		}
		_, ok := l.html.Get(file)
		return ok
	})
	l.sendNPCPage(live, h.ObjectID(), file)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// sendNPCPage opens the datapack page file as the window of the NPC
// objectID, with %objectId% naming it; a missing page reads as a "My html
// is missing" notice naming it.
func (l *GameClientLink) sendNPCPage(live *livePlayer, objectID int32, file string) {
	page := "<html><body>My html is missing:<br>" + file + "</body></html>"
	if l.html != nil {
		page = l.setPage(file)
	}
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(objectID)))
	sendFilledHTML(live, objectID, page, 0)
}

// callSkill makes live use the skill of e on its target, on live's queue, as a
// script asked: the cast animation for everyone watching live, then the
// skill's effects through its handler. A skill with no definition, or a
// target no longer in the world, does nothing.
func (l *GameClientLink) callSkill(live *livePlayer, e event.SkillCalled) {
	def, ok := l.skills.Definition(e.Skill)
	if !ok {
		return
	}
	var target skillhandler.Actor = live
	if e.TargetID != live.ObjectID() {
		if l.world == nil {
			return
		}
		obj, found := l.world.Object(e.TargetID)
		actor, isActor := obj.(skillhandler.Actor)
		if !found || !isActor {
			return
		}
		target = actor
	}
	caster := skillCastObject(live)
	targetAt := caster
	if t, ok := target.(actorcast.Target); ok {
		targetAt = skillCastObject(t)
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(caster, targetAt, int32(def.ID), int32(def.Level), def.HitTime, 0, false)
	})
	handlers := l.castEffects()
	handlers.Sink = l.playerMessageSink(live, nil)
	result := actorcast.CallSkill(handlers, live, target, def)
	l.syncCubicTargets(live, result, def)
}

// showScriptMultisell shows live the multisell list name as the civilian
// NPC objectID offers it, as a script opens it; an NPC no longer in the
// world shows nothing.
func (l *GameClientLink) showScriptMultisell(live *livePlayer, objectID int32, name string) {
	if l.world == nil {
		return
	}
	obj, ok := l.world.Object(objectID)
	if !ok {
		return
	}
	if f, ok := obj.(*npc.Folk); ok {
		l.openMultisell(live, f, name, false)
	}
}
