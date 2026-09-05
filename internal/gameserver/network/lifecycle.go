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

func (l *GameClientLink) detachLivePlayer(ctx context.Context, live *livePlayer) {
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
	live.shadowExpiryMu.Lock()
	live.detaching = true
	live.shadowExpiryMu.Unlock()

	// Serializes against TaskEffects.Save (the autosave tick), which takes
	// the same mutex before writing the online column: whichever of the two
	// gets here first runs its whole save sequence to completion before the
	// other's write can start, so an autosave write already in flight can
	// never land after SaveOfflineRecency below (#1948).
	live.saveMu.Lock()
	defer live.saveMu.Unlock()

	// One budget for the whole detach, not one per store: a logout with an
	// active pet writes the character row, the skill state, the player
	// inventory and the pet inventory, and each of those taking its own
	// timeout makes the worst case grow with however many things detach
	// happens to save. WithoutCancel because a client that already
	// disconnected cancels ctx, and these writes must still land.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), livePlayerDetachSaveTimeout)
	defer cancelSave()

	if l.roster != nil || l.skills != nil {
		if l.roster != nil {
			if err := l.roster.Save(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player full stats")
			}
			if err := l.roster.SavePosition(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player position")
			}
			if err := l.roster.SaveDeathPenaltyLevel(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player death penalty level")
			}
			if err := l.roster.SaveOfflineRecency(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player offline recency")
			}
		}
		if l.skills != nil {
			if err := l.skills.Save(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player skill state")
			}
		}
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
				l.savePet(saveCtx, pet, live.Inventory())
				l.transferPetInventory(pet, live.Inventory())
				if inv := pet.PetInventory(); inv != nil {
					inv.SetUpdateNotifier(nil)
					l.flushItemPersistence(saveCtx, inv)
				}
			}
		}
		l.world.Despawn(live)
		l.world.RemovePlayer(live.ObjectID())
	}
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
		l.flushItemPersistence(saveCtx, inv)
	}
}

func (l *GameClientLink) savePet(ctx context.Context, actor *summon.Actor, ownerInv *itemcontainer.Inventory) {
	savePet(ctx, l.petStore, actor, ownerInv, l.log)
}

func savePet(ctx context.Context, store petStore, actor *summon.Actor, ownerInv *itemcontainer.Inventory, log zerolog.Logger) {
	if store == nil {
		return
	}
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		return
	}
	if err := store.Save(ctx, itemObjectID, state); err != nil {
		log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("save pet")
		// A failed pets-row write is not a restore source of truth. Skip
		// the live control-item lift and its inventory update until a
		// later save actually lands.
		return
	}
	// The control item's enchant is the pet's displayed level. Lift it on
	// the same save that writes the pets row so inventory, persistence, and
	// a later restore all see the saved level.
	if ownerInv == nil {
		return
	}
	ownerInv.SetEnchantLevel(ownerInv.ItemByObjectID(itemObjectID), state.Level)
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
// and writes their current state straight to the database, matching the
// reference's ItemContainer.deleteMe: a container that goes away drops out
// of the pending set and is saved immediately, rather than leaving rows for
// a tick that will never see the container again.
//
// The items leave the pending set only once the write has actually
// succeeded. UpdateItems writes the whole container as one atomic flush, so
// a deadline expiring partway through leaves none of it written; keeping
// the container pending hands all of it to the next tick, or to the
// shutdown flush, instead of dropping it on the floor.
func (l *GameClientLink) flushItemPersistence(ctx context.Context, inv *itemcontainer.Inventory) {
	inv.SetItemPersister(nil)
	if l.itemInstances == nil {
		return
	}
	items := inv.Items()
	if len(items) == 0 {
		return
	}

	if err := l.itemInstances.UpdateItems(ctx, items); err != nil {
		l.log.Error().Err(err).Int32("owner_id", inv.OwnerID()).Msg("save container items")
		return
	}
	l.itemInstances.RemoveItems(items)
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
