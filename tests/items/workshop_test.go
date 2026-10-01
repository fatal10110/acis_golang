package items

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// actionCommonManufacture is the action-bar command opening a common
// workshop.
const actionCommonManufacture = 51

// workshopCost is what the test workshop charges for the common recipe.
const workshopCost = 100

func encodeStoreActionUse(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeRecipeShopListSet(rows ...[2]int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRecipeShopListSet)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r[0])
		w.WriteInt32(r[1])
	}
	return w.Bytes()
}

func encodeRecipeShopOrder(opcode byte, crafterID, recipeID int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(crafterID)
	w.WriteInt32(recipeID)
	if opcode == clientpackets.OpcodeRequestRecipeShopMakeItem {
		w.WriteInt32(0)
	}
	return w.Bytes()
}

// workshop is a crafter running a common workshop and a customer next to
// it, both in the world.
type workshop struct {
	srv                   *gameservertest.Server
	crafter, customer     *testsupport.ScriptedClient
	crafterID, customerID int32
}

// openWorkshop boots a crafter knowing Create Common Item with the common
// recipe in its book and a customer holding customerAdena and materials of
// the recipe's material, then sets the workshop up: the common page opens,
// the name is set while setting up, and the list opens the workshop.
func openWorkshop(t *testing.T, customerAdena, materials int32, opts ...gameservertest.Option) workshop {
	t.Helper()
	return openWorkshopWithBook(t, customerAdena, materials, nil, opts...)
}

// openWorkshopWithBook is openWorkshop with the extra recipes in the
// crafter's book too; the workshop still lists only the common recipe.
func openWorkshopWithBook(t *testing.T, customerAdena, materials int32, extra []int, opts ...gameservertest.Option) workshop {
	t.Helper()
	srv, crafterID := bootCraft(t, opts...)
	knowSkill(t, srv, crafterID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, crafterID, append([]int{commonRecipeID}, extra...)...)
	customerID := srv.SeedCharacterFor(t, "player2", "Customer", 1, 0).ID
	if customerAdena > 0 {
		srv.GiveItem(t, customerID, item.AdenaID, customerAdena)
	}
	if materials > 0 {
		srv.GiveItem(t, customerID, commonMaterialID, materials)
	}
	crafter := srv.Client
	startInWorld(t, crafter)
	customer := srv.DialClient(t, "player2", 1)
	startInWorld(t, customer)
	drainUntilQuiet(t, crafter)
	drainUntilQuiet(t, customer)

	crafter.Send(encodeStoreActionUse(actionCommonManufacture))
	manage := crafter.Read()
	assertFrameOpcode(t, manage, serverpackets.OpcodeRecipeShopManageList, "RecipeShopManageList")
	r := wire.NewReader(manage[1:])
	owner, _, page, recipes := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	var book []int
	for range recipes {
		book = append(book, int(r.ReadInt32()))
		r.ReadInt32()
	}
	if owner != crafterID || page != 1 || len(book) != 1+len(extra) || !slices.Contains(book, commonRecipeID) {
		t.Fatalf("manage window owner %d page %d recipes %v", owner, page, book)
	}
	crafter.Send(encodeStoreText(clientpackets.OpcodeRequestRecipeShopMessageSet, "Potions"))
	crafter.Send(encodeRecipeShopListSet([2]int32{commonRecipeID, workshopCost}))
	srv.Settle(t)
	crafterFrames := collectUntilQuiet(t, crafter)
	customerFrames := collectUntilQuiet(t, customer)
	for who, frames := range map[string][][]byte{"crafter": crafterFrames, "customer": customerFrames} {
		msg := lastFrame(t, frames, serverpackets.OpcodeRecipeShopMsg, who)
		r := wire.NewReader(msg[1:])
		if owner, name := r.ReadInt32(), r.ReadString(); owner != crafterID || name != "Potions" {
			t.Fatalf("%s RecipeShopMsg = %d %q", who, owner, name)
		}
	}
	if got := srv.PlayerOperateType(t, crafterID); got != privatestore.OperateManufacture {
		t.Fatalf("crafter operate type = %d, want manufacture", got)
	}
	return workshop{srv: srv, crafter: crafter, customer: customer, crafterID: crafterID, customerID: customerID}
}

func encodeStoreText(opcode byte, text string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(text)
	return w.Bytes()
}

