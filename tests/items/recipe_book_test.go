package items

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Recipe fixtures from gameservertest.RecipeTemplates.
const (
	commonRecipeID     = 686  // mk_lesser_healing_potion
	commonRecipeItemID = 6926 // Recipe: Lesser Healing Potion
	commonMaterialID   = 6908 // two per craft
	commonProductID    = 1060
	dwarvenRecipeID    = 1    // mk_wooden_arrow
	dwarvenRecipeItem  = 1666 // Recipe: Wooden Arrow
)

// craftSkillsPersistence knows Create Common Item and Create Item (passive
// level caps) and the two craft skills that open the recipe book.
func craftSkillsPersistence(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{ID: modelskill.CreateCommonSkillID, Level: 1, Activation: modelskill.ActivationPassive},
		{ID: modelskill.CreateDwarvenSkillID, Level: 1, Activation: modelskill.ActivationPassive},
		{
			ID: 1322, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "COMMON_CRAFT", HitTime: 500, StaticHitTime: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// craftItemTemplates adds the recipe items and materials the craft suites
// use to the shared catalog.
func craftItemTemplates() *item.Table {
	templates := append(slices.Clone(gameservertest.ItemTemplates().All()),
		&item.Template{
			ID: commonRecipeItemID, Name: "Recipe: Lesser Healing Potion", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemRecipe, Handler: "Recipes"},
		},
		&item.Template{
			ID: dwarvenRecipeItem, Name: "Recipe: Wooden Arrow", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemRecipe, Handler: "Recipes"},
		},
		&item.Template{
			ID: commonMaterialID, Name: "Gunpowder", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemMaterial},
		},
	)
	return item.NewTable(templates)
}

func bootCraft(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithSkills(craftSkillsPersistence(t)),
		gameservertest.WithItemTemplates(craftItemTemplates()),
		gameservertest.WithCharacter("Crafter", 5, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	return srv, srv.SoleObjectID(t)
}

func knowSkill(t *testing.T, srv *gameservertest.Server, objID int32, skillID modelskill.ID) {
	t.Helper()
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(skillID), 1); err != nil {
		t.Fatalf("grant skill %d: %v", skillID, err)
	}
}

func seedRecipes(t *testing.T, srv *gameservertest.Server, objID int32, ids ...int) {
	t.Helper()
	for _, id := range ids {
		if err := srv.RecipeBooks.Insert(context.Background(), objID, id); err != nil {
			t.Fatalf("seed recipe %d: %v", id, err)
		}
	}
}

func savedRecipes(t *testing.T, srv *gameservertest.Server, objID int32) []int {
	t.Helper()
	srv.FlushPersistence(t)
	ids, err := srv.RecipeBooks.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list recipes: %v", err)
	}
	return ids
}

func encodeRecipeRequest(opcode byte, value int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(value)
	return w.Bytes()
}

// textSysMsg decodes a SystemMessage carrying one text parameter and
// returns its id and text.
func textSysMsg(t *testing.T, frame []byte) (int32, string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	if n, typ := r.ReadInt32(), r.ReadInt32(); n != 1 || typ != serverpackets.SystemMessageParamText {
		t.Fatalf("SystemMessage %d params = %d of type %d, want one text", id, n, typ)
	}
	text := r.ReadString()
	if err := r.Err(); err != nil {
		t.Fatalf("decode SystemMessage %x: %v", frame, err)
	}
	return id, text
}

func assertSysMsg(t *testing.T, frame []byte, id int, params ...grantParam) {
	t.Helper()
	assertGrantMessage(t, frame, int32(id), params...)
}

// recipeBookList decodes RecipeBookItemList: its page type, max MP and
// recipe ids, requiring each position to count up from 1.
func recipeBookList(t *testing.T, frame []byte) (typ, maxMP int32, ids []int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeRecipeBookItemList, "RecipeBookItemList")
	r := wire.NewReader(frame[1:])
	typ, maxMP = r.ReadInt32(), r.ReadInt32()
	n := r.ReadInt32()
	for i := range n {
		ids = append(ids, r.ReadInt32())
		if pos := r.ReadInt32(); pos != i+1 {
			t.Fatalf("RecipeBookItemList position %d = %d", i, pos)
		}
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("RecipeBookItemList %x: err %v, %d trailing bytes", frame, err, r.Remaining())
	}
	return typ, maxMP, ids
}

