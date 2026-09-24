package network

import (
	"cmp"
	"context"
	"sync"
	"time"

	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

const livePlayerDetachSaveTimeout = 2 * time.Second

// detachLivePlayer takes live out of the world and enqueues its final saves:
// the character row, position, death penalty, offline mark and skill state,
// then the container flushes, on the owners' lanes, and an active pet's row
// on its control item's lane. Values are copied before the teardown below
// changes them, and every save is enqueued before live leaves world state, so
// a later selection of the same character that waits on the lane sees them.
// It returns the container owner ids whose lanes received saves; pass them to
// awaitPersistence. A summon does not wait for the pet row (see queuedPets).
func (l *GameClientLink) detachLivePlayer(live *livePlayer) []int32 {
	if live == nil {
		return nil
	}
	owners := []int32{live.ObjectID()}
	l.abortFusionTargeting(live)
	// Stop any in-flight attack/movement timers before the session detaches
	// below — otherwise a timer goroutine can still fire after detach.
	live.Stop()
	l.cancelActiveTrade(live)
	// TaskEffects.Save runs on this queue too, so every autosave job is
	// already on the lane, or will never be, before the jobs below (#1948).
	live.markDetaching()

	if l.roster != nil || l.skills != nil {
		roster, skills, log := l.roster, l.skills, l.log
		character := live.Character
		charState := character.SaveState()
		skillState := skills.SaveState(character)
		l.persist.Enqueue(live.ObjectID(), func() {
			// One budget for the whole character save, not one per store,
			// started when the job runs rather than when it was queued.
			ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
			defer cancel()
			if roster != nil {
				if err := roster.Save(ctx, charState); err != nil {
					log.Error().Err(err).Int32("object_id", charState.ID).Msg("save player full stats")
				}
				if err := roster.SavePosition(ctx, charState); err != nil {
					log.Error().Err(err).Int32("object_id", charState.ID).Msg("save player position")
				}
				if err := roster.SaveDeathPenaltyLevel(ctx, charState); err != nil {
					log.Error().Err(err).Int32("object_id", charState.ID).Msg("save player death penalty level")
				}
				if err := roster.SaveOfflineRecency(ctx, character); err != nil {
					log.Error().Err(err).Int32("object_id", charState.ID).Msg("save player offline recency")
				}
			}
			if err := skills.Save(ctx, skillState); err != nil {
				log.Error().Err(err).Int32("object_id", charState.ID).Msg("save player skill state")
			}
		})
	}
	if l.playerClock != nil {
		l.playerClock.Remove(live.ObjectID())
	}
	if l.autosave != nil {
		l.autosave.Remove(live.ObjectID())
	}
	if l.water != nil {
		l.water.Remove(live)
	}
	if l.shadowItems != nil {
		l.shadowItems.Remove(live.ObjectID())
	}
	if l.attackStance != nil {
		// Unconditional, alongside the other registry removals: the
		// reference drops the stance entry here too (Player.java:6292,
		// between the water and pvp-flag removals), and live.Stop's
		// stopLiveAutoAttack cannot stand in for it. That helper clears the
		// in-combat flag before it reaches the tracker, so a detach that
		// resumes after an aborted first pass finds the flag already down,
		// returns early, and would leave the entry behind for good: the
		// sweep re-posts its expiry to the owner's queue, which q.Close
		// below has shut, so nothing ever removes it.
		//
		// No AutoAttackStop broadcast here: on the ordinary path
		// stopLiveAutoAttack already sent it and this call reports no
		// change, and on the aborted path the session is going away.
		l.attackStance.Remove(live)
	}
	if l.pvpFlags != nil {
		// reset=false, matching Player.java:6293's deleteMe cleanup: a
		// disconnecting character's flag isn't persisted, so there's
		// nothing to reset it to — just stop tracking it.
		l.pvpFlags.Remove(live.Character, false)
	}
	if l.zones != nil && live.zoneActor != nil {
		position := live.CurrentLocation()
		live.zoneActor.removeFrom(l.zones, position.X, position.Y)
	}
	if l.world != nil {
		// The summon leaves with its owner. Its despawn settles a pet -- items back to
		// live, row and collar saved -- so that must happen before live's
		// own inventory is flushed below, and before live is despawned and
		// the pet inventory's notifier, which holds live, can no longer
		// reach it.
		if obj, ok := l.world.Summon(live.ObjectID()); ok {
			if s, ok := obj.(*summon.Actor); ok {
				if inv := s.PetInventory(); inv != nil {
					owners = append(owners, inv.OwnerID())
				}
				s.LeaveWithOwner()
			}
		}
	}
	if inv := live.Character.Inventory(); inv != nil {
		l.flushItemPersistence(inv)
	}
	if l.world != nil {
		l.world.Despawn(live)
		l.world.RemovePlayer(live.ObjectID())
	}
	// Stop the periodic effect sweep from reaching this character's list:
	// it left world.State above, but a still-held buff/debuff keeps the
	// list registered with task.Effects (see effect.List.Untrack) until
	// something tells it the owner is gone.
	live.Character.EffectList().Untrack()
	// From here on the session no longer delivers this character's
	// session-only events, and a kill reward can no longer apply a herb to it.
	live.Character.DetachSession()
	// Timers still armed on the queue are cancelled, and later posts to it
	// (another actor's command, a straggling tick) are dropped.
	live.Queue().Close()
	return owners
}

// livePlayerPersistWait bounds how long a connection waits for a detached
// player's saves: a detach with an active pet queues three jobs on the
// player's lane, each under livePlayerDetachSaveTimeout, and one item-tick
// chunk under task.ItemInstanceSaveTimeout can be running ahead of them.
//
// The player's lane also carries the single-row writes its handlers queued
// (item rows, shortcut rows, character_skills rows), so a degraded database
// can put more than those four jobs in front of this wait. The wait is a
// bound, not a guarantee: awaitPersistence logs a wait it gave up on and the
// caller continues, exactly as it does when the saves themselves fail.
const livePlayerPersistWait = 3*livePlayerDetachSaveTimeout + task.ItemInstanceSaveTimeout

// awaitPersistence waits until every save already enqueued for owners has
// run, so a read that follows sees the rows those saves write. It reports,
// and logs, a wait that gave up first. It runs on conn's goroutine, never on
// game-state paths.
func (l *GameClientLink) awaitPersistence(conn *Conn, owners ...int32) error {
	if observe := conn.observePersistWait; observe != nil {
		defer observe(owners)()
	}
	ctx, cancel := context.WithTimeout(context.Background(), cmp.Or(l.persistWait, livePlayerPersistWait))
	defer cancel()
	err := l.persist.Flush(ctx, owners...)
	if err != nil {
		l.log.Error().Err(err).Ints32("owner_ids", owners).Msg("wait for queued saves")
	}
	return err
}

// savePet queues actor's pets-row write on its control item's lane. Every
// write of one pets row is keyed by that item, not by the player holding it,
// so writes stay ordered when the collar changes hands.
func (l *GameClientLink) savePet(actor *summon.Actor, ownerInv *itemcontainer.Inventory) {
	itemObjectID, state, write := savePet(l.petStore, actor, ownerInv, l.log)
	if write == nil {
		return
	}
	seq := l.queuedPets.add(itemObjectID, state)
	l.persist.Enqueue(itemObjectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		write(ctx)
		l.queuedPets.written(itemObjectID, seq)
	})
}

