package npcs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: ManorManagerNpc.onBypassFeedback (asks 1, 2 and 9),
// RequestBuySeed, RequestProcureCropList, RequestBuyProcure and the
// BuyListSeed, ExShowSellCropList and ExShowProcureCropDetail windows.
//
// The trade world: Gludio (1), owned by the Rivals, sells 7 of 10 Dark
// Coda seeds (5016) at 5 and 5 Red Coda seeds (5012, too heavy to carry)
// at 8, and buys 40 of 100 Dark Coda crops (5073) at 50, paid with reward
// 1 (item 1864, worth 100). Dion (2) buys 10 of its crop 5074 at 20, paid
// with reward 2 (item 1880, worth 10). The talker, of no clan, holds 12
// Dark Coda crops and 3 of Dion's.
const (
	tradeDionID          = 2
	tradeDarkCodaSeed    = 5016
	tradeRedCodaSeed     = 5012
	tradeDarkCodaCrop    = 5073
	tradeDionCrop        = 5074
	tradeGludioReward    = 1864
	tradeDionReward      = 1880
	tradeHeldDarkCoda    = 12
	tradeHeldDionCrop    = 3
	tradeGludioCastleTag = "Gludio Castle"
)

var manorTradeRows = []string{
	"INSERT INTO castle_manor_production VALUES (1, 5016, 7, 10, 5, 0), (1, 5012, 5, 5, 8, 0)",
	"INSERT INTO castle_manor_procure VALUES (1, 5073, 40, 100, 50, 1, 0), (2, 5074, 10, 10, 20, 2, 0)",
}

// tradeCastles are Gludio, whose NPCs the manor manager is, and Dion.
func tradeCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: tradeGludioCastleTag, CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax}, NPCs: []int{castleManorID},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dion, err := castledata.NewCastle(castledata.CastleAttrs{ID: tradeDionID, Alias: "dion_castle", Name: "Dion Castle", CircletID: 6839}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio, dion})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// tradeTemplates is the behavior catalog plus the seeds, crops and reward
// items, each stackable and weighing 1 but the Red Coda seed.
func tradeTemplates() *item.Table {
	etc := func(id, weight, price int32) *item.Template {
		return &item.Template{
			ID: id, Name: "Manor", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Weight: weight, ReferencePrice: price,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		etc(tradeDarkCodaSeed, 1, 5), etc(tradeRedCodaSeed, 100_000_000, 10),
		etc(tradeDarkCodaCrop, 1, 51), etc(tradeDionCrop, 1, 20),
		etc(tradeGludioReward, 1, 100), etc(tradeDionReward, 1, 10)))
}

// manorTradeWorld is the trade world with the object ids of the crops held.
type manorTradeWorld struct {
	*folkWorld
	darkCoda, dionCrop int32
}

// bootManorTrade boots the trade world with the talker holding adena, the
// manor running from manorSetupStart, extra applied last.
func bootManorTrade(t *testing.T, adena int32, extra ...gameservertest.Option) *manorTradeWorld {
	t.Helper()
	cfg := castlemanor.DefaultConfig()
	cfg.SavePeriod = 0
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(residencePages()),
		gameservertest.WithItemTemplates(tradeTemplates()),
		gameservertest.WithCastles(tradeCastles(t)),
		gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: 1, Seeds: manorSetupSeeds()}),
		gameservertest.WithCastleManor(cfg, manorSetupStart),
		noBypassReuse,
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedResidenceChat(t, db, residenceSetup{clanless: true})
			for _, q := range manorTradeRows {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("seed manor: %v", err)
				}
			}
		}),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &manorTradeWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	w.darkCoda = srv.GiveItem(t, w.player, tradeDarkCodaCrop, tradeHeldDarkCoda)
	w.dionCrop = srv.GiveItem(t, w.player, tradeDionCrop, tradeHeldDionCrop)
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

func encodeBuySeed(manorID int, rows ...[2]int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBuySeed)
	w.WriteInt32(int32(manorID))
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(int32(r[0]))
		w.WriteInt32(int32(r[1]))
	}
	return w.Bytes()
}

