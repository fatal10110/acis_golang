package network

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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
	// attack and cast are the summon's attack and cast controllers, whose
	// timers run on the summon's queue: a pet corpse handed back to its
	// owner's new session moves them there (relinkToOwner).
	attack *attack.Controller
	cast   *actorcast.Controller
	// corpseQueue is the queue a corpse its owner left behind has of its
	// own, until the corpse leaves the world or its owner comes back for it.
	corpseQueue atomic.Pointer[sim.Queue]
	// cleanupMu guards cleanup and despawned. Registration runs on the
	// owner's queue while the summon can already be driven to Despawned
	// from another actor's queue (a hostile Erase, a signet), so the two
	// sides must not race: onDespawn runs fn immediately when the summon
	// has already left the world, which is what keeps a registration that
	// lost that race from leaking.
	cleanupMu sync.Mutex
	cleanup   []func()
	despawned bool
	// known is the scratch known-list snapshot the health-bar update reads.
	known world.KnownBuffer
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
	case event.Flight:
		x, y, z := actor.Position()
		at := location.Location{X: x, Y: y, Z: z}
		l.broadcastSummon(actor, func() wire.Frame { return frames.FlyTo(actor.ObjectID(), e.Dest, at, e.Flight) })
	case event.PositionCorrected:
		x, y, z := actor.Position()
		at, heading := location.Location{X: x, Y: y, Z: z}, actor.Heading()
		l.broadcastSummon(actor, func() wire.Frame { return frames.ValidateLocation(actor.ObjectID(), at, heading) })
	case event.MagicSkillUse:
		l.broadcastSummon(actor, func() wire.Frame {
			return frames.SkillUse(e.CasterID, e.CasterAt, e.TargetID, e.TargetAt, e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false)
		})
	case event.SkillLaunched:
		l.broadcastSummon(actor, func() wire.Frame {
			return frames.SkillLaunched(actor.ObjectID(), e.SkillID, e.Level, e.TargetIDs)
		})
	case event.HitLanded:
		l.chance.AttackHit(actor, e)
	case event.AttackStanceRequested, event.Attacked:
		l.startSummonAttackStance(actor)
	case event.CastAborted:
		// The cancel animation goes to every observer; the owner reads an
		// interrupt only once the AI has moved on (see CastFinished).
		l.broadcastSummon(actor, func() wire.Frame { return frames.SkillCanceled(actor.ObjectID()) })
	case event.AutoAttackStopped:
		l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStop(actor.ObjectID()))
	case event.SocialAction:
		l.broadcastSummon(actor, func() wire.Frame { return frames.SocialAction(actor.ObjectID(), e.ID) })
	case event.StatusChanged:
		l.broadcastSummonStatus(actor)
	case event.HPChanged:
		// A vitals change first updates the health bar of the players
		// targeting the summon, then refreshes its owner and observers.
		s.broadcastHP()
		l.broadcastSummonStatus(actor)
	case event.OwnerInfoChanged:
		sendSummonInfosToOwner(actor)
	case event.AbnormalEffectChanged:
		l.refreshSummonAbnormalEffect(actor)
	case event.AttackTargetRefused:
		if owner, ok := l.livePlayerByID(actor.OwnerID()); ok {
			owner.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		}
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
	case event.HitDealt:
		owner, ok := l.livePlayerByID(actor.OwnerID())
		if !ok {
			return
		}
		source := skillhandler.DamageByServitor
		if actor.IsPet() {
			source = skillhandler.DamageByPet
		}
		sendDamageMessage(owner, hitDamage(e, source))
	case event.Died:
		// Observers see the summon fall, then its own combat stance end. The
		// stance is its owner's: the owner stays in combat and keeps its own
		// stance icon.
		l.broadcastSummon(actor, func() wire.Frame { return frames.Die(actor.ObjectID(), false) })
		l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStop(actor.ObjectID()))
	case event.DeathSettled:
		l.notifyOwnerOfSummonDeath(actor)
		if l.decay != nil {
			actor.SetCorpseDeadline(l.decay.Add(actor, actor.DecayDelay()))
		}
	case event.Revived:
		l.broadcastSummon(actor, func() wire.Frame { return serverpackets.FrameRevive(actor.ObjectID()) })
	case event.DecayCanceled:
		if l.decay != nil {
			l.decay.Cancel(actor)
		}
	case event.PetCorpseDecayed:
		l.destroyDecayedPet(actor)
	case event.CorpseLeftBehind:
		s.leaveCorpseBehind()
	case event.OwnerRelinked:
		s.relinkToOwner()
	case event.AttackFinished:
		actor.FinishedAttack()
	case event.CastFinished:
		if !e.Interrupted {
			actor.FinishedCasting()
			return
		}
		// A stopped cast moves the AI on and sends the summon idle; an
		// interrupt then tells the owner, since a summon's own messages
		// reach its owner.
		actor.CastStopped()
		if e.Broken {
			if owner, ok := l.livePlayerByID(actor.OwnerID()); ok {
				owner.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCastingInterrupted))
			}
		}
	case event.Arrived:
		actor.SyncPosition(s.move.Position())
		if s.brain.Arrived() {
			actor.TryToIdle()
			return
		}
		s.brain.Think()
	case event.MoveBlocked:
		s.move.BroadcastBlockedCorrection()
	case event.Unsummoning:
		if actor.OwnerLeft() {
			// A corpse its owner left behind has not acted since its row was
			// saved (leaveCorpseBehind); its items are all that is left to
			// settle.
			s.runCleanup(func() { l.settleLeftCorpseItems(actor) })
			return
		}
		// Recovered like a despawn cleanup: this runs inside the summon's
		// one-time despawn, so a panic escaping it would leave the summon in
		// the world with no second despawn to take it out. The abort comes
		// first, so a stop broadcast still reaches observers and a cast or
		// attack in flight cannot land from a summon that is leaving.
		if s.brain != nil {
			s.runCleanup(s.brain.AbortAll)
		}
		s.runCleanup(func() { l.releasePet(actor) })
	case event.SummonRemoved:
		if owner, ok := l.currentSummonOwner(actor); ok {
			owner.SendFrame(serverpackets.FramePetDelete(actor.SummonType(), actor.ObjectID()))
		}
	case event.Despawned:
		s.runDespawn()
	}
}

