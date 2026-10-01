package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/craft"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// craftingDisabledText and registerDisabledText are the plain chat lines a
// craft and a recipe registration answer while crafting is switched off.
const (
	craftingDisabledText = "Item creation is currently disabled."
	registerDisabledText = "Crafting is disabled, you cannot register this recipe."
)

// restoreRecipeBook fills c's recipe book from its saved rows before c
// enters the world. A row naming a recipe that is no longer loaded is
// skipped, and a failed read leaves the book empty; both are logged.
func (l *GameClientLink) restoreRecipeBook(ctx context.Context, c *player.Character) {
	if l.recipeBooks == nil {
		return
	}
	ids, err := l.recipeBooks.ListByOwner(ctx, c.ID)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: list recipe book")
		return
	}
	book := c.RecipeBook()
	for _, id := range ids {
		r, ok := l.craft.Recipe(id)
		if !ok {
			l.log.Error().Int32("object_id", c.ID).Int("recipe_id", id).Msg("enter world: recipe book row names no loaded recipe")
			continue
		}
		book.Put(r)
	}
}

// openRecipeBook answers RequestRecipeBookOpen, which the craft window's
// back button sends: the page it names, unless live is casting or cannot
// use skills.
func (l *GameClientLink) openRecipeBook(live *livePlayer, req clientpackets.RequestRecipeBookOpen) {
	if live.CastingNow() || live.AllSkillsDisabled() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRecipeBookWhileCasting))
		return
	}
	sendRecipeBook(live, req.Dwarven)
}

// sendRecipeBook sends live the dwarven or common page of its recipe book.
func sendRecipeBook(live *livePlayer, dwarven bool) {
	live.SendFrame(recipeBookFrame(live, dwarven))
}

// recipeBookFrame builds the dwarven or common page of live's recipe book.
func recipeBookFrame(live *livePlayer, dwarven bool) wire.Frame {
	maxMP := int32(live.ResourceValues().MaxMP)
	return serverpackets.FrameRecipeBookItemList(dwarven, maxMP, live.RecipeBook().Recipes(dwarven))
}

// destroyRecipe answers RequestRecipeBookDestroy: the recipe leaves the
// book together with every shortcut pointing at it, then live sees the
// deletion and its page again. An unknown recipe id is dropped without a
// word, as specified; the book window stays as it was and no
// client action waits on the answer.
func (l *GameClientLink) destroyRecipe(live *livePlayer, req clientpackets.RequestRecipeBookDestroy) {
	// The book of a running workshop is locked, the recipe kept.
	if live.OperateType() == privatestore.OperateManufacture {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantAlterRecipeBookWhileCrafting))
		return
	}
	r, ok := l.craft.Forget(live.Character, int(req.RecipeID))
	if !ok {
		return
	}
	l.deleteTargetShortcuts(live, shortcut.Recipe, int32(r.ID))
	if l.recipeBooks != nil {
		recipeID := r.ID
		l.queueRowWrite(live.ObjectID(), "delete recipe", func(ctx context.Context, ownerID int32) error {
			return l.recipeBooks.Delete(ctx, ownerID, recipeID)
		})
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1HasBeenDeleted, r.ItemID))
	sendRecipeBook(live, r.Dwarven)
}

// sendRecipeItemMakeInfo answers RequestRecipeItemMakeInfo with the craft
// window of any loaded recipe, whether or not live holds it. An unknown
// recipe id gets nothing: the specified answer is a packet carrying no
// bytes at all, which the client has nothing to read from, and the
// request leaves no client action waiting.
func (l *GameClientLink) sendRecipeItemMakeInfo(live *livePlayer, req clientpackets.RequestRecipeItemMakeInfo) {
	r, ok := l.craft.Recipe(int(req.RecipeID))
	if !ok {
		return
	}
	sendRecipeMakeInfo(live, r, serverpackets.RecipeMakeInfoNone)
}

func sendRecipeMakeInfo(live *livePlayer, r recipe.Recipe, status int32) {
	res := live.ResourceValues()
	live.SendFrame(serverpackets.FrameRecipeItemMakeInfo(r, int32(res.CurrentMP), int32(res.MaxMP), status))
}

// makeRecipeSelf answers RequestRecipeItemMakeSelf. A request specified to
// drop without a word stays silent here too: an unknown recipe, or one not
// on the matching page of live's book. The craft window only asks again on
// the player's next click, so nothing waits on those.
func (l *GameClientLink) makeRecipeSelf(live *livePlayer, req clientpackets.RequestRecipeItemMakeSelf) {
	busy := l.trades != nil && l.trades.ProcessingTransaction(live.ObjectID())
	attempt := l.craft.MakeSelf(live.Character, int(req.RecipeID), busy)
	sendCraftNotices(live, attempt.Notices)
	if !attempt.Made {
		return
	}
	status := serverpackets.RecipeMakeInfoFailed
	if attempt.Success {
		status = serverpackets.RecipeMakeInfoSuccess
	}
	sendRecipeMakeInfo(live, attempt.Recipe, status)
}