func lastFrame(t *testing.T, frames [][]byte, opcode byte, what string) []byte {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == opcode {
			return frames[i]
		}
	}
	t.Fatalf("%s: no %#x among %d frames", what, opcode, len(frames))
	return nil
}

// mixedSysMsg decodes a SystemMessage whose parameters may mix text with
// int32 parameters; a text parameter's value is reported in text.
func mixedSysMsg(t *testing.T, frame []byte) (int32, []string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	n := r.ReadInt32()
	var params []string
	for range n {
		switch kind := r.ReadInt32(); kind {
		case serverpackets.SystemMessageParamText:
			params = append(params, "text:"+r.ReadString())
		default:
			params = append(params, strconv.Itoa(int(kind))+":"+strconv.Itoa(int(r.ReadInt32())))
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode SystemMessage %x: %v", frame, err)
	}
	return id, params
}

func assertMixedSysMsg(t *testing.T, frame []byte, id int, want ...string) {
	t.Helper()
	gotID, got := mixedSysMsg(t, frame)
	if gotID != int32(id) || len(got) != len(want) {
		t.Fatalf("SystemMessage %d %v, want %d %v", gotID, got, id, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SystemMessage %d %v, want %v", id, got, want)
		}
	}
}

// keepWorkshopFrames keeps the system messages and workshop windows among
// frames, in order.
func keepWorkshopFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodeRecipeShopItemInfo, serverpackets.OpcodeActionFailed:
			out = append(out, f)
		}
	}
	return out
}