// recipeMakeInfo decodes RecipeItemMakeInfo.
func recipeMakeInfo(t *testing.T, frame []byte) [5]int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeRecipeItemMakeInfo, "RecipeItemMakeInfo")
	r := wire.NewReader(frame[1:])
	var out [5]int32
	for i := range out {
		out[i] = r.ReadInt32()
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("RecipeItemMakeInfo %x: err %v, %d trailing bytes", frame, err, r.Remaining())
	}
	return out
}

// craftFrames keeps the frames a craft request answers with: system
// messages, status updates and the craft window, in order.
func craftFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range collectUntilQuiet(t, c) {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodeStatusUpdate,
			serverpackets.OpcodeRecipeItemMakeInfo, serverpackets.OpcodeRecipeBookItemList,
			serverpackets.OpcodeActionFailed, serverpackets.OpcodeShortCutDelete:
			out = append(out, f)
		}
	}
	return out
}

func requireFrames(t *testing.T, frames [][]byte, n int) {
	t.Helper()
	if len(frames) != n {
		ops := make([]byte, len(frames))
		for i, f := range frames {
			ops[i] = f[0]
		}
		t.Fatalf("got %d frames (opcodes %x), want %d", len(frames), ops, n)
	}
}

func syntheticDwarvenRecipes(ids ...int) *recipe.Table {
	rows := []recipe.Recipe{}
	for _, id := range ids {
		rows = append(rows, recipe.Recipe{
			ID: id, ItemID: int32(20000 + id), Level: 1, Dwarven: true,
			Materials: []recipe.Ingredient{{ItemID: commonMaterialID, Count: 1}}, Product: recipe.Ingredient{ItemID: 17, Count: 1},
		})
	}
	return recipe.NewTable(rows)
}

// TestRecipeBookRestoresInReferenceOrder rebuilds a saved book at login and
// opens it. The page lists recipes in the order the reference's hash map
// iterates them after the same ascending restore, which a probe of
// java.util.HashMap<Integer,…> (eclipse-temurin 21, puts in the listed
// order, then values()) printed as the want slices below: 14 recipes grow
// the table to 32 buckets, and nine recipes landing in one of 16 buckets
// double it early.
func TestRecipeBookRestoresInReferenceOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		saved []int
		want  []int32
	}{
		{
			"grown",
			[]int{1, 2, 16, 17, 18, 32, 33, 48, 100, 200, 300, 400, 500, 600},
			[]int32{32, 1, 33, 2, 100, 200, 300, 16, 48, 400, 17, 18, 500, 600},
		},
		{
			"crowded bucket",
			[]int{1, 16, 32, 48, 64, 80, 96, 112, 128, 144},
			[]int32{32, 64, 96, 128, 1, 16, 48, 80, 112, 144},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootCraft(t, gameservertest.WithRecipes(syntheticDwarvenRecipes(tc.saved...)))
			c := srv.Client
			seedRecipes(t, srv, objID, tc.saved...)
			startInWorld(t, c)

			c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookOpen, 0))
			typ, maxMP, ids := recipeBookList(t, c.Read())
			if typ != 0 || !slices.Equal(ids, tc.want) {
				t.Fatalf("dwarven page = type %d ids %v, want type 0 ids %v", typ, ids, tc.want)
			}
			if want := int32(srv.PlayerMaxMP(t, objID)); maxMP != want {
				t.Fatalf("RecipeBookItemList max MP = %d, want %d", maxMP, want)
			}

			c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookOpen, 1))
			if typ, _, ids := recipeBookList(t, c.Read()); typ != 1 || len(ids) != 0 {
				t.Fatalf("common page = type %d ids %v, want type 1 and empty", typ, ids)
			}
		})
	}
}