// broadcastHP sends the summon's current HP to the known players targeting
// it, its owner included.
func (s *summonSink) broadcastHP() {
	if s.link.world == nil || s.actor == nil {
		return
	}
	known := s.known.SnapshotCopy(s.link.world, s.actor)
	defer known.Release()
	sendHPToWatchers(known.Tracked(), s.actor.ObjectID(), s.actor.HPStatusUpdate)
}

// leaveCorpseBehind settles a dead summon whose owner is leaving the world
// while its corpse stays: a pet's row and collar are saved, as for any other
// departure, and its items stay with the corpse until it decays or its owner
// comes back for it. The corpse moves to a queue of its own, which closes
// when the corpse leaves the world or its owner comes back. It runs on the
// leaving owner's queue, before that queue closes.
func (s *summonSink) leaveCorpseBehind() {
	l, actor := s.link, s.actor
	if actor.IsPet() {
		var ownerInv *itemcontainer.Inventory
		if owner, ok := liveSummonOwner(actor); ok {
			ownerInv = owner.Inventory()
		}
		s.runCleanup(func() { l.savePet(actor, ownerInv) })
	}
	if l.queues == nil {
		return
	}
	q := l.queues.NewQueue(fmt.Sprintf("corpse-%d", actor.ObjectID()))
	s.corpseQueue.Store(q)
	actor.AdoptCorpseQueue(q)
	s.onDespawn(q.Close)
}

