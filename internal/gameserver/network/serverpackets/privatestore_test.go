package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// TestFrameRecipeBookPackets pins RecipeBookItemList (0xd6: page type with 0
// dwarven and 1 common, max MP, count, then id and 1-based position per
// recipe) and RecipeItemMakeInfo (0xd7: recipe id, page type, MP, max MP,
// status).
func TestFrameRecipeBookPackets(t *testing.T) {
	got := framePayload(t, FrameRecipeBookItemList(true, 150, []recipe.Recipe{{ID: 32}, {ID: 1}}))
	want := []byte{
		0xd6,
		0x00, 0x00, 0x00, 0x00,
		0x96, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x20, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRecipeBookItemList() = %x, want %x", got, want)
	}
	if got := framePayload(t, FrameRecipeBookItemList(false, 0, nil)); !bytes.Equal(got, []byte{0xd6, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("empty common FrameRecipeBookItemList() = %x", got)
	}

	got = framePayload(t, FrameRecipeItemMakeInfo(recipe.Recipe{ID: 686}, 12, 150, -1))
	want = []byte{
		0xd7,
		0xae, 0x02, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x0c, 0x00, 0x00, 0x00,
		0x96, 0x00, 0x00, 0x00,
		0xff, 0xff, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRecipeItemMakeInfo() = %x, want %x", got, want)
	}
}

// storePacketTemplates is a one-handed sword (weapon, type2 0, right hand
// slot 0x80, reference price 1000) and a stackable potion (etc item, type2
// 5, no slot, reference price 40).
func storePacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, ReferencePrice: 1000},
		{ID: 1060, Kind: item.KindEtcItem, Stackable: true, ReferencePrice: 40},
	})
}

// TestFramePrivateStoreSellPackets pins the sell store packets against the
// field order the reference writes: PrivateStoreManageListSell (0x9a: owner,
// package flag, adena, then each listable item as type2 D, object, item,
// count, H 0, H enchant, H 0, body part D, reference price D, then each
// listed row the same plus its price before the reference price) and
// PrivateStoreListSell (0x9b: owner, package flag, buyer adena, then each
// listed row as type2, object, item, count, H 0, H enchant, H 0, body part,
// price, reference price).
func TestFramePrivateStoreSellPackets(t *testing.T) {
	templates := storePacketTemplates()
	candidates := []privatestore.SellCandidate{{Item: item.InstanceState{ObjectID: 500, TemplateID: 1060, Count: 7}, Count: 4}}
	listed := []privatestore.SellItem{{ObjectID: 501, TemplateID: 1, Enchant: 3, Count: 1, Quantity: 1, Price: 9000}}
	frame, err := FramePrivateStoreManageListSell(100, true, 1234, candidates, listed, templates)
	if err != nil {
		t.Fatalf("FramePrivateStoreManageListSell: %v", err)
	}
	want := []byte{0x9a}
	want = appendD(want, 100)
	want = appendD(want, 1)
	want = appendD(want, 1234)
	want = appendD(want, 1)
	want = appendD(want, 5)
	want = appendD(want, 500)
	want = appendD(want, 1060)
	want = appendD(want, 4)
	want = appendH(want, 0)
	want = appendH(want, 0)
	want = appendH(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 40)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 501)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendH(want, 0)
	want = appendH(want, 3)
	want = appendH(want, 0)
	want = appendD(want, 0x80)
	want = appendD(want, 9000)
	want = appendD(want, 1000)
	if got := framePayload(t, frame); !bytes.Equal(got, want) {
		t.Fatalf("FramePrivateStoreManageListSell() = %x, want %x", got, want)
	}

	frame, err = FramePrivateStoreListSell(100, false, 77, listed, templates)
	if err != nil {
		t.Fatalf("FramePrivateStoreListSell: %v", err)
	}
	want = []byte{0x9b}
	want = appendD(want, 100)
	want = appendD(want, 0)
	want = appendD(want, 77)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 501)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendH(want, 0)
	want = appendH(want, 3)
	want = appendH(want, 0)
	want = appendD(want, 0x80)
	want = appendD(want, 9000)
	want = appendD(want, 1000)
	if got := framePayload(t, frame); !bytes.Equal(got, want) {
		t.Fatalf("FramePrivateStoreListSell() = %x, want %x", got, want)
	}

	if _, err := FramePrivateStoreListSell(100, false, 0, []privatestore.SellItem{{TemplateID: 999}}, templates); err == nil {
		t.Fatal("FramePrivateStoreListSell with an unknown template built a frame")
	}
}

