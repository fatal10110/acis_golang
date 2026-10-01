package network

import (
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// Action-bar commands that set up a store or workshop.
const (
	actionPrivateStoreSell   = 10
	actionPrivateStoreBuy    = 28
	actionDwarvenManufacture = 37
	actionCommonManufacture  = 51
	actionPackageSell        = 61
)

// storeActionUse runs an action-bar store command and reports whether
// actionID was one. A command refused before it reaches the store, or by a
// store check that has no message of its own, is released with
// ActionFailed: the action bar waits on an answer.
func (l *GameClientLink) storeActionUse(live *livePlayer, actionID int32) bool {
	var open func() bool
	switch actionID {
	case actionPrivateStoreSell:
		open = func() bool { return l.tryOpenSellStore(live, false) }
	case actionPackageSell:
		open = func() bool { return l.tryOpenSellStore(live, true) }
	case actionPrivateStoreBuy:
		open = func() bool { return l.tryOpenBuyStore(live) }
	case actionDwarvenManufacture:
		open = func() bool { return l.tryOpenWorkshop(live, true) }
	case actionCommonManufacture:
		open = func() bool { return l.tryOpenWorkshop(live, false) }
	default:
		return false
	}
	if actionUseRefused(live) {
		return true
	}
	if !open() {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	return true
}

// canOpenPrivateStore runs the store set-up checks for live, cancelling its
// open trade first when cancelTrade is set. A refusal other than a dead
// owner's closes the store, silently, and sends the refusal's message. It
// reports whether the store may be set up and, when refused, whether a
// message answered.
func (l *GameClientLink) canOpenPrivateStore(live *livePlayer, cancelTrade bool) (ok, answered bool) {
	if !live.InStoreMode() && cancelTrade {
		l.cancelActiveTrade(live)
	}
	refusal := privatestore.CanOpen(l.storeOpenState(live))
	if refusal == privatestore.OpenAllowed {
		return true, false
	}
	if refusal == privatestore.OpenRefusedDead {
		return false, false
	}
	live.SetOperateType(privatestore.OperateNone)
	msg := 0
	switch refusal {
	case privatestore.OpenRefusedFighting:
		msg = serverpackets.SystemMessageCantOperateStoreDuringCombat
	case privatestore.OpenRefusedCasting:
		msg = serverpackets.SystemMessagePrivateStoreNotWhileCasting
	case privatestore.OpenRefusedZone:
		msg = serverpackets.SystemMessageNoPrivateStoreHere
	}
	if msg == 0 {
		return false, false
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
	return false, true
}

// storeOpenState reads what the store set-up checks need from live. No
// duel (#215) and no Olympiad (#216) are modeled yet, so neither ever
// refuses.
func (l *GameClientLink) storeOpenState(live *livePlayer) privatestore.OpenState {
	attacking := live.attack != nil && live.attack.AttackingNow()
	requesting := l.trades != nil && l.trades.ProcessingRequest(live.ObjectID())
	noStore := live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagNoStore)
	return privatestore.OpenState{
		Operate:     live.OperateType(),
		AlikeDead:   live.AlikeDead(),
		Fighting:    live.InCombat() || live.PvPFlagState() != task.PvPFlagNone || attacking,
		Casting:     live.CastingNow(),
		NoStoreZone: noStore,
		Busy:        live.Mounted() || requesting || liveOutOfControl(live),
		Seated:      live.Seated(),
		StandingNow: live.StandingNow(),
	}
}

// tryOpenSellStore opens live's sell store manage window, standing up from
// an open sell store first. It reports false for a refusal no message
// answered.
func (l *GameClientLink) tryOpenSellStore(live *livePlayer, packaged bool) bool {
	return l.tryOpenStore(live, privatestore.OperateSellManage, func() {
		l.sendSellManageList(live, packaged)
	}, privatestore.OperateSell, privatestore.OperatePackageSell)
}

// tryOpenBuyStore opens live's buy store manage window, standing up from an
// open buy store first.
func (l *GameClientLink) tryOpenBuyStore(live *livePlayer) bool {
	return l.tryOpenStore(live, privatestore.OperateBuyManage, func() {
		l.sendBuyManageList(live)
	}, privatestore.OperateBuy)
}

// tryOpenWorkshop opens live's workshop manage window on the dwarven or
// common page, standing up from an open workshop first.
func (l *GameClientLink) tryOpenWorkshop(live *livePlayer, dwarven bool) bool {
	return l.tryOpenStore(live, privatestore.OperateManufactureManage, func() {
		l.sendRecipeShopManageList(live, dwarven)
	}, privatestore.OperateManufacture)
}

// tryOpenStore is the shared set-up path: a player with no store, or with
// one of the open kinds, is put in manage, standing up from an open store,
// and shown its manage window. Anything else, a sit-down under way
// included, changes nothing.
func (l *GameClientLink) tryOpenStore(live *livePlayer, manage privatestore.OperateType, show func(), open ...privatestore.OperateType) bool {
	if ok, answered := l.canOpenPrivateStore(live, true); !ok {
		return answered
	}
	current := live.OperateType()
	isOpen := false
	for _, t := range open {
		isOpen = isOpen || current == t
	}
	if current != privatestore.OperateNone && !isOpen {
		return false
	}
	if live.SittingNow() {
		return false
	}
	if isOpen {
		live.Character.StandUp()
	}
	live.SetOperateType(manage)
	show()
	return true
}

// sendSellManageList sends live its sell store manage window, first dropping
// listed rows whose items are gone or worn.
func (l *GameClientLink) sendSellManageList(live *livePlayer, packaged bool) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	listed, wasPackaged := live.PrivateStore().RefreshSell(inv)
	candidates := privatestore.ItemsToSell(inv, listed, l.storeHidden(live))
	frame, err := serverpackets.FramePrivateStoreManageListSell(live.ObjectID(), wasPackaged || packaged, inv.Adena(), candidates, listed, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build private store manage sell list")
		return
	}
	live.SendFrame(frame)
}

// sendBuyManageList sends live its buy store manage window, first dropping
// wanted rows for items it no longer holds.
func (l *GameClientLink) sendBuyManageList(live *livePlayer) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	listed := live.PrivateStore().RefreshBuy(inv)
	candidates := privatestore.ItemsToBuy(inv, l.storeHidden(live))
	frame, err := serverpackets.FramePrivateStoreManageListBuy(live.ObjectID(), inv.Adena(), candidates, listed, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build private store manage buy list")
		return
	}
	live.SendFrame(frame)
}

