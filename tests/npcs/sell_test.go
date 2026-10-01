package npcs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Merchant.onBypassFeedback "Sell" (Merchant.java:62-80) opens
// SellList(adena, PcInventory.getSellableItems()) — items not worn,
// sellable (not augmented, template is_sellable) and not the summoned pet's
// collar — or <folder>/<npcId>-empty.htm when there is nothing to sell and
// the page exists. RequestSellItem.java:49-107 needs a Merchant or
// MercenaryManagerNpc target the player can interact with and, for a list
// id above 1000000, that merchant's npc id plus 1000000; each row passing
// checkItemManipulation and isSellable is destroyed for
// referencePrice/2 per unit; a row or total above Integer.MAX_VALUE stops
// the sale; the adena is added without a message; <folder>/<npcId>-sold.htm
// follows when it exists. Every refusal is silent. aCis revision in the
// outer repo.

const (
	swordID     = 30
	potionID    = 20
	soulshotID  = 1463
	tunicID     = 40
	scrollID    = 955
	goldBarID   = 9800
	fisherID    = 31562
	mercID      = 35102
	listOffset  = 1000000
	startAdena  = 1000
	sellPage    = `<html><body>Trader:<br><a action="bypass -h npc_%objectId%_Sell">Sell</a></body></html>`
	emptyPage   = `<html><body>Nothing to sell %objectId%</body></html>`
	soldPage    = `<html><body>Thanks %objectId%</body></html>`
	fisherEmpty = `<html><body>No fish %objectId%</body></html>`
	fisherSold  = `<html><body>Fish thanks %objectId%</body></html>`
)

// sellCatalog is the fixture catalog with the sword (price 1000), potion
// (40), tunic (300) and enchant scroll (60) sellable, the soulshot not, and
// a gold bar whose half price is 1,000,000,000 adena.
func sellCatalog() *item.Table {
	prices := map[int32]int32{swordID: 1000, potionID: 40, tunicID: 300, scrollID: 60}
	all := gameservertest.ItemTemplates().All()
	for _, tmpl := range all {
		if price, ok := prices[tmpl.ID]; ok {
			tmpl.Sellable, tmpl.ReferencePrice = true, price
		}
	}
	all = append(all, &item.Template{
		ID: goldBarID, Name: "Gold Bar", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Sellable: true, ReferencePrice: 2000000000, EtcItem: &item.EtcItemDetail{},
	})
	return item.NewTable(all)
}

func sellPages() map[string]string {
	return map[string]string{
		"merchant/30001.htm":        sellPage,
		"merchant/30001-sold.htm":   soldPage,
		"merchant/30002.htm":        sellPage,
		"merchant/30003.htm":        sellPage,
		"merchant/30003-empty.htm":  emptyPage,
		"fisherman/31562.htm":       strings.Replace(sellPage, "_Sell", "_sell", 1),
		"fisherman/31562-empty.htm": fisherEmpty,
		"fisherman/31562-sold.htm":  fisherSold,
		"merchant/31562-sold.htm":   soldPage,
		"merchant/35102-sold.htm":   soldPage,
		"warehouse/30005.htm":       sellPage,
	}
}

// sellWorld is a folkWorld whose player entered holding the seeded stacks.
type sellWorld struct {
	*folkWorld
	items map[int32]int32 // template id -> object id
}

// bootSeller boots with the sell catalog and pages, seeds startAdena plus
// each {template, count} stack, and enters the world.
func bootSeller(t *testing.T, stacks ...[2]int32) *sellWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Seller", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(sellPages()),
		gameservertest.WithItemTemplates(sellCatalog()),
		noBypassReuse,
	)
	w := &sellWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	w.items[item.AdenaID] = srv.GiveItem(t, w.player, item.AdenaID, startAdena)
	for _, s := range stacks {
		w.items[s[0]] = srv.GiveItem(t, w.player, s[0], s[1])
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at.X, w.at.Y, w.at.Z = x, y, z
	return w
}

type sellRow struct{ objectID, itemID, count int32 }

func encodeRequestSellItem(listID int32, rows ...sellRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSellItem)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.itemID)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

// sellListRow is one decoded SellList row.
type sellListRow struct{ itemID, count, price int32 }