// TestWorkshopCraftsForCustomer walks a workshop order end to end, as the
// reference runs it (RequestRecipeShopListSet, Player.onInteract,
// RequestRecipeShopMakeInfo, RequestRecipeShopMakeItem, RecipeItemMaker
// with a customer): the customer's click shows the workshop's list with
// the crafter's MP and the customer's adena; the craft window answers; the
// order takes the customer's materials and 100 adena, gives it the potion
// and tells both sides who made what for how much; the crafter pays the MP.
func TestWorkshopCraftsForCustomer(t *testing.T) {
	t.Parallel()
	w := openWorkshop(t, 500, 2)

	w.customer.Send(encodeAction(w.crafterID, spawnX, spawnY, spawnZ, false))
	drainUntilQuiet(t, w.customer)
	drainUntilQuiet(t, w.crafter)
	w.customer.Send(encodeAction(w.crafterID, spawnX, spawnY, spawnZ, false))
	list := lastFrame(t, collectUntilQuiet(t, w.customer), serverpackets.OpcodeRecipeShopSellList, "workshop window")
	r := wire.NewReader(list[1:])
	crafter, _, maxMP, adena, rows := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	recipeID, _, cost := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if crafter != w.crafterID || maxMP <= 0 || adena != 500 || rows != 1 || recipeID != commonRecipeID || cost != workshopCost {
		t.Fatalf("workshop window crafter %d maxMP %d adena %d rows %d recipe %d cost %d", crafter, maxMP, adena, rows, recipeID, cost)
	}
	drainUntilQuiet(t, w.crafter)

	w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeInfo, w.crafterID, commonRecipeID))
	info := w.customer.Read()
	assertFrameOpcode(t, info, serverpackets.OpcodeRecipeShopItemInfo, "RecipeShopItemInfo")
	if got := wire.NewReader(info[5:]).ReadInt32(); got != commonRecipeID {
		t.Fatalf("RecipeShopItemInfo recipe = %d, want %d", got, commonRecipeID)
	}
	mpBefore := w.srv.PlayerCurrentMP(t, w.crafterID)

	w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, commonRecipeID))
	got := keepWorkshopFrames(collectUntilQuiet(t, w.customer))
	requireFrames(t, got, 4)
	assertSysMsg(t, got[0], serverpackets.SystemMessageS2S1Disappeared, itemNameParam(commonMaterialID), itemNumberParam(2))
	assertMixedSysMsg(t, got[1], serverpackets.SystemMessageS1CreatedS2ForS3Adena, "text:Crafter", "3:1060", "6:100")
	assertSysMsg(t, got[2], serverpackets.SystemMessageEarnedItemS1, itemNameParam(commonProductID))
	assertFrameOpcode(t, got[3], serverpackets.OpcodeRecipeShopItemInfo, "refreshed craft window")
	crafterGot := keepWorkshopFrames(collectUntilQuiet(t, w.crafter))
	requireFrames(t, crafterGot, 1)
	assertMixedSysMsg(t, crafterGot[0], serverpackets.SystemMessageS2CreatedForS1ForS3Adena, "text:Customer", "3:1060", "6:100")
	if mp := w.srv.PlayerCurrentMP(t, w.crafterID); mp != mpBefore-30 {
		t.Fatalf("crafter MP = %d, want %d", mp, mpBefore-30)
	}

	w.srv.InventoryUpdates.Tick()
	w.srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, commonProductID); inst.Count != 1 {
		t.Fatalf("customer product count = %d, want 1", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, item.AdenaID); inst.Count != 400 {
		t.Fatalf("customer adena = %d, want 400", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, w.srv, w.crafterID, item.AdenaID); inst.Count != workshopCost {
		t.Fatalf("crafter adena = %d, want %d", inst.Count, workshopCost)
	}
	for _, inst := range persistedItems(t, w.srv, w.customerID) {
		if inst.TemplateID == commonMaterialID {
			t.Fatalf("customer kept a material row: %+v", inst)
		}
	}
}

// TestWorkshopOrderRefusals pins the orders a workshop refuses before
// anything is spent: a customer short of the cost reads
// YOU_NOT_ENOUGH_ADENA, one short of a material its MISSING line, each
// followed by the refreshed craft window; an order to a player no longer
// running the workshop is dropped without a word.
func TestWorkshopOrderRefusals(t *testing.T) {
	t.Parallel()
	t.Run("adena", func(t *testing.T) {
		t.Parallel()
		w := openWorkshop(t, workshopCost-1, 2)
		w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, commonRecipeID))
		got := keepWorkshopFrames(collectUntilQuiet(t, w.customer))
		requireFrames(t, got, 2)
		assertStaticSystemMessage(t, got[0], serverpackets.SystemMessageYouNotEnoughAdena)
		assertFrameOpcode(t, got[1], serverpackets.OpcodeRecipeShopItemInfo, "craft window")
	})
	t.Run("materials", func(t *testing.T) {
		t.Parallel()
		w := openWorkshop(t, 500, 1)
		w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, commonRecipeID))
		got := keepWorkshopFrames(collectUntilQuiet(t, w.customer))
		requireFrames(t, got, 2)
		assertSysMsg(t, got[0], serverpackets.SystemMessageMissingS2S1ToCreate, itemNameParam(commonMaterialID), itemNumberParam(1))
		assertFrameOpcode(t, got[1], serverpackets.OpcodeRecipeShopItemInfo, "craft window")
		w.srv.InventoryUpdates.Tick()
		w.srv.FlushItems(t)
		if inst := mustFindItemByTemplate(t, w.srv, w.customerID, item.AdenaID); inst.Count != 500 {
			t.Fatalf("customer adena = %d, want 500", inst.Count)
		}
	})
	t.Run("closed", func(t *testing.T) {
		t.Parallel()
		w := openWorkshop(t, 500, 2)
		w.crafter.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRecipeShopManageQuit).Bytes())
		w.srv.Settle(t)
		drainUntilQuiet(t, w.customer)
		w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, commonRecipeID))
		w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeInfo, w.crafterID, commonRecipeID))
		assertNoFrameFor(t, w.customer, 300*time.Millisecond, "orders to a closed workshop")
	})
}

// TestWorkshopRefusesUnlistedRecipe pins an order for a recipe the
// crafter's book holds but the workshop does not list: it is dropped
// without a word and spends nothing, neither the customer's adena and
// materials nor the crafter's MP. The reference crafts it for free
// (RecipeItemMaker.java:75-80 sets the price only for a listed recipe);
// the workshop window never offers one, so only a crafted packet names it.
func TestWorkshopRefusesUnlistedRecipe(t *testing.T) {
	t.Parallel()
	const unlistedRecipeID = 687
	common, _ := gameservertest.RecipeTemplates().Find(commonRecipeID)
	recipes := recipe.NewTable([]recipe.Recipe{common, {
		ID: unlistedRecipeID, ItemID: 6927, Level: 1, SuccessRate: 100, MPCost: 10,
		Materials: []recipe.Ingredient{{ItemID: commonMaterialID, Count: 1}},
		Product:   recipe.Ingredient{ItemID: commonProductID, Count: 1},
	}})
	w := openWorkshopWithBook(t, 500, 2, []int{unlistedRecipeID}, gameservertest.WithRecipes(recipes))
	mpBefore := w.srv.PlayerCurrentMP(t, w.crafterID)

	w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, unlistedRecipeID))
	assertNoFrameFor(t, w.customer, 300*time.Millisecond, "an order for an unlisted recipe")
	if got := keepWorkshopFrames(collectUntilQuiet(t, w.crafter)); len(got) != 0 {
		t.Fatalf("crafter got %d workshop frames for an unlisted order", len(got))
	}
	if mp := w.srv.PlayerCurrentMP(t, w.crafterID); mp != mpBefore {
		t.Fatalf("crafter MP = %d, want %d", mp, mpBefore)
	}
	w.srv.InventoryUpdates.Tick()
	w.srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, commonMaterialID); inst.Count != 2 {
		t.Fatalf("customer materials = %d, want 2", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, item.AdenaID); inst.Count != 500 {
		t.Fatalf("customer adena = %d, want 500", inst.Count)
	}
	for _, inst := range persistedItems(t, w.srv, w.customerID) {
		if inst.TemplateID == commonProductID {
			t.Fatalf("customer got a product: %+v", inst)
		}
	}
}

