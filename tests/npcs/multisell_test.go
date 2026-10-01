package npcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: MultisellData.separateAndSend (MultisellData.java:85-108)
// resolves a list by its file name's String.hashCode, refuses it to an NPC
// outside its <npcs>, prepares it (PreparedListContainer/PreparedEntry:
// adena ingredients merged last, tax ingredients only under a taxing castle,
// an inventory-only list built per unworn armor or weapon with its enchant
// kept on maintainEnchantment lists) and sends one MultiSellList per 40
// entries before remembering it. Npc.onBypassFeedback answers
// "multisell"/"exc_multisell" (Npc.java:1307-1314), Merchant.onBypassFeedback
// "Multisell", "Exc_Multisell", "Newbie_Exc_Multisell" and "Multisell_Shadow"
// (Merchant.java:88-127). MultiSellChoose.runImpl (MultiSellChoose.java:38-
// 337) forgets the list without a word on a reuse, amount, list, entry, NPC,
// reach or stackable refusal; answers WEIGHT_LIMIT_EXCEEDED, SLOTS_FULL,
// YOU_HAVE_EXCEEDED_QUANTITY_THAT_CAN_BE_INPUTTED, YOU_ARE_NOT_A_CLAN_MEMBER
// or NOT_ENOUGH_ITEMS keeping it; takes each ingredient with its
// S1_DISAPPEARED/S2_S1_DISAPPEARED, hands over each product with
// EARNED_ITEM_S1, EARNED_S2_S1_S or ACQUIRED_S1_S2, and closes with
// SUCCESSFULLY_TRADED_WITH_NPC. aCis revision in the outer repo.

const (
	msBladeID   = 9101 // a tradable weapon product
	msOreID     = 9102 // a stackable ingredient
	msAnvilID   = 9103 // a stackable product too heavy to carry
	msSwordID   = 30   // the fixture sword: tradable
	msBowID     = 14   // the fixture bow: neither tradable nor sellable
	msPotionID  = 20   // the fixture potion: stackable
	msClanRepID = 65336
	msFolkID    = 30100
)

// msListFiles are the multisell lists the suite loads, by file name.
var msListFiles = map[string]string{
	// 9001: the merchant's list, taxed by name only (no castle has an
	// owner), with a tax adena ingredient and adena placed first.
	"9001.xml": `<list applyTaxes="true"><npcs><npc>30001</npc></npcs>
		<item><ingredient id="57" count="100"/><ingredient id="9102" count="2"/>
			<production id="9101" count="1"/><ingredient id="57" count="1000" isTaxIngredient="true"/></item>
		<item><ingredient id="57" count="10"/><production id="20" count="5"/></item>
		<item><ingredient id="9102" count="1"/><production id="9103" count="1"/></item>
		<item><ingredient id="65336" count="10"/><production id="20" count="1"/></item>
		<item><ingredient id="9102" count="1" maintainIngredient="true"/><ingredient id="57" count="5"/>
			<production id="20" count="1"/></item>
	</list>`,
	// 9003: an exchange keeping enchantment.
	"9003.xml": `<list maintainEnchantment="true">
		<item><ingredient id="30" count="1"/><production id="9101" count="1"/></item>
		<item><ingredient id="14" count="1"/><production id="20" count="1"/></item>
	</list>`,
}

// msPagedList is 9002, open to every NPC, with 41 entries: two pages.
func msPagedList() string {
	body := "<list>"
	for range 41 {
		body += `<item><ingredient id="9102" count="1"/><production id="20" count="1"/></item>`
	}
	return body + "</list>"
}

func msCatalog() *item.Table {
	all := gameservertest.ItemTemplates().All()
	all = append(all,
		&item.Template{
			ID: msBladeID, Name: "Blade", Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
			Tradable: true, Dropable: true, Destroyable: true, Weapon: &item.WeaponDetail{Type: item.WeaponSword},
		},
		&item.Template{
			ID: msOreID, Name: "Ore", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Tradable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		},
		&item.Template{
			ID: msAnvilID, Name: "Anvil", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Weight: 10000000, EtcItem: &item.EtcItemDetail{},
		},
	)
	return item.NewTable(all)
}

