package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestValidAcquireRequest(t *testing.T) {
	if !ValidAcquireRequest(3, 1) {
		t.Fatal("ValidAcquireRequest(3, 1) = false")
	}
	if ValidAcquireRequest(0, 1) {
		t.Fatal("ValidAcquireRequest(0, 1) = true")
	}
	if ValidAcquireRequest(3, 0) {
		t.Fatal("ValidAcquireRequest(3, 0) = true")
	}
}

func TestLearnableGeneralRequiresNextLevelAndCost(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 100},
		{SkillID: 4, Level: 1, MinLevel: 5, Cost: 0},
	}}

	grant, ok := LearnableGeneral(tmpl, 5, player.SkillLevels{}, 3, 1)
	if !ok {
		t.Fatal("LearnableGeneral returned ok=false")
	}
	if grant.SkillID != 3 || grant.Level != 1 {
		t.Fatalf("grant = %+v, want skill 3 level 1", grant)
	}
	if _, ok := LearnableGeneral(tmpl, 4, player.SkillLevels{}, 4, 1); ok {
		t.Fatal("zero-cost skill was learnable")
	}
	if _, ok := LearnableGeneral(tmpl, 5, player.SkillLevels{3: 1}, 3, 1); ok {
		t.Fatal("already-known skill level was learnable")
	}
}

func TestGeneralOfferForRequiresLoadedDefinitionAndIncludesBook(t *testing.T) {
	ch, tmpl := testLearner(5, 50)
	books := testBookPolicy(t)

	if _, ok := GeneralOfferFor(ch, tmpl, NewPersistence(nil, modelskill.NewTable(nil)), books, 3, 1); ok {
		t.Fatal("GeneralOfferFor() with missing definition returned ok=true")
	}

	offer, ok := GeneralOfferFor(ch, tmpl, testSkillPersistence(3, 1), books, 3, 1)
	if !ok {
		t.Fatal("GeneralOfferFor() returned ok=false")
	}
	if offer.Grant.SkillID != 3 || offer.Grant.Level != 1 || offer.BookID != item.AdenaID {
		t.Fatalf("offer = %+v, want skill 3/1 with book %d", offer, item.AdenaID)
	}
}

func TestLearnGeneralConsumesBookRecordsSkillAndRemovesSP(t *testing.T) {
	ch, tmpl := testLearner(5, 50)
	ch.AttachRuntime(tmpl, testInventory(ch.ID, item.AdenaID, 1))

	result, status, err := LearnGeneral(context.Background(), ch, tmpl, testSkillPersistence(3, 1), testBookPolicy(t), 3, 1)
	if err != nil {
		t.Fatalf("LearnGeneral() error: %v", err)
	}
	if status != LearnDone {
		t.Fatalf("LearnGeneral() status = %v, want LearnDone", status)
	}
	if result.SkillID != 3 || result.Level != 1 || result.Cost != 50 {
		t.Fatalf("result = %+v, want skill 3/1 cost 50", result)
	}
	if ch.SkillLevel(3) != 1 || ch.SP != 0 {
		t.Fatalf("character skill/SP = %d/%d, want 1/0", ch.SkillLevel(3), ch.SP)
	}
	if got := ch.Inventory().ItemCount(item.AdenaID, -1, true); got != 0 {
		t.Fatalf("book count = %d, want 0", got)
	}
}

func TestLearnGeneralReportsNeedsSPBeforeConsumingBook(t *testing.T) {
	ch, tmpl := testLearner(5, 49)
	ch.AttachRuntime(tmpl, testInventory(ch.ID, item.AdenaID, 1))

	result, status, err := LearnGeneral(context.Background(), ch, tmpl, testSkillPersistence(3, 1), testBookPolicy(t), 3, 1)
	if err != nil {
		t.Fatalf("LearnGeneral() error: %v", err)
	}
	if status != LearnNeedsSP || result.Cost != 50 {
		t.Fatalf("LearnGeneral() = result %+v status %v, want cost 50 needs SP", result, status)
	}
	if ch.SkillLevel(3) != 0 || ch.SP != 49 {
		t.Fatalf("character skill/SP = %d/%d, want 0/49", ch.SkillLevel(3), ch.SP)
	}
	if got := ch.Inventory().ItemCount(item.AdenaID, -1, true); got != 1 {
		t.Fatalf("book count = %d, want 1", got)
	}
}

