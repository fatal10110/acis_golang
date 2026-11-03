package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestBuySeed and RequestProcureCropList, their proceed loops
// on a row repeated past what is left or held, and their weight and slot
// refusals, which count the bought seed and the paid reward item.

// TestBuySeedsDuplicateRows pins a purchase naming the same seed twice: both
// rows pass the checks against the 7 seeds left, the first takes 5 seeds and
// their 25 adena, and the second, finding only 2 left, is skipped unpaid.
func TestBuySeedsDuplicateRows(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 100)
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)

	wantOnly(t, "buy twice", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 5}, [2]int{tradeDarkCodaSeed, 5})),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeDarkCodaSeed), itemNumberParam(5)),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, itemNumberParam(25)))
	if got := w.held(t, tradeDarkCodaSeed); got != 5 {
		t.Fatalf("seeds held = %d, want 5", got)
	}
	if got := w.held(t, item.AdenaID); got != 75 {
		t.Fatalf("adena = %d, want 75", got)
	}
	if got := w.amountLeft(t, 1, tradeDarkCodaSeed, false); got != 2 {
		t.Fatalf("seeds left = %d, want 2", got)
	}
	if got := w.seedIncome(t); got != 25 {
		t.Fatalf("seed income = %d, want 25", got)
	}
}

// TestSellCropsDuplicateRows pins a sale naming the same 12 held crops twice
// for 7 each: the first row destroys 7 crops for 3 rewards; the second takes
// 7 from what the castle wants, finds only 5 crops left to destroy, answers
// not enough items and pays nothing.
func TestSellCropsDuplicateRows(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 0)
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)

	wantOnly(t, "sell twice", w.send(t, encodeProcureCropList(
		[4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 7}, [4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 7},
	)),
		sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(tradeDarkCodaCrop), itemNumberParam(7)),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeGludioReward), itemNumberParam(3)),
		sysMsg(serverpackets.SystemMessageNotEnoughItems),
	)
	if got := w.held(t, tradeDarkCodaCrop); got != tradeHeldDarkCoda-7 {
		t.Fatalf("crops held = %d, want %d", got, tradeHeldDarkCoda-7)
	}
	if got := w.held(t, tradeGludioReward); got != 3 {
		t.Fatalf("rewards = %d, want 3", got)
	}
	if got := w.amountLeft(t, 1, tradeDarkCodaCrop, true); got != 26 {
		t.Fatalf("crops wanted = %d, want 26", got)
	}
}

// TestManorTradeSlotsFull pins the slot refusals with every inventory slot
// taken by the adena and the two crops held: a seed purchase needing a new
// stack, and a crop sale paying a reward not yet held, answer slots full and
// change nothing.
func TestManorTradeSlotsFull(t *testing.T) {
	t.Parallel()
	w := bootManorTrade(t, 100, gameservertest.WithInventorySlots(3, 3))
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)

	wantOnly(t, "buy a new seed", w.send(t, encodeBuySeed(1, [2]int{tradeDarkCodaSeed, 1})),
		sysMsg(serverpackets.SystemMessageSlotsFull))
	wantOnly(t, "sell for a new reward", w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, 5})),
		sysMsg(serverpackets.SystemMessageSlotsFull))

	if w.held(t, tradeDarkCodaSeed) != 0 || w.held(t, item.AdenaID) != 100 || w.amountLeft(t, 1, tradeDarkCodaSeed, false) != 7 || w.seedIncome(t) != 0 {
		t.Fatal("a refused purchase changed the seeds, the adena or the income")
	}
	if w.held(t, tradeDarkCodaCrop) != tradeHeldDarkCoda || w.held(t, tradeGludioReward) != 0 || w.amountLeft(t, 1, tradeDarkCodaCrop, true) != 40 {
		t.Fatal("a refused sale changed the crops, the rewards or the castle's wants")
	}
}

// heavyGludioReward weighs the Gludio reward so that a sale of 7 crops,
// weighed as 7 rewards, sums past the int range to 3.
const heavyGludioReward = 613_566_757

// TestSellCropsWeightLimit pins the crop sale weight refusal, which weighs
// each crop sold as one reward item, not as the light crop: 2 crops for a
// heavy reward are over the limit, and 7, whose weight sum wraps to 3 as a
// 32-bit int, are over it too. A crop paid in a light reward still sells.
func TestSellCropsWeightLimit(t *testing.T) {
	t.Parallel()
	var templates []*item.Template
	for _, tpl := range tradeTemplates().All() {
		if tpl.ID == tradeGludioReward {
			heavy := *tpl
			heavy.Weight = heavyGludioReward
			tpl = &heavy
		}
		templates = append(templates, tpl)
	}
	w := bootManorTrade(t, 2, gameservertest.WithItemTemplates(item.NewTable(templates)))
	spawnManorNPC(t, w.folkWorld, "ManorManagerNpc", castleManorID, 30)

	for _, c := range []struct {
		what  string
		count int32
	}{
		{"a heavy reward", 2},
		{"a weight past the int range", 7},
	} {
		wantOnly(t, c.what, w.send(t, encodeProcureCropList([4]int32{w.darkCoda, tradeDarkCodaCrop, 1, c.count})),
			sysMsg(serverpackets.SystemMessageWeightLimitExceeded))
	}
	if w.held(t, tradeDarkCodaCrop) != tradeHeldDarkCoda || w.held(t, tradeGludioReward) != 0 || w.amountLeft(t, 1, tradeDarkCodaCrop, true) != 40 || w.held(t, item.AdenaID) != 2 {
		t.Fatal("a refused sale changed the crops, the rewards, the adena or the castle's wants")
	}

	// 1 Dion crop at 20 costs a fee of 1 and buys 2 light rewards.
	wantOnly(t, "sell for a light reward", w.send(t, encodeProcureCropList([4]int32{w.dionCrop, tradeDionCrop, tradeDionID, 1})),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1)),
		sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(tradeDionCrop)),
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(tradeDionReward), itemNumberParam(2)),
	)
}
