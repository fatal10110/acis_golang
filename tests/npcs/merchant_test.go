package npcs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Merchant fixture items.
const (
	shopPotionID int32 = 1060 // stackable, weight 5
	shopSwordID  int32 = 2369 // D grade one-hander, weight 1000
	shopTunicID  int32 = 1104 // D grade chest piece
	shirtID      int32 = 1101 // no-grade chest piece
	anvilID      int32 = 1900 // stackable, too heavy to carry
	ticketID     int32 = 3960 // a siege guard ticket
	arrowID      int32 = 17   // stackable, three in stock
	cGradeAxeID  int32 = 160  // C grade weapon, above a no-expertise player
	otherListID        = 2
	shopListID         = 1
)

func merchantItems() *item.Table {
	weapon := func(id int32, crystal item.CrystalType) *item.Template {
		return &item.Template{
			ID: id, Name: "weapon", Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Crystal: crystal, Weight: 100,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, Weapon: &item.WeaponDetail{Type: item.WeaponSword},
		}
	}
	chest := func(id int32, crystal item.CrystalType) *item.Template {
		return &item.Template{
			ID: id, Name: "chest", Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1, Crystal: crystal, Weight: 50,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, Armor: &item.ArmorDetail{Type: item.ArmorLight},
		}
	}
	etc := func(id int32, weight int32) *item.Template {
		return &item.Template{
			ID: id, Name: "etc", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Weight: weight,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	return item.NewTable([]*item.Template{
		etc(item.AdenaID, 0), etc(shopPotionID, 5), etc(anvilID, 1_000_000), etc(ticketID, 0), etc(arrowID, 1),
		weapon(shopSwordID, item.CrystalD), weapon(cGradeAxeID, item.CrystalC),
		chest(shopTunicID, item.CrystalD), chest(shirtID, item.CrystalNone),
	})
}

// shopList is the merchant's buylist: potions without a stock limit, two
// swords restocking an hour after a sale, the C grade axe, two chest
// pieces, the heavy anvil, a siege guard ticket and three arrows.
func shopList() buylist.List {
	return buylist.List{ID: shopListID, NPCID: merchantID, Products: []buylist.Product{
		{BuyListID: shopListID, ItemID: shopPotionID, Price: 50, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shopSwordID, Price: 1000, MaxCount: 2, RestockDelayMillis: 3600000},
		{BuyListID: shopListID, ItemID: cGradeAxeID, Price: 5000, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shopTunicID, Price: 300, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shirtID, Price: 100, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: anvilID, Price: 1, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: ticketID, Price: 100, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: arrowID, Price: 1, MaxCount: 3, RestockDelayMillis: 3600000},
	}}
}

// otherList belongs to another NPC.
func otherList() buylist.List {
	return buylist.List{ID: otherListID, NPCID: merchantID + 1, Products: []buylist.Product{
		{BuyListID: otherListID, ItemID: shopPotionID, Price: 10, MaxCount: -1},
	}}
}

const boughtPage = `<html><body>Thanks %objectId%<a action="bypass -h npc_%objectId%_Chat 0">Back</a></body></html>`

// shopClock is a settable clock for the stock's restock times.
type shopClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *shopClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *shopClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// shopWorld is a folkWorld standing by a merchant selling shopList.
type shopWorld struct {
	*folkWorld
	merchant *npc.Folk
	clock    *shopClock
}

// bootShop boots with shopList and otherList, gives the character adena,
// enters the world beside a merchant and selects it.
func bootShop(t *testing.T, adena int32, extra ...gameservertest.Option) *shopWorld {
	t.Helper()
	clock := &shopClock{now: time.Unix(1_800_000_000, 0)}
	pages := map[string]string{
		"merchant/30001.htm":        `<html><body><a action="bypass -h npc_%objectId%_Buy 1">Buy</a><a action="bypass -h npc_%objectId%_Wear 1">Wear</a><a action="bypass -h npc_%objectId%_Buy 2">Other</a><a action="bypass -h npc_%objectId%_Buy 99">Missing</a><a action="bypass -h npc_%objectId%_Buy x">Bad</a><a action="bypass -h npc_%objectId%_Buy">Bare</a></body></html>`,
		"merchant/30001-bought.htm": boughtPage,
	}
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Buyer", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(merchantItems()),
		gameservertest.WithBuyLists(shopList(), otherList()),
		gameservertest.WithBuyListClock(clock.Now),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, f)
	return &shopWorld{folkWorld: w, merchant: f, clock: clock}
}

// buyRow is one requested item and count.
type buyRow struct{ itemID, count int32 }

func encodeRequestBuyItem(listID int32, rows ...buyRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBuyItem)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.itemID)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

