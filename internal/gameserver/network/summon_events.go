package network

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// summonSink maps one live summon's events to packets and follow-up actions.
type summonSink struct {
	link  *GameClientLink
	actor *summon.Actor
	// brain and move are the summon's AI loop and movement controller its
	// attack and arrival events re-evaluate; move is nil without geodata.
	brain *ai.Summon
	move  *move.Controller
	// cleanupMu guards cleanup and despawned. Registration runs on the
	// spawning goroutine while the AI task and the offensive-follow ticker
	// can already reach this sink and drive it to Despawned, so the two
	// sides must not race: onDespawn runs fn immediately when the summon
	// has already left the world, which is what keeps a registration that
	// lost that race from leaking.
	cleanupMu sync.Mutex
	cleanup   []func()
	despawned bool
}

// onDespawn registers fn as runtime cleanup that runs exactly once when the
// summon leaves the world, or immediately when it already has. Register a
// cleanup only after the resource it releases exists, so that the immediate
// path cannot run before the thing it undoes.
func (s *summonSink) onDespawn(fn func()) {
	if fn == nil {
		return
	}
	s.cleanupMu.Lock()
	if s.despawned {
		s.cleanupMu.Unlock()
		fn()
		return
	}
	s.cleanup = append(s.cleanup, fn)
	s.cleanupMu.Unlock()
}

// runDespawn releases every registered cleanup, in registration order, and
// marks the summon despawned so later registrations release themselves.
//
// Each cleanup runs under its own recover. The sink is already marked
// despawned by the time any of them runs, so a panic escaping one would
// strand every cleanup after it with no way to ask again: a later
// event.Despawned returns at the guard above, and only newly registered
// cleanups would still fire. The queue worker recovers and keeps going
// (sim/pool.go), so that would surface as nothing at all -- the AI-task
// registration this type exists to hand back would simply never come back.
// Isolating each one keeps a failing cleanup from taking the rest with it,
// and logs the failure rather than dropping it silently.
func (s *summonSink) runDespawn() {
	s.cleanupMu.Lock()
	if s.despawned {
		s.cleanupMu.Unlock()
		return
	}
	s.despawned = true
	pending := s.cleanup
	s.cleanup = nil
	s.cleanupMu.Unlock()
	for _, fn := range pending {
		s.runCleanup(fn)
	}
}

// runCleanup runs one cleanup, recovering and logging a panic so the
// cleanups after it still run.
func (s *summonSink) runCleanup(fn func()) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if s.link == nil {
			return
		}
		event := s.link.log.Error().Interface("panic", r)
		if s.actor != nil {
			event = event.Int32("summon_id", s.actor.ObjectID())
		}
		event.Msg("summon: recovered panic in despawn cleanup")
	}()
	fn()
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
	case event.AttackFinished:
		s.brain.Think()
	case event.Arrived:
		actor.SyncPosition(s.move.Position())
		s.brain.Think()
	case event.MoveBlocked:
		s.move.BroadcastBlockedCorrection()
	case event.Unsummoning:
		// Recovered like a despawn cleanup: this runs inside the summon's
		// one-time despawn, so a panic escaping it would leave the summon in
		// the world with no second despawn to take it out.
		s.runCleanup(func() { l.releasePet(actor) })
	case event.Despawned:
		s.runDespawn()
	}
}

// releasePet settles a pet on its way out of the world, whatever took it out:
// its items go back to its owner, its row and collar are saved, and its
// container is then flushed and unregistered, since nothing else will ever
// write it. It runs while the pet and its owner are both still in the world,
// before PetDelete is sent.
//
// A dead pet is settled the same way. The routes that must leave a corpse
// alone refuse before reaching here (Actor.Unsummon), so a dead pet only
// arrives with its owner's logout or after dying mid-despawn. Its container
// is keyed by an object id no later summon reuses, so items left in it would
// be lost for good, and skipping the row would restore it alive from an
// older save.
func (l *GameClientLink) releasePet(actor *summon.Actor) {
	if !actor.IsPet() {
		return
	}
	var ownerInv *itemcontainer.Inventory
	if owner, ok := liveSummonOwner(actor); ok {
		ownerInv = owner.Inventory()
	}
	l.transferPetInventory(actor, ownerInv)
	l.savePet(actor, ownerInv)
	if inv := actor.PetInventory(); inv != nil {
		l.flushItemPersistence(inv)
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
