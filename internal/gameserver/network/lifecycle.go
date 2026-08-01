package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const livePlayerDetachSaveTimeout = 2 * time.Second

func (l *GameClientLink) detachLivePlayer(ctx context.Context, live *livePlayer) {
	if live == nil {
		return
	}
	// Stop any in-flight attack/movement timers before anything below nulls
	// the hooks they call into (SetFrameSender/SetAttackBroadcaster) —
	// otherwise a timer goroutine can still fire after detach and race
	// those writes.
	live.Stop()
	l.cancelActiveTrade(live)

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
			if err := l.roster.SavePosition(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player position")
			}
			if err := l.roster.SaveDeathPenaltyLevel(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player death penalty level")
			}
		}
		if l.skills != nil {
			if err := l.skills.Save(saveCtx, live.Character, true); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player skill state")
			}
		}
	}
	if l.playerClock != nil {
		l.playerClock.Remove(live.ObjectID())
	}
	if l.water != nil {
		l.water.Remove(live)
	}
	if l.shadowItems != nil {
		l.shadowItems.Remove(live.ObjectID())
	}
	if l.zones != nil && live.zoneActor != nil {
		position := live.CurrentLocation()
		l.zones.RemoveFrom(live.zoneActor, position.X, position.Y)
	}
	if l.world != nil {
		// A still-active pet's inventory notifier closure holds live too;
		// detach it before live itself is despawned and its client-frame
		// hooks are cleared below, so it can't run against an already
		// detached player.
		if obj, ok := l.world.Summon(live.ObjectID()); ok {
			if pet, ok := obj.(*summon.Actor); ok {
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
	live.Character.SetAttackBroadcaster(nil)
	live.Character.SetDieBroadcaster(nil)
	// The herb consumer reaches skill reuse and effect application without
	// going through SendFrame, so a kill reward resolving against an already
	// detached character would still mutate it. Unwire it here, and the
	// UserInfo updater with it, so detaching really does unwire every hook.
	live.Character.SetHerbConsumer(nil)
	live.Character.SetUserInfoUpdater(nil)
	live.Character.SetLevelRefresher(nil)
	live.Character.SetWeightPenaltyUpdater(nil)
	if inv := live.Character.Inventory(); inv != nil {
		inv.SetUpdateNotifier(nil)
		inv.SetWeightNotifier(nil)
		l.flushItemPersistence(saveCtx, inv)
	}
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
