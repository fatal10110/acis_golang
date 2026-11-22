package network

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// clanWarehouseRestoreTimeout bounds the read of a clan warehouse's rows.
const clanWarehouseRestoreTimeout = 5 * time.Second

// clanWarehouseBook holds each clan's warehouse, restored from its rows the
// first time it is used and shared by every member's session from then on.
// mu guards byClan. Each entry's restoring serializes its read, so two
// members opening it at once read it once, and its mu guards the warehouse
// itself: a lookup of a restored warehouse never waits on a read. Item moves
// in and out of a warehouse are serialized by the item operation that names
// the clan id (task.ItemInstances.BeginOperation), not by the queue of any
// one member.
type clanWarehouseBook struct {
	mu     sync.Mutex
	byClan map[int32]*clanWarehouseEntry
}

type clanWarehouseEntry struct {
	restoring sync.Mutex
	mu        sync.Mutex
	wh        *itemcontainer.Container
}

func (e *clanWarehouseEntry) warehouse() *itemcontainer.Container {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.wh
}

func (b *clanWarehouseBook) entry(clanID int32) *clanWarehouseEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byClan == nil {
		b.byClan = map[int32]*clanWarehouseEntry{}
	}
	e := b.byClan[clanID]
	if e == nil {
		e = &clanWarehouseEntry{}
		b.byClan[clanID] = e
	}
	return e
}

// forget drops the warehouse of the clan clanID, dissolved.
func (b *clanWarehouseBook) forget(clanID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.byClan, clanID)
}

// holds reports whether store is still the warehouse of the clan clanID:
// a dissolution forgets it inside the item operation that destroys its
// items.
func (b *clanWarehouseBook) holds(clanID int32, store invops.Store) bool {
	b.mu.Lock()
	e := b.byClan[clanID]
	b.mu.Unlock()
	if e == nil {
		return false
	}
	wh := e.warehouse()
	return wh != nil && invops.Store(wh) == store
}

// restored returns the warehouse of the clan clanID when it is in memory,
// nil when no one has used it yet or its clan was dissolved. It never reads
// the database.
func (b *clanWarehouseBook) restored(clanID int32) *itemcontainer.Container {
	b.mu.Lock()
	e := b.byClan[clanID]
	b.mu.Unlock()
	if e == nil {
		return nil
	}
	return e.warehouse()
}

// storeStillOpen reports whether live's active store may still take items
// in or out; the caller holds the item operation naming the store's owner.
// A clan warehouse its clan's dissolution destroyed while the request
// waited for that operation is closed: the request is dropped silently, as
// the gate drops one from a player who already left the clan, and nothing
// is charged.
func (l *GameClientLink) storeStillOpen(live *livePlayer, active activeStore) bool {
	sim.AssertOwner(live.Queue())
	if active.clanID == 0 || l.clanWarehouses.holds(active.clanID, active.store) {
		return true
	}
	if live.storage.active.store == active.store {
		live.storage.active = activeStore{}
	}
	return false
}

// clanWarehouse returns the warehouse of the clan clanID, restoring it
// from its CLANWH rows on first use, for a server task: the manor's crops,
// a dissolution, a clan hall's fee or refund. A player's request never
// calls it on its queue; it waits for the read off the queue instead
// (clanWarehouseReady).
func (l *GameClientLink) clanWarehouse(clanID int32) (*itemcontainer.Container, error) {
	return l.restoreClanWarehouse(context.Background(), clanID)
}

// restoreClanWarehouse returns the warehouse of the clan clanID, reading
// its CLANWH rows under ctx when no one has used it yet. A failed read is
// not kept: the next use reads again.
func (l *GameClientLink) restoreClanWarehouse(ctx context.Context, clanID int32) (*itemcontainer.Container, error) {
	e := l.clanWarehouses.entry(clanID)
	e.restoring.Lock()
	defer e.restoring.Unlock()
	if wh := e.warehouse(); wh != nil {
		return wh, nil
	}
	wh := itemcontainer.NewContainerWithPersister(clanID, item.LocationClanWarehouse, l.itemTemplates, l.itemPersister(clanID))
	if l.belongings.items != nil {
		ctx, cancel := context.WithTimeout(ctx, clanWarehouseRestoreTimeout)
		defer cancel()
		rows, err := l.belongings.items.ListByOwner(ctx, clanID)
		if err != nil {
			return nil, err
		}
		wh.Restore(l.restoreRows(clanID, rows, func(loc item.Location) bool { return loc == item.LocationClanWarehouse }))
	}
	e.mu.Lock()
	e.wh = wh
	e.mu.Unlock()
	return wh, nil
}