// encodeProcureCropList lays out (object id, crop id, manor id, count)
// rows.
func encodeProcureCropList(rows ...[4]int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestProcureCropList)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		for _, v := range r {
			w.WriteInt32(v)
		}
	}
	return w.Bytes()
}

func (w *manorTradeWorld) amountLeft(t *testing.T, castleID int, id int32, crop bool) int32 {
	t.Helper()
	m := w.srv.CastleManor
	if crop {
		cp, ok := m.CropProcureOf(castleID, id, false)
		if !ok {
			t.Fatalf("castle %d buys no %d", castleID, id)
		}
		return cp.Amount()
	}
	sp, ok := m.SeedProduct(castleID, id, false)
	if !ok {
		t.Fatalf("castle %d sells no %d", castleID, id)
	}
	return sp.Amount()
}

func (w *manorTradeWorld) seedIncome(t *testing.T) int64 {
	t.Helper()
	c, ok := w.srv.Castles.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio")
	}
	return c.SeedIncome()
}

// TestManorManagerTradeWindows pins the manor manager's trade windows, at
// any distance: the seed purchase window lists its own castle's seeds with
// seeds left and refuses another castle's by name; the crop sale window
// lists each crop held with what the castle asked for still buys, and is
// silent for a castle the manor holds no lists of; the crop sale detail
// lists each castle still buying the crop.
func TestManorManagerTradeWindows(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 1000)
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 1000)

	wantOnly(t, "seed purchase", w.bypass(t, "manor_menu_select?ask=1&state=-1&time=0"), cat(
		le(byte(serverpackets.OpcodeBuyListSeed), 1000, 1, uint16(2)),
		le(uint16(4), 5012, 5012, 5, uint16(4), uint16(0), 8),
		le(uint16(4), 5016, 5016, 7, uint16(4), uint16(0), 5),
	))
	wantOnly(t, "another castle's seeds", w.bypass(t, "manor_menu_select?ask=1&state=2&time=0"),
		sysMsg(serverpackets.SystemMessageHereYouCanBuyOnlySeedsOfS1Manor, textParam(tradeGludioCastleTag)))

	wantOnly(t, "crop sale", w.bypass(t, "manor_menu_select?ask=2&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x21), 1, 2),
		le(int(w.darkCoda), 5073), seedRewards(manorDarkCoda), le(1, 40, 50, byte(1)), le(tradeHeldDarkCoda),
		le(int(w.dionCrop), 5074), seedRewards(manorDionSeed), le(-1, 0, 0, byte(0)), le(tradeHeldDionCrop),
	))
	wantOnly(t, "crop sale to Dion", w.bypass(t, "manor_menu_select?ask=2&state=2&time=0"), cat(
		le(opcodeFE, uint16(0x21), 2, 2),
		le(int(w.darkCoda), 5073), seedRewards(manorDarkCoda), le(-1, 0, 0, byte(0)), le(tradeHeldDarkCoda),
		le(int(w.dionCrop), 5074), seedRewards(manorDionSeed), le(2, 10, 20, byte(2)), le(tradeHeldDionCrop),
	))
	wantOnly(t, "crop sale to no castle", w.bypass(t, "manor_menu_select?ask=2&state=9&time=0"))

	wantOnly(t, "crop sale detail", w.bypass(t, "manor_menu_select?ask=9&state=5074&time=0"),
		le(opcodeFE, uint16(0x22), 5074, 1, 2, 10, 20, byte(2)))
	wantOnly(t, "crop no castle buys", w.bypass(t, "manor_menu_select?ask=9&state=5068&time=0"),
		le(opcodeFE, uint16(0x22), 5068, 0))
}

