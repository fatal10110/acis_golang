package network

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// clanWarehouseRestoreTimeout bounds the read of a clan warehouse's rows.
const clanWarehouseRestoreTimeout = 5 * time.Second

// clanWarehouseBook holds each clan's warehouse, restored from its rows the
// first time a member opens it and shared by every member's session from
// then on. mu guards byClan; each entry's own mu serializes its restore, so
// two members opening it at once restore it once. Item moves in and out of
// a warehouse are serialized by the item operation that names the clan id
// (task.ItemInstances.BeginOperation), not by the queue of any one member.
type clanWarehouseBook struct {
	mu     sync.Mutex
	byClan map[int32]*clanWarehouseEntry
}

type clanWarehouseEntry struct {
	mu sync.Mutex
	wh *itemcontainer.Container
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

// clanWarehouse returns the warehouse of the clan clanID, restoring it
// from its CLANWH rows on first use. A failed read is not kept: the next
// opening reads again.
func (l *GameClientLink) clanWarehouse(clanID int32) (*itemcontainer.Container, error) {
	e := l.clanWarehouses.entry(clanID)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wh != nil {
		return e.wh, nil
	}
	wh := itemcontainer.NewContainerWithPersister(clanID, item.LocationClanWarehouse, l.itemTemplates, l.itemPersister(clanID))
	if l.items != nil {
		ctx, cancel := context.WithTimeout(context.Background(), clanWarehouseRestoreTimeout)
		defer cancel()
		rows, err := l.items.ListByOwner(ctx, clanID)
		if err != nil {
			return nil, err
		}
		wh.Restore(l.restoreRows(clanID, rows, func(loc item.Location) bool { return loc == item.LocationClanWarehouse }))
	}
	e.wh = wh
	return wh, nil
}

// activateClanWarehouse makes the warehouse of live's clan cl the active
// store, limited to the configured clan warehouse size, or reports false
// when it cannot be read.
func (l *GameClientLink) activateClanWarehouse(live *livePlayer, cl *clan.Clan) (*itemcontainer.Container, bool) {
	wh, err := l.clanWarehouse(cl.ID())
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", cl.ID()).Msg("warehouse: restore clan warehouse")
		return nil, false
	}
	limit := l.playerConfig.StorageSlots.ClanWarehouseSlots()
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
