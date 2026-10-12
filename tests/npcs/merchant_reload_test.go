package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// reloadGMLevel is the access level allowed //reload.
const reloadGMLevel = 7

// buyListsReloaded is AdminReload's answer to //reload buylist.
const buyListsReloaded = "Buylists have been reloaded."

// reloadAdmin grants reloadGMLevel //reload.
func reloadAdmin(t *testing.T) gameservertest.Option {
	t.Helper()
	data, err := admin.NewData([]admin.AccessLevel{{Level: 0}, {Level: reloadGMLevel, IsGM: true}}, []admin.Command{{Name: "admin_reload", AccessLevel: reloadGMLevel}})
	if err != nil {
		t.Fatal(err)
	}
	return gameservertest.WithAdmin(data)
}

// grantReload sets name's access level to reloadGMLevel.
func grantReload(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE char_name = ?", reloadGMLevel, name); err != nil {
		t.Fatalf("grant //reload: %v", err)
	}
}

func encodeReloadBuyList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString("reload buylist")
	return w.Bytes()
}

// reloadedShopList is shopList as buyLists.xml reads after an edit: the
// potion and the sword cost more, the tunic and the ticket have a limited
// stock, and the arrows are gone.
func reloadedShopList() buylist.List {
	return buylist.List{ID: shopListID, NPCID: merchantID, Products: []buylist.Product{
		{BuyListID: shopListID, ItemID: shopPotionID, Price: 70, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shopSwordID, Price: 1200, MaxCount: 2, RestockDelayMillis: 3600000},
		{BuyListID: shopListID, ItemID: cGradeAxeID, Price: 5000, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shopTunicID, Price: 300, MaxCount: 5, RestockDelayMillis: 3600000},
		{BuyListID: shopListID, ItemID: shirtID, Price: 100, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: anvilID, Price: 1, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: ticketID, Price: 100, MaxCount: 4, RestockDelayMillis: 3600000},
	}}
}

// stockRows reads the buylists rows once every queued write has landed,
// ordered by list and item.
func (w *shopWorld) stockRows(t *testing.T) []merchant.StockRow {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.BuyListRows.LoadStock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(rows, func(a, b merchant.StockRow) int {
		if a.ListID != b.ListID {
			return a.ListID - b.ListID
		}
		return int(a.ItemID - b.ItemID)
	})
	return rows
}

func assertStockRows(t *testing.T, got []merchant.StockRow, want ...merchant.StockRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("buylists rows = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i].ListID != want[i].ListID || got[i].ItemID != want[i].ItemID || got[i].Count != want[i].Count || !got[i].NextRestock.Equal(want[i].NextRestock) {
			t.Fatalf("buylists row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestAdminReloadBuyListRebuildsStock pins //reload buylist
// (BuyListManager.reload): the buy window shows the reloaded lists, and
// each limited product's stock is rebuilt from the buylists rows as the
// boot load reads them — a row still ahead sets the count it saved (the
// count the first sale after a restock left, not the one later sales
// brought down) and keeps its restock, a row already past is deleted, and
// a row naming no limited product is left alone. A product the reload no
// longer limits still has its row deleted when its restock comes.
func TestAdminReloadBuyListRebuildsStock(t *testing.T) {
	t.Parallel()
	reloaded := buylist.NewTable([]buylist.List{reloadedShopList(), otherList()})
	reloads := network.DataReloads{BuyLists: func() (*buylist.Table, error) { return reloaded, nil }}
	w := bootShop(t, 10000, reloadAdmin(t), gameservertest.WithDataReloads(reloads),
		gameservertest.WithClanSeed(func(db *sql.DB) { grantReload(t, db, "Buyer") }))
	srv := w.srv
	base := w.clock.Now()
	restock := base.Add(time.Hour)

	w.openBuy(t, "1")
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}))
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}))
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{arrowID, 1}))
	if swords, arrows := w.countOf(t, shopSwordID), w.countOf(t, arrowID); swords != 2 || arrows != 1 {
		t.Fatalf("bought swords %d arrows %d, want 2 and 1", swords, arrows)
	}

	w.clock.advance(10 * time.Minute)
	now := w.clock.Now()
	ctx := context.Background()
	for _, row := range []merchant.StockRow{
		{ListID: shopListID, ItemID: shopTunicID, Count: 3, NextRestock: now.Add(5 * time.Minute)},
		{ListID: shopListID, ItemID: ticketID, Count: 1, NextRestock: now.Add(-time.Minute)},
		{ListID: 7, ItemID: shopPotionID, Count: 1, NextRestock: now.Add(-time.Minute)},
	} {
		if err := srv.BuyListRows.SaveStock(ctx, row); err != nil {
			t.Fatal(err)
		}
	}

	frames := w.send(t, encodeReloadBuyList())
	if len(frames) != 1 || !bytes.Equal(frames[0], noticeFrame(buyListsReloaded)) {
		t.Fatalf("//reload buylist = %x, want the %q notice", testsupport.FrameOpcodes(frames), buyListsReloaded)
	}

	_, _, header, rows := decodeBuyList(t, w.openBuy(t, "1")[0])
	want := []buyListRow{
		{shopPotionID, 0, 70, 0},
		{shopSwordID, 1, 1200, int32(item.SlotRHand)},
		{cGradeAxeID, 0, 5000, int32(item.SlotRHand)},
		{shopTunicID, 3, 300, int32(item.SlotChest)},
		{shirtID, 0, 100, int32(item.SlotChest)},
		{anvilID, 0, 1, 0},
		{ticketID, 4, 100, 0},
	}
	if header != 7 || !slices.Equal(rows, want) {
		t.Fatalf("reloaded BuyList header %d rows %+v, want 7 and %+v", header, rows, want)
	}
	assertStockRows(t, w.stockRows(t),
		merchant.StockRow{ListID: shopListID, ItemID: arrowID, Count: 2, NextRestock: restock},
		merchant.StockRow{ListID: shopListID, ItemID: shopTunicID, Count: 3, NextRestock: now.Add(5 * time.Minute)},
		merchant.StockRow{ListID: shopListID, ItemID: shopSwordID, Count: 1, NextRestock: restock},
		merchant.StockRow{ListID: 7, ItemID: shopPotionID, Count: 1, NextRestock: now.Add(-time.Minute)},
	)

	// The reloaded prices are the ones paid.
	adena := w.countOf(t, item.AdenaID)
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1}))
	if got := adena - w.countOf(t, item.AdenaID); got != 70 {
		t.Fatalf("potion after the reload cost %d, want 70", got)
	}

	// The restored restocks run on.
	w.clock.advance(5 * time.Minute)
	srv.BuyListStock.Restock()
	if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); rows[3].itemID != shopTunicID || rows[3].count != 5 {
		t.Fatalf("tunic after its restock = %+v, want count 5", rows[3])
	}
	w.clock.advance(restock.Sub(w.clock.Now()))
	srv.BuyListStock.Restock()
	if _, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0]); rows[1].itemID != shopSwordID || rows[1].count != 2 {
		t.Fatalf("sword after its restock = %+v, want count 2", rows[1])
	}
	assertStockRows(t, w.stockRows(t), merchant.StockRow{ListID: 7, ItemID: shopPotionID, Count: 1, NextRestock: now.Add(-time.Minute)})
}