func encodeRequestPreviewItem(listID int32, itemIDs ...int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPreviewItem)
	w.WriteInt32(0)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(itemIDs)))
	for _, id := range itemIDs {
		w.WriteInt32(id)
	}
	return w.Bytes()
}

func (w *shopWorld) send(t *testing.T, payload []byte) [][]byte {
	t.Helper()
	w.c.Send(payload)
	return drainFrames(t, w.c)
}

// openBuy clicks Buy <listID> on the merchant's first page.
func (w *shopWorld) openBuy(t *testing.T, listID string) [][]byte {
	t.Helper()
	w.talkTo(t, w.merchant)
	return w.bypass(t, npcCommand(w.merchant, "Buy "+listID))
}

// buyListRow is one decoded BuyList row.
type buyListRow struct {
	itemID, count, price int32
	slot                 int32
}

// decodeBuyList reads a BuyList frame: the adena shown, the list id, the
// header count and the rows.
func decodeBuyList(t *testing.T, frame []byte) (money, listID int32, header uint16, rows []buyListRow) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeBuyList {
		t.Fatalf("opcode = %#x, want BuyList", frame[0])
	}
	r := wire.NewReader(frame[1:])
	money, listID, header = r.ReadInt32(), r.ReadInt32(), r.ReadUint16()
	for r.Remaining() > 0 {
		r.ReadUint16() // type1
		objectID, itemID, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if objectID != itemID {
			t.Fatalf("BuyList row object id %d != item id %d", objectID, itemID)
		}
		r.ReadUint16() // type2
		r.ReadUint16()
		slot := r.ReadInt32()
		r.ReadUint16()
		r.ReadUint16()
		r.ReadUint16()
		rows = append(rows, buyListRow{itemID: itemID, count: count, price: r.ReadInt32(), slot: slot})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode BuyList: %v", err)
	}
	return money, listID, header, rows
}

func systemMessageID(frame []byte) int32 {
	return wire.NewReader(frame[1:]).ReadInt32()
}

// assertSystemMessages checks frames are exactly the system messages ids.
func assertSystemMessages(t *testing.T, frames [][]byte, ids ...int32) {
	t.Helper()
	if len(frames) != len(ids) {
		t.Fatalf("answer = %x, want %d system messages %v", opcodes(frames), len(ids), ids)
	}
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage || systemMessageID(f) != ids[i] {
			t.Fatalf("frame %d = %x, want SystemMessage %d", i, f, ids[i])
		}
	}
}

func (w *shopWorld) countOf(t *testing.T, itemID int32) int {
	t.Helper()
	inv := w.srv.PlayerInventory(t, w.player)
	n := 0
	for _, it := range inv.Items() {
		if st := it.Snapshot(); st.TemplateID == itemID {
			n += st.Count
		}
	}
	return n
}