// TestBuySeeds walks a seed purchase at the Gludio manor manager: the
// seeds arrive named, the adena paid is named once and goes to the
// castle's seed income, and the castle has that many fewer seeds left.
// Each refusal leaves the seeds, the adena and the income as they were.
func TestBuySeeds(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 20)
	far := spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 1000)
	near := spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)
	actionFailed := []byte{serverpackets.OpcodeActionFailed}

	wantOnly(t, "buy", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 3})),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeDarkCodaSeed), itemNumberParam(3)),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, itemNumberParam(15)))
	if got := w.held(t, tradeDarkCodaSeed); got != 3 {
		t.Fatalf("seeds held = %d, want 3", got)
	}
	if got := w.held(t, item.AdenaID); got != 5 {
		t.Fatalf("adena = %d, want 5", got)
	}
	if got := w.amountLeft(t, 1, tradeDarkCodaSeed, false); got != 4 {
		t.Fatalf("seeds left = %d, want 4", got)
	}
	if got := w.seedIncome(t); got != 15 {
		t.Fatalf("seed income = %d, want 15", got)
	}
	if got := w.savedCount(t, tradeDarkCodaSeed); got != 3 {
		t.Fatalf("saved seeds = %d, want 3", got)
	}

	for _, c := range []struct {
		what    string
		payload []byte
		want    []byte
	}{
		{"more than left", encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 5}), actionFailed},
		{"not enough adena", encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 2}), sysMsg(serverpackets.SystemMessageYouNotEnoughAdena)},
		{"too heavy", encodeBuySeed(1, [2]int{tradeRedCodaSeed, 1}), sysMsg(serverpackets.SystemMessageWeightLimitExceeded)},
		{"a seed not sold", encodeBuySeed(1, [2]int{5100, 1}), actionFailed},
		{"another castle", encodeBuySeed(tradeDionID, [2]int{tradeDarkCodaSeed, 1}), actionFailed},
		{"no castle", encodeBuySeed(9, [2]int{tradeDarkCodaSeed, 1}), actionFailed},
		{"refused list", encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 0}), actionFailed},
	} {
		wantOnly(t, c.what, w.send(t, c.payload), c.want)
	}
	w.selectFolk(t, far)
	wantOnly(t, "out of reach", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 1})), actionFailed)
	if w.held(t, tradeDarkCodaSeed) != 3 || w.held(t, item.AdenaID) != 5 || w.amountLeft(t, 1, tradeDarkCodaSeed, false) != 4 || w.seedIncome(t) != 15 {
		t.Fatal("a refused purchase changed the seeds, the adena or the income")
	}

	// The rollover itself moves the income; the purchase changes nothing.
	w.selectFolk(t, near)
	w.srv.CastleManorClock.Advance(14*time.Hour + 2*time.Minute)
	wantOnly(t, "under maintenance", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 1})), actionFailed)
	if w.held(t, tradeDarkCodaSeed) != 3 || w.held(t, item.AdenaID) != 5 {
		t.Fatal("a purchase under maintenance changed the seeds or the adena")
	}
}

// TestBuySeedsReuseDelay pins the seed purchase reuse gate: a second
// purchase inside ManorTime answers nothing and buys nothing.
func TestBuySeedsReuseDelay(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 100, gameservertest.WithManorDelay(time.Hour))
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)

	if got := w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 1})); len(got) != 2 {
		t.Fatalf("first purchase = % x, want the seed and the adena named", opcodes(got))
	}
	wantOnly(t, "second purchase", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 1})))
	if got := w.held(t, tradeDarkCodaSeed); got != 1 {
		t.Fatalf("seeds held = %d, want 1", got)
	}
}