func msLoadLists(t *testing.T, items *item.Table) gameservertest.Option {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"9002.xml": msPagedList()}
	for name, body := range msListFiles {
		files[name] = body
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`<?xml version="1.0" encoding="utf-8"?>`+"\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	table, err := gamexml.LoadMultiSellLists(dir, items)
	if err != nil {
		t.Fatal(err)
	}
	return gameservertest.WithMultisells(table)
}

// msWorld is a folkWorld whose player entered holding seeded stacks, with
// the merchant 30001 beside it.
type msWorld struct {
	*folkWorld
	merchant *npc.Folk
	items    map[int32]int32 // template id -> object id
}

// bootMultisell boots with the multisell catalog and lists, seeds each
// {template, count} stack, lets seed adjust the rows, enters the world and
// spawns the merchant.
func bootMultisell(t *testing.T, character gameservertest.Option, stacks [][2]int32, seed func(srv *gameservertest.Server, items map[int32]int32), extra ...gameservertest.Option) *msWorld {
	t.Helper()
	catalog := msCatalog()
	pages := dialogPages()
	pages["exchangelvlimit.htm"] = "<html><body>Too experienced %objectId%</body></html>"
	pages["common/shadow_item-lowlevel.htm"] = "<html><body>Shadow low %objectId%</body></html>"
	opts := append([]gameservertest.Option{
		character,
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(catalog),
		msLoadLists(t, catalog),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &msWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	for _, s := range stacks {
		w.items[s[0]] = srv.GiveItem(t, w.player, s[0], s[1])
	}
	if seed != nil {
		seed(srv, w.items)
	}
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at.X, w.at.Y, w.at.Z = x, y, z
	w.merchant = w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, w.merchant)
	return w
}

func msTalker() gameservertest.Option {
	return gameservertest.WithCharacter("Trader", playerLevel, 0)
}

// open sends f's multisell command and returns its answer.
func (w *msWorld) open(t *testing.T, f *npc.Folk, command string) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, npcCommand(f, command))
}

// mustOpen opens list name at the merchant and checks it arrived.
func (w *msWorld) mustOpen(t *testing.T, name string) []msPage {
	t.Helper()
	frames := w.open(t, w.merchant, "multisell "+name)
	pages := msPages(t, frames)
	if len(pages) == 0 {
		t.Fatalf("multisell %s = %x, want MultiSellList", name, opcodes(frames))
	}
	return pages
}

func (w *msWorld) choose(t *testing.T, name string, entry, amount int32) [][]byte {
	t.Helper()
	return w.chooseID(t, commons.LegacyStringHash(name), entry, amount)
}

func (w *msWorld) chooseID(t *testing.T, listID, entry, amount int32) [][]byte {
	t.Helper()
	pkt := wire.NewPacketWriter(clientpackets.OpcodeMultiSellChoose)
	pkt.WriteInt32(listID)
	pkt.WriteInt32(entry)
	pkt.WriteInt32(amount)
	w.c.Send(pkt.Bytes())
	return drainFrames(t, w.c)
}

// held is how many units of templateID the live inventory holds, worn or
// not.
func (w *msWorld) held(t *testing.T, templateID int32) int {
	t.Helper()
	return w.srv.PlayerInventory(t, w.player).ItemCount(templateID, -1, true)
}

type msItem struct {
	id, type2   int32
	bodyPart    int32
	count       int32
	enchant     int32
	isIngredent bool
}

type msEntry struct {
	number    int32
	stackable bool
	products  []msItem
	ins       []msItem
}

type msPage struct {
	listID, page, size int32
	finished           bool
	entries            []msEntry
}