// TestFramePrivateStoreBuyPackets pins PrivateStoreManageListBuy (0xb7:
// owner, adena, then each wantable item as item D, H enchant, count D,
// reference price D, H 0, body part D, H type2, then each wanted row as item,
// H enchant, quantity, reference price, H 0, body part, H type2, price,
// reference price) and PrivateStoreListBuy (0xb8: owner, seller adena, then
// each row as the seller's object, item, H enchant, count, reference price,
// H 0, body part, H type2, price, quantity).
func TestFramePrivateStoreBuyPackets(t *testing.T) {
	templates := storePacketTemplates()
	candidates := []item.InstanceState{{ObjectID: 600, TemplateID: 1, Count: 1, EnchantLevel: 2}}
	listed := []privatestore.BuyItem{{TemplateID: 1060, Quantity: 20, Price: 35}}
	frame, err := FramePrivateStoreManageListBuy(100, 5000, candidates, listed, templates)
	if err != nil {
		t.Fatalf("FramePrivateStoreManageListBuy: %v", err)
	}
	want := []byte{0xb7}
	want = appendD(want, 100)
	want = appendD(want, 5000)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendH(want, 2)
	want = appendD(want, 1)
	want = appendD(want, 1000)
	want = appendH(want, 0)
	want = appendD(want, 0x80)
	want = appendH(want, 0)
	want = appendD(want, 1)
	want = appendD(want, 1060)
	want = appendH(want, 0)
	want = appendD(want, 20)
	want = appendD(want, 40)
	want = appendH(want, 0)
	want = appendD(want, 0)
	want = appendH(want, 5)
	want = appendD(want, 35)
	want = appendD(want, 40)
	if got := framePayload(t, frame); !bytes.Equal(got, want) {
		t.Fatalf("FramePrivateStoreManageListBuy() = %x, want %x", got, want)
	}

	offers := []privatestore.BuyOffer{{BuyItem: listed[0], ObjectID: 700, Count: 6}}
	frame, err = FramePrivateStoreListBuy(100, 88, offers, templates)
	if err != nil {
		t.Fatalf("FramePrivateStoreListBuy: %v", err)
	}
	want = []byte{0xb8}
	want = appendD(want, 100)
	want = appendD(want, 88)
	want = appendD(want, 1)
	want = appendD(want, 700)
	want = appendD(want, 1060)
	want = appendH(want, 0)
	want = appendD(want, 6)
	want = appendD(want, 40)
	want = appendH(want, 0)
	want = appendD(want, 0)
	want = appendH(want, 5)
	want = appendD(want, 35)
	want = appendD(want, 20)
	if got := framePayload(t, frame); !bytes.Equal(got, want) {
		t.Fatalf("FramePrivateStoreListBuy() = %x, want %x", got, want)
	}
}

// TestFramePrivateStoreTitles pins the three title packets: object id then
// the null-terminated UTF-16 text, under 0x9c (sell), 0xb9 (buy) and 0xdb
// (workshop).
func TestFramePrivateStoreTitles(t *testing.T) {
	for _, tc := range []struct {
		frame  wire.Frame
		opcode byte
	}{
		{FramePrivateStoreMsgSell(100, "ab"), 0x9c},
		{FramePrivateStoreMsgBuy(100, "ab"), 0xb9},
		{FrameRecipeShopMsg(100, "ab"), 0xdb},
	} {
		want := []byte{tc.opcode, 0x64, 0, 0, 0, 'a', 0, 'b', 0, 0, 0}
		if got := framePayload(t, tc.frame); !bytes.Equal(got, want) {
			t.Fatalf("title %#x = %x, want %x", tc.opcode, got, want)
		}
	}
}

// TestFrameRecipeShopPackets pins RecipeShopManageList (0xd8: owner, adena,
// page with 0 dwarven and 1 common, book recipes as id and 1-based position,
// then offered recipes as id, 0, cost), RecipeShopSellList (0xd9: crafter,
// MP, max MP, customer adena, then offered recipes as id, 0, cost) and
// RecipeShopItemInfo (0xda: crafter, recipe, MP, max MP, -1).
func TestFrameRecipeShopPackets(t *testing.T) {
	offered := []privatestore.ManufactureItem{{RecipeID: 1, Cost: 300, Dwarven: true}}
	got := framePayload(t, FrameRecipeShopManageList(100, 250, true, []recipe.Recipe{{ID: 1}, {ID: 2}}, offered))
	want := []byte{0xd8}
	want = appendD(want, 100)
	want = appendD(want, 250)
	want = appendD(want, 0)
	want = appendD(want, 2)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 2)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 300)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRecipeShopManageList() = %x, want %x", got, want)
	}
	got = framePayload(t, FrameRecipeShopManageList(100, 0, false, nil, nil))
	if want := []byte{0xd8, 0x64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("empty FrameRecipeShopManageList() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameRecipeShopSellList(100, 40, 90, 1500, offered))
	want = []byte{0xd9}
	want = appendD(want, 100)
	want = appendD(want, 40)
	want = appendD(want, 90)
	want = appendD(want, 1500)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 300)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRecipeShopSellList() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameRecipeShopItemInfo(100, 686, 40, 90))
	want = []byte{0xda, 0x64, 0, 0, 0, 0xae, 0x02, 0, 0, 40, 0, 0, 0, 90, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRecipeShopItemInfo() = %x, want %x", got, want)
	}
}