// TestRecipeItemRegistersRecipe uses a common recipe item: it is used up,
// S1_ADDED names it, the common page is resent with the recipe, and the row
// is saved. A second copy is refused as already registered and stays in
// the inventory.
func TestRecipeItemRegistersRecipe(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t)
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	scrolls := srv.GiveItem(t, objID, commonRecipeItemID, 2)
	startInWorld(t, c)

	c.Send(encodeUseItem(scrolls, false))
	assertSysMsg(t, c.Read(), serverpackets.SystemMessageS1Added, itemNameParam(commonRecipeItemID))
	if typ, _, ids := recipeBookList(t, c.Read()); typ != 1 || !slices.Equal(ids, []int32{commonRecipeID}) {
		t.Fatalf("common page after register = type %d ids %v", typ, ids)
	}
	barrier(t, c)
	if got := savedRecipes(t, srv, objID); !slices.Equal(got, []int{commonRecipeID}) {
		t.Fatalf("saved recipes = %v, want [%d]", got, commonRecipeID)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scrolls); inst.Count != 1 {
		t.Fatalf("recipe item count = %d, want 1", inst.Count)
	}

	c.Send(encodeUseItem(scrolls, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageRecipeAlreadyRegistered)
	barrier(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scrolls); inst.Count != 1 {
		t.Fatalf("recipe item count after refusal = %d, want 1", inst.Count)
	}
}

// TestRecipeItemRegisterRefusals pins each registration refusal and that
// none of them uses the item up or saves a row.
func TestRecipeItemRegisterRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		opts   []gameservertest.Option
		skill  modelskill.ID
		itemID int32
		check  func(t *testing.T, frame []byte)
	}{
		{"no craft ability", nil, 0, dwarvenRecipeItem, func(t *testing.T, f []byte) {
			assertStaticSystemMessage(t, f, serverpackets.SystemMessageCantRegisterNoAbilityToCraft)
		}},
		{"level too low", []gameservertest.Option{gameservertest.WithRecipes(recipe.NewTable([]recipe.Recipe{
			{ID: commonRecipeID, ItemID: commonRecipeItemID, Level: 2, Product: recipe.Ingredient{ItemID: commonProductID, Count: 1}},
		}))}, modelskill.CreateCommonSkillID, commonRecipeItemID, func(t *testing.T, f []byte) {
			assertStaticSystemMessage(t, f, serverpackets.SystemMessageCreateLvlTooLowToRegister)
		}},
		{"book full", []gameservertest.Option{gameservertest.WithStorageSlots(player.StorageSlots{
			WarehouseNoDwarf: 100, WarehouseDwarf: 120, Freight: 20, PrivateStoreNoDwarf: 4, PrivateStoreDwarf: 5, DwarfRecipe: 50, CommonRecipe: 0,
		})}, modelskill.CreateCommonSkillID, commonRecipeItemID, func(t *testing.T, f []byte) {
			assertSysMsg(t, f, serverpackets.SystemMessageUpToS1RecipesCanRegister, numberParam(0))
		}},
		{"crafting disabled", []gameservertest.Option{gameservertest.WithCraftingDisabled()}, modelskill.CreateCommonSkillID, commonRecipeItemID, func(t *testing.T, f []byte) {
			if id, text := textSysMsg(t, f); id != serverpackets.SystemMessageS1 || text != "Crafting is disabled, you cannot register this recipe." {
				t.Fatalf("disabled register = %d %q", id, text)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootCraft(t, tc.opts...)
			c := srv.Client
			if tc.skill != 0 {
				knowSkill(t, srv, objID, tc.skill)
			}
			scroll := srv.GiveItem(t, objID, tc.itemID, 1)
			startInWorld(t, c)

			c.Send(encodeUseItem(scroll, false))
			tc.check(t, c.Read())
			barrier(t, c)
			srv.FlushItems(t)
			mustFindItem(t, srv, objID, scroll)
			if got := savedRecipes(t, srv, objID); len(got) != 0 {
				t.Fatalf("saved recipes after refusal = %v", got)
			}
		})
	}
}