// decodeSellList reads a SellList frame: the adena shown and each row by
// object id.
func decodeSellList(t *testing.T, frame []byte) (int32, map[int32]sellListRow) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSellList {
		t.Fatalf("opcode = %#x, want SellList", frame[0])
	}
	r := wire.NewReader(frame[1:])
	adena := r.ReadInt32()
	if zero := r.ReadInt32(); zero != 0 {
		t.Fatalf("SellList list id = %d, want 0", zero)
	}
	rows := map[int32]sellListRow{}
	for n := int(r.ReadUint16()); n > 0; n-- {
		r.ReadUint16() // type1
		objectID, itemID, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		r.ReadUint16() // type2
		r.ReadUint16() // custom type 1
		r.ReadInt32()  // body part
		r.ReadUint16() // enchant
		r.ReadUint16() // custom type 2
		r.ReadUint16()
		rows[objectID] = sellListRow{itemID: itemID, count: count, price: r.ReadInt32()}
	}
	if r.Remaining() != 0 || r.Err() != nil {
		t.Fatalf("SellList has %d trailing bytes (err %v)", r.Remaining(), r.Err())
	}
	return adena, rows
}

// held is the live count of the stack objectID, 0 once it is gone.
func (w *sellWorld) held(t *testing.T, objectID int32) int {
	t.Helper()
	inst := w.srv.PlayerInventory(t, w.player).ItemByObjectID(objectID)
	if inst == nil {
		return 0
	}
	return inst.CountValue()
}

func (w *sellWorld) adena(t *testing.T) int {
	t.Helper()
	return w.srv.PlayerInventory(t, w.player).Adena()
}

// savedCounts flushes item writes and returns the persisted count of each
// of the player's rows by object id.
func (w *sellWorld) savedCounts(t *testing.T) map[int32]int {
	t.Helper()
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	out := map[int32]int{}
	for _, row := range rows {
		out[row.ObjectID] = row.Count
	}
	return out
}

func (w *sellWorld) requireSilent(t *testing.T, what string) {
	t.Helper()
	if frame := w.c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s answered %#x, want nothing", what, frame[0])
	}
}

// TestSellWindowListsSellableItems pins the Sell command: the sell window
// shows the player's adena and every sellable item not worn, at half its
// reference price, then the dispatcher's ActionFailed. The worn tunic, the
// unsellable soulshot and adena are left out.
func TestSellWindowListsSellableItems(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{swordID, 1}, [2]int32{potionID, 10}, [2]int32{soulshotID, 50}, [2]int32{tunicID, 1})
	w.c.Send(encodeUseItem(w.items[tunicID]))
	drainUntilQuiet(t, w.c)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, f)

	frames := w.bypass(t, npcCommand(f, "Sell"))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSellList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Sell = %x, want SellList, ActionFailed", got)
	}
	adena, rows := decodeSellList(t, frames[0])
	if adena != startAdena {
		t.Fatalf("SellList adena = %d, want %d", adena, startAdena)
	}
	want := map[int32]sellListRow{
		w.items[swordID]:  {itemID: swordID, count: 1, price: 500},
		w.items[potionID]: {itemID: potionID, count: 10, price: 20},
	}
	if len(rows) != len(want) {
		t.Fatalf("SellList rows = %v, want %v", rows, want)
	}
	for objectID, row := range want {
		if rows[objectID] != row {
			t.Fatalf("SellList row %d = %+v, want %+v", objectID, rows[objectID], row)
		}
	}
}

// TestSellWindowWithNothingToSell pins the empty sell window: the NPC's
// empty page in its own folder when it has one (a fisherman reads
// fisherman/, and takes the command in any case), else an empty SellList.
func TestSellWindowWithNothingToSell(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{soulshotID, 5})
	withPage := w.spawnFolk(t, folkTemplate("Merchant", 30003), 50)
	fisher := w.spawnFolk(t, folkTemplate("Fisherman", fisherID), 50)
	without := w.spawnFolk(t, folkTemplate("Merchant", 30002), 50)

	w.talkTo(t, withPage)
	assertAnswer(t, w.bypass(t, npcCommand(withPage, "Sell")), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, withPage, wantChatPage(emptyPage, withPage))
	w.talkTo(t, fisher)
	assertAnswer(t, w.bypass(t, npcCommand(fisher, "sell")), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, fisher, wantChatPage(fisherEmpty, fisher))

	w.talkTo(t, without)
	frames := w.bypass(t, npcCommand(without, "Sell"))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSellList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Sell = %x, want SellList, ActionFailed", got)
	}
	if adena, rows := decodeSellList(t, frames[0]); adena != startAdena || len(rows) != 0 {
		t.Fatalf("SellList = adena %d rows %v, want %d and none", adena, rows, startAdena)
	}
}