// relinkToOwner moves the rest of a relinked pet's work onto its owner's
// queue, which the pet itself already runs on (summon.Actor.RelinkOwner):
// its attack and cast timers and its offensive-follow ticker, which stopped
// with the queue of the session that left it. The corpse's own queue then
// closes. It runs as that queue's owner, as the relink does.
func (s *summonSink) relinkToOwner() {
	q := s.actor.Queue()
	if s.attack != nil {
		s.attack.SetQueue(q)
	}
	if s.cast != nil {
		s.cast.SetQueue(q)
	}
	if s.brain != nil {
		s.onDespawn(s.brain.StartOffensiveFollowTicker(q))
	}
	if corpse := s.corpseQueue.Swap(nil); corpse != nil {
		corpse.Close()
	}
}

// reclaimPetCorpse hands the pet corpse live's character left behind, if
// any, back to live: the corpse answers to live from then on and its work
// runs on live's queue. It runs on live's queue, so a logout of live cannot
// interleave with it, and takes over the corpse's own queue for the relink,
// so none of the corpse's work (its decay) runs meanwhile; work posted there
// earlier runs first, and work posted since moves on to live's queue.
// Nothing on a corpse's queue waits on its owner's, so taking it over from
// here cannot deadlock.
func (l *GameClientLink) reclaimPetCorpse(live *livePlayer) {
	if l.world == nil {
		return
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return
	}
	actor, ok := obj.(*summon.Actor)
	if !ok || !actor.IsPet() || !actor.OwnerLeft() {
		return
	}
	corpse := actor.Queue()
	if corpse == nil {
		return
	}
	sim.RunOwned(corpse, func() {
		actor.RelinkOwner(live, live.Inventory(), live.Queue())
	})
}

// settleLeftCorpseItems hands the items of a pet corpse its owner left
// behind to that owner as the corpse decays. An owner back in the world
// whose session has not taken the corpse over yet gets them in its
// inventory, on its own queue, as for any other departure (releasePet).
// Otherwise the owner is offline: every item becomes a row of the owner's
// carried inventory, without a capacity check or a stack merge, since the
// inventory of a session that left holds nothing; the owner's next login
// merges the stacks. The rows are written on the owner's persistence lane,
// ahead of the collar delete the decay queues next.
func (l *GameClientLink) settleLeftCorpseItems(actor *summon.Actor) {
	petInv := actor.PetInventory()
	if petInv == nil {
		return
	}
	ownerID := actor.OwnerID()
	offline := func() {
		// The pet's container stops persisting first, so the items leave
		// it carrying no writer; the owner's rows are written below.
		l.flushItemPersistence(petInv)
		carried := itemcontainer.NewPlayerInventory(ownerID, petInv.Templates())
		for _, inst := range petInv.Items() {
			st := inst.Snapshot()
			petInv.TransferItem(st.ObjectID, st.Count, carried, 0)
		}
		l.flushItemPersistence(carried)
	}
	owner, ok := l.livePlayerByID(ownerID)
	if !ok {
		offline()
		return
	}
	// A session on its way out has queued its inventory's last writes by
	// the time it is marked detached or its queue refuses this: the owner is
	// offline then.
	posted := postLive(owner, func() {
		if owner.detached() {
			offline()
			return
		}
		l.transferPetInventory(actor, owner.Inventory())
		l.flushItemPersistence(petInv)
	})
	if !posted {
		offline()
	}
}

// currentSummonOwner returns the connected player actor answers to. That is
// its owner's session, until the owner leaves the world with actor lying
// dead. A pet's corpse then answers to whichever session its owner comes back
// with, and a servitor's corpse to nobody.
func (l *GameClientLink) currentSummonOwner(actor *summon.Actor) (*livePlayer, bool) {
	if !actor.OwnerLeft() {
		return liveSummonOwner(actor)
	}
	if !actor.IsPet() {
		return nil, false
	}
	return l.livePlayerByID(actor.OwnerID())
}