// TestCraftSelfMakesProduct crafts the potion recipe: MP pays the cost and
// is reported, each material's disappearance is named, the product is
// earned, and the craft window comes back with the success status. The
// inventory rows follow.
func TestCraftSelfMakesProduct(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t, gameservertest.WithCraftRoll(func(int) int { return 99 }))
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, objID, commonRecipeID)
	material := srv.GiveItem(t, objID, commonMaterialID, 3)
	startInWorld(t, c)
	mp, maxMP := srv.PlayerCurrentMP(t, objID), srv.PlayerMaxMP(t, objID)
	if mp < 30 {
		t.Fatalf("fixture MP %d below the recipe's 30", mp)
	}

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeInfo, commonRecipeID))
	if got := recipeMakeInfo(t, c.Read()); got != [5]int32{commonRecipeID, 1, int32(mp), int32(maxMP), -1} {
		t.Fatalf("RecipeItemMakeInfo before craft = %v", got)
	}

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	frames := craftFrames(t, c)
	requireFrames(t, frames, 4)
	assertFrameOpcode(t, frames[0], serverpackets.OpcodeStatusUpdate, "craft MP StatusUpdate")
	assertSysMsg(t, frames[1], serverpackets.SystemMessageS2S1Disappeared, itemNameParam(commonMaterialID), itemNumberParam(2))
	assertSysMsg(t, frames[2], serverpackets.SystemMessageEarnedItemS1, itemNameParam(commonProductID))
	if got := recipeMakeInfo(t, frames[3]); got != [5]int32{commonRecipeID, 1, int32(mp - 30), int32(maxMP), 1} {
		t.Fatalf("RecipeItemMakeInfo after craft = %v", got)
	}

	srv.InventoryUpdates.Tick()
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, material); inst.Count != 1 {
		t.Fatalf("material count = %d, want 1", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, srv, objID, commonProductID); inst.Count != 1 {
		t.Fatalf("product count = %d, want 1", inst.Count)
	}
}

// TestCraftSelfFailedRollUsesMaterials rolls at the recipe's success rate:
// the materials still go, ITEM_MIXING_FAILED is sent, no product arrives,
// and the craft window reports the failure.
func TestCraftSelfFailedRollUsesMaterials(t *testing.T) {
	t.Parallel()
	table := recipe.NewTable([]recipe.Recipe{{
		ID: commonRecipeID, ItemID: commonRecipeItemID, Level: 1, MPCost: 10, SuccessRate: 50,
		Materials: []recipe.Ingredient{{ItemID: commonMaterialID, Count: 1}},
		Product:   recipe.Ingredient{ItemID: commonProductID, Count: 3},
	}})
	srv, objID := bootCraft(t, gameservertest.WithRecipes(table), gameservertest.WithCraftRoll(func(n int) int {
		if n != 100 {
			t.Errorf("craft roll bound = %d, want 100", n)
		}
		return 50
	}))
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, objID, commonRecipeID)
	material := srv.GiveItem(t, objID, commonMaterialID, 1)
	startInWorld(t, c)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	frames := craftFrames(t, c)
	requireFrames(t, frames, 4)
	assertSysMsg(t, frames[1], serverpackets.SystemMessageS1Disappeared, itemNameParam(commonMaterialID))
	assertStaticSystemMessage(t, frames[2], serverpackets.SystemMessageItemMixingFailed)
	if got := recipeMakeInfo(t, frames[3]); got[4] != 0 {
		t.Fatalf("RecipeItemMakeInfo status = %d, want 0", got[4])
	}
	srv.InventoryUpdates.Tick()
	srv.FlushItems(t)
	assertItemGone(t, srv, objID, material)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == commonProductID {
			t.Fatalf("failed craft left a product row: %+v", inst)
		}
	}
}

// TestCraftSelfEarnsStack names a multi-unit product with EARNED_S2_S1_S,
// whose count is a plain number.
func TestCraftSelfEarnsStack(t *testing.T) {
	t.Parallel()
	table := recipe.NewTable([]recipe.Recipe{{
		ID: commonRecipeID, ItemID: commonRecipeItemID, Level: 1, MPCost: 0, SuccessRate: 100,
		Materials: []recipe.Ingredient{{ItemID: commonMaterialID, Count: 1}},
		Product:   recipe.Ingredient{ItemID: commonProductID, Count: 3},
	}})
	srv, objID := bootCraft(t, gameservertest.WithRecipes(table))
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, objID, commonRecipeID)
	srv.GiveItem(t, objID, commonMaterialID, 1)
	startInWorld(t, c)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	frames := craftFrames(t, c)
	requireFrames(t, frames, 3)
	assertSysMsg(t, frames[0], serverpackets.SystemMessageS1Disappeared, itemNameParam(commonMaterialID))
	assertSysMsg(t, frames[1], serverpackets.SystemMessageEarnedS2S1S, itemNameParam(commonProductID), numberParam(3))
	srv.InventoryUpdates.Tick()
	srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, srv, objID, commonProductID); inst.Count != 3 {
		t.Fatalf("product count = %d, want 3", inst.Count)
	}
}