func TestLearnableFishingRequiresLoadedDefinition(t *testing.T) {
	trees := &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1},
	}}
	loaded := func(skillID, level int) bool { return skillID == 1368 && level == 1 }

	node, ok := LearnableFishing(trees, 1, false, player.SkillLevels{}, loaded, 1368, 1)
	if !ok {
		t.Fatal("LearnableFishing returned ok=false")
	}
	if node.ID != 1368 || node.Level != 1 {
		t.Fatalf("node = %+v, want skill 1368 level 1", node)
	}
	if _, ok := LearnableFishing(trees, 1, false, player.SkillLevels{}, nil, 1368, 1); ok {
		t.Fatal("missing definition lookup was learnable")
	}
}

func TestLearnFishingConsumesItemAndReportsStorageSync(t *testing.T) {
	ch, tmpl := testLearner(5, 0)
	ch.AttachRuntime(tmpl, testInventory(ch.ID, item.AdenaID, 2))
	trees := &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 5, ItemID: item.AdenaID, ItemCount: 2},
	}}

	result, status, err := LearnFishing(context.Background(), ch, trees, testSkillPersistence(1368, 1), 1368, 1)
	if err != nil {
		t.Fatalf("LearnFishing() error: %v", err)
	}
	if status != LearnDone {
		t.Fatalf("LearnFishing() status = %v, want LearnDone", status)
	}
	if result.SkillID != 1368 || result.Level != 1 || !result.StorageSync {
		t.Fatalf("result = %+v, want skill 1368/1 with storage sync", result)
	}
	if ch.SkillLevel(1368) != 1 {
		t.Fatalf("SkillLevel(1368) = %d, want 1", ch.SkillLevel(1368))
	}
	if got := ch.Inventory().ItemCount(item.AdenaID, -1, true); got != 0 {
		t.Fatalf("item count = %d, want 0", got)
	}
}

func TestNeedsStorageSync(t *testing.T) {
	for _, id := range []int32{1368, 1372} {
		if !NeedsStorageSync(id) {
			t.Fatalf("NeedsStorageSync(%d) = false", id)
		}
	}
	for _, id := range []int32{1367, 1373} {
		if NeedsStorageSync(id) {
			t.Fatalf("NeedsStorageSync(%d) = true", id)
		}
	}
}

func testLearner(level, sp int) (*player.Character, *player.Template) {
	tmpl := &player.Template{Skills: []player.SkillGrant{{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50}}}
	ch := &player.Character{ID: 1, Level: level, SP: sp}
	return ch, tmpl
}

func testSkillPersistence(skillID, level int) *Persistence {
	return NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{ID: modelskill.ID(skillID), Level: level}}))
}

func testInventory(ownerID, itemID int32, count int) *itemcontainer.Inventory {
	templates := item.NewTable([]*item.Template{
		{ID: itemID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(ownerID, templates)
	inv.AddNew(itemID, count, 100)
	return inv
}

func testBookPolicy(t *testing.T) modelskill.BookPolicy {
	t.Helper()
	table, err := modelskill.NewSpellbookTable([]modelskill.Spellbook{{SkillID: 3, ItemID: item.AdenaID}})
	if err != nil {
		t.Fatalf("NewSpellbookTable() error: %v", err)
	}
	return modelskill.BookPolicy{Table: table, SPBookNeeded: true, DivineBookNeeded: true}
}