// sendRecipeShopManageList sends live its workshop manage window for the
// dwarven or common workshop. Without the dwarven craft the window shows
// the common book.
func (l *GameClientLink) sendRecipeShopManageList(live *livePlayer, dwarven bool) {
	book := live.RecipeBook()
	recipes := book.Recipes(dwarven && live.HasDwarvenCraft())
	offered := live.PrivateStore().ShowManufacture(dwarven, book.Has)
	adena := 0
	if inv := live.Inventory(); inv != nil {
		adena = inv.Adena()
	}
	live.SendFrame(serverpackets.FrameRecipeShopManageList(live.ObjectID(), adena, dwarven, recipes, offered))
}

// storeHidden reports the items a store manage window leaves out: a
// summoned pet's collar (a ridden mount's is shown) and the selected
// enchant scroll. The item a cast in flight consumes would be left out too,
// but a manage window only opens after the set-up checks refused a casting
// player, so no cast is ever in flight here.
func (l *GameClientLink) storeHidden(live *livePlayer) func(objectID int32) bool {
	scroll := l.enchantStateStore().Active(live.ObjectID())
	return func(objectID int32) bool {
		return objectID == scroll || (objectID != live.MountObjectID() && live.ControlItemInUse(objectID))
	}
}

// storeTrader is live as one side of a store deal.
func (l *GameClientLink) storeTrader(live *livePlayer) privatestore.Trader {
	scroll := l.enchantStateStore().Active(live.ObjectID())
	return privatestore.Trader{
		Inv: live.Inventory(),
		Bound: func(objectID int32) bool {
			return objectID == scroll || live.ControlItemInUse(objectID)
		},
		Casting: live.CastingNow(),
	}
}

// quitPrivateStore answers RequestPrivateStoreQuitSell,
// RequestPrivateStoreQuitBuy and RequestRecipeShopManageQuit: the store
// closes and everyone around sees it. Its owner stays seated.
func (l *GameClientLink) quitPrivateStore(live *livePlayer) {
	live.SetOperateType(privatestore.OperateNone)
	l.broadcastCharacterInfo(live)
}

// storeMessageFits reports whether text is short enough for a store title
// or workshop name.
func storeMessageFits(text string) bool {
	return len(utf16.Encode([]rune(text))) <= privatestore.MaxMessageLength
}