// TestSellCrops walks crop sales at the Gludio manor manager: a crop sold
// to Gludio is paid in its reward item at the reward's reference price,
// fee free; one sold to Dion costs 5% of its price in adena first. A row
// whose price buys no reward, or whose fee the seller cannot pay, is named
// and skipped. Each refusal leaves the crops and the castles' wants as they
// were.
func TestSellCrops(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 2)
	far := spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 1000)
	near := spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)
	actionFailed := []byte{serverpackets.OpcodeActionFailed}

	// 5 crops at 50 buy 2 rewards worth 100; 1 crop buys none.
	wantOnly(t, "sell to Gludio", w.send(t, encodeProcureCropList(
		[4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 5}, [4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 1},
	)),
		sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(tradeDarkCodaCrop), itemNumberParam(5)),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeGludioReward), itemNumberParam(2)),
		sysMsg(serverpackets.SystemMessageFailedInTradingS2OfCropS1, itemNameParam(tradeDarkCodaCrop), itemNumberParam(1)),
	)
	if w.held(t, tradeDarkCodaCrop) != tradeHeldDarkCoda-5 || w.held(t, tradeGludioReward) != 2 || w.amountLeft(t, 1, tradeDarkCodaCrop, true) != 35 {
		t.Fatalf("after the Gludio sale: crops %d, rewards %d, wanted %d; want 7, 2, 35",
			w.held(t, tradeDarkCodaCrop), w.held(t, tradeGludioReward), w.amountLeft(t, 1, tradeDarkCodaCrop, true))
	}

	// 3 crops at 20 cost a fee of 3 the seller cannot pay; 1 crop costs 1
	// and buys 2 rewards worth 10.
	wantOnly(t, "sell to Dion", w.send(t, encodeProcureCropList(
		[4]int32{w.dionCrop, tradeDionCrop, tradeDionID, 3}, [4]int32{w.dionCrop, tradeDionCrop, tradeDionID, 1},
	)),
		sysMsg(serverpackets.SystemMessageFailedInTradingS2OfCropS1, itemNameParam(tradeDionCrop), itemNumberParam(3)),
		sysMsg(serverpackets.SystemMessageYouNotEnoughAdena),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1)),
		sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(tradeDionCrop)),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeDionReward), itemNumberParam(2)),
	)
	if w.held(t, tradeDionCrop) != tradeHeldDionCrop-1 || w.held(t, tradeDionReward) != 2 || w.held(t, item.AdenaID) != 1 || w.amountLeft(t, tradeDionID, tradeDionCrop, true) != 9 {
		t.Fatal("the Dion sale did not take one crop and one adena for two rewards")
	}
	if got := w.savedCount(t, tradeGludioReward); got != 2 {
		t.Fatalf("saved rewards = %d, want 2", got)
	}

	for _, c := range []struct {
		what    string
		payload []byte
		want    [][]byte
	}{
		{"more than held", encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 8}), [][]byte{actionFailed}},
		{"another item", encodeProcureCropList([4]int32{w.darkCoda, tradeDionCrop, 1, 1}), [][]byte{actionFailed}},
		{"a crop not bought", encodeProcureCropList([4]int32{w.dionCrop, tradeDionCrop, 1, 1}), [][]byte{actionFailed}},
		{"no castle", encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 9, 1}), nil},
		{"refused list", encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, -1, 1}), nil},
	} {
		wantOnly(t, c.what, w.send(t, c.payload), c.want...)
	}
	w.selectFolk(t, far)
	wantOnly(t, "out of reach", w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 5})), actionFailed)
	w.selectFolk(t, near)
	w.srv.CastleManorClock.Advance(14*time.Hour + 2*time.Minute)
	wantOnly(t, "under maintenance", w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 5})), actionFailed)

	if w.held(t, tradeDarkCodaCrop) != tradeHeldDarkCoda-5 || w.held(t, tradeDionCrop) != tradeHeldDionCrop-1 {
		t.Fatal("a refused sale took crops")
	}
}

// TestBuyProcureIsNeverRun pins RequestBuyProcure: its rows never fit the
// count it checks, so it answers nothing and the session reads on.
func TestBuyProcureIsNeverRun(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 0)
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)
	pw := wire.NewPacketWriter(clientpackets.OpcodeRequestBuyProcure)
	for _, v := range []int32{1, 2, 0, tradeDarkCodaCrop, 1, 0} {
		pw.WriteInt32(v)
	}
	wantOnly(t, "procure sale", w.send(t, pw.Bytes()))
	wantOnly(t, "crop sale detail after it", w.bypass(t, "manor_menu_select?ask=9&state=5073&time=0"),
		le(opcodeFE, uint16(0x22), 5073, 1, 1, 40, 50, byte(1)))
}