// notifyOwnerOfSummonDeath closes a summon's death for its owner: the
// owner's summon auto-shots are turned off, then the owner reads the death
// message for a servitor or a pet.
func (l *GameClientLink) notifyOwnerOfSummonDeath(actor *summon.Actor) {
	owner, ok := liveSummonOwner(actor)
	if !ok {
		return
	}
	for _, itemID := range item.SummonShotIDs() {
		if owner.AutoSoulShotEnabled(itemID) {
			l.disableAutoShot(owner, itemID)
		}
	}
	message := serverpackets.SystemMessageServitorPassedAway
	if actor.IsPet() {
		message = serverpackets.SystemMessageResurrectPetWithin20Minutes
	}
	owner.SendFrame(serverpackets.FrameSystemMessage(message))
}

// petRowDeleter deletes a pet's saved row. The production pet store has it;
// a store without it keeps the row.
type petRowDeleter interface {
	DeleteByItemObjectID(ctx context.Context, itemObjectID int32) error
}

// destroyDecayedPet ends a pet whose corpse decayed: its owner loses the
// collar and the pets row is deleted. The pet has already left the world
// through the same settle as any other departure (releasePet), so its items
// are already back with its owner and its row was saved; the delete is queued
// on the collar's persistence lane, behind that save.
//
// A corpse whose owner left had its row saved when the owner left, and its
// items went to the owner as it left the world (settleLeftCorpseItems). The
// collar then goes from the owner's current session, on that session's queue,
// behind the items, or, with the owner offline, straight from the items table
// on the owner's lane, behind the rows the owner's logout and the items
// wrote.
func (l *GameClientLink) destroyDecayedPet(actor *summon.Actor) {
	itemObjectID := actor.ControlItemID()
	switch owner, ok := l.currentSummonOwner(actor); {
	case ok && !actor.OwnerLeft():
		if inv := owner.Inventory(); inv != nil {
			inv.DestroyByObjectID(itemObjectID, 1)
		}
	case ok:
		// A session on its way out has queued its inventory's last writes
		// by the time it is marked detached or its queue refuses this: the
		// owner is offline then.
		posted := postLive(owner, func() {
			if owner.detached() {
				l.deleteOfflineItem(actor.OwnerID(), itemObjectID)
				return
			}
			if inv := owner.Inventory(); inv != nil {
				inv.DestroyByObjectID(itemObjectID, 1)
			}
		})
		if !posted {
			l.deleteOfflineItem(actor.OwnerID(), itemObjectID)
		}
	case actor.OwnerLeft():
		l.deleteOfflineItem(actor.OwnerID(), itemObjectID)
	}
	store, ok := l.petStore.(petRowDeleter)
	if !ok || l.persist == nil {
		return
	}
	log := l.log
	l.persist.Enqueue(itemObjectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := store.DeleteByItemObjectID(ctx, itemObjectID); err != nil {
			log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("delete decayed pet")
		}
	})
}

// deleteOfflineItem deletes an offline owner's item row, queued on the
// owner's persistence lane behind every write the owner's logout queued for
// it. A row the owner no longer owns is left alone: the item has changed
// hands since, and it is its new owner's now.
func (l *GameClientLink) deleteOfflineItem(ownerID, objectID int32) {
	if l.items == nil || l.persist == nil {
		return
	}
	l.queueItemWrite(ownerID, l.itemWrites.Reserve(objectID), func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if _, err := l.items.DeleteOwned(ctx, ownerID, objectID); err != nil {
			l.log.Error().Err(err).Int32("object_id", objectID).Msg("delete offline item")
		}
	})
}

// releasePet settles a pet on its way out of the world, whatever took it out:
// its items go back to its owner, its row and collar are saved, and its
// container is then flushed and unregistered, since nothing else will ever
// write it. It runs while the pet and its owner are both still in the world,
// before PetDelete is sent.
//
// A dead pet is settled the same way. The routes that must leave a corpse
// alone refuse before reaching here (Actor.Unsummon), so a dead pet only
// arrives with its corpse's decay while its owner is here, or after dying
// mid-despawn. Its items go to the owner now, since the collar goes with the
// decay, and skipping the row would restore it alive from an older save. A
// corpse its owner left behind is settled apart (settleLeftCorpseItems).
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