// setSellStoreTitle answers SetPrivateStoreMsgSell: the title is kept and
// shown back to live. A title too long is dropped without a word, as
// specified; the client waits on nothing.
func (l *GameClientLink) setSellStoreTitle(live *livePlayer, req clientpackets.StoreMessage) {
	if !storeMessageFits(req.Text) {
		return
	}
	store := live.PrivateStore()
	store.SetSellTitle(req.Text)
	live.SendFrame(serverpackets.FramePrivateStoreMsgSell(live.ObjectID(), store.SellTitle()))
}

// setBuyStoreTitle answers SetPrivateStoreMsgBuy the way setSellStoreTitle
// answers the sell title.
func (l *GameClientLink) setBuyStoreTitle(live *livePlayer, req clientpackets.StoreMessage) {
	if !storeMessageFits(req.Text) {
		return
	}
	store := live.PrivateStore()
	store.SetBuyTitle(req.Text)
	live.SendFrame(serverpackets.FramePrivateStoreMsgBuy(live.ObjectID(), store.BuyTitle()))
}

// setWorkshopName answers RequestRecipeShopMessageSet: the name is kept
// while live runs or sets up a workshop. Nothing is sent back; a name too
// long, or set with no workshop, is dropped without a word.
func (l *GameClientLink) setWorkshopName(live *livePlayer, req clientpackets.StoreMessage) {
	if !storeMessageFits(req.Text) {
		return
	}
	switch live.OperateType() {
	case privatestore.OperateManufacture, privatestore.OperateManufactureManage:
		live.PrivateStore().SetShopName(req.Text)
	}
}

// setSellStoreList answers SetPrivateStoreListSell. The sell list is
// emptied first, whatever follows. Rows that fail the packet's count, the
// owner's holdings, its access level, the set-up checks or its store limit
// close the store; a row that cannot be listed, or a list past the largest
// int32 in total, leaves the manage window open again on what did get
// listed. Otherwise live sits down and the store opens.
//
// A refusal closing the store without a message stays silent, as
// specified: the manage window already closed when the client sent
// the list, so nothing waits on an answer.
func (l *GameClientLink) setSellStoreList(live *livePlayer, req clientpackets.SetPrivateStoreListSell) {
	store := live.PrivateStore()
	store.ClearSell()
	inv := live.Inventory()
	if len(req.Items) == 0 || inv == nil {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectItemCount))
		return
	}
	rows := make([]privatestore.SellRow, len(req.Items))
	for i, row := range req.Items {
		rows[i] = privatestore.SellRow{ObjectID: row.ObjectID, Count: int(row.Count), Price: int(row.Price)}
	}
	if !privatestore.CanPassSell(inv, rows) {
		live.SetOperateType(privatestore.OperateNone)
		return
	}
	if !live.accessLevel().AllowTransaction {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	if ok, _ := l.canOpenPrivateStore(live, false); !ok {
		return
	}
	if len(rows) > live.PrivateSellStoreLimit() {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectItemCount))
		return
	}
	if store.FillSell(inv, rows, req.Packaged, inv.Adena()) != privatestore.FillOK {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExceededTheMaximum))
		l.sendSellManageList(live, req.Packaged)
		return
	}
	operate := privatestore.OperateSell
	if req.Packaged {
		operate = privatestore.OperatePackageSell
	}
	l.openStore(live, operate, func() wire.Frame {
		return serverpackets.FramePrivateStoreMsgSell(live.ObjectID(), store.SellTitle())
	})
}

// setBuyStoreList answers SetPrivateStoreListBuy the way setSellStoreList
// answers a sell list. A list costing more than live holds leaves the
// manage window open again too.
func (l *GameClientLink) setBuyStoreList(live *livePlayer, req clientpackets.SetPrivateStoreListBuy) {
	store := live.PrivateStore()
	store.ClearBuy()
	inv := live.Inventory()
	if len(req.Items) == 0 || inv == nil {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectItemCount))
		return
	}
	rows := make([]privatestore.BuyRow, len(req.Items))
	for i, row := range req.Items {
		rows[i] = privatestore.BuyRow{TemplateID: row.ItemID, Enchant: row.Enchant, Count: int(row.Count), Price: int(row.Price)}
	}
	if !privatestore.CanPassBuy(inv, rows) {
		live.SetOperateType(privatestore.OperateNone)
		return
	}
	if !live.accessLevel().AllowTransaction {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	if ok, _ := l.canOpenPrivateStore(live, false); !ok {
		return
	}
	if len(rows) > live.PrivateBuyStoreLimit() {
		live.SetOperateType(privatestore.OperateNone)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectItemCount))
		return
	}
	switch store.FillBuy(inv.Templates(), rows, inv.Adena()) {
	case privatestore.FillExceeded:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExceededTheMaximum))
		l.sendBuyManageList(live)
		return
	case privatestore.FillPriceAboveAdena:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePurchasePriceHigherThanMoney))
		l.sendBuyManageList(live)
		return
	}
	l.openStore(live, privatestore.OperateBuy, func() wire.Frame {
		return serverpackets.FramePrivateStoreMsgBuy(live.ObjectID(), store.BuyTitle())
	})
}

