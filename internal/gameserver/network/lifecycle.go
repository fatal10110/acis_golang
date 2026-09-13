package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

const livePlayerDetachSaveTimeout = 2 * time.Second

// detachLivePlayer takes live out of the world and enqueues its final saves on
// its persistence lane: the character row, position, death penalty and skill
// state, then the offline mark, then the pet and container writes. Values
// are copied before the teardown below changes them. Call
// awaitPersistence afterwards, before anything reads those rows back.
func (l *GameClientLink) detachLivePlayer(live *livePlayer) {
	if live == nil {
		return
	}
	l.abortFusionTargeting(live)
	// Stop any in-flight attack/movement timers before anything below nulls
	// the hooks they call into (SetFrameSender/SetAttackBroadcaster) —
	// otherwise a timer goroutine can still fire after detach and race
	// those writes.
	live.Stop()
	l.cancelActiveTrade(live)
	// Excludes TaskEffects.Save's check-and-enqueue: every autosave job is
	// already on the lane, or will never be, before the jobs below (#1948).
	live.shadowExpiryMu.Lock()
	live.detaching = true
	live.shadowExpiryMu.Unlock()

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
		// A still-active pet's inventory notifier closure holds live too;
		// detach it before live itself is despawned and its client-frame
		// hooks are cleared below, so it can't run against an already
		// detached player.
		if obj, ok := l.world.Summon(live.ObjectID()); ok {
			if pet, ok := obj.(*summon.Actor); ok {
				l.savePet(live.ObjectID(), pet, live.Inventory())
				l.transferPetInventory(pet, live.Inventory())
				if inv := pet.PetInventory(); inv != nil {
					inv.SetUpdateNotifier(nil)
					l.flushItemPersistence(inv)
				}
			}
		}
		l.world.Despawn(live)
		l.world.RemovePlayer(live.ObjectID())
	}
	// Stop the periodic effect sweep from reaching this character's list:
	// it left world.State above, but a still-held buff/debuff keeps the
	// list registered with task.Effects (see effect.List.Untrack) until
	// something tells it the owner is gone.
	live.Character.EffectList().Untrack()
	live.Character.SetFrameSender(nil)
	live.Character.SetBroadcastFrameSender(nil)
	live.Character.SetAttackBroadcaster(nil)
	live.Character.SetBowDrawNotifier(nil)
	live.Character.SetDieBroadcaster(nil)
	// The herb consumer reaches skill reuse and effect application without
	// going through SendFrame, so a kill reward resolving against an already
	// detached character would still mutate it. Unwire it here, and the
	// UserInfo updater with it, so detaching really does unwire every hook.
	live.Character.SetHerbConsumer(nil)
	live.Character.SetRegenMaxSender(nil)
	live.Character.SetLackHPNotifier(nil)
	live.Character.SetLackMPNotifier(nil)
	live.Character.SetRelaxHPFullNotifier(nil)
	live.Character.SetHealRestoredNotifiers(nil, nil)
	live.Character.SetCPRestoredNotifier(nil)
	live.Character.SetEffectExpiryNotifiers(nil, nil, nil)
	live.Character.SetSpoilNotifiers(nil, nil)
	live.Character.SetServitorVanishedNotifier(nil)
	live.Character.SetShieldBlockNotifiers(nil, nil)
	live.Character.SetMagicFailureNotifiers(nil, nil, nil)
	live.Character.SetUserInfoUpdater(nil)
	live.Character.SetPvPFlagHook(nil)
	live.Character.SetRelationBroadcaster(nil)
	live.Character.SetLevelRefresher(nil)
	live.Character.SetWeightPenaltyUpdater(nil)
	if inv := live.Character.Inventory(); inv != nil {
		inv.SetUpdateNotifier(nil)
		inv.SetWeightNotifier(nil)
		l.flushItemPersistence(inv)
	}
}

// livePlayerPersistWait bounds how long a connection waits for a detached
// player's saves: the detach budget plus room for jobs queued ahead on the
// same lanes.
const livePlayerPersistWait = 3 * livePlayerDetachSaveTimeout

// awaitPersistence waits until every save enqueued so far has run, so a
// character list or login that follows reads the rows a detach just wrote.
// It runs on the connection goroutine, never on game-state paths.
func (l *GameClientLink) awaitPersistence() {
	ctx, cancel := context.WithTimeout(context.Background(), livePlayerPersistWait)
	defer cancel()
	if err := l.persist.Flush(ctx); err != nil {
		l.log.Error().Err(err).Msg("wait for detach saves")
	}
}

func (l *GameClientLink) savePet(ownerID int32, actor *summon.Actor, ownerInv *itemcontainer.Inventory) {
	if write := savePet(l.petStore, actor, ownerInv, l.log); write != nil {
		l.persist.Enqueue(ownerID, func() {
			ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
			defer cancel()
			write(ctx)
		})
	}
}

// savePet copies actor's pets-row state, syncs the control item's enchant to
// the pet's level, and returns the write for that row, or nil when there is
// nothing to save. The control item's enchant is the pet's displayed level;
// it is set whether or not the row write later succeeds.
func savePet(store petStore, actor *summon.Actor, ownerInv *itemcontainer.Inventory, log zerolog.Logger) func(context.Context) {
	if store == nil {
		return nil
	}
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		return nil
	}
	if ownerInv != nil {
		ownerInv.SetEnchantLevel(ownerInv.ItemByObjectID(itemObjectID), state.Level)
	}
	return func(ctx context.Context) {
		if err := store.Save(ctx, itemObjectID, state); err != nil {
			log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("save pet")
		}
	}
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

// flushItemPersistence unwires inv's items from the lazy persistence task
// and writes their state on the owner's persistence lane, matching the
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
func (l *GameClientLink) flushItemPersistence(inv *itemcontainer.Inventory) {
	inv.SetItemPersister(nil)
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
