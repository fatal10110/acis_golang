package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// talkToFolk is a civilian NPC's answer to a player's interact in reach:
// its talk animation for everyone watching it; the NPC becomes the player's
// last quest NPC, and a script holding its first talk answers instead of
// it; a wedding manager skips both. Otherwise its chat window opens, with
// ActionFailed — first for a Seven Signs priest, after the page for
// everyone else. A muted NPC does nothing. A wedding manager greets with
// its own page alone. A Mammon NPC refusing the talker says why instead,
// releasing the client after a cabal refusal. An NPC whose first dialog
// reads state of a system not in place opens nothing; the interact's think
// has already released the client.
func (l *GameClientLink) talkToFolk(live *livePlayer, f *npc.Folk) {
	if f.Muted() {
		return
	}
	if id, ok := f.TalkAnimation(time.Now()); ok {
		l.broadcastNPCFrame(f, func() wire.Frame { return serverpackets.FrameSocialAction(f.ObjectID(), id) })
	}
	if f.QuestTalker() && l.talkThroughScripts(live, f) {
		return
	}
	if showObserverGroups(live, f) || l.showSiegeMessenger(live, f) {
		return
	}
	html, outcome := f.ChatWindow(setPages{l.html}, l.playerConfig.chatRules(), live.Karma(), folkChatState{l: l, live: live})
	switch outcome {
	case npc.ChatWedding:
		l.weddingGreeting(live, f)
	case npc.ChatUnported:
		l.log.Debug().Int("npc_id", f.NpcID()).Str("type", f.Instance.Template.Type).Msg("npc: chat window not modeled")
	default:
		sendFolkChat(live, f, html, outcome)
	}
}

// sendFolkChat opens html, f's chat window, the way outcome says: with
// ActionFailed first for a Seven Signs priest, after the page for everyone
// else. A Mammon NPC refusing the talker says why instead, releasing the
// client after a cabal refusal.
func sendFolkChat(live *livePlayer, f *npc.Folk, html string, outcome npc.ChatOutcome) {
	switch outcome {
	case npc.ChatShownReleased:
		live.SendFrame(serverpackets.FrameActionFailed())
		sendFilledHTML(live, f.ObjectID(), html, 0)
	case npc.ChatDawnOnly, npc.ChatDuskOnly:
		message := serverpackets.SystemMessageCanBeUsedByDawn
		if outcome == npc.ChatDuskOnly {
			message = serverpackets.SystemMessageCanBeUsedByDusk
		}
		live.SendFrame(serverpackets.FrameSystemMessage(message))
		live.SendFrame(serverpackets.FrameActionFailed())
	case npc.ChatCompetitionOnly:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageQuestEventPeriod))
	default:
		sendFilledHTML(live, f.ObjectID(), html, 0)
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// folkChatState is what a civilian NPC's chat window reads of live and of
// the Seven Signs, festival and noble state.
type folkChatState struct {
	l    *GameClientLink
	live *livePlayer
}

func (s folkChatState) SevenSigns() sevensigns.Record {
	if s.l.sevenSigns == nil {
		return sevensigns.Record{}
	}
	return s.l.sevenSigns.Record(s.live.ObjectID())
}

func (s folkChatState) FestivalNotice() string {
	if s.l.festival == nil {
		return ""
	}
	return s.l.festival.NextFestivalNotice()
}

func (s folkChatState) Noble() bool { return s.live.IsNoble() }

func (s folkChatState) Hero() (isHero, inactive bool) {
	return s.live.IsHero(), s.l.heroes != nil && s.l.heroes.IsInactive(s.live.ObjectID())
}

