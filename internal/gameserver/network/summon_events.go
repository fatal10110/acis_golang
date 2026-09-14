package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// summonSink maps one live summon's events to packets and follow-up actions.
type summonSink struct {
	link  *GameClientLink
	actor *summon.Actor
	// despawn is the runtime cleanup that runs exactly when the summon
	// leaves the world.
	despawn func()
}

// Emit maps ev to its packets. Each arm keeps the send order its packets
// reach clients in.
func (s *summonSink) Emit(ev event.Event) {
	l, actor := s.link, s.actor
	frames := serverpackets.NpcFrameBuilder{}
	switch e := ev.(type) {
	case event.Attack:
		l.broadcastSummon(actor, func() wire.Frame { return frames.Attack(e) })
	case event.Move:
		l.broadcastSummon(actor, func() wire.Frame { return frames.Move(actor.ObjectID(), e) })
	case event.MoveToPawn:
		l.broadcastSummon(actor, func() wire.Frame {
			return frames.MoveToPawn(actor.ObjectID(), e.TargetID, e.Distance, e.Origin)
		})
	case event.Stopped:
		x, y, z := actor.Position()
		l.broadcastSummon(actor, func() wire.Frame {
			return frames.Stop(actor.ObjectID(), location.Location{X: x, Y: y, Z: z}, actor.Heading())
		})
	case event.MagicSkillUse:
		l.broadcastSummon(actor, func() wire.Frame {
			return frames.SkillUse(e.CasterID, e.CasterAt, e.TargetID, e.TargetAt, e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false)
		})
	case event.AutoAttackStopped:
		l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStop(actor.ObjectID()))
	case event.StatusChanged:
		l.broadcastSummonStatus(actor)
	case event.OwnerInfoChanged:
		sendSummonInfosToOwner(actor)
	case event.AbnormalEffectChanged:
		l.refreshSummonAbnormalEffect(actor)
	case event.ExpGained:
		if owner, ok := l.livePlayerByID(actor.OwnerID()); ok {
			owner.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessagePetEarnedS1Exp, int32(e.Exp)))
		}
	case event.Damaged:
		owner, ok := l.livePlayerByID(actor.OwnerID())
		if !ok {
			return
		}
		messageID := serverpackets.SystemMessageSummonReceivedS2ByS1
		if actor.IsPet() {
			messageID = serverpackets.SystemMessagePetReceivedS2DamageByS1
		}
		owner.SendFrame(serverpackets.FrameSystemMessageStringNumber(messageID, e.AttackerName, e.Damage))
	case event.Despawned:
		if s.despawn != nil {
			s.despawn()
		}
	}
}

// broadcastSummon builds one frame lazily, only once a known observer capable
// of receiving frames is found, and hands every such receiver its own copy.
func (l *GameClientLink) broadcastSummon(actor *summon.Actor, build func() wire.Frame) {
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		l.world.ForEachKnown(actor, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// broadcastSummonFrame sends an already-built frame to every known observer
// of actor capable of receiving one, taking ownership of frame.
func (l *GameClientLink) broadcastSummonFrame(actor *summon.Actor, frame wire.Frame) {
	defer frame.Release()
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(actor, func(o world.Tracked) {
		receiver, ok := o.(frameReceiver)
		if !ok {
			return
		}
		if owned, ok := serverpackets.CopyFrame(frame); ok {
			receiver.BroadcastFrame(owned)
		}
	})
}