// savePet copies actor's pets-row state, syncs the control item's enchant to
// the pet's level, and returns the copied state with the write for that row,
// or a nil write when there is nothing to save. The control item's enchant is
// the pet's displayed level; it is set whether or not the row write later
// succeeds.
func savePet(store petStore, actor *summon.Actor, ownerInv *itemcontainer.Inventory, log zerolog.Logger) (int32, petmodel.State, func(context.Context)) {
	if store == nil {
		return 0, petmodel.State{}, nil
	}
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		return 0, petmodel.State{}, nil
	}
	if ownerInv != nil {
		ownerInv.SetEnchantLevel(ownerInv.ItemByObjectID(itemObjectID), state.Level)
	}
	return itemObjectID, state, func(ctx context.Context) {
		if err := store.Save(ctx, itemObjectID, state); err != nil {
			log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("save pet")
		}
	}
}

// queuedPets remembers, per control item, the newest pets-row state an
// unsummon or logout queued and whose write has not run yet. A summon restores
// from it instead of reading a row that is still behind the owner's lane, so
// it never waits on persistence.
//
// mu stays a lock: add runs on the saving owner's queue, written on the
// control item's persistence lane, and latest on whichever player summons
// with that item next, which need not be the one who saved it.
type queuedPets struct {
	mu      sync.Mutex
	seq     uint64
	pending map[int32]queuedPet
}