// raceList is the merchant's list priced for a reload race: a potion and
// a tunic at the given prices, and arrows limited the same way in every
// version of the list.
func raceList(potion, tunic int) buylist.List {
	return buylist.List{ID: shopListID, NPCID: merchantID, Products: []buylist.Product{
		{BuyListID: shopListID, ItemID: shopPotionID, Price: potion, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: shopTunicID, Price: tunic, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: shopListID, ItemID: arrowID, Price: 1, MaxCount: 1000, RestockDelayMillis: 3600000},
	}}
}

// TestAdminReloadBuyListRacesPurchases runs //reload buylist from a game
// master, flipping the merchant's list between two prices, while a player
// buys from it: under -race, every purchase pays one list's prices for all
// its rows, never a mix of both, and gets every item it paid for.
func TestAdminReloadBuyListRacesPurchases(t *testing.T) {
	t.Parallel()
	const rounds = 15
	cheap := buylist.NewTable([]buylist.List{raceList(50, 300)})
	dear := buylist.NewTable([]buylist.List{raceList(70, 400)})
	var flip atomic.Bool
	reloads := network.DataReloads{BuyLists: func() (*buylist.Table, error) {
		if flip.Load() {
			flip.Store(false)
			return cheap, nil
		}
		flip.Store(true)
		return dear, nil
	}}
	w := bootShop(t, 100000, gameservertest.WithBuyLists(raceList(50, 300)), reloadAdmin(t), gameservertest.WithDataReloads(reloads))
	w.srv.SeedCharacterFor(t, "master", "Master", 1, 0)
	grantReload(t, w.srv.DB, "Master")
	gm := w.srv.DialClient(t, "master", 1)
	startInWorld(t, w.srv, gm)
	w.openBuy(t, "1")
	drainFrames(t, w.c)

	// The buyer's purchases reach the game master too; only its system
	// messages count.
	var notices [][]byte
	var wg sync.WaitGroup
	wg.Go(func() {
		for range rounds {
			gm.Send(encodeReloadBuyList())
			for _, frame := range drainFrames(t, gm) {
				if frame[0] == serverpackets.OpcodeSystemMessage {
					notices = append(notices, frame)
				}
			}
		}
	})
	for i := range rounds {
		adena := w.countOf(t, item.AdenaID)
		w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 1}, buyRow{shopTunicID, 1}, buyRow{arrowID, 1}))
		if paid := adena - w.countOf(t, item.AdenaID); paid != 50+300+1 && paid != 70+400+1 {
			t.Fatalf("purchase %d paid %d, want one list's prices: 351 or 471", i, paid)
		}
	}
	wg.Wait()
	if len(notices) != rounds {
		t.Fatalf("//reload buylist answered %d system messages over %d reloads", len(notices), rounds)
	}
	for _, frame := range notices {
		if !bytes.Equal(frame, noticeFrame(buyListsReloaded)) {
			t.Fatalf("//reload buylist answered %x, want the %q notice", frame, buyListsReloaded)
		}
	}
	if potions, tunics, arrows := w.countOf(t, shopPotionID), w.countOf(t, shopTunicID), w.countOf(t, arrowID); potions != rounds || tunics != rounds || arrows != rounds {
		t.Fatalf("bought potions %d tunics %d arrows %d, want %d each", potions, tunics, arrows, rounds)
	}
}