// TestMerchantBuyOpensTheBuyWindow pins Buy <list>: the window shows the
// player's adena and the list, each limited product with its current
// count, and the client is released after it. A list of another NPC, an
// unknown list or a bare Buy release the client alone; a list id that does
// not parse answers nothing.
func TestMerchantBuyOpensTheBuyWindow(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)

	frames := w.openBuy(t, "1")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeBuyList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Buy 1 = %x, want BuyList then ActionFailed", got)
	}
	money, listID, header, rows := decodeBuyList(t, frames[0])
	if money != 10000 || listID != shopListID || header != 8 || len(rows) != 8 {
		t.Fatalf("BuyList = money %d list %d header %d rows %d, want 10000/1/8/8", money, listID, header, len(rows))
	}
	want := []buyListRow{
		{shopPotionID, 0, 50, 0},
		{shopSwordID, 2, 1000, int32(item.SlotRHand)},
		{cGradeAxeID, 0, 5000, int32(item.SlotRHand)},
		{shopTunicID, 0, 300, int32(item.SlotChest)},
		{shirtID, 0, 100, int32(item.SlotChest)},
		{anvilID, 0, 1, 0},
		{ticketID, 0, 100, 0},
		{arrowID, 3, 1, 0},
	}
	for i, row := range rows {
		if row != want[i] {
			t.Fatalf("BuyList row %d = %+v, want %+v", i, row, want[i])
		}
	}

	for _, tc := range []struct {
		command string
		want    []byte
	}{
		{"Buy 2", releaseOnly},
		{"Buy 99", releaseOnly},
		{"Buy", releaseOnly},
		{"Buy x", nil},
	} {
		w.talkTo(t, w.merchant)
		if got := opcodes(w.bypass(t, npcCommand(w.merchant, tc.command))); string(got) != string(tc.want) {
			t.Fatalf("%s = %x, want %x", tc.command, got, tc.want)
		}
	}
}

// TestMerchantWindowHoldsTheItemList pins the 1.5 s window a shop window
// opens: an item list request inside it is not answered at all, and one
// after it is. A refused Buy opens no window.
func TestMerchantWindowHoldsTheItemList(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	itemList := wire.NewPacketWriter(clientpackets.OpcodeRequestItemList).Bytes()

	w.openBuy(t, "2")
	if got := opcodes(w.send(t, itemList)); len(got) == 0 || got[len(got)-1] != serverpackets.OpcodeItemList {
		t.Fatalf("item list after a refused Buy = %x, want an ItemList", got)
	}

	w.openBuy(t, "1")
	if got := w.send(t, itemList); len(got) != 0 {
		t.Fatalf("item list inside the window = %x, want nothing", opcodes(got))
	}
	w.srv.Advance(t, 1500*time.Millisecond)
	if got := opcodes(w.send(t, itemList)); len(got) == 0 || got[len(got)-1] != serverpackets.OpcodeItemList {
		t.Fatalf("item list after the window = %x, want an ItemList", got)
	}
}

// TestMerchantBuyPaysAndGrants pins a completed purchase: the adena is
// taken, the items are created and saved, the merchant's -bought page
// opens, then the full item list. The sale of a limited product saves its
// row with the count left and the restock time.
func TestMerchantBuyPaysAndGrants(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	w.openBuy(t, "1")

	frames := w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 3}, buyRow{shopSwordID, 1}, buyRow{ticketID, 2}))
	html, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("purchase = %x, want the -bought page", opcodes(frames))
	}
	if objectID, got, _ := htmlMessage(t, html); objectID != w.merchant.ObjectID() || got != wantChatPage(boughtPage, w.merchant) {
		t.Fatalf("-bought page = %d %q", objectID, got)
	}
	if got := opcodes(frames); got[len(got)-1] != serverpackets.OpcodeItemList {
		t.Fatalf("purchase = %x, want it to end with the ItemList", got)
	}
	// 3 * 50 + 1000 + 2 * 100 (siege guard rate 1).
	if adena, potions, swords, tickets := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID), w.countOf(t, shopSwordID), w.countOf(t, ticketID); adena != 10000-1350 || potions != 3 || swords != 1 || tickets != 2 {
		t.Fatalf("after the purchase: adena %d potions %d swords %d tickets %d, want 8650/3/1/2", adena, potions, swords, tickets)
	}

	w.srv.FlushItems(t)
	w.srv.FlushPersistence(t)
	rows, err := w.srv.BuyListRows.LoadStock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := merchant.StockRow{ListID: shopListID, ItemID: shopSwordID, Count: 1, NextRestock: w.clock.Now().Add(time.Hour)}
	if len(rows) != 1 || !rows[0].NextRestock.Equal(want.NextRestock) || rows[0].Count != 1 || rows[0].ListID != shopListID || rows[0].ItemID != shopSwordID {
		t.Fatalf("buylists rows = %+v, want [%+v]", rows, want)
	}
	items, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatal(err)
	}
	saved := map[int32]int{}
	for _, it := range items {
		saved[it.TemplateID] += it.Count
	}
	if saved[item.AdenaID] != 8650 || saved[shopPotionID] != 3 || saved[shopSwordID] != 1 || saved[ticketID] != 2 {
		t.Fatalf("saved items = %v", saved)
	}
}