// TestSellItemsPaysHalfTheReferencePrice sells a sword, part of a potion
// stack, the worn tunic, an unsellable soulshot stack and a missing item
// to the targeted merchant. The sellable rows go for half their reference
// price each, paid without a message; the others are skipped. The sold page
// follows, the inventory update reports the change, and the rows persist.
func TestSellItemsPaysHalfTheReferencePrice(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{swordID, 1}, [2]int32{potionID, 10}, [2]int32{soulshotID, 50}, [2]int32{tunicID, 1})
	w.c.Send(encodeUseItem(w.items[tunicID]))
	drainUntilQuiet(t, w.c)
	w.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, w.c)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.selectFolk(t, f)

	w.c.Send(encodeRequestSellItem(merchantID+listOffset,
		sellRow{w.items[swordID], swordID, 1},
		sellRow{w.items[potionID], potionID, 4},
		sellRow{w.items[soulshotID], soulshotID, 10},
		sellRow{w.items[tunicID], tunicID, 1},
		sellRow{999999, swordID, 1},
	))
	frames := drainFrames(t, w.c)
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("sale answered %x, want the sold page", opcodes(frames))
	}
	if objectID, html, _ := htmlMessage(t, page); objectID != f.ObjectID() || html != wantChatPage(soldPage, f) {
		t.Fatalf("sold page = %d %q, want %d %q", objectID, html, f.ObjectID(), wantChatPage(soldPage, f))
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage); ok {
		t.Fatalf("sale answered %x, want no system message", opcodes(frames))
	}

	wantAdena := startAdena + 500 + 4*20 + 150
	if got := w.adena(t); got != wantAdena {
		t.Fatalf("adena = %d, want %d", got, wantAdena)
	}
	wantHeld := map[int32]int{w.items[swordID]: 0, w.items[potionID]: 6, w.items[soulshotID]: 50, w.items[tunicID]: 0}
	for objectID, want := range wantHeld {
		if got := w.held(t, objectID); got != want {
			t.Fatalf("held %d = %d, want %d", objectID, got, want)
		}
	}
	w.srv.InventoryUpdates.Tick()
	if frames := drainFrames(t, w.c); !containsOpcode(frames, serverpackets.OpcodeInventoryUpdate) {
		t.Fatalf("after the sale = %x, want an InventoryUpdate", opcodes(frames))
	}

	saved := w.savedCounts(t)
	for objectID, want := range wantHeld {
		if got := saved[objectID]; got != want {
			t.Fatalf("saved %d = %d, want %d", objectID, got, want)
		}
	}
	if got := saved[w.items[item.AdenaID]]; got != wantAdena {
		t.Fatalf("saved adena = %d, want %d", got, wantAdena)
	}
}