// msPages decodes every MultiSellList among frames.
func msPages(t *testing.T, frames [][]byte) []msPage {
	t.Helper()
	var out []msPage
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeMultiSellList {
			continue
		}
		r := wire.NewReader(f[1:])
		p := msPage{listID: r.ReadInt32(), page: r.ReadInt32(), finished: r.ReadInt32() == 1}
		if size := r.ReadInt32(); size != 40 {
			t.Fatalf("MultiSellList page size = %d, want 40", size)
		}
		p.size = r.ReadInt32()
		for range p.size {
			e := msEntry{number: r.ReadInt32()}
			r.ReadInt32()
			r.ReadInt32()
			e.stackable = r.ReadUint8() == 1
			nProducts, nIngredients := int(r.ReadUint16()), int(r.ReadUint16())
			for range nProducts {
				it := msItem{id: int32(r.ReadUint16()), bodyPart: r.ReadInt32(), type2: int32(r.ReadUint16()), count: r.ReadInt32(), enchant: int32(r.ReadUint16())}
				r.ReadInt32()
				r.ReadInt32()
				e.products = append(e.products, it)
			}
			for range nIngredients {
				it := msItem{id: int32(r.ReadUint16()), type2: int32(r.ReadUint16()), count: r.ReadInt32(), enchant: int32(r.ReadUint16()), isIngredent: true}
				r.ReadInt32()
				r.ReadInt32()
				e.ins = append(e.ins, it)
			}
			p.entries = append(p.entries, e)
		}
		if err := r.Err(); err != nil || r.Remaining() != 0 {
			t.Fatalf("MultiSellList decode: %v, %d bytes left", err, r.Remaining())
		}
		out = append(out, p)
	}
	return out
}

// msMessage is one decoded SystemMessage: its id and parameter values.
type msMessage struct {
	id     int32
	params []int32
}

// msMessages decodes the SystemMessages among frames, in order. Every
// parameter the exchange sends is a number or an item name.
func msMessages(t *testing.T, frames [][]byte) []msMessage {
	t.Helper()
	var out []msMessage
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		m := msMessage{id: r.ReadInt32()}
		for n := r.ReadInt32(); n > 0; n-- {
			r.ReadInt32()
			m.params = append(m.params, r.ReadInt32())
		}
		out = append(out, m)
	}
	return out
}

func assertMessages(t *testing.T, frames [][]byte, want ...msMessage) {
	t.Helper()
	got := msMessages(t, frames)
	if len(got) != len(want) {
		t.Fatalf("system messages = %+v, want %+v (frames %x)", got, want, opcodes(frames))
	}
	for i := range want {
		if got[i].id != want[i].id || len(got[i].params) != len(want[i].params) {
			t.Fatalf("system messages = %+v, want %+v", got, want)
		}
		for j := range want[i].params {
			if got[i].params[j] != want[i].params[j] {
				t.Fatalf("system messages = %+v, want %+v", got, want)
			}
		}
	}
}

func msg(id int, params ...int32) msMessage { return msMessage{id: int32(id), params: params} }

var traded = msg(serverpackets.SystemMessageSuccessfullyTradedWithNpc)

// TestMultisellOpensPreparedPages pins the bypass side: a list opens as
// MultiSellList pages of 40 entries, numbered across the list, then the
// dispatcher's ActionFailed. The prepared entry drops its tax adena and
// moves its adena last. A list outside the NPC's <npcs>, an unknown name, a
// merchant command without a list and a non-merchant's capitalised command
// answer the ActionFailed alone; a merchant's command matches in any case.
func TestMultisellOpensPreparedPages(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), nil, nil)
	folk := w.spawnFolk(t, folkTemplate("Folk", msFolkID), 50)

	frames := w.open(t, w.merchant, "multisell 9002")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeMultiSellList, serverpackets.OpcodeMultiSellList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("multisell 9002 = %x, want two MultiSellList then ActionFailed", got)
	}
	pages := msPages(t, frames)
	id := commons.LegacyStringHash("9002")
	if p := pages[0]; p.listID != id || p.page != 1 || p.finished || p.size != 40 || p.entries[0].number != 1 || p.entries[39].number != 40 {
		t.Fatalf("page 1 = %+v", p)
	}
	if p := pages[1]; p.listID != id || p.page != 2 || !p.finished || p.size != 1 || p.entries[0].number != 41 {
		t.Fatalf("page 2 = %+v", p)
	}

	pages = msPages(t, w.open(t, w.merchant, "MULTISELL 9001"))
	first := pages[0].entries[0]
	wantIns := []msItem{
		{id: msOreID, type2: int32(item.SubCategoryOther), count: 2, isIngredent: true},
		{id: item.AdenaID, type2: int32(item.SubCategoryMoney), count: 100, isIngredent: true},
	}
	if len(pages) != 1 || pages[0].size != 5 || first.stackable || len(first.ins) != 2 || first.ins[0] != wantIns[0] || first.ins[1] != wantIns[1] {
		t.Fatalf("9001 = %+v, want entry 1 ingredients %+v", pages, wantIns)
	}
	if p := first.products[0]; p.id != msBladeID || p.bodyPart != int32(item.SlotRHand) || p.type2 != int32(item.SubCategoryWeapon) || p.count != 1 {
		t.Fatalf("9001 entry 1 product = %+v", p)
	}

	for _, tc := range []struct {
		f       *npc.Folk
		command string
	}{
		{folk, "multisell 9001"},
		{w.merchant, "multisell nosuchlist"},
		{w.merchant, "Multisell"},
		{folk, "Multisell 9002"},
	} {
		if got := opcodes(w.open(t, tc.f, tc.command)); string(got) != string(releaseOnly) {
			t.Fatalf("%s at npc %d = %x, want ActionFailed alone", tc.command, tc.f.NpcID(), got)
		}
	}
	if pages := msPages(t, w.open(t, folk, "multisell 9002")); len(pages) != 2 {
		t.Fatalf("multisell 9002 at a plain folk = %d pages, want 2", len(pages))
	}
}