// TestCraftSelfRefusals pins each refusal of a craft request and that none
// of them takes MP or materials.
func TestCraftSelfRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		opts     []gameservertest.Option
		material int32
		seed     bool
		prepare  func(t *testing.T, srv *gameservertest.Server, objID int32)
		check    func(t *testing.T, frames [][]byte)
	}{
		{name: "missing material", material: 1, seed: true, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 2)
			assertSysMsg(t, f[0], serverpackets.SystemMessageMissingS2S1ToCreate, itemNameParam(commonMaterialID), itemNumberParam(1))
			if got := recipeMakeInfo(t, f[1]); got[4] != 0 {
				t.Fatalf("status = %d, want 0", got[4])
			}
		}},
		{name: "not enough MP", material: 2, seed: true, prepare: func(t *testing.T, srv *gameservertest.Server, objID int32) {
			srv.DrainPlayerMP(t, objID, srv.PlayerCurrentMP(t, objID)-29)
		}, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 2)
			assertStaticSystemMessage(t, f[0], serverpackets.SystemMessageNotEnoughMP)
			if got := recipeMakeInfo(t, f[1]); got[4] != 0 || got[2] != 29 {
				t.Fatalf("craft window = %v, want MP 29 status 0", got)
			}
		}},
		{name: "in combat", material: 2, seed: true, prepare: func(t *testing.T, srv *gameservertest.Server, objID int32) {
			srv.SetPlayerInCombat(t, objID, true)
		}, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 1)
			assertStaticSystemMessage(t, f[0], serverpackets.SystemMessageCantOperateStoreDuringCombat)
		}},
		{name: "dead", material: 2, seed: true, prepare: func(t *testing.T, srv *gameservertest.Server, objID int32) {
			srv.MarkPlayerDead(t, objID)
		}, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 2)
			assertFrameOpcode(t, f[0], serverpackets.OpcodeActionFailed, "dead craft")
			recipeMakeInfo(t, f[1])
		}},
		{name: "not in book", material: 2, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 0)
		}},
		{name: "crafting disabled", opts: []gameservertest.Option{gameservertest.WithCraftingDisabled()}, material: 2, seed: true, check: func(t *testing.T, f [][]byte) {
			requireFrames(t, f, 2)
			if id, text := textSysMsg(t, f[0]); id != serverpackets.SystemMessageS1 || text != "Item creation is currently disabled." {
				t.Fatalf("disabled craft = %d %q", id, text)
			}
			recipeMakeInfo(t, f[1])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootCraft(t, tc.opts...)
			c := srv.Client
			knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
			if tc.seed {
				seedRecipes(t, srv, objID, commonRecipeID)
			}
			material := srv.GiveItem(t, objID, commonMaterialID, tc.material)
			startInWorld(t, c)
			if tc.prepare != nil {
				tc.prepare(t, srv, objID)
			}
			mp := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
			tc.check(t, craftFrames(t, c))
			if got := srv.PlayerCurrentMP(t, objID); got != mp {
				t.Fatalf("MP after refusal = %d, want %d", got, mp)
			}
			srv.FlushItems(t)
			if inst := mustFindItem(t, srv, objID, material); inst.Count != int(tc.material) {
				t.Fatalf("material count after refusal = %d, want %d", inst.Count, tc.material)
			}
		})
	}
}

// TestCraftSelfManufactureReuse drops a craft sent inside the manufacture
// reuse window without an answer.
func TestCraftSelfManufactureReuse(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t, gameservertest.WithManufactureDelay(time.Hour))
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	seedRecipes(t, srv, objID, commonRecipeID)
	srv.GiveItem(t, objID, commonMaterialID, 1)
	startInWorld(t, c)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	if f := craftFrames(t, c); len(f) != 2 {
		t.Fatalf("first craft answered %d frames, want MISSING + craft window", len(f))
	}
	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeItemMakeSelf, commonRecipeID))
	requireFrames(t, craftFrames(t, c), 0)
}