// TestSellItemRefusals pins the silent refusals: a list id naming another
// merchant, a target that does not buy, a merchant out of reach, and rows
// paying more than the largest int32 in all (the earlier sword row
// included) change nothing and answer nothing. The selected enchant scroll
// is skipped while the rest of its sale goes through, and a sale paying
// exactly up to the cap is made.
func TestSellItemRefusals(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{swordID, 1}, [2]int32{potionID, 10}, [2]int32{scrollID, 1}, [2]int32{goldBarID, 3})
	near := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	far := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 1000)
	keeper := w.spawnFolk(t, folkTemplate("WarehouseKeeper", 30005), 50)
	sword := sellRow{w.items[swordID], swordID, 1}

	for _, tc := range []struct {
		name   string
		target *npc.Folk
		req    []byte
	}{
		{"another merchant's list", near, encodeRequestSellItem(30002+listOffset, sword)},
		{"a warehouse keeper", keeper, encodeRequestSellItem(0, sword)},
		{"a merchant out of reach", far, encodeRequestSellItem(0, sword)},
		{"a sale past the int32 cap", near, encodeRequestSellItem(0, sword, sellRow{w.items[goldBarID], goldBarID, 2}, sellRow{w.items[goldBarID], goldBarID, 1})},
	} {
		w.selectFolk(t, tc.target)
		w.c.Send(tc.req)
		w.requireSilent(t, tc.name)
		if got := w.held(t, w.items[swordID]); got != 1 {
			t.Fatalf("%s: sword held = %d, want kept", tc.name, got)
		}
		if got := w.adena(t); got != startAdena {
			t.Fatalf("%s: adena = %d, want %d", tc.name, got, startAdena)
		}
	}

	// near is still the target: selecting it again would talk to it.
	w.c.Send(encodeUseItem(w.items[scrollID]))
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeRequestSellItem(0, sellRow{w.items[scrollID], scrollID, 1}, sellRow{w.items[potionID], potionID, 1}))
	drainUntilQuiet(t, w.c)
	if scroll, potions := w.held(t, w.items[scrollID]), w.held(t, w.items[potionID]); scroll != 1 || potions != 9 {
		t.Fatalf("after selling the selected scroll and a potion: scroll %d potions %d, want 1 and 9", scroll, potions)
	}

	// Two gold bars pay 2,000,000,000, within the cap. The repeated row
	// asks for two of the one bar the first row left, so it is skipped
	// rather than counted against the three held, which would refuse the
	// whole sale as 4,000,000,000.
	bar := sellRow{w.items[goldBarID], goldBarID, 2}
	w.c.Send(encodeRequestSellItem(0, bar, bar))
	drainUntilQuiet(t, w.c)
	if bars, adena := w.held(t, w.items[goldBarID]), w.adena(t); bars != 1 || adena != startAdena+20+2000000000 {
		t.Fatalf("after selling two gold bars: bars %d adena %d, want 1 and %d", bars, adena, startAdena+20+2000000000)
	}
}

// TestSellItemSoldPageByTargetType pins the sold page per buyer type: a
// fisherman answers from fisherman/, not merchant/, and a mercenary
// manager buys but sends no page, even with a merchant/ page under its id.
func TestSellItemSoldPageByTargetType(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{potionID, 10})
	fisher := w.spawnFolk(t, folkTemplate("Fisherman", fisherID), 50)
	merc := w.spawnFolk(t, folkTemplate("MercenaryManagerNpc", mercID), 50)
	potion := sellRow{w.items[potionID], potionID, 1}

	w.selectFolk(t, fisher)
	w.c.Send(encodeRequestSellItem(0, potion))
	frames := drainFrames(t, w.c)
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("sale to the fisherman answered %x, want its sold page", opcodes(frames))
	}
	if objectID, html, _ := htmlMessage(t, page); objectID != fisher.ObjectID() || html != wantChatPage(fisherSold, fisher) {
		t.Fatalf("fisherman sold page = %d %q, want %d %q", objectID, html, fisher.ObjectID(), wantChatPage(fisherSold, fisher))
	}
	if potions, adena := w.held(t, w.items[potionID]), w.adena(t); potions != 9 || adena != startAdena+20 {
		t.Fatalf("after selling the fisherman a potion: potions %d adena %d, want 9 and %d", potions, adena, startAdena+20)
	}

	w.selectFolk(t, merc)
	w.c.Send(encodeRequestSellItem(0, potion))
	frames = drainFrames(t, w.c)
	if containsOpcode(frames, serverpackets.OpcodeNpcHtmlMessage) {
		t.Fatalf("sale to the mercenary manager answered %x, want no page", opcodes(frames))
	}
	if potions, adena := w.held(t, w.items[potionID]), w.adena(t); potions != 8 || adena != startAdena+40 {
		t.Fatalf("after selling the mercenary manager a potion: potions %d adena %d, want 8 and %d", potions, adena, startAdena+40)
	}
}

func containsOpcode(frames [][]byte, opcode byte) bool {
	_, ok := firstOpcode(frames, opcode)
	return ok
}
