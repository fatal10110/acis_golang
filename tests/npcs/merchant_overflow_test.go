package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Overflow fixture: list 3 at the castle merchant. Taxed at 10%, eleven
// potions cost 11 * 214,500,000 = 2,359,500,000, past the int32 range
// (2,145,000,000 before the tax, so the untaxed bound lets it through);
// ten arrows cost 10 * 193,546,830 = 1,935,468,300. Summed with the potion
// row wrapped to 32 bits, the two rows would cost 1,004 adena.
const (
	overflowListID      = 3
	overflowPotionPrice = 195_000_000
	overflowArrowPrice  = 175_951_664
	overflowAdena       = 300_000_000
)

func overflowList() buylist.List {
	return buylist.List{ID: overflowListID, NPCID: merchantID, Products: []buylist.Product{
		{BuyListID: overflowListID, ItemID: shopPotionID, Price: overflowPotionPrice, MaxCount: -1, RestockDelayMillis: -60000},
		{BuyListID: overflowListID, ItemID: arrowID, Price: overflowArrowPrice, MaxCount: -1, RestockDelayMillis: -60000},
	}}
}

// assertOverflowRefusal checks frames are the system message id then
// ActionFailed.
func assertOverflowRefusal(t *testing.T, frames [][]byte, id int32) {
	t.Helper()
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeSystemMessage || systemMessageID(frames[0]) != id ||
		frames[1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("answer = %x, want SystemMessage %d then ActionFailed", opcodes(frames), id)
	}
}

// TestMerchantTaxedBuyRowOverflowRefused pins a taxed row whose total
// leaves the int32 range: the purchase is refused as too costly, with
// ActionFailed, and nothing is taken or given, even though the other row
// would bring a 32-bit sum back to a few adena. A row within the range
// still costs count times the taxed unit price.
func TestMerchantTaxedBuyRowOverflowRefused(t *testing.T) {
	t.Parallel()
	opts := append(withTaxCastle(t, "Buyer", false), gameservertest.WithBuyLists(overflowList()))
	w := bootShop(t, overflowAdena, opts...)
	taxCastle(t, w.srv, false)

	frames := w.send(t, encodeRequestBuyItem(overflowListID, buyRow{shopPotionID, 11}, buyRow{arrowID, 10}))
	assertOverflowRefusal(t, frames, serverpackets.SystemMessageYouNotEnoughAdena)
	if adena, potions, arrows := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID), w.countOf(t, arrowID); adena != overflowAdena || potions != 0 || arrows != 0 {
		t.Fatalf("after the overflowing buy: adena %d potions %d arrows %d, want %d/0/0", adena, potions, arrows, overflowAdena)
	}

	w.send(t, encodeRequestBuyItem(overflowListID, buyRow{shopPotionID, 1}))
	if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != overflowAdena-214_500_000 || potions != 1 {
		t.Fatalf("after one taxed potion: adena %d potions %d, want %d/1", adena, potions, overflowAdena-214_500_000)
	}
}

// TestMerchantBuyWeightOverflowRefused pins a row whose weight leaves the
// int32 range: 4,294 anvils weigh 4,294,000,000, which wraps to -967,296 in
// 32 bits, and 193,460 potions weigh 967,300, so a 32-bit sum would be a
// carriable 4. The purchase is refused as too heavy, with ActionFailed, and
// nothing is taken or given.
func TestMerchantBuyWeightOverflowRefused(t *testing.T) {
	t.Parallel()
	const adena = 10_000_000
	w := bootShop(t, adena)

	frames := w.send(t, encodeRequestBuyItem(shopListID, buyRow{anvilID, 4294}, buyRow{shopPotionID, 193_460}))
	assertOverflowRefusal(t, frames, serverpackets.SystemMessageWeightLimitExceeded)
	if got, anvils, potions := w.countOf(t, item.AdenaID), w.countOf(t, anvilID), w.countOf(t, shopPotionID); got != adena || anvils != 0 || potions != 0 {
		t.Fatalf("after the overweight buy: adena %d anvils %d potions %d, want %d/0/0", got, anvils, potions, adena)
	}
}
