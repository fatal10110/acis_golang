package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// shotRechargeRadius is how far an NPC's shot-recharge animation reaches.
const shotRechargeRadius = 600

// HostileSinks returns the event-sink factory for hostile NPCs spawned into
// state.
func HostileSinks(state *world.State) func(*npc.Hostile) event.Sink {
	return func(h *npc.Hostile) event.Sink { return &hostileSink{world: state, h: h} }
}

// hostileSink maps one hostile NPC's events to packets for its observers.
type hostileSink struct {
	world *world.State
	h     *npc.Hostile
	known world.KnownBuffer
}

// Emit maps ev to the frame every known observer receives.
func (s *hostileSink) Emit(ev event.Event) {
	h := s.h
	frames := serverpackets.NpcFrameBuilder{}
	switch e := ev.(type) {
	case event.Attack:
		s.broadcast(func() wire.Frame { return frames.Attack(e) })
	case event.MagicSkillUse:
		s.broadcast(func() wire.Frame {
			return frames.SkillUse(e.CasterID, e.CasterAt, e.TargetID, e.TargetAt, e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false)
		})
	case event.SkillLaunched:
		s.broadcast(func() wire.Frame { return frames.SkillLaunched(h.ObjectID(), e.SkillID, e.Level, e.TargetIDs) })
	case event.SkillCanceled:
		s.broadcast(func() wire.Frame { return frames.SkillCanceled(e.ObjectID) })
	case event.Died:
		s.broadcast(func() wire.Frame { return frames.Die(h.ObjectID(), h.SpoilPool().Sweepable()) })
	case event.Move:
		s.broadcast(func() wire.Frame { return frames.Move(h.ObjectID(), e) })
	case event.MoveToPawn:
		s.broadcast(func() wire.Frame { return frames.MoveToPawn(h.ObjectID(), e.TargetID, e.Distance, e.Origin) })
	case event.Stopped:
		x, y, z := h.Position()
		at := location.Location{X: x, Y: y, Z: z}
		s.broadcast(func() wire.Frame { return frames.Stop(h.ObjectID(), at, h.Heading()) })
	case event.Status:
		attrs := npcStatusAttributes(e.Attrs)
		s.broadcast(func() wire.Frame { return frames.Status(h.ObjectID(), attrs) })
	case event.AbnormalEffectChanged:
		s.broadcast(func() wire.Frame { return frames.Info(h.NPCInfoSnapshot()) })
	case event.NPCInfoChanged:
		if e.ServerObject {
			s.broadcast(func() wire.Frame { return frames.ObjectInfo(h.ServerObjectInfoSnapshot()) })
			return
		}
		s.broadcast(func() wire.Frame { return frames.Info(h.NPCInfoSnapshot()) })
	case event.MoveTypeChanged:
		s.broadcast(func() wire.Frame { return frames.ChangeMoveType(h.ObjectID(), e.Running) })
	case event.Flight:
		x, y, z := h.Position()
		s.broadcast(func() wire.Frame {
			return frames.FlyTo(h.ObjectID(), e.Dest, location.Location{X: x, Y: y, Z: z}, e.Flight)
		})
	case event.PositionCorrected:
		x, y, z := h.Position()
		s.broadcast(func() wire.Frame {
			return frames.ValidateLocation(h.ObjectID(), location.Location{X: x, Y: y, Z: z}, h.Heading())
		})
	case event.SocialAction:
		s.broadcast(func() wire.Frame { return frames.SocialAction(h.ObjectID(), e.ID) })
	case event.NpcSay:
		s.broadcast(func() wire.Frame { return frames.NpcSay(h.ObjectID(), e.NpcID, e.Text) })
	case event.ShotRecharged:
		broadcastFrame(func() wire.Frame {
			return frames.SkillUse(h.ObjectID(), e.At, h.ObjectID(), e.At, e.SkillID, 1, 0, 0, false)
		}, func(send func(frameReceiver)) {
			s.world.ForEachKnownInRadius(h, shotRechargeRadius, func(o world.Tracked) {
				if receiver, ok := o.(frameReceiver); ok {
					send(receiver)
				}
			})
		})
	}
}

// broadcast fans one lazily built frame out over a detached snapshot of the
// NPC's known list, so no region lock is held while frames are sent.
func (s *hostileSink) broadcast(build func() wire.Frame) {
	known := s.known.SnapshotCopy(s.world, s.h)
	defer known.Release()
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, o := range known.Tracked() {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		}
	})
}

func npcStatusAttributes(attrs []event.StatusAttr) []npcinfo.StatusAttribute {
	out := make([]npcinfo.StatusAttribute, len(attrs))
	for i, attr := range attrs {
		typ := npcinfo.StatusCurrentHP
		switch attr.Kind {
		case event.StatusMaxHP:
			typ = npcinfo.StatusMaxHP
		case event.StatusPhysicalSpeed:
			typ = npcinfo.StatusPhysicalSpeed
		case event.StatusMagicSpeed:
			typ = npcinfo.StatusMagicSpeed
		}
		out[i] = npcinfo.StatusAttribute{Type: typ, Value: attr.Value}
	}
	return out
}

// EffectPointSinks returns the event-sink factory for signet effect points
// spawned into state.
func EffectPointSinks(state *world.State) func(*npc.EffectPoint) event.Sink {
	return func(ep *npc.EffectPoint) event.Sink { return &effectPointSink{world: state, ep: ep} }
}

// effectPointSink maps one signet effect point's cast events to packets.
type effectPointSink struct {
	world *world.State
	ep    *npc.EffectPoint
}

// Emit maps ev to the frame every known observer receives.
func (s *effectPointSink) Emit(ev event.Event) {
	frames := serverpackets.NpcFrameBuilder{}
	var frame wire.Frame
	switch e := ev.(type) {
	case event.MagicSkillUse:
		frame = frames.SkillUse(e.CasterID, e.CasterAt, e.TargetID, e.TargetAt, e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false)
	case event.SkillLaunched:
		frame = frames.SkillLaunched(s.ep.ObjectID(), e.SkillID, e.Level, e.TargetIDs)
	default:
		return
	}
	defer frame.Release()
	s.world.ForEachKnown(s.ep, func(o world.Tracked) {
		receiver, ok := o.(frameReceiver)
		if !ok {
			return
		}
		if owned, ok := serverpackets.CopyFrame(frame); ok {
			receiver.BroadcastFrame(owned)
		}
	})
}

// DoorSinks returns the event-sink factory for doors spawned into state.
func DoorSinks(state *world.State) func(*door.Object) event.Sink {
	return func(o *door.Object) event.Sink { return &doorSink{world: state, door: o} }
}

// doorSink maps one door's status events to packets.
type doorSink struct {
	world *world.State
	door  *door.Object
	known world.KnownBuffer
}

// Emit sends the door's open/close state to every known observer.
func (s *doorSink) Emit(ev event.Event) {
	if _, ok := ev.(event.StatusChanged); !ok {
		return
	}
	known := s.known.SnapshotCopy(s.world, s.door)
	defer known.Release()
	broadcastFrame(func() wire.Frame {
		return serverpackets.DoorFrameBuilder{}.StatusUpdate(s.door, false)
	}, func(send func(frameReceiver)) {
		for _, o := range known.Tracked() {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		}
	})
}
