package network

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	// inventoryDisableWindow is how long opening a shop or warehouse window
	// keeps the player's item list request unanswered.
	inventoryDisableWindow = 1500 * time.Millisecond
	// expertiseSkillID is the skill whose level is the highest item grade
	// a player can use without penalty.
	expertiseSkillID = 239
	// maxPreviewListID bounds the buylist id a try-on may name.
	maxPreviewListID = 4000000
)

// tempInventoryDisable starts a window, inventoryDisableWindow long, in
// which p's item list requests go unanswered. Every call arms its own end:
// a window opened while another is running ends with the earlier one.
// A shop or warehouse window opens one, so the client's own item list
// request does not race the window's contents.
func (p *livePlayer) tempInventoryDisable() {
	p.inventoryDisabled.Store(true)
	p.after(inventoryDisableWindow, func() { p.inventoryDisabled.Store(false) })
}

// showBuyWindow opens the buy window of buylist listID at merchant f. A
// list f may not sell answers nothing.
func (l *GameClientLink) showBuyWindow(live *livePlayer, f *npc.Folk, listID int) {
	list, ok := l.merchant.List(listID)
	inv := live.Inventory()
	if !ok || !list.AllowsNPC(f.NpcID()) || inv == nil {
		return
	}
	live.tempInventoryDisable()
	// ponytail: no castle taxes yet; the buy window shows untaxed prices
	// until #239 gives a merchant its castle's tax rate.
	frame, err := serverpackets.FrameBuyList(list, l.merchant.Count, inv.Adena(), 0, l.merchant.Config().SiegeGuardsPriceRate, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Int("buylist", listID).Msg("build BuyList")
		return
	}
	live.SendFrame(frame)
}

// showWearWindow opens the try-on window of buylist listID at merchant f:
// the equipable products of a grade the player's expertise covers. A list
// f may not sell answers nothing.
func (l *GameClientLink) showWearWindow(live *livePlayer, f *npc.Folk, listID int) {
	list, ok := l.merchant.List(listID)
	inv := live.Inventory()
	if !ok || !list.AllowsNPC(f.NpcID()) || inv == nil {
		return
	}
	live.tempInventoryDisable()
	frame, err := serverpackets.FrameShopPreviewList(list, inv.Adena(), live.SkillLevel(expertiseSkillID), l.merchant.Config().WearPrice, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Int("buylist", listID).Msg("build ShopPreviewList")
		return
	}
	live.SendFrame(frame)
}

// requestBuyItem buys the rows of req from its buylist. A list sold by an
// NPC needs that NPC, a merchant or mercenary manager, targeted and within
// interaction reach. A completed purchase shows the merchant's -bought
// page when it has one, then the full item list.
//
// Every refusal the reference answers with a system message answers so
// here; the others (an unknown list, an untargeted or unreachable seller,
// an item off the list, an unpriced item, a count over the stock, a price
// overflowing the adena range) answer nothing, as in the reference. The
// buy window's submit leaves no client action pending, so that silence
// cannot hang the client.
func (l *GameClientLink) requestBuyItem(live *livePlayer, req clientpackets.RequestBuyItem) {
	list, ok := l.merchant.List(int(req.ListID))
	inv := live.Inventory()
	if !ok || inv == nil {
		return
	}
	var seller *npc.Folk
	if list.NPCID > 0 {
		f, _ := live.Target().(*npc.Folk)
		if f == nil || !(f.Merchant() || f.MercenaryManager()) || !list.AllowsNPC(f.NpcID()) || !l.playerCanDoInteract(live, f) {
			return
		}
		seller = f
	}
	rows := make([]merchant.BuyRow, len(req.Items))
	for i, it := range req.Items {
		rows[i] = merchant.BuyRow{ItemID: it.ItemID, Count: it.Count}
	}
	// ponytail: no castle taxes yet; prices stay untaxed and no revenue
	// reaches a castle until #239 gives a merchant its castle's tax rate.
	res, err := l.merchant.Buy(inv, list, rows, 0, live.access.IsGM)
	if err != nil {
		l.log.Error().Err(err).Int32("buylist", req.ListID).Msg("buy items: allocate item id")
	}
	switch res.Outcome {
	case merchant.BuyQuantityExceeded:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted))
		return
	case merchant.BuyWeightExceeded:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWeightLimitExceeded))
		return
	case merchant.BuySlotsFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return
	case merchant.BuyNotEnoughAdena:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	case merchant.BuyDone:
	default:
		return
	}
	if seller != nil {
		if path, ok := seller.BoughtPage(); ok {
			if page, ok := l.html.Get(path); ok {
				id := seller.ObjectID()
				sendValidatedHTML(live, id, strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(id))), 0)
			}
		}
	}
	inv.UpdateWeight()
	frame, err := live.buildItemList(l.itemTemplates, true)
	if err != nil {
		l.log.Error().Err(err).Msg("build ItemList")
		return
	}
	live.SendFrame(frame)
}

// requestPreviewItem tries the items of req on at the targeted merchant,
// for adena: the items show worn until WearDelay has passed, when the
// player's own appearance comes back. A GM may try items on from any
// distance.
//
// An empty request or an out-of-range list id answers ActionFailed, and
// two items for one slot or too little adena a system message. The other
// refusals (an untargeted or unreachable merchant, an unknown list, an
// item off the list) answer nothing, as in the reference; the try-on
// window's submit leaves no client action pending.
func (l *GameClientLink) requestPreviewItem(live *livePlayer, req clientpackets.RequestPreviewItem) {
	if len(req.Items) < 1 || req.ListID >= maxPreviewListID {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	f, _ := live.Target().(*npc.Folk)
	if f == nil || !f.Merchant() || (!live.access.IsGM && !interactInRange(live, f, interactionDistance)) {
		return
	}
	list, ok := l.merchant.List(int(req.ListID))
	inv := live.Inventory()
	if !ok || inv == nil {
		return
	}
	res := l.merchant.TryOn(inv, list, req.Items)
	switch res.Outcome {
	case merchant.TryOnSameSlot:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouCanNotTryThoseItemsOnAtTheSameTime))
		return
	case merchant.TryOnNegativePrice:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	case merchant.TryOnNotEnoughAdena:
		// Once for the failed payment, once for the refused try-on.
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	case merchant.TryOnDone:
	default:
		return
	}
	if res.Paid > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(res.Paid)))
	}
	if !res.Shown {
		return
	}
	live.SendFrame(serverpackets.FrameShopPreviewInfo(res.Items))
	live.after(l.merchant.Config().WearDelay, func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoLongerTryingOn))
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	})
}