type queuedPet struct {
	seq   uint64
	state petmodel.State
}

func (q *queuedPets) add(itemObjectID int32, state petmodel.State) uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending == nil {
		q.pending = make(map[int32]queuedPet)
	}
	q.seq++
	q.pending[itemObjectID] = queuedPet{seq: q.seq, state: state}
	return q.seq
}

// written drops itemObjectID's entry once the write queued as seq has run,
// unless a newer save has replaced it.
func (q *queuedPets) written(itemObjectID int32, seq uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if p, ok := q.pending[itemObjectID]; ok && p.seq == seq {
		delete(q.pending, itemObjectID)
	}
}

func (q *queuedPets) latest(itemObjectID int32) (petmodel.State, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	p, ok := q.pending[itemObjectID]
	return p.state, ok
}

func (l *GameClientLink) transferPetInventory(actor *summon.Actor, owner *itemcontainer.Inventory) {
	if actor == nil || owner == nil {
		return
	}
	petInventory := actor.PetInventory()
	if petInventory == nil {
		return
	}
	for _, inst := range petInventory.Items() {
		state := inst.Snapshot()
		if !owner.ValidateCapacity(1) {
			l.dropPetItem(actor, petInventory, state.ObjectID, state.Count)
			continue
		}
		petInventory.TransferItem(state.ObjectID, state.Count, owner, 0)
	}
}

func (l *GameClientLink) dropPetItem(actor *summon.Actor, inv *itemcontainer.Inventory, objectID int32, count int) {
	if l.inventory == nil || l.groundItems == nil {
		return
	}
	res, ok, err := l.inventory.DropItem(inv, objectID, count)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("allocate pet inventory overflow drop")
		return
	}
	if !ok {
		return
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("build pet inventory overflow drop")
		return
	}
	x, y, z := actor.Position()
	l.groundItems.Drop(ground, task.DropOptions{X: x, Y: y, Z: z, DropperID: actor.ObjectID()})
}

// flushItemPersistence releases inv's persistence dependency, so its items stop
// scheduling with the lazy persistence task, and writes their state on the
// owner's persistence lane, matching the
// reference's ItemContainer.deleteMe: a container that goes away drops out
// of the pending set and is saved at once, rather than leaving rows for a
// tick that will never see the container again. The items are read when the
// job runs, so a change made before it — such as a pet save's control item
// enchant — is included.
//
// The items leave the pending set only once the write has actually
// succeeded. UpdateItems writes the whole container as one atomic flush, so
// a deadline expiring partway through leaves none of it written; keeping
// the container pending hands all of it to the next tick, or to the
// shutdown flush, instead of dropping it on the floor.
//
// So a pending entry here means "this item's write has not landed yet", not
// "this item left its container" — the entry carries the container's final
// state and the item is still the owner's. The login side reads it that way:
// restoreItemRows (character_flow.go) does not drop a row just for being
// pending, it asks the entry's own state whether the item is still owned and
// restores it from that state when it is. The two sites have to keep
// agreeing about what an entry means; changing the rule here without
// changing it there turns a failed logout write into a missing inventory.
func (l *GameClientLink) flushItemPersistence(inv *itemcontainer.Inventory) {
	inv.ReleasePersistence()
	if l.itemInstances == nil {
		return
	}
	itemInstances, log := l.itemInstances, l.log
	l.persist.Enqueue(inv.OwnerID(), func() {
		items := inv.Items()
		if len(items) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := itemInstances.UpdateItems(ctx, items); err != nil {
			log.Error().Err(err).Int32("owner_id", inv.OwnerID()).Msg("save container items")
			return
		}
		itemInstances.RemoveItems(items)
	})
}

func (l *GameClientLink) notifyPlayerLogout(account string) {
	loginLink := l.loginLink()
	if account == "" || loginLink == nil {
		return
	}
	if err := loginLink.SendPlayerLogout(account); err != nil {
		l.log.Debug().Err(err).Str("account", account).Msg("notify player logout")
	}
}