// TestMultisellExchange pins a completed exchange: each ingredient taken
// in the prepared order with its message, each product handed over with
// its own, then SUCCESSFULLY_TRADED_WITH_NPC, the inventory and the item
// rows following.
func TestMultisellExchange(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 1000}, {msOreID, 10}}, nil)
	w.mustOpen(t, "9001")

	assertMessages(t, w.choose(t, "9001", 1, 1),
		msg(serverpackets.SystemMessageS2S1Disappeared, msOreID, 2),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 100),
		msg(serverpackets.SystemMessageEarnedItemS1, msBladeID),
		traded)
	assertMessages(t, w.choose(t, "9001", 2, 3),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 30),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 15),
		traded)
	for id, want := range map[int32]int{item.AdenaID: 870, msOreID: 8, msBladeID: 1, msPotionID: 15} {
		if got := w.held(t, id); got != want {
			t.Fatalf("held %d = %d, want %d", id, got, want)
		}
	}
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int32]int{}
	for _, row := range rows {
		got[row.TemplateID] += row.Count
	}
	for id, want := range map[int32]int{item.AdenaID: 870, msOreID: 8, msBladeID: 1, msPotionID: 15} {
		if got[id] != want {
			t.Fatalf("persisted %d = %d, want %d (rows %v)", id, got[id], want, got)
		}
	}
}

// TestMultisellChooseRefusals pins the refusals: a shortfall answers
// NOT_ENOUGH_ITEMS, a product too heavy WEIGHT_LIMIT_EXCEEDED, a full
// inventory SLOTS_FULL, a clanless player buying with clan reputation
// YOU_ARE_NOT_A_CLAN_MEMBER, each keeping the list; an amount outside 1 to
// 9999, another list's id, an entry out of range or several units of an
// entry with a non-stackable product answer nothing and forget the list,
// so a valid choice that follows answers nothing too.
func TestMultisellChooseRefusals(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 1000}, {msOreID, 10}}, nil)
	w.mustOpen(t, "9001")

	assertMessages(t, w.choose(t, "9001", 2, 101), msg(serverpackets.SystemMessageNotEnoughItems))
	assertMessages(t, w.choose(t, "9001", 3, 1), msg(serverpackets.SystemMessageWeightLimitExceeded))
	assertMessages(t, w.choose(t, "9001", 4, 1), msg(serverpackets.SystemMessageYouAreNotAClanMember))
	w.srv.SetInventorySlotLimit(t, w.player, 2)
	assertMessages(t, w.choose(t, "9001", 1, 1), msg(serverpackets.SystemMessageSlotsFull))
	w.srv.SetInventorySlotLimit(t, w.player, 100)

	for _, tc := range []struct {
		name                string
		listID, entry, many int32
	}{
		{"amount 0", commons.LegacyStringHash("9001"), 2, 0},
		{"amount 10000", commons.LegacyStringHash("9001"), 2, 10000},
		{"another list", commons.LegacyStringHash("9002"), 2, 1},
		{"entry 0", commons.LegacyStringHash("9001"), 0, 1},
		{"entry 6", commons.LegacyStringHash("9001"), 6, 1},
		{"two blades", commons.LegacyStringHash("9001"), 1, 2},
	} {
		w.mustOpen(t, "9001")
		if frames := w.chooseID(t, tc.listID, tc.entry, tc.many); len(frames) != 0 {
			t.Fatalf("%s = %x, want nothing", tc.name, opcodes(frames))
		}
		if frames := w.choose(t, "9001", 2, 1); len(frames) != 0 {
			t.Fatalf("valid choice after %s = %x, want nothing: the list is forgotten", tc.name, opcodes(frames))
		}
	}
	if got := w.held(t, item.AdenaID); got != 1000 {
		t.Fatalf("adena = %d after refusals, want 1000", got)
	}
}

