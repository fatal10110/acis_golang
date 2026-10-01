package network

import (
	"cmp"
	"context"
	"slices"
	"strconv"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// depositFeePerItem is the adena a warehouse deposit costs per row.
const depositFeePerItem = 30

// FreightConfig is the server's freight service settings.
type FreightConfig struct {
	// Allow turns the freight service on.
	Allow bool
	// RegionBased ties each freight item to the region of the warehouse
	// keeper it was opened at: it is listed only at keepers of that region.
	RegionBased bool
	// Price is the adena a package costs per row.
	Price int
}

// DefaultFreightConfig is the shipped freight settings.
func DefaultFreightConfig() FreightConfig {
	return FreightConfig{Allow: true, RegionBased: true, Price: 1000}
}

func (c PlayerConfig) freight() FreightConfig {
	if c.Freight == nil {
		return DefaultFreightConfig()
	}
	return *c.Freight
}

// playerStorage is a player's warehouse and freight state, restored at
// login. Every field is owned by the player's queue.
type playerStorage struct {
	// warehouse is the player's private warehouse and freight its own
	// freight.
	warehouse *itemcontainer.Container
	freight   *itemcontainer.Freight
	// deposits holds the freight of each other character of the account
	// that could be read at login, by character id: what a package is sent
	// into. Those characters are offline while this one plays, so nothing
	// else changes their freight meanwhile.
	deposits map[int32]*itemcontainer.Freight
	// accountChars are the account's other characters, in the order the
	// recipient list shows them.
	accountChars []serverpackets.PackageRecipient
	// active is the warehouse the player last opened.
	active activeStore
}

// activeStore is the warehouse or freight a deposit or withdrawal moves
// items into or out of.
type activeStore struct {
	store invops.Store
	// private marks the player's private warehouse; every other store is
	// a public one, which only tradable items enter.
	private bool
	// fits reports whether slots more stacks fit in store.
	fits func(slots int) bool
}

// fitsLimit reports whether slots more stacks fit beside size held ones
// under limit; adding nothing always fits.
func fitsLimit(size, slots, limit int) bool {
	return slots == 0 || size+slots <= limit
}

// activatePrivateWarehouse makes live's private warehouse the active
// store, limited to live's warehouse size.
func activatePrivateWarehouse(live *livePlayer) *itemcontainer.Container {
	wh := live.storage.warehouse
	live.storage.active = activeStore{store: wh, private: true, fits: func(slots int) bool {
		return fitsLimit(wh.Size(), slots, live.Character.WarehouseLimit())
	}}
	return wh
}

// activateOwnFreight makes live's own freight the active store, limited to
// live's freight size.
func activateOwnFreight(live *livePlayer) *itemcontainer.Freight {
	f := live.storage.freight
	live.storage.active = activeStore{store: f, fits: func(slots int) bool {
		return fitsLimit(f.VisibleSize(), slots, live.Character.FreightLimit())
	}}
	return f
}

// activateDeposit makes the freight of the account's character id the
// active store, or reports false when it is not loaded. Another
// character's freight holds the configured base size.
func (l *GameClientLink) activateDeposit(live *livePlayer, id int32) (*itemcontainer.Freight, bool) {
	f := live.storage.deposits[id]
	if f == nil {
		return nil, false
	}
	limit := l.playerConfig.StorageSlots.BaseFreight()
	live.storage.active = activeStore{store: f, fits: func(slots int) bool {
		return fitsLimit(f.VisibleSize(), slots, limit)
	}}
	return f, true
}

// itemHolder is what live is using its items for right now.
func (l *GameClientLink) itemHolder(live *livePlayer) invops.Holder {
	mount := live.MountObjectID()
	h := invops.Holder{
		PetCollar: func(objectID int32) bool {
			return objectID != mount && live.ControlItemInUse(objectID)
		},
		MountCollar:   mount,
		EnchantScroll: l.enchantStateStore().Active(live.ObjectID()),
		Casting:       live.Character.CastingNow(),
	}
	if live.cast != nil {
		if def, ok := live.cast.CurrentSkill(); ok {
			h.CastConsumeID = int32(def.ItemConsumeID)
		}
	}
	return h
}

// warehouseBypass runs a warehouse keeper's storage command for live at f.
// It reports false when the command aborted, so that nothing more is
// sent.
//
// The clan commands answer a clanless player's refusal.
// ponytail: a clan member's clan warehouse command needs the clan system
// (#3016); it is logged and the client released until that lands.
func (l *GameClientLink) warehouseBypass(live *livePlayer, f *npc.Folk, reply npc.BypassReply) bool {
	freight := l.playerConfig.freight()
	switch reply.Warehouse {
	case npc.WithdrawPrivate:
		wh := activatePrivateWarehouse(live)
		if wh.Size() == 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoItemDepositedInWH))
			return true
		}
		l.sendWithdrawList(live, serverpackets.WarehousePrivate, wh.Items())
		live.SendFrame(serverpackets.FrameActionFailed())
	case npc.DepositPrivate:
		live.SendFrame(serverpackets.FrameActionFailed())
		activatePrivateWarehouse(live)
		live.tempInventoryDisable()
		l.sendDepositList(live, serverpackets.WarehousePrivate, true)
	case npc.WithdrawClan:
		if live.Character.ClanID == 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRightToUseClanWarehouse))
			return true
		}
		l.log.Debug().Int32("object_id", live.ObjectID()).Msg("warehouse: clan warehouse not modeled (#3016)")
	case npc.DepositClan:
		if live.Character.ClanID == 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse))
			return true
		}
		l.log.Debug().Int32("object_id", live.ObjectID()).Msg("warehouse: clan warehouse not modeled (#3016)")
	case npc.WithdrawFreight:
		if !freight.Allow {
			return true
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		// The size is read with the town the freight was last opened at,
		// before this keeper's town is selected.
		own := live.storage.freight
		if own.VisibleSize() <= 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoItemDepositedInWH))
			return true
		}
		own.ActiveLocation = freightLocation(f, freight)
		activateOwnFreight(live)
		l.sendWithdrawList(live, serverpackets.WarehouseFreight, own.VisibleItems())
	case npc.DepositFreight:
		if !freight.Allow {
			return true
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		if len(live.storage.accountChars) == 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCharacterDoesNotExist))
			return true
		}
		live.SendFrame(serverpackets.FramePackageToList(live.storage.accountChars))
	case npc.FreightCharacter:
		if !freight.Allow {
			return true
		}
		id, err := strconv.ParseInt(reply.FreightTarget, 10, 32)
		if err != nil {
			return false
		}
		// Only the account's own characters have a freight to open here;
		// one naming any other character opens nothing.
		target, ok := l.activateDeposit(live, int32(id))
		if !ok {
			return true
		}
		target.ActiveLocation = freightLocation(f, freight)
		live.SendFrame(serverpackets.FrameActionFailed())
		live.tempInventoryDisable()
		l.sendDepositList(live, serverpackets.WarehouseFreight, false)
	}
	return true
}