// broadcastNPCFrame sends one serialized frame to every player that knows
// f, an NPC, each an independently owned copy.
func (l *GameClientLink) broadcastNPCFrame(f world.Tracked, build func() wire.Frame) {
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		l.world.ForEachKnown(f, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// A civilian NPC is a skill cast participant.
var _ skillhandler.NPC = (*npc.Folk)(nil)

// FolkSinks returns the event-sink factory for civilian NPCs spawned into
// state; stance tracks the attack stance a hit puts one in.
func FolkSinks(state *world.State, stance AttackStanceTracker) func(*npc.Folk) event.Sink {
	return func(f *npc.Folk) event.Sink { return &folkSink{world: state, stance: stance, f: f} }
}

// FolkSinks is the package FolkSinks whose sinks also carry out what a
// clan hall manager's AI asks of l: the check of its own support buff and
// its answers to the players it casts support magic on.
func (l *GameClientLink) FolkSinks(state *world.State, stance AttackStanceTracker) func(*npc.Folk) event.Sink {
	return func(f *npc.Folk) event.Sink { return &folkSink{world: state, stance: stance, f: f, link: l} }
}

// folkSink maps one civilian NPC's events to packets for its observers.
// link, when set, carries out a clan hall manager's AI requests.
type folkSink struct {
	world  *world.State
	stance AttackStanceTracker
	f      *npc.Folk
	known  world.KnownBuffer
	link   *GameClientLink
}

// Emit maps ev to the frame every known observer receives.
func (s *folkSink) Emit(ev event.Event) {
	f := s.f
	frames := serverpackets.NpcFrameBuilder{}
	switch e := ev.(type) {
	case event.Move:
		s.broadcast(func() wire.Frame { return frames.Move(f.ObjectID(), e) })
	case event.Stopped:
		x, y, z := f.Position()
		at := location.Location{X: x, Y: y, Z: z}
		s.broadcast(func() wire.Frame { return frames.Stop(f.ObjectID(), at, f.Heading()) })
	case event.Teleported:
		s.broadcast(func() wire.Frame { return serverpackets.FrameTeleportToLocation(f.ObjectID(), e.To, false) })
	case event.SocialAction:
		s.broadcast(func() wire.Frame { return frames.SocialAction(f.ObjectID(), e.ID) })
	case event.NpcSay:
		s.broadcast(func() wire.Frame { return frames.NpcSay(f.ObjectID(), e.NpcID, e.Text) })
	case event.HPChanged:
		known := s.known.SnapshotCopy(s.world, f)
		defer known.Release()
		sendHPToWatchers(known.Tracked(), f.ObjectID(), f.PublishHP)
	case event.Status:
		attrs := npcStatusAttributes(e.Attrs)
		s.broadcast(func() wire.Frame { return frames.Status(f.ObjectID(), attrs) })
	case event.Attacked, event.AttackStanceRequested:
		s.startAttackStance()
	case event.Died:
		s.broadcast(func() wire.Frame { return frames.Die(f.ObjectID(), false) })
		s.stopAttackStance()
	case event.MagicSkillUse:
		s.broadcast(func() wire.Frame {
			return frames.SkillUse(e.CasterID, e.CasterAt, e.TargetID, e.TargetAt, e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false)
		})
	case event.SkillLaunched:
		s.broadcast(func() wire.Frame { return frames.SkillLaunched(f.ObjectID(), e.SkillID, e.Level, e.TargetIDs) })
	case event.SkillCanceled:
		s.broadcast(func() wire.Frame { return frames.SkillCanceled(e.ObjectID) })
	case event.MoveToPawn:
		s.broadcast(func() wire.Frame { return frames.MoveToPawn(f.ObjectID(), e.TargetID, e.Distance, e.Origin) })
	case event.AutoAttackStopped:
		s.broadcast(func() wire.Frame { return serverpackets.FrameAutoAttackStop(f.ObjectID()) })
	case event.MoveTypeChanged:
		s.broadcast(func() wire.Frame { return frames.ChangeMoveType(f.ObjectID(), e.Running) })
	case event.AbnormalEffectChanged:
		s.broadcast(func() wire.Frame { return frames.Info(f.NPCInfoSnapshot()) })
	case event.HallManagerBuffCheck:
		if s.link != nil {
			s.link.hallManagerSelfBuff(f)
		}
	case event.HallSupportCast:
		if s.link != nil {
			s.link.hallSupportAnswer(f, e)
		}
	case event.NPCInfoChanged:
		if e.ServerObject {
			s.broadcast(func() wire.Frame { return frames.ObjectInfo(f.ServerObjectInfoSnapshot()) })
			return
		}
		s.broadcast(func() wire.Frame { return frames.Info(f.NPCInfoSnapshot()) })
	}
}

// startAttackStance enters or refreshes the NPC's attack stance. Entering
// it shows AutoAttackStart to the NPC's observers.
func (s *folkSink) startAttackStance() {
	if s.stance == nil {
		return
	}
	s.stance.Add(s.f)
	if !s.f.SetInCombat(true) {
		return
	}
	s.broadcast(func() wire.Frame { return serverpackets.FrameAutoAttackStart(s.f.ObjectID()) })
}

// stopAttackStance ends the attack stance of an NPC that died: observers
// that saw it fall see its stance end.
func (s *folkSink) stopAttackStance() {
	if s.stance == nil {
		return
	}
	s.f.SetInCombat(false)
	if !s.stance.Remove(s.f) {
		return
	}
	s.broadcast(func() wire.Frame { return serverpackets.FrameAutoAttackStop(s.f.ObjectID()) })
}

// broadcast fans one lazily built frame out to the NPC's observers.
func (s *folkSink) broadcast(build func() wire.Frame) {
	broadcastKnown(&s.known, s.world, s.f, build)
}

// chatRules are the karma gates on service NPC dialogs.
func (c PlayerConfig) chatRules() npc.ChatRules {
	return npc.ChatRules{
		KarmaCanShop:         c.KarmaPlayerCanShop,
		KarmaCanUseGK:        c.KarmaPlayerCanUseGK,
		KarmaCanUseWarehouse: c.KarmaPlayerCanUseWareHouse,
	}
}