// TestMultisellChooseNeedsTheListsNpcInReach pins the NPC gate: an
// exchange from a list another NPC opened, once the player selected an NPC
// the list is closed to, or with the NPC out of reach, answers nothing and
// forgets the list.
func TestMultisellChooseNeedsTheListsNpcInReach(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 1000}}, nil)
	folk := w.spawnFolk(t, folkTemplate("Folk", msFolkID), 60)
	far := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 1000)

	for _, other := range []*npc.Folk{folk, far} {
		w.talkTo(t, w.merchant)
		w.mustOpen(t, "9001")
		w.selectFolk(t, other)
		if frames := w.choose(t, "9001", 2, 1); len(frames) != 0 {
			t.Fatalf("choice at npc %d = %x, want nothing", other.NpcID(), opcodes(frames))
		}
		w.selectFolk(t, w.merchant)
		if frames := w.choose(t, "9001", 2, 1); len(frames) != 0 {
			t.Fatalf("choice back at the merchant = %x, want nothing: the list is forgotten", opcodes(frames))
		}
	}
	if got := w.held(t, item.AdenaID); got != 1000 {
		t.Fatalf("adena = %d, want 1000", got)
	}
}

// TestMultisellInventoryOnlyKeepsEnchantment pins exc_multisell on a list
// maintaining enchantment: one set of entries per unworn armor or weapon
// that may change hands (the untradable, unsellable bow is left out), at
// that item's enchant level; the exchange takes the item of exactly that
// level and hands over the product at it, carrying the item's augmentation,
// named with ACQUIRED_S1_S2.
func TestMultisellInventoryOnlyKeepsEnchantment(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{msSwordID, 1}, {msBowID, 1}}, func(srv *gameservertest.Server, items map[int32]int32) {
		rows, err := srv.Items.ListByOwner(context.Background(), srv.SoleObjectID(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ObjectID == items[msSwordID] {
				row.EnchantLevel = 3
				if err := srv.Items.Update(context.Background(), row); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
	aug := item.Augmentation{Attributes: 0x12345, SkillID: 3203, SkillLevel: 2}
	w.srv.PlayerInventory(t, w.player).ItemByObjectID(w.items[msSwordID]).SetAugmentation(&aug)

	pages := msPages(t, w.open(t, w.merchant, "Exc_Multisell 9003"))
	if len(pages) != 1 || pages[0].size != 1 {
		t.Fatalf("Exc_Multisell 9003 = %+v, want the sword's entry alone", pages)
	}
	if e := pages[0].entries[0]; e.number != 1 || e.ins[0].id != msSwordID || e.ins[0].enchant != 3 || e.products[0].id != msBladeID || e.products[0].enchant != 3 {
		t.Fatalf("entry = %+v, want sword +3 for blade +3", e)
	}

	assertMessages(t, w.choose(t, "9003", 1, 1),
		msg(serverpackets.SystemMessageS1Disappeared, msSwordID),
		msg(serverpackets.SystemMessageAcquiredS1S2, 3, msBladeID),
		traded)
	inv := w.srv.PlayerInventory(t, w.player)
	blade := inv.ItemByTemplateID(msBladeID)
	if blade == nil || w.held(t, msSwordID) != 0 {
		t.Fatalf("after the exchange: blade %v, swords %d", blade, w.held(t, msSwordID))
	}
	if st := blade.Snapshot(); st.EnchantLevel != 3 || st.Augmentation == nil || *st.Augmentation != aug {
		t.Fatalf("blade = +%d augmentation %v, want +3 %v", st.EnchantLevel, st.Augmentation, aug)
	}
	w.srv.FlushItems(t)
	var attributes, skillID int32
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT attributes, skill_id FROM augmentations WHERE item_oid = ?", blade.ObjectID).Scan(&attributes, &skillID); err != nil {
		t.Fatalf("blade augmentation row: %v", err)
	}
	if attributes != aug.Attributes || skillID != aug.SkillID {
		t.Fatalf("blade augmentation row = %d/%d, want %d/%d", attributes, skillID, aug.Attributes, aug.SkillID)
	}
}

// TestMultisellNewbieAndShadowPages pins the merchant's level-bound
// commands for a level 20 player: Newbie_Exc_Multisell opens for a level 6
// to 25 player who made at most the first occupation change, and
// Multisell_Shadow opens the shadow weapon page for the player's level.
func TestMultisellNewbieAndShadowPages(t *testing.T) {
	t.Parallel()
	young := bootMultisell(t, msTalker(), [][2]int32{{msSwordID, 1}}, nil)
	if pages := msPages(t, young.open(t, young.merchant, "Newbie_Exc_Multisell 9003")); len(pages) != 1 || pages[0].size != 1 {
		t.Fatalf("Newbie_Exc_Multisell at level %d = %+v, want the list", playerLevel, pages)
	}
	assertAnswer(t, young.open(t, young.merchant, "Multisell_Shadow"),
		[]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, young.merchant,
		wantChatPage("<html><body>Shadow low %objectId%</body></html>", young.merchant))
}

// TestMultisellNewbieExchangeRefusesVeterans pins Newbie_Exc_Multisell for
// a level 30 player: exchangelvlimit.htm as a chat window, and no list.
func TestMultisellNewbieExchangeRefusesVeterans(t *testing.T) {
	t.Parallel()
	old := bootMultisell(t, gameservertest.WithCharacter("Veteran", 30, 0), [][2]int32{{msSwordID, 1}}, nil)
	assertAnswer(t, old.open(t, old.merchant, "Newbie_Exc_Multisell 9003"), chatWindowAnswer, old.merchant,
		wantChatPage("<html><body>Too experienced %objectId%</body></html>", old.merchant))
}

// TestMultisellKeepsMaintainedIngredients pins BlacksmithUseRecipes off: an
// ingredient marked maintainIngredient is only looked for, once, and kept.
func TestMultisellKeepsMaintainedIngredients(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 100}, {msOreID, 1}}, nil, gameservertest.WithKeepMaintainedIngredients())
	w.mustOpen(t, "9001")
	assertMessages(t, w.choose(t, "9001", 5, 3),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 15),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 3),
		traded)
	if ore, adena := w.held(t, msOreID), w.held(t, item.AdenaID); ore != 1 || adena != 85 {
		t.Fatalf("ore %d adena %d, want 1 and 85", ore, adena)
	}
}

// TestMultisellReuseDelay pins MultisellTime: an exchange inside the reuse
// window answers nothing and forgets the list.
func TestMultisellReuseDelay(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 100}}, nil, gameservertest.WithMultisellDelay(time.Hour))
	w.mustOpen(t, "9001")
	assertMessages(t, w.choose(t, "9001", 2, 1),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 10),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 5),
		traded)
	w.mustOpen(t, "9001")
	if frames := w.choose(t, "9001", 2, 1); len(frames) != 0 {
		t.Fatalf("exchange inside the reuse window = %x, want nothing", opcodes(frames))
	}
	if got := w.held(t, item.AdenaID); got != 90 {
		t.Fatalf("adena = %d, want 90", got)
	}
}