// clanWarehouseReady reports whether the warehouse of the clan clanID is
// in memory, for the request running on live's queue to use it there. When
// no one has used it yet, its rows are read first on the connection
// goroutine, under the connection's context and off the queue
// (deferLookup), and then rerun answers the request on live's queue,
// checking its gates again; a read that fails is logged and the client
// released. Only a request that may defer lookups may call it.
func (l *GameClientLink) clanWarehouseReady(live *livePlayer, clanID int32, rerun func()) bool {
	sim.AssertOwner(live.Queue())
	if l.clanWarehouses.restored(clanID) != nil {
		return true
	}
	live.deferLookup(func(ctx context.Context) {
		_, err := l.restoreClanWarehouse(ctx, clanID)
		onLive(live, func() {
			if err != nil {
				l.log.Error().Err(err).Int32("clan_id", clanID).Msg("warehouse: restore clan warehouse")
				live.SendFrame(serverpackets.FrameActionFailed())
				return
			}
			rerun()
		})
	})
	return false
}

// activateClanWarehouse makes the warehouse of live's clan cl, which
// clanWarehouseReady found in memory, the active store, limited to the
// configured clan warehouse size, or reports false when it is gone: its
// clan was dissolved meanwhile.
func (l *GameClientLink) activateClanWarehouse(live *livePlayer, cl *clan.Clan) (*itemcontainer.Container, bool) {
	sim.AssertOwner(live.Queue())
	wh := l.clanWarehouses.restored(cl.ID())
	if wh == nil {
		return nil, false
	}
	limit := l.playerConfig().StorageSlots.ClanWarehouseSlots()
	live.storage.active = activeStore{store: wh, clanID: cl.ID(), fits: func(slots int) bool {
		return fitsLimit(wh.Size(), slots, limit)
	}}
	return wh, true
}

// clanWarehouseBypass runs a warehouse keeper's clan warehouse command,
// withdrawal when withdraw is set and deposit otherwise. A clan warehouse
// that cannot be read is logged and the client released.
func (l *GameClientLink) clanWarehouseBypass(live *livePlayer, withdraw bool) {
	cl, refusal := l.clanService().OpenWarehouse(live.Character, withdraw)
	switch refusal {
	case clan.WarehouseNoRight:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRightToUseClanWarehouse))
		return
	case clan.WarehouseLevelTooLow:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse))
		return
	}
	if !l.clanWarehouseReady(live, cl.ID(), func() { l.clanWarehouseBypass(live, withdraw) }) {
		return
	}
	wh, ok := l.activateClanWarehouse(live, cl)
	if !ok {
		return
	}
	if !withdraw {
		live.tempInventoryDisable()
		l.sendDepositList(live, serverpackets.WarehouseClan, false)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if wh.Size() == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoItemDepositedInWH))
		return
	}
	l.sendWithdrawList(live, serverpackets.WarehouseClan, wh.Items())
	live.SendFrame(serverpackets.FrameActionFailed())
}

// clanWithdrawGate refuses a withdrawal from a clan warehouse its player
// may not take from: silently when it left the clan or lacks the
// warehouse-search privilege the members' right needs, with a message when
// only the leader may withdraw.
func (l *GameClientLink) clanWithdrawGate(live *livePlayer, active activeStore) bool {
	switch l.clanService().CanWithdraw(live.Character, active.clanID) {
	case clan.WithdrawSilent:
		return false
	case clan.WithdrawLeaderOnly:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderCanRetrieveItemsFromClanWarehouse))
		return false
	}
	return true
}