// openStore stops live, sits it down and opens its store as operate: the
// new state reaches everyone around, then the store's title.
func (l *GameClientLink) openStore(live *livePlayer, operate privatestore.OperateType, title func() wire.Frame) {
	if live.move != nil {
		live.move.Stop()
	}
	live.Character.Sit()
	live.SetOperateType(operate)
	l.broadcastCharacterInfo(live)
	l.broadcastLiveFrame(live, title)
}

// storeOwner resolves the store owner a buy or sell request names: in the
// world, alive and within interaction distance of live.
func (l *GameClientLink) storeOwner(live *livePlayer, ownerID int32) (*livePlayer, bool) {
	if live.Dead() || live.CursedWeaponEquipped() {
		return nil, false
	}
	owner, ok := l.livePlayerByID(ownerID)
	if !ok || owner.Dead() || !livePlayersInRange(live, owner, tradeInteractionDistance) {
		return nil, false
	}
	return owner, true
}

// buyFromStore answers RequestPrivateStoreBuy: live buys the rows from a
// sell store. A store left with nothing to sell closes, and everyone around
// sees it.
//
// A refusal specified with no answer stays silent: the store
// window closed when the client sent the request, so nothing waits on an
// answer.
func (l *GameClientLink) buyFromStore(live *livePlayer, req clientpackets.RequestPrivateStoreBuy) {
	if len(req.Items) == 0 {
		return
	}
	owner, ok := l.storeOwner(live, req.StoreID)
	if !ok {
		return
	}
	if op := owner.OperateType(); op != privatestore.OperateSell && op != privatestore.OperatePackageSell {
		return
	}
	if !live.accessLevel().AllowTransaction {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	rows := make([]privatestore.PurchaseRow, len(req.Items))
	for i, row := range req.Items {
		rows[i] = privatestore.PurchaseRow{ObjectID: row.ObjectID, Count: int(row.Count), Price: int(row.Price)}
	}
	end := l.itemInstances.BeginOperation(live.ObjectID(), owner.ObjectID())
	defer end()
	deal, err := owner.PrivateStore().Buy(l.inventory, l.storeTrader(owner), l.storeTrader(live), rows)
	l.applyPersistActions(deal.Persist)
	end()
	if err != nil {
		l.log.Error().Err(err).Msg("private store buy")
		return
	}
	if l.answerDeal(live, deal) {
		for _, row := range deal.Rows {
			owner.SendFrame(storeDealFrame(row, live.Name, false))
			live.SendFrame(storeDealFrame(row, owner.Name, true))
		}
	}
	l.closeSoldOutStore(owner, deal)
}

// sellToStore answers RequestPrivateStoreSell: live sells the rows into a
// buy store. A store left wanting nothing closes, and everyone around sees
// it. Refusals specified with no answer stay silent, as buyFromStore's do.
func (l *GameClientLink) sellToStore(live *livePlayer, req clientpackets.RequestPrivateStoreSell) {
	if len(req.Items) == 0 {
		return
	}
	owner, ok := l.storeOwner(live, req.StoreID)
	if !ok || owner.OperateType() != privatestore.OperateBuy {
		return
	}
	if !live.accessLevel().AllowTransaction {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	rows := make([]privatestore.SaleRow, len(req.Items))
	for i, row := range req.Items {
		rows[i] = privatestore.SaleRow{ObjectID: row.ObjectID, ItemID: row.ItemID, Enchant: row.Enchant, Count: int(row.Count), Price: int(row.Price)}
	}
	end := l.itemInstances.BeginOperation(live.ObjectID(), owner.ObjectID())
	defer end()
	deal, err := owner.PrivateStore().Sell(l.inventory, l.storeTrader(owner), l.storeTrader(live), rows)
	l.applyPersistActions(deal.Persist)
	end()
	if err != nil {
		l.log.Error().Err(err).Msg("private store sell")
		return
	}
	if l.answerDeal(live, deal) {
		for _, row := range deal.Rows {
			owner.SendFrame(storeDealFrame(row, live.Name, true))
			live.SendFrame(storeDealFrame(row, owner.Name, false))
		}
	}
	l.closeSoldOutStore(owner, deal)
}

// answerDeal sends live a refused deal's message and reports whether the
// deal went through.
func (l *GameClientLink) answerDeal(live *livePlayer, deal privatestore.Deal) bool {
	msg := 0
	switch deal.Status {
	case privatestore.DealDone:
		return true
	case privatestore.DealNotEnoughAdena:
		msg = serverpackets.SystemMessageYouNotEnoughAdena
	case privatestore.DealWeightExceeded:
		msg = serverpackets.SystemMessageWeightLimitExceeded
	case privatestore.DealSlotsFull:
		msg = serverpackets.SystemMessageSlotsFull
	}
	if msg != 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
	}
	return false
}

// closeSoldOutStore shows everyone around owner that a deal emptying its
// list closed its store. The broadcast runs on owner's own queue, after the
// deal's messages to owner.
func (l *GameClientLink) closeSoldOutStore(owner *livePlayer, deal privatestore.Deal) {
	if deal.Status != privatestore.DealDone || !deal.Emptied {
		return
	}
	postLive(owner, func() {
		if owner.detached() {
			return
		}
		l.broadcastCharacterInfo(owner)
	})
}

// storeDealFrame is the message naming who row changed hands with. buyer
// reports that the reader paid for row; the other side reads that name
// bought it.
func storeDealFrame(row privatestore.Moved, name string, buyer bool) wire.Frame {
	switch {
	case row.Stackable:
		id := serverpackets.SystemMessageS1PurchasedS3S2S
		if buyer {
			id = serverpackets.SystemMessagePurchasedS3S2SFromS1
		}
		return serverpackets.FrameSystemMessageParams(id, serverpackets.TextParam(name), serverpackets.ItemNameParam(row.ItemID), serverpackets.NumberParam(int32(row.Count)))
	case row.Enchant > 0:
		id := serverpackets.SystemMessageS1PurchasedS2S3
		if buyer {
			id = serverpackets.SystemMessagePurchasedS2S3FromS1
		}
		return serverpackets.FrameSystemMessageStringNumberItemName(id, name, int32(row.Enchant), row.ItemID)
	default:
		id := serverpackets.SystemMessageS1PurchasedS2
		if buyer {
			id = serverpackets.SystemMessagePurchasedS2FromS1
		}
		return serverpackets.FrameSystemMessageStringItemName(id, name, row.ItemID)
	}
}

// showPrivateStore opens other's store window for live, the interact on a
// player running one. A store being set up shows nothing.
func (l *GameClientLink) showPrivateStore(live, other *livePlayer) {
	inv := live.Inventory()
	adena := 0
	if inv != nil {
		adena = inv.Adena()
	}
	switch other.OperateType() {
	case privatestore.OperateSell, privatestore.OperatePackageSell:
		listed, packaged := other.PrivateStore().SellList()
		frame, err := serverpackets.FramePrivateStoreListSell(other.ObjectID(), packaged, adena, listed, l.itemTemplates)
		if err != nil {
			l.log.Error().Err(err).Msg("build private store sell list")
			return
		}
		live.SendFrame(frame)
	case privatestore.OperateBuy:
		offers := privatestore.OffersFor(other.PrivateStore().BuyList(), inv)
		frame, err := serverpackets.FramePrivateStoreListBuy(other.ObjectID(), adena, offers, l.itemTemplates)
		if err != nil {
			l.log.Error().Err(err).Msg("build private store buy list")
			return
		}
		live.SendFrame(frame)
	case privatestore.OperateManufacture:
		l.sendRecipeShopSellList(live, other)
	}
}

// storeTitleFrame is the title other's open store shows over it, or false
// when other runs none.
func storeTitleFrame(other *livePlayer) (wire.Frame, bool) {
	store := other.PrivateStore()
	switch other.OperateType() {
	case privatestore.OperateSell, privatestore.OperatePackageSell:
		return serverpackets.FramePrivateStoreMsgSell(other.ObjectID(), store.SellTitle()), true
	case privatestore.OperateBuy:
		return serverpackets.FramePrivateStoreMsgBuy(other.ObjectID(), store.BuyTitle()), true
	case privatestore.OperateManufacture:
		return serverpackets.FrameRecipeShopMsg(other.ObjectID(), store.ShopName()), true
	}
	return wire.Frame{}, false
}

// closeStoreOnTeleport closes the store of a player that finished a
// teleport. Nothing is sent and the player stays seated.
func closeStoreOnTeleport(live *livePlayer) {
	if live.InStoreMode() {
		live.SetOperateType(privatestore.OperateNone)
	}
}