// freightLocation is the town tag freight opened at f carries: f's region
// when freight is region based, otherwise none.
func freightLocation(f *npc.Folk, cfg FreightConfig) int {
	if !cfg.RegionBased {
		return 0
	}
	x, y, _ := f.Position()
	return world.RegionKey(x, y)
}

// sendDepositList opens the deposit window of a warehouse of whType on
// live's depositable items.
func (l *GameClientLink) sendDepositList(live *livePlayer, whType serverpackets.WarehouseType, private bool) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	items := l.inventory.DepositableItems(inv, l.itemHolder(live), private)
	frame, err := serverpackets.FrameWarehouseDepositList(whType, int32(inv.Adena()), items, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Msg("build WarehouseDepositList")
		return
	}
	live.SendFrame(frame)
}

// sendWithdrawList opens the withdrawal window of a warehouse of whType
// holding items.
func (l *GameClientLink) sendWithdrawList(live *livePlayer, whType serverpackets.WarehouseType, items []*item.Instance) {
	adena := 0
	if inv := live.Inventory(); inv != nil {
		adena = inv.Adena()
	}
	frame, err := serverpackets.FrameWarehouseWithdrawList(whType, int32(adena), items, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Msg("build WarehouseWithdrawList")
		return
	}
	live.SendFrame(frame)
}