// TestRecipeBookDestroyRemovesRecipeAndShortcuts deletes a held recipe: its
// bar shortcut goes first, then S1_HAS_BEEN_DELETED names the recipe item
// and the page comes back without it; the recipe and shortcut rows are
// gone. An unknown recipe id is dropped silently.
func TestRecipeBookDestroyRemovesRecipeAndShortcuts(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t)
	c := srv.Client
	seedRecipes(t, srv, objID, commonRecipeID)
	if err := srv.Shortcuts.Save(context.Background(), objID, shortcut.Shortcut{Slot: 4, Page: 1, Type: shortcut.Recipe, ID: commonRecipeID, Level: -1}); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}
	startInWorld(t, c)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookDestroy, 9999))
	requireFrames(t, craftFrames(t, c), 0)

	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookDestroy, commonRecipeID))
	frames := craftFrames(t, c)
	requireFrames(t, frames, 3)
	assertFrameOpcode(t, frames[0], serverpackets.OpcodeShortCutDelete, "ShortCutDelete")
	if slot := wire.NewReader(frames[0][1:]).ReadInt32(); slot != 16 {
		t.Fatalf("ShortCutDelete slot = %d, want 16 (slot 4 page 1)", slot)
	}
	assertSysMsg(t, frames[1], serverpackets.SystemMessageS1HasBeenDeleted, itemNameParam(commonRecipeItemID))
	if typ, _, ids := recipeBookList(t, frames[2]); typ != 1 || len(ids) != 0 {
		t.Fatalf("common page after delete = type %d ids %v", typ, ids)
	}
	if got := savedRecipes(t, srv, objID); len(got) != 0 {
		t.Fatalf("saved recipes after delete = %v", got)
	}
	rows, err := srv.Shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range rows {
		if sc.Type == shortcut.Recipe {
			t.Fatalf("recipe shortcut row survived: %+v", sc)
		}
	}
}

// TestRecipeShortcutNeedsRecipe registers a recipe shortcut for a recipe
// the book does not hold: the bar still shows it, but no row is saved.
func TestRecipeShortcutNeedsRecipe(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t)
	c := srv.Client
	startInWorld(t, c)

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutReg)
	w.WriteInt32(int32(shortcut.Recipe))
	w.WriteInt32(5)
	w.WriteInt32(commonRecipeID)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeShortCutRegister, "ShortCutRegister")
	barrier(t, c)
	srv.FlushPersistence(t)
	rows, err := srv.Shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range rows {
		if sc.Type == shortcut.Recipe {
			t.Fatalf("recipe shortcut saved without the recipe: %+v", sc)
		}
	}
}

// TestCommonCraftSkillOpensBook casts Common Craft: while the cast is in
// flight the book refuses to open, and when the cast lands the common page
// of the book is sent.
func TestCommonCraftSkillOpensBook(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t)
	c := srv.Client
	knowSkill(t, srv, objID, 1322)
	seedRecipes(t, srv, objID, commonRecipeID)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(1322))
	c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookOpen, 1))
	refused := false
	for _, f := range craftFrames(t, c) {
		switch f[0] {
		case serverpackets.OpcodeRecipeBookItemList:
			t.Fatal("recipe book opened while the craft skill was casting")
		case serverpackets.OpcodeSystemMessage:
			refused = refused || systemMessageID(t, f) == serverpackets.SystemMessageNoRecipeBookWhileCasting
		}
	}
	if !refused {
		t.Fatal("RequestRecipeBookOpen mid-cast sent no NO_RECIPE_BOOK_WHILE_CASTING")
	}
	var book []byte
	srv.AdvanceUntil(t, "common craft cast lands", func() bool {
		for {
			f := c.ReadWithTimeout(50 * time.Millisecond)
			if f == nil {
				return book != nil
			}
			if f[0] == serverpackets.OpcodeRecipeBookItemList {
				book = f
			}
		}
	})
	if typ, _, ids := recipeBookList(t, book); typ != 1 || !slices.Equal(ids, []int32{commonRecipeID}) {
		t.Fatalf("craft skill page = type %d ids %v", typ, ids)
	}
}