// TestMerchantBuyRefusals pins the refused purchases: a non-stackable item
// asked for more than once, a load over the weight limit, too few free
// slots and too little adena each answer their system message; an item
// off the list, a count over the stock, the list of another NPC, a
// purchase out of reach and one with nothing targeted answer nothing, as
// in the reference. Nothing is taken.
func TestMerchantBuyRefusals(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 1200)
	w.openBuy(t, "1")

	assertSystemMessages(t, w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 2})), serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted)
	assertSystemMessages(t, w.send(t, encodeRequestBuyItem(shopListID, buyRow{anvilID, 1})), serverpackets.SystemMessageWeightLimitExceeded)
	assertSystemMessages(t, w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}, buyRow{shopPotionID, 5})), serverpackets.SystemMessageYouNotEnoughAdena)
	for _, req := range [][]byte{
		encodeRequestBuyItem(shopListID, buyRow{arrowID, 4}),
		encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1}, buyRow{shirtID + 1, 1}),
		encodeRequestBuyItem(otherListID, buyRow{shopPotionID, 1}),
		encodeRequestBuyItem(99, buyRow{shopPotionID, 1}),
	} {
		if got := w.send(t, req); len(got) != 0 {
			t.Fatalf("refused purchase = %x, want nothing", opcodes(got))
		}
	}

	w.srv.SetInventorySlotLimit(t, w.player, 2)
	assertSystemMessages(t, w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1}, buyRow{shirtID, 1})), serverpackets.SystemMessageSlotsFull)
	w.srv.SetInventorySlotLimit(t, w.player, 80)

	// Out of reach: the merchant steps away from the player.
	far := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 1000)
	w.selectFolk(t, far)
	if got := w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1})); len(got) != 0 {
		t.Fatalf("purchase out of reach = %x, want nothing", opcodes(got))
	}
	if adena, potions, swords := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID), w.countOf(t, shopSwordID); adena != 1200 || potions != 0 || swords != 0 {
		t.Fatalf("after the refusals: adena %d potions %d swords %d, want 1200/0/0", adena, potions, swords)
	}
}

// TestMerchantStockSellsOutAndRestocks pins a limited product: selling the
// last one leaves it out of the window, and the restock puts it back to
// its full count and deletes its row. A count past the stock already paid
// for creates nothing.
func TestMerchantStockSellsOutAndRestocks(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	w.openBuy(t, "1")
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}, buyRow{shopSwordID, 1}))
	if swords := w.countOf(t, shopSwordID); swords != 2 {
		t.Fatalf("swords = %d, want 2", swords)
	}

	_, _, header, rows := decodeBuyList(t, w.openBuy(t, "1")[0])
	for _, row := range rows {
		if row.itemID == shopSwordID {
			t.Fatalf("sold-out sword still shown: %+v", row)
		}
	}
	if header != 8 || len(rows) != 7 {
		t.Fatalf("BuyList header %d rows %d, want 8 and 7", header, len(rows))
	}

	w.clock.advance(time.Hour - time.Second)
	w.srv.BuyListStock.Restock()
	if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); len(rows) != 7 {
		t.Fatalf("rows before the restock = %d, want 7", len(rows))
	}
	w.clock.advance(time.Second)
	w.srv.BuyListStock.Restock()
	_, _, _, rows = decodeBuyList(t, w.openBuy(t, "1")[0])
	if len(rows) != 8 || rows[1].itemID != shopSwordID || rows[1].count != 2 {
		t.Fatalf("rows after the restock = %+v, want the sword back at 2", rows)
	}
	w.srv.FlushPersistence(t)
	if saved, err := w.srv.BuyListRows.LoadStock(context.Background()); err != nil || len(saved) != 0 {
		t.Fatalf("buylists rows after the restock = %+v, %v; want none", saved, err)
	}
}