// sendPackageSendableItemList lists the items live can send as a package
// to objectID.
func (l *GameClientLink) sendPackageSendableItemList(live *livePlayer, objectID int32) {
	if live == nil {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	items := l.inventory.SendableItems(inv, l.itemHolder(live))
	frame, err := serverpackets.FramePackageSendableList(objectID, int32(inv.Adena()), items, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Msg("build PackageSendableList")
		return
	}
	live.SendFrame(frame)
}

// warehouseRequestGate runs the gates a deposit and a withdrawal share and
// returns the active store, or reports false when the request stops. A
// player tied up in a trade is told so, and a public warehouse refused by
// the access level says so; every other refusal is silent. The client
// closes its window when it sends the request, so no click is left
// pending.
func (l *GameClientLink) warehouseRequestGate(live *livePlayer) (activeStore, bool) {
	if l.trades != nil && l.trades.ProcessingTransaction(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadyTrading))
		return activeStore{}, false
	}
	l.cancelActiveEnchant(live)
	active := live.storage.active
	if active.store == nil {
		return activeStore{}, false
	}
	f := live.currentFolk.Load()
	if f == nil || !f.Warehouse() || !l.playerCanDoInteract(live, f) {
		return activeStore{}, false
	}
	if !active.private && !live.access.AllowTransaction {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return activeStore{}, false
	}
	if !l.playerConfig.KarmaPlayerCanUseWareHouse && live.Karma() > 0 {
		return activeStore{}, false
	}
	return active, true
}

// requestWarehouseDeposit moves the requested inventory rows into the
// active warehouse, for 30 adena a row.
func (l *GameClientLink) requestWarehouseDeposit(live *livePlayer, req clientpackets.SendWarehouseDepositList) {
	if live == nil {
		return
	}
	active, ok := l.warehouseRequestGate(live)
	if !ok {
		return
	}
	// A player with a trade open was refused above, so no open trade can
	// stop the deposit between the fee and the moves.
	end := l.itemInstances.BeginOperation()
	defer end()
	res, outcome, err := l.inventory.Deposit(live.Inventory(), active.store, storeMoves(req.Items), l.itemHolder(live),
		active.private, len(req.Items)*depositFeePerItem, active.fits)
	if err != nil {
		l.log.Error().Err(err).Msg("warehouse deposit")
	}
	l.applyPersistActions(res.Persist)
	end()
	sendStoreOutcome(live, outcome)
}

// requestWarehouseWithdraw moves the requested active-warehouse rows into
// live's inventory.
func (l *GameClientLink) requestWarehouseWithdraw(live *livePlayer, req clientpackets.SendWarehouseWithdrawList) {
	if live == nil {
		return
	}
	active, ok := l.warehouseRequestGate(live)
	if !ok {
		return
	}
	end := l.itemInstances.BeginOperation()
	defer end()
	res, outcome, err := l.inventory.Withdraw(active.store, live.Inventory(), storeMoves(req.Items))
	if err != nil {
		l.log.Error().Err(err).Msg("warehouse withdraw")
	}
	l.applyPersistActions(res.Persist)
	end()
	sendStoreOutcome(live, outcome)
}