// useRecipeItem answers UseItem on a recipe item: its recipe goes into the
// book when live may take it. It reports false for any other item.
func (l *GameClientLink) useRecipeItem(live *livePlayer, inst *item.Instance, tmpl *item.Template) bool {
	reg, ok := l.craft.Register(live.Character, inst, tmpl)
	if !ok {
		return false
	}
	sendCraftNotices(live, reg.Notices)
	if reg.Registered == nil {
		return true
	}
	if l.recipeBooks != nil {
		recipeID := reg.Registered.ID
		l.queueRowWrite(live.ObjectID(), "store recipe", func(ctx context.Context, ownerID int32) error {
			return l.recipeBooks.Insert(ctx, ownerID, recipeID)
		})
	}
	sendRecipeBook(live, reg.Registered.Dwarven)
	return true
}

// queueRowWrite queues one row write on ownerID's persistence lane, where
// character selection waits for it before reading the rows back. Nothing
// waits for it here: the row is written after the book changes and a failed
// write is only logged, so the client's answer never depends on it.
func (l *GameClientLink) queueRowWrite(ownerID int32, op string, write func(ctx context.Context, ownerID int32) error) {
	l.persist.Enqueue(ownerID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := write(ctx, ownerID); err != nil {
			l.log.Error().Err(err).Int32("object_id", ownerID).Msg(op)
		}
	})
}

// sendCraftNotices sends each craft or registration notice as its message.
func sendCraftNotices(live *livePlayer, notices []any) {
	for _, n := range notices {
		switch n := n.(type) {
		case craft.ActionFailed:
			live.SendFrame(serverpackets.FrameActionFailed())
		case craft.InCombat:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantOperateStoreDuringCombat))
		case craft.MissingMaterial:
			live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageMissingS2S1ToCreate, n.ItemID, int32(n.Count)))
		case craft.NotEnoughMP:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughMP))
		case craft.CraftingDisabled:
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, craftingDisabledText))
		case craft.MaterialConsumed:
			if n.Count > 1 {
				live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageS2S1Disappeared, n.ItemID, int32(n.Count)))
			} else {
				live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disappeared, n.ItemID))
			}
		case craft.ProductEarned:
			if n.Count > 1 {
				live.SendFrame(serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageEarnedS2S1S, n.ItemID, int32(n.Count)))
			} else {
				live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEarnedItemS1, n.ItemID))
			}
		case craft.MixingFailed:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMixingFailed))
		case craft.RegisterDisabled:
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, registerDisabledText))
		case craft.AlreadyRegistered:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageRecipeAlreadyRegistered))
		case craft.NoCraftAbility:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantRegisterNoAbilityToCraft))
		case craft.LevelTooLow:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCreateLvlTooLowToRegister))
		case craft.BookFull:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageUpToS1RecipesCanRegister, int32(n.Limit)))
		case craft.RecipeAdded:
			live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Added, n.ItemID))
		case craft.BookLocked:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantAlterRecipeBookWhileCrafting))
		case craft.NotEnoughAdena:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		case craft.CraftedFor:
			live.SendFrame(workshopCraftFrame(serverpackets.SystemMessageS2CreatedForS1ForS3Adena,
				serverpackets.SystemMessageS2S3SCreatedForS1ForS4Adena, n.Customer, n.ItemID, n.Count, n.Price))
		case craft.CraftedBy:
			live.SendFrame(workshopCraftFrame(serverpackets.SystemMessageS1CreatedS2ForS3Adena,
				serverpackets.SystemMessageS1CreatedS2S3SForS4Adena, n.Crafter, n.ItemID, n.Count, n.Price))
		case craft.CraftForFailed:
			live.SendFrame(workshopCraftFrame(serverpackets.SystemMessageCreationOfS2ForS1AtS3AdenaFail, 0, n.Customer, n.ItemID, 1, n.Price))
		case craft.CraftByFailed:
			live.SendFrame(workshopCraftFrame(serverpackets.SystemMessageS1FailedToCreateS2ForS3Adena, 0, n.Crafter, n.ItemID, 1, n.Price))
		}
	}
}

// workshopCraftFrame is a workshop craft's message naming the other side,
// the product and the price: single for one unit, multiple (with the
// count) for more.
func workshopCraftFrame(single, multiple int, name string, itemID int32, count, price int) wire.Frame {
	if count > 1 && multiple != 0 {
		return serverpackets.FrameSystemMessageParams(multiple, serverpackets.TextParam(name), serverpackets.NumberParam(int32(count)),
			serverpackets.ItemNameParam(itemID), serverpackets.ItemNumberParam(int32(price)))
	}
	return serverpackets.FrameSystemMessageParams(single, serverpackets.TextParam(name), serverpackets.ItemNameParam(itemID),
		serverpackets.ItemNumberParam(int32(price)))
}
