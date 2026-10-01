package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
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

// FolkSinks returns the event-sink factory for route-walking civilian NPCs
// spawned into state.
func FolkSinks(state *world.State) func(*npc.Folk) event.Sink {
	return func(f *npc.Folk) event.Sink { return &folkSink{world: state, f: f} }
}

// folkSink maps one walking civilian NPC's events to packets for its
// observers.
type folkSink struct {
	world *world.State
	f     *npc.Folk
	known world.KnownBuffer
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
	}
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