// TestMerchantStockRestoresSavedRows pins the boot restore: a row whose
// restock is ahead sets its product's count, and one whose restock has
// passed is deleted, leaving its product at its full count.
func TestMerchantStockRestoresSavedRows(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_800_000_000, 0)
	t.Run("restock ahead", func(t *testing.T) {
		t.Parallel()
		w := bootShop(t, 10000, gameservertest.WithBuyListRows(merchant.StockRow{ListID: shopListID, ItemID: shopSwordID, Count: 1, NextRestock: base.Add(time.Minute)}))
		if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); rows[1].itemID != shopSwordID || rows[1].count != 1 {
			t.Fatalf("restored sword row = %+v, want count 1", rows[1])
		}
	})
	t.Run("restock passed", func(t *testing.T) {
		t.Parallel()
		w := bootShop(t, 10000, gameservertest.WithBuyListRows(merchant.StockRow{ListID: shopListID, ItemID: shopSwordID, Count: 0, NextRestock: base.Add(-time.Minute)}))
		if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); rows[1].itemID != shopSwordID || rows[1].count != 2 {
			t.Fatalf("expired sword row = %+v, want count 2", rows[1])
		}
		if saved, err := w.srv.BuyListRows.LoadStock(context.Background()); err != nil || len(saved) != 0 {
			t.Fatalf("buylists rows = %+v, %v; want the expired row deleted", saved, err)
		}
	})
}

// previewRow is one decoded ShopPreviewList row.
type previewRow struct {
	itemID int32
	slot   uint16
	price  int32
}

// TestMerchantWearOpensTheTryOnWindow pins Wear <list>: the equipable
// products the player's expertise covers, each at WearPrice, then the
// release. With AllowWear off the command is not a shop command.
func TestMerchantWearOpensTheTryOnWindow(t *testing.T) {
	t.Parallel()
	t.Run("allowed", testWearWindow)
	t.Run("AllowWear off", func(t *testing.T) {
		t.Parallel()
		off := bootShop(t, 10000, gameservertest.WithMerchantConfig(merchant.Config{SiegeGuardsPriceRate: 1, WearDelay: time.Second, WearPrice: 10}))
		off.talkTo(t, off.merchant)
		if got := opcodes(off.bypass(t, npcCommand(off.merchant, "Wear 1"))); string(got) != string(releaseOnly) {
			t.Fatalf("Wear 1 with AllowWear off = %x, want ActionFailed alone", got)
		}
	})
}

func testWearWindow(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	w.talkTo(t, w.merchant)
	frames := w.bypass(t, npcCommand(w.merchant, "Wear 1"))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeShopPreviewList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Wear 1 = %x, want ShopPreviewList then ActionFailed", got)
	}
	r := wire.NewReader(frames[0][1:])
	if head := []byte{r.ReadUint8(), r.ReadUint8(), r.ReadUint8(), r.ReadUint8()}; string(head) != "\xc0\x13\x00\x00" {
		t.Fatalf("ShopPreviewList head = %x", head)
	}
	money, listID, n := r.ReadInt32(), r.ReadInt32(), r.ReadUint16()
	var rows []previewRow
	for range n {
		id := r.ReadInt32()
		r.ReadUint16()
		rows = append(rows, previewRow{id, r.ReadUint16(), r.ReadInt32()})
	}
	// No expertise: only the no-grade shirt is shown.
	if money != 10000 || listID != shopListID || len(rows) != 1 || rows[0] != (previewRow{shirtID, uint16(item.SlotChest), 10}) {
		t.Fatalf("ShopPreviewList = money %d list %d rows %+v", money, listID, rows)
	}
}

