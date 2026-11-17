package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const wyvernListID = 7

// wyvernList is the buylist of the paged wyvern manager.
func wyvernList() buylist.List {
	return buylist.List{ID: wyvernListID, NPCID: wyvernPagedID, Products: []buylist.Product{
		{BuyListID: wyvernListID, ItemID: shopPotionID, Price: 50, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: wyvernListID, ItemID: shopTunicID, Price: 300, MaxCount: -1, RestockDelayMillis: -60000},
	}}
}

// bootWyvernShop boots a shop world with a wyvern manager beside the player,
// selected, carrying wyvernList.
func bootWyvernShop(t *testing.T, adena int32) (*shopWorld, *npc.Folk) {
	t.Helper()
	cfg := merchant.Config{SiegeGuardsPriceRate: 1, AllowWear: true, WearDelay: time.Second, WearPrice: 10}
	w := bootShop(t, adena,
		gameservertest.WithBuyLists(wyvernList()),
		gameservertest.WithMerchantConfig(cfg),
		gameservertest.WithHTMLPages(map[string]string{"merchant/35420-bought.htm": boughtPage}),
	)
	wyvern := w.spawnFolk(t, folkTemplate("WyvernManagerNpc", wyvernPagedID), 50)
	w.selectFolk(t, wyvern)
	return w, wyvern
}

// TestBuyItemFromWyvernManager pins a wyvern manager as a merchant-type
// seller: RequestBuyItem against its own list pays, grants, and answers its
// -bought page from merchant/; a list that belongs to another NPC is refused
// without a change.
func TestBuyItemFromWyvernManager(t *testing.T) {
	t.Parallel()
	w, wyvern := bootWyvernShop(t, 1000)

	frames := w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1}))
	if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != 1000 || potions != 0 {
		t.Fatalf("after buying from another NPC's list (%x): adena %d potions %d, want 1000/0", opcodes(frames), adena, potions)
	}

	frames = w.send(t, encodeRequestBuyItem(wyvernListID, buyRow{shopPotionID, 3}))
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("purchase = %x, want the -bought page", opcodes(frames))
	}
	if objectID, html, _ := htmlMessage(t, page); objectID != wyvern.ObjectID() || html != wantChatPage(boughtPage, wyvern) {
		t.Fatalf("-bought page = %d %q", objectID, html)
	}
	if got := opcodes(frames); got[len(got)-1] != serverpackets.OpcodeItemList {
		t.Fatalf("purchase = %x, want it to end with the ItemList", got)
	}
	if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != 850 || potions != 3 {
		t.Fatalf("after buying 3 potions: adena %d potions %d, want 850/3", adena, potions)
	}
}

// TestTryOnAtWyvernManager pins RequestPreviewItem against a wyvern manager:
// the try-on is charged WearPrice, then shows the item worn.
func TestTryOnAtWyvernManager(t *testing.T) {
	t.Parallel()
	w, _ := bootWyvernShop(t, 25)

	frames := w.send(t, encodeRequestPreviewItem(wyvernListID, shopTunicID))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeShopPreviewInfo}) {
		t.Fatalf("try-on = %x, want SystemMessage then ShopPreviewInfo", got)
	}
	r := wire.NewReader(frames[1][1:])
	if total := r.ReadInt32(); total != 17 {
		t.Fatalf("ShopPreviewInfo slots = %d, want 17", total)
	}
	var shown [17]int32
	for i := range shown {
		shown[i] = r.ReadInt32()
	}
	if want := [17]int32{9: shopTunicID}; shown != want {
		t.Fatalf("ShopPreviewInfo = %v, want %v", shown, want)
	}
	if adena := w.countOf(t, item.AdenaID); adena != 15 {
		t.Fatalf("adena after the try-on = %d, want 15", adena)
	}
}