// requestPackageSend sends the requested inventory rows to the freight of
// another character of live's account, for the configured price a row.
// It needs a civilian NPC selected within interaction reach, which a GM
// does not. Every refusal but the access level, the freight's capacity and
// the fee is silent; the client closes its window when it sends the
// request.
func (l *GameClientLink) requestPackageSend(live *livePlayer, req clientpackets.RequestPackageSend) {
	freight := l.playerConfig.freight()
	if live == nil || len(req.Items) == 0 || !freight.Allow {
		return
	}
	if _, ok := l.activateDeposit(live, req.ObjectID); !ok {
		return
	}
	active := live.storage.active
	f := live.currentFolk.Load()
	if (f == nil || !interactInRange(live, f, interactionDistance)) && !live.access.IsGM {
		return
	}
	if !live.access.AllowTransaction {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	if !l.playerConfig.KarmaPlayerCanUseWareHouse && live.Karma() > 0 {
		return
	}
	inv := live.Inventory()
	end := l.itemInstances.BeginOperation()
	defer end()
	res, outcome, err := l.inventory.SendPackage(inv, active.store, storeMoves(req.Items), l.itemHolder(live),
		len(req.Items)*freight.Price, active.fits)
	if err != nil {
		l.log.Error().Err(err).Msg("package send")
	}
	l.applyPersistActions(res.Persist)
	end()
	if len(res.Changed) > 0 {
		l.applyEquipStatChanges(live, inv, res)
	}
	sendStoreOutcome(live, outcome)
}

// sendStoreOutcome answers a refused deposit, withdrawal or package with
// its system message. The rest answer nothing of their own: the moved
// items' InventoryUpdate is the answer.
func sendStoreOutcome(live *livePlayer, outcome invops.StoreOutcome) {
	var msg int
	switch outcome {
	case invops.StoreOverCapacity:
		msg = serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted
	case invops.StoreNotEnoughAdena:
		msg = serverpackets.SystemMessageYouNotEnoughAdena
	case invops.StoreSlotsFull:
		msg = serverpackets.SystemMessageSlotsFull
	case invops.StoreWeightExceeded:
		msg = serverpackets.SystemMessageWeightLimitExceeded
	default:
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

func storeMoves(rows []clientpackets.ItemRequest) []invops.Move {
	moves := make([]invops.Move, len(rows))
	for i, row := range rows {
		moves[i] = invops.Move{ObjectID: row.ObjectID, Count: int(row.Count)}
	}
	return moves
}

// restoreStorage builds c's warehouse and freight from rows, its restored
// item rows, and reads the account's other characters with the freight of
// each. A character whose saves cannot be waited for, or whose rows cannot
// be read, gets no freight: a package to it is refused this session.
func (l *GameClientLink) restoreStorage(ctx context.Context, account string, c *player.Character, rows []*item.Instance) playerStorage {
	st := playerStorage{
		warehouse: itemcontainer.NewContainerWithPersister(c.ID, item.LocationWarehouse, l.itemTemplates, l.itemPersister(c.ID)),
		freight:   itemcontainer.NewFreightWithPersister(c.ID, l.itemTemplates, l.itemPersister(c.ID)),
		deposits:  map[int32]*itemcontainer.Freight{},
	}
	st.warehouse.Restore(rows)
	st.freight.Restore(rows)
	if l.roster == nil || account == "" {
		return st
	}
	chars, err := l.roster.List(ctx, account)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: list account characters")
		return st
	}
	for _, other := range chars {
		if other.ID == c.ID {
			continue
		}
		st.accountChars = append(st.accountChars, serverpackets.PackageRecipient{ObjectID: other.ID, Name: other.Name})
		if f := l.restoreDepositedFreight(ctx, other.ID); f != nil {
			st.deposits[other.ID] = f
		}
	}
	sortRecipients(st.accountChars)
	return st
}

// accountPlayerInWorld returns a character of chars in the world, if any.
func (l *GameClientLink) accountPlayerInWorld(chars []*player.Character) (*livePlayer, bool) {
	for _, ch := range chars {
		if obj, ok := l.world.Player(ch.ObjectID()); ok {
			if live, ok := obj.(*livePlayer); ok {
				return live, true
			}
		}
	}
	return nil, false
}

// restoreDepositedFreight reads the freight of the offline character id,
// once its last session's saves have landed, or returns nil.
func (l *GameClientLink) restoreDepositedFreight(ctx context.Context, id int32) *itemcontainer.Freight {
	if l.items == nil {
		return nil
	}
	// A character still in the world writes its own freight.
	if l.world != nil {
		if _, ok := l.world.Player(id); ok {
			return nil
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, cmp.Or(l.persistWait, LivePlayerPersistWait))
	defer cancel()
	if err := l.persist.Flush(waitCtx, id); err != nil {
		l.log.Error().Err(err).Int32("object_id", id).Msg("enter world: wait for account character saves")
		return nil
	}
	rows, err := l.items.ListByOwner(ctx, id)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", id).Msg("enter world: read account character freight")
		return nil
	}
	f := itemcontainer.NewFreightWithPersister(id, l.itemTemplates, l.itemPersister(id))
	f.Restore(l.restoreRows(id, rows, func(loc item.Location) bool { return loc == item.LocationFreight }))
	return f
}

// sortRecipients puts the recipient list in the order the client is sent
// it: by bucket of a 16-bucket hash table keyed by character id, then by
// id within a bucket.
func sortRecipients(r []serverpackets.PackageRecipient) {
	bucket := func(id int32) uint32 {
		h := uint32(id)
		return (h ^ h>>16) & 15
	}
	slices.SortFunc(r, func(a, b serverpackets.PackageRecipient) int {
		return cmp.Or(cmp.Compare(bucket(a.ObjectID), bucket(b.ObjectID)), cmp.Compare(a.ObjectID, b.ObjectID))
	})
}

// releaseStorage queues the final write of live's warehouse and freight
// items and returns the other characters whose freight it holds, whose
// lanes the logout also waits on.
func (l *GameClientLink) releaseStorage(live *livePlayer) []int32 {
	st := &live.storage
	st.active = activeStore{}
	if st.warehouse != nil {
		l.flushItemPersistence(st.warehouse)
	}
	if st.freight != nil {
		l.flushItemPersistence(st.freight)
	}
	owners := make([]int32, 0, len(st.deposits))
	for id, f := range st.deposits {
		l.flushItemPersistence(f)
		owners = append(owners, id)
	}
	return owners
}