// TestMerchantTryOn pins RequestPreviewItem: the try-on costs WearPrice an
// item that goes on a slot, shows them worn, and WearDelay later ends with
// a notice and the player's own appearance. Two items for one slot, too
// little adena and an empty request are refused; an item off the list
// answers nothing.
func TestMerchantTryOn(t *testing.T) {
	t.Parallel()
	cfg := merchant.Config{SiegeGuardsPriceRate: 1, AllowWear: true, WearDelay: time.Second, WearPrice: 10}
	w := bootShop(t, 25, gameservertest.WithMerchantConfig(cfg))

	frames := w.send(t, encodeRequestPreviewItem(shopListID, shopSwordID, shopTunicID, shopPotionID))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeShopPreviewInfo}) {
		t.Fatalf("try-on = %x, want SystemMessage then ShopPreviewInfo", got)
	}
	if r := wire.NewReader(frames[0][1:]); r.ReadInt32() != serverpackets.SystemMessageS1DisappearedAdena || r.ReadInt32() != 1 || r.ReadInt32() != 1 || r.ReadInt32() != 20 {
		t.Fatalf("payment notice = %x, want S1_DISAPPEARED_ADENA 20", frames[0])
	}
	r := wire.NewReader(frames[1][1:])
	if total := r.ReadInt32(); total != 17 {
		t.Fatalf("ShopPreviewInfo slots = %d, want 17", total)
	}
	var shown [17]int32
	for i := range shown {
		shown[i] = r.ReadInt32()
	}
	// Wire order REAR, LEAR, NECK, RFINGER, LFINGER, HEAD, RHAND, LHAND,
	// GLOVES, CHEST, ...: the sword in RHAND, the tunic in CHEST.
	if want := [17]int32{6: shopSwordID, 9: shopTunicID}; shown != want {
		t.Fatalf("ShopPreviewInfo = %v, want %v", shown, want)
	}
	if adena := w.countOf(t, item.AdenaID); adena != 5 {
		t.Fatalf("adena after the try-on = %d, want 5", adena)
	}
	end := w.c.ReadWithTimeout(2 * time.Second)
	if end == nil || end[0] != serverpackets.OpcodeSystemMessage || systemMessageID(end) != serverpackets.SystemMessageNoLongerTryingOn {
		t.Fatalf("try-on end = %x, want NO_LONGER_TRYING_ON", end)
	}
	if info := w.c.ReadWithTimeout(time.Second); info == nil || info[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("after the try-on end = %x, want UserInfo", info)
	}

	assertSystemMessages(t, w.send(t, encodeRequestPreviewItem(shopListID, shopTunicID, shirtID)), serverpackets.SystemMessageYouCanNotTryThoseItemsOnAtTheSameTime)
	assertSystemMessages(t, w.send(t, encodeRequestPreviewItem(shopListID, shopSwordID)), serverpackets.SystemMessageYouNotEnoughAdena, serverpackets.SystemMessageYouNotEnoughAdena)
	if got := opcodes(w.send(t, encodeRequestPreviewItem(shopListID))); string(got) != string(releaseOnly) {
		t.Fatalf("empty try-on = %x, want ActionFailed", got)
	}
	if got := w.send(t, encodeRequestPreviewItem(shopListID, shirtID+1)); len(got) != 0 {
		t.Fatalf("try-on of an item off the list = %x, want nothing", opcodes(got))
	}
}

// TestMerchantPricing pins the price rules: a siege guard ticket costs its
// price times RateSiegeGuardsPrice, in the window and at the purchase, and
// an unpriced item of a list sold by no NPC is refused to a player who is
// not a GM, with nothing answered.
func TestMerchantPricing(t *testing.T) {
	t.Parallel()
	gmShop := buylist.List{ID: 3, NPCID: -1, Products: []buylist.Product{{BuyListID: 3, ItemID: shopPotionID, MaxCount: -1}}}
	cfg := merchant.DefaultConfig()
	cfg.SiegeGuardsPriceRate = 1.5
	w := bootShop(t, 1000, gameservertest.WithMerchantConfig(cfg), gameservertest.WithBuyLists(gmShop))

	if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); rows[6].itemID != ticketID || rows[6].price != 150 {
		t.Fatalf("ticket row = %+v, want price 150", rows[6])
	}
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{ticketID, 3}))
	if adena, tickets := w.countOf(t, item.AdenaID), w.countOf(t, ticketID); adena != 550 || tickets != 3 {
		t.Fatalf("after 3 tickets: adena %d tickets %d, want 550/3", adena, tickets)
	}

	if got := w.send(t, encodeRequestBuyItem(3, buyRow{shopPotionID, 1})); len(got) != 0 {
		t.Fatalf("unpriced item = %x, want nothing", opcodes(got))
	}
	if potions := w.countOf(t, shopPotionID); potions != 0 {
		t.Fatalf("potions = %d, want 0", potions)
	}
}
