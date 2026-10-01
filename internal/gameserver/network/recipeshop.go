package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/craft"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// workshopOrderRange is how close a customer must stand to a workshop to
// order a craft, measured between the two bodies' edges.
const workshopOrderRange = 150

// setWorkshopList answers RequestRecipeShopListSet. A packet naming a recipe
// that is not loaded is dropped without a word, as the reference drops a
// packet it cannot read. No recipes, more than the workshop holds, a recipe
// missing from its book page, or a failed set-up check keeps live setting up
// the workshop and shows the manage window again; otherwise live sits down
// and the workshop opens.
func (l *GameClientLink) setWorkshopList(live *livePlayer, req clientpackets.RequestRecipeShopListSet) {
	items := make([]privatestore.ManufactureItem, len(req.Items))
	for i, row := range req.Items {
		r, ok := l.craft.Recipe(int(row.RecipeID))
		if !ok {
			return
		}
		items[i] = privatestore.ManufactureItem{RecipeID: r.ID, Cost: int(row.Cost), Dwarven: r.Dwarven}
	}
	store := live.PrivateStore()
	keepManaging := func() {
		live.SetOperateType(privatestore.OperateManufactureManage)
		_, dwarven := store.ManufactureList()
		l.sendRecipeShopManageList(live, dwarven)
	}
	switch {
	case len(items) == 0:
		keepManaging()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRecipesRegistered))
		return
	case len(items) > privatestore.MaxManufactureItems:
		keepManaging()
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageUpToS1RecipesCanRegister, privatestore.MaxManufactureItems))
		return
	}
	book := live.RecipeBook()
	for _, m := range items {
		if !book.HasOn(m.RecipeID, m.Dwarven) {
			keepManaging()
			return
		}
	}
	if ok, _ := l.canOpenPrivateStore(live, false); !ok {
		keepManaging()
		return
	}
	store.SetManufacture(items)
	l.openStore(live, privatestore.OperateManufacture, func() wire.Frame {
		return serverpackets.FrameRecipeShopMsg(live.ObjectID(), store.ShopName())
	})
}

// sendRecipeShopSellList shows live crafter's workshop: every recipe it
// lists, with crafter's MP and live's adena.
func (l *GameClientLink) sendRecipeShopSellList(live, crafter *livePlayer) {
	adena := 0
	if inv := live.Inventory(); inv != nil {
		adena = inv.Adena()
	}
	offered, _ := crafter.PrivateStore().ManufactureList()
	res := crafter.ResourceValues()
	live.SendFrame(serverpackets.FrameRecipeShopSellList(crafter.ObjectID(), int32(res.CurrentMP), int32(res.MaxMP), adena, offered))
}

// sendRecipeShopItemInfo sends live crafter's craft window for recipeID.
func sendRecipeShopItemInfo(live, crafter *livePlayer, recipeID int32) {
	res := crafter.ResourceValues()
	live.SendFrame(serverpackets.FrameRecipeShopItemInfo(crafter.ObjectID(), recipeID, int32(res.CurrentMP), int32(res.MaxMP)))
}

// showWorkshopRecipe answers RequestRecipeShopMakeInfo with a workshop's
// craft window for the recipe asked. A crafter not running a workshop gets
// nothing, as in the reference; the window only asks again on the next
// click.
func (l *GameClientLink) showWorkshopRecipe(live *livePlayer, req clientpackets.RequestRecipeShopMakeInfo) {
	crafter, ok := l.livePlayerByID(req.CrafterID)
	if !ok || crafter.OperateType() != privatestore.OperateManufacture {
		return
	}
	sendRecipeShopItemInfo(live, crafter, req.RecipeID)
}

// showWorkshopOf answers RequestRecipeShopManagePrev, the craft window's
// back button: live's target's workshop list. A dead player is released
// with ActionFailed; a target that is no player gets nothing, as in the
// reference.
func (l *GameClientLink) showWorkshopOf(live *livePlayer) {
	if live.AlikeDead() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	crafter, ok := live.Target().(*livePlayer)
	if !ok {
		return
	}
	l.sendRecipeShopSellList(live, crafter)
}

// orderWorkshopCraft answers RequestRecipeShopMakeItem: live orders a craft
// from a workshop within reach. The craft runs with the workshop held, so
// orders on one workshop never overlap and its list cannot change under
// one. Requests the reference drops without a word stay silent: the craft
// window only asks again on the next click.
func (l *GameClientLink) orderWorkshopCraft(live *livePlayer, req clientpackets.RequestRecipeShopMakeItem) {
	crafter, ok := l.livePlayerByID(req.CrafterID)
	if !ok || live.Operating() || crafter.OperateType() != privatestore.OperateManufacture {
		return
	}
	if crafter.InCombat() || live.InCombat() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantOperateStoreDuringCombat))
		return
	}
	if !workshopInReach(live, crafter) {
		return
	}
	r, ok := l.craft.Recipe(int(req.RecipeID))
	if !ok || !crafter.RecipeBook().HasOn(r.ID, r.Dwarven) {
		return
	}
	busy := l.trades != nil && (l.trades.ProcessingTransaction(crafter.ObjectID()) || l.trades.ProcessingTransaction(live.ObjectID()))
	var attempt craft.ShopAttempt
	crafter.PrivateStore().Craft(r.ID, func(cost int) {
		attempt = l.craft.MakeFor(crafter.Character, live.Character, r, cost, busy, func(price int) bool {
			return l.payAdena(live, crafter, price)
		})
	})
	sendCraftNotices(crafter, attempt.ToCrafter)
	sendCraftNotices(live, attempt.ToCustomer)
	sendRecipeShopItemInfo(live, crafter, req.RecipeID)
}

// workshopInReach reports whether live stands within workshopOrderRange of
// crafter, counting both bodies' collision radii and the height difference.
func workshopInReach(live, crafter *livePlayer) bool {
	ax, ay, az := live.Position()
	bx, by, bz := crafter.Position()
	dx, dy, dz := int64(ax-bx), int64(ay-by), int64(az-bz)
	reach := workshopOrderRange + live.CollisionRadius() + crafter.CollisionRadius()
	return float64(dx*dx+dy*dy+dz*dz) <= reach*reach
}

// payAdena moves amount adena from payer to payee as one step: payer must
// still hold it all when the move runs. It reports whether the adena moved.
func (l *GameClientLink) payAdena(payer, payee *livePlayer, amount int) bool {
	from, to := payer.Inventory(), payee.Inventory()
	if from == nil || to == nil || amount <= 0 {
		return false
	}
	adena := from.ItemByTemplateID(item.AdenaID)
	if adena == nil {
		return false
	}
	move := []invops.Move{{ObjectID: adena.ObjectID, Count: amount}}
	res, moved, err := l.inventory.Exchange(from, to, move, nil, func(held, _ itemcontainer.Held) bool {
		inst := held.ItemByObjectID(adena.ObjectID)
		return inst != nil && inst.CountValue() >= amount
	})
	l.applyPersistActions(res.Persist)
	if err != nil {
		l.log.Error().Err(err).Msg("pay workshop adena")
		return false
	}
	return moved
}