// TestWorkshopLocksRecipeBook pins the recipe book changes a running
// workshop refuses: deleting a recipe answers
// CANT_ALTER_RECIPEBOOK_WHILE_CRAFTING (RequestRecipeBookDestroy.java:28-32),
// a craft for oneself is dropped without a word
// (RequestRecipeItemMakeSelf.java:31-32), and using a recipe item stops at
// UseItem's store gate (UseItem.java:41), so the recipe handler's own
// workshop lock (Recipes.java:51-52,68-69) is never reached from the
// client. Nothing changes. Setting a workshop up locks nothing.
func TestWorkshopLocksRecipeBook(t *testing.T) {
	t.Parallel()
	const (
		secondRecipeID   = 687
		secondRecipeItem = 6927
	)
	common, _ := gameservertest.RecipeTemplates().Find(commonRecipeID)
	recipes := append([]recipe.Recipe{common}, recipe.Recipe{
		ID: secondRecipeID, ItemID: secondRecipeItem, Level: 1, SuccessRate: 100,
		Materials: []recipe.Ingredient{{ItemID: commonMaterialID, Count: 1}},
		Product:   recipe.Ingredient{ItemID: commonProductID, Count: 1},
	})
	templates := append(slices.Clone(craftItemTemplates().All()), &item.Template{
		ID: secondRecipeItem, Name: "Recipe: Second", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
		EtcItem: &item.EtcItemDetail{Type: item.EtcItemRecipe, Handler: "Recipes"},
	})
	srv, crafterID := bootCraft(t, gameservertest.WithManufactureDelay(0),
		gameservertest.WithRecipes(recipe.NewTable(recipes)), gameservertest.WithItemTemplates(item.NewTable(templates)))
	c := srv.Client
	knowSkill(t, srv, crafterID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, crafterID, commonRecipeID)
	srv.GiveItem(t, crafterID, commonMaterialID, 2)
	scroll := srv.GiveItem(t, crafterID, secondRecipeItem, 1)
	startInWorld(t, c)
	srv.SetPlayerOperateType(t, crafterID, privatestore.OperateManufacture)
	drainUntilQuiet(t, c)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookDestroy, commonRecipeID))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCantAlterRecipeBookWhileCrafting)
	barrier(t, c)
	// Using the recipe item stops at UseItem's own store gate, ahead of the
	// recipe handler's workshop lock.
	c.Send(encodeUseItem(scroll, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageItemsUnavailableForStore)
	barrier(t, c)
	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	barrier(t, c)

	if got := savedRecipes(t, srv, crafterID); !slices.Equal(got, []int{commonRecipeID}) {
		t.Fatalf("saved recipes = %v, want only the common recipe kept", got)
	}
	srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, srv, crafterID, commonMaterialID); inst.Count != 2 {
		t.Fatalf("materials after a refused self craft = %d, want 2", inst.Count)
	}
	if inst := mustFindItem(t, srv, crafterID, scroll); inst.Count != 1 {
		t.Fatalf("recipe item after a refused registration = %d, want 1", inst.Count)
	}

	srv.SetPlayerOperateType(t, crafterID, privatestore.OperateManufactureManage)
	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookDestroy, commonRecipeID))
	assertSysMsg(t, c.Read(), serverpackets.SystemMessageS1HasBeenDeleted, itemNameParam(commonRecipeItemID))
}
