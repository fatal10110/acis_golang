package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// talkToFolk is a civilian NPC's answer to a player's interact in reach:
// its talk animation for everyone watching it, then its chat window with
// ActionFailed. A muted NPC does nothing. An NPC whose first dialog reads
// state of a system not in place opens nothing; the interact's think has
// already released the client.
func (l *GameClientLink) talkToFolk(live *livePlayer, f *npc.Folk) {
	if f.Muted() {
		return
	}
	if id, ok := f.TalkAnimation(time.Now()); ok {
		l.broadcastFolkFrame(f, func() wire.Frame { return serverpackets.FrameSocialAction(f.ObjectID(), id) })
	}
	html, outcome := f.ChatWindow(l.html, l.playerConfig.chatRules(), live.Karma())
	if outcome == npc.ChatUnported {
		l.log.Debug().Int("npc_id", f.NpcID()).Str("type", f.Instance.Template.Type).Msg("npc: chat window not modeled")
		return
	}
	sendValidatedHTML(live, f.ObjectID(), html, 0)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// broadcastFolkFrame sends one serialized frame to every player that knows
// f, each an independently owned copy.
func (l *GameClientLink) broadcastFolkFrame(f *npc.Folk, build func() wire.Frame) {
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

// folkSink maps one civilian NPC's events to packets for its observers.
type folkSink struct {
	world  *world.State
	stance AttackStanceTracker
	f      *npc.Folk
	known  world.KnownBuffer
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
		sendHPToWatchers(known.Tracked(), f.ObjectID(), f.HPStatusUpdate)
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
