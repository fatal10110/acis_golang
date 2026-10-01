package npcs

import (
	"context"
	"database/sql"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The symbols these scenarios draw, from the shipped hennas.xml: 1, 2 and 3
// are open to the Warrior (class 1, a first class with two symbol slots),
// 7 is not.
const (
	symbolMakerID    = 30613
	warriorClass     = 1
	dyeSTRCON        = 4445 // symbol 1: STR +1, CON -3, 37000 adena
	dyeSTRDEX        = 4446 // symbol 2: STR +1, DEX -3, 37000 adena
	dyeCONSTR        = 4447 // symbol 3: STR -3, CON +1, 37000 adena
	dyeINTMEN        = 4451 // symbol 7: INT +1, MEN -3, mystic classes only
	symbolPrice      = 37000
	removePrice      = symbolPrice / henna.RemoveAmount
	symbolStartAdena = 4 * symbolPrice
)

// symbolMakerPage is the shipped symbolmaker/SymbolMaker.htm links.
const symbolMakerPage = `<html><body>Symbols<br>` +
	`<a action="bypass -h npc_%objectId%_Draw">Draw a Symbol.</a><br>` +
	`<a action="bypass -h npc_%objectId%_RemoveList">Delete a Symbol.</a></body></html>`

// shippedHennas loads the datapack's hennas.xml, the oracle table the
// symbol windows list from.
func shippedHennas(t *testing.T) *henna.Table {
	t.Helper()
	table, err := gamexml.LoadHennas(datapack.Path(t, "data", "xml", "hennas.xml"))
	if err != nil {
		t.Fatalf("load hennas.xml: %v", err)
	}
	return table
}

// dyeTemplates is the behavior catalog plus the four dyes.
func dyeTemplates() *item.Table {
	all := gameservertest.ItemTemplates().All()
	for _, id := range []int32{dyeSTRCON, dyeSTRDEX, dyeCONSTR, dyeINTMEN} {
		all = append(all, &item.Template{
			ID: id, Name: "Dye", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return item.NewTable(all)
}

// bootSymbolWorld enters the world as a Warrior holding adena and
// dyeCounts of each dye.
func bootSymbolWorld(t *testing.T, adena int32, dyeCounts map[int32]int32) *folkWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(map[string]string{"symbolmaker/SymbolMaker.htm": symbolMakerPage}),
		gameservertest.WithHennaTable(shippedHennas(t)),
		gameservertest.WithItemTemplates(dyeTemplates()),
		noBypassReuse,
		gameservertest.WithHennaSeed(func(db *sql.DB, _ *gamesql.HennaStore) {
			if _, err := db.ExecContext(context.Background(), `UPDATE characters SET classid = ?`, warriorClass); err != nil {
				t.Fatalf("set classid: %v", err)
			}
		}),
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	for id, count := range dyeCounts {
		srv.GiveItem(t, w.player, id, count)
	}
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at.X, w.at.Y, w.at.Z = x, y, z
	return w
}

func encodeSymbolRequest(opcode byte, value int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(value)
	return w.Bytes()
}

// send sends payload and returns the answer without the batched
// InventoryUpdate, which trails the handler on its own tick.
func (w *folkWorld) send(t *testing.T, payload []byte) [][]byte {
	t.Helper()
	w.c.Send(payload)
	var out [][]byte
	for _, f := range drainFrames(t, w.c) {
		if f[0] != serverpackets.OpcodeInventoryUpdate {
			out = append(out, f)
		}
	}
	return out
}

func le32(v ...int32) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, uint32(x))
	}
	return b
}

func sysMsg(id int32, params ...[]byte) []byte {
	out := append([]byte{serverpackets.OpcodeSystemMessage}, le32(id, int32(len(params)))...)
	for _, p := range params {
		out = append(out, p...)
	}
	return out
}

func numberParam(n int32) []byte { return le32(serverpackets.SystemMessageParamNumber, n) }
func itemNameParam(id int32) []byte {
	return le32(serverpackets.SystemMessageParamItemName, id)
}

func itemNumberParam(n int32) []byte {
	return le32(serverpackets.SystemMessageParamItemNumber, n)
}

// assertFrames checks frames are exactly want, byte for byte, except that a
// want of a lone opcode only matches the opcode.
func assertFrames(t *testing.T, what string, frames [][]byte, want ...[]byte) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("%s: answer = %x, want %d frames %x", what, opcodes(frames), len(want), want)
	}
	for i, f := range frames {
		if len(want[i]) == 1 {
			if f[0] != want[i][0] {
				t.Fatalf("%s: frame %d opcode = %#x, want %#x", what, i, f[0], want[i][0])
			}
			continue
		}
		if string(f) != string(want[i]) {
			t.Fatalf("%s: frame %d = %x, want %x", what, i, f, want[i])
		}
	}
}

type baseStats struct{ INT, STR, CON, MEN, DEX, WIT int }

func (w *folkWorld) baseStats(t *testing.T) baseStats {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player not online")
	}
	s, ok := obj.(interface {
		INT() int
		STR() int
		CON() int
		MEN() int
		DEX() int
		WIT() int
	})
	if !ok {
		t.Fatalf("live player %T has no base stats", obj)
	}
	return baseStats{s.INT(), s.STR(), s.CON(), s.MEN(), s.DEX(), s.WIT()}
}

func (w *folkWorld) held(t *testing.T, templateID int32) int {
	t.Helper()
	if inst := w.srv.PlayerInventory(t, w.player).ItemByTemplateID(templateID); inst != nil {
		return inst.CountValue()
	}
	return 0
}

func (w *folkWorld) savedHennas(t *testing.T) []henna.Row {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.Hennas.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatalf("list hennas: %v", err)
	}
	return rows
}

// statsInfo encodes a symbol window's stat block: each attribute's current
// value, then the low byte of its value after the change.
func statsInfo(now baseStats, d baseStats) []byte {
	var b []byte
	for _, p := range [][2]int{{now.INT, d.INT}, {now.STR, d.STR}, {now.CON, d.CON}, {now.MEN, d.MEN}, {now.DEX, d.DEX}, {now.WIT, d.WIT}} {
		b = append(b, le32(int32(p[0]))...)
		b = append(b, byte(p[0]+p[1]))
	}
	return b
}

// TestSymbolMakerDrawAndDelete walks the symbol maker end to end against
// the shipped hennas.xml: the RemoveList command with no symbol worn, the
// Draw window listing the class's symbols whose dye is carried, each draw
// refusal, two draws filling the Warrior's two slots, the deletion window
// and a deletion handing five dyes back, with the character_hennas rows
// following every change.
func TestSymbolMakerDrawAndDelete(t *testing.T) {
	t.Parallel()
	w := bootSymbolWorld(t, symbolStartAdena, map[int32]int32{dyeSTRCON: 25, dyeSTRDEX: 10, dyeCONSTR: 9, dyeINTMEN: 10})
	maker := w.spawnFolk(t, folkTemplate("SymbolMaker", symbolMakerID), 50)
	w.talkTo(t, maker)

	assertFrames(t, "RemoveList with no symbol", w.bypass(t, npcCommand(maker, "RemoveList")),
		sysMsg(serverpackets.SystemMessageSymbolNotFound), []byte{serverpackets.OpcodeActionFailed})

	equipList := func(adena int32) []byte {
		b := append([]byte{serverpackets.OpcodeHennaEquipList}, le32(adena, 2, 3)...)
		b = append(b, le32(1, dyeSTRCON, henna.DrawAmount, symbolPrice, 1)...)
		b = append(b, le32(2, dyeSTRDEX, henna.DrawAmount, symbolPrice, 1)...)
		return append(b, le32(3, dyeCONSTR, henna.DrawAmount, symbolPrice, 1)...)
	}
	assertFrames(t, "Draw", w.bypass(t, npcCommand(maker, "Draw")), equipList(symbolStartAdena), []byte{serverpackets.OpcodeActionFailed})
	assertFrames(t, "RequestHennaItemList", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaItemList, 0)), equipList(symbolStartAdena))

	now := w.baseStats(t)
	info := append([]byte{serverpackets.OpcodeHennaItemInfo}, le32(1, dyeSTRCON, henna.DrawAmount, symbolPrice, 1, symbolStartAdena)...)
	info = append(info, statsInfo(now, baseStats{STR: 1, CON: -3})...)
	assertFrames(t, "RequestHennaItemInfo 1", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaItemInfo, 1)), info)
	// A symbol the class may not draw still answers its details.
	info = append([]byte{serverpackets.OpcodeHennaItemInfo}, le32(7, dyeINTMEN, henna.DrawAmount, symbolPrice, 1, symbolStartAdena)...)
	info = append(info, statsInfo(now, baseStats{INT: 1, MEN: -3})...)
	assertFrames(t, "RequestHennaItemInfo 7", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaItemInfo, 7)), info)
	assertFrames(t, "RequestHennaItemInfo unknown", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaItemInfo, 999)))

	cantDraw := sysMsg(serverpackets.SystemMessageCantDrawSymbol)
	assertFrames(t, "draw another class's symbol", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 7)), cantDraw)
	assertFrames(t, "draw with 9 dyes", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 3)), cantDraw)
	assertFrames(t, "draw an unknown symbol", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 999)))
	if got := w.held(t, item.AdenaID); got != symbolStartAdena {
		t.Fatalf("adena after refusals = %d, want symbolStartAdena", got)
	}

	drawn := func(symbol, dye int32) [][]byte {
		return [][]byte{
			sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(symbolPrice)),
			sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(dye), itemNumberParam(henna.DrawAmount)),
			{serverpackets.OpcodeHennaInfo},
			{serverpackets.OpcodeUserInfo},
			sysMsg(serverpackets.SystemMessageSymbolAdded),
		}
	}
	frames := w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 1))
	assertFrames(t, "draw symbol 1", frames, drawn(1, dyeSTRCON)...)
	// STR +1, CON -3 with one of two slots taken.
	wantInfo := append([]byte{serverpackets.OpcodeHennaInfo, 0, 1, 0xfd, 0, 0, 0}, le32(2, 1, 1, 1)...)
	if string(frames[2]) != string(wantInfo) {
		t.Fatalf("HennaInfo after draw = %x, want %x", frames[2], wantInfo)
	}
	if got := w.baseStats(t); got.STR != now.STR+1 || got.CON != now.CON-3 {
		t.Fatalf("stats after draw = %+v, want STR %d CON %d", got, now.STR+1, now.CON-3)
	}
	if adena, dyes := w.held(t, item.AdenaID), w.held(t, dyeSTRCON); adena != symbolStartAdena-symbolPrice || dyes != 15 {
		t.Fatalf("after draw: adena %d dyes %d, want %d and 15", adena, dyes, symbolStartAdena-symbolPrice)
	}
	if rows := w.savedHennas(t); len(rows) != 1 || rows[0] != (henna.Row{Slot: 1, SymbolID: 1}) {
		t.Fatalf("saved hennas = %+v, want symbol 1 in slot 1", rows)
	}

	assertFrames(t, "draw symbol 2", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 2)), drawn(2, dyeSTRDEX)...)
	// Both slots taken: the full check comes before the dye check.
	assertFrames(t, "draw with the slots full", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 3)), sysMsg(serverpackets.SystemMessageSymbolsFull))
	// The draw window no longer lists symbol 2, its last dyes used up.
	adena := int32(symbolStartAdena - 2*symbolPrice)
	list := append([]byte{serverpackets.OpcodeHennaEquipList}, le32(adena, 2, 2)...)
	list = append(list, le32(1, dyeSTRCON, henna.DrawAmount, symbolPrice, 1)...)
	list = append(list, le32(3, dyeCONSTR, henna.DrawAmount, symbolPrice, 1)...)
	assertFrames(t, "draw window after two draws", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaItemList, 0)), list)

	unequipList := append([]byte{serverpackets.OpcodeHennaUnequipList}, le32(adena, 0, 2)...)
	unequipList = append(unequipList, le32(1, dyeSTRCON, henna.RemoveAmount, removePrice, 1)...)
	unequipList = append(unequipList, le32(2, dyeSTRDEX, henna.RemoveAmount, removePrice, 1)...)
	assertFrames(t, "RemoveList", w.bypass(t, npcCommand(maker, "RemoveList")), unequipList, []byte{serverpackets.OpcodeActionFailed})
	assertFrames(t, "RequestHennaUnequipList", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequipList, 0)), unequipList)

	now = w.baseStats(t)
	info = append([]byte{serverpackets.OpcodeHennaItemUnequipInfo}, le32(1, dyeSTRCON, henna.RemoveAmount, removePrice, 1, adena)...)
	info = append(info, statsInfo(now, baseStats{STR: -1, CON: 3})...)
	assertFrames(t, "RequestHennaUnequipInfo 1", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequipInfo, 1)), info)
	assertFrames(t, "RequestHennaUnequipInfo unknown", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequipInfo, 999)))

	frames = w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequip, 1))
	assertFrames(t, "delete symbol 1", frames,
		[]byte{serverpackets.OpcodeHennaInfo},
		[]byte{serverpackets.OpcodeUserInfo},
		sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(dyeSTRCON), itemNumberParam(henna.RemoveAmount)),
		sysMsg(serverpackets.SystemMessageSymbolDeleted))
	// STR +1, DEX -3 from symbol 2 alone, now in slot 2.
	wantInfo = append([]byte{serverpackets.OpcodeHennaInfo, 0, 1, 0, 0, 0xfd, 0}, le32(2, 1, 2, 2)...)
	if string(frames[0]) != string(wantInfo) {
		t.Fatalf("HennaInfo after delete = %x, want %x", frames[0], wantInfo)
	}
	if got, dyes := w.held(t, item.AdenaID), w.held(t, dyeSTRCON); got != int(adena-removePrice) || dyes != 20 {
		t.Fatalf("after delete: adena %d dyes %d, want %d and 20", got, dyes, adena-removePrice)
	}
	if rows := w.savedHennas(t); len(rows) != 1 || rows[0] != (henna.Row{Slot: 2, SymbolID: 2}) {
		t.Fatalf("saved hennas = %+v, want symbol 2 in slot 2", rows)
	}
	assertFrames(t, "delete a symbol not worn", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequip, 1)))

	// The freed first slot takes the next draw.
	assertFrames(t, "draw into the freed slot", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 1)), drawn(1, dyeSTRCON)...)
	if rows := w.savedHennas(t); len(rows) != 2 || rows[0] != (henna.Row{Slot: 1, SymbolID: 1}) {
		t.Fatalf("saved hennas = %+v, want symbol 1 back in slot 1", rows)
	}
}

// TestSymbolMakerAdenaRefusals pins YOU_NOT_ENOUGH_ADENA for a draw and a
// deletion the player cannot pay: nothing is taken, the symbol stays, and
// no dye is handed back.
func TestSymbolMakerAdenaRefusals(t *testing.T) {
	t.Parallel()
	w := bootSymbolWorld(t, symbolPrice+removePrice-1, map[int32]int32{dyeSTRCON: 10, dyeSTRDEX: 10})
	drainUntilQuiet(t, w.c)

	assertFrames(t, "draw symbol 1", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 1)),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(symbolPrice)),
		sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(dyeSTRCON), itemNumberParam(henna.DrawAmount)),
		[]byte{serverpackets.OpcodeHennaInfo},
		[]byte{serverpackets.OpcodeUserInfo},
		sysMsg(serverpackets.SystemMessageSymbolAdded))

	notEnough := sysMsg(serverpackets.SystemMessageYouNotEnoughAdena)
	assertFrames(t, "draw without the adena", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaEquip, 2)), notEnough)
	assertFrames(t, "delete without the adena", w.send(t, encodeSymbolRequest(clientpackets.OpcodeRequestHennaUnequip, 1)), notEnough)
	if adena, dyes2 := w.held(t, item.AdenaID), w.held(t, dyeSTRDEX); adena != removePrice-1 || dyes2 != 10 {
		t.Fatalf("after refusals: adena %d symbol-2 dyes %d, want %d and 10", adena, dyes2, removePrice-1)
	}
	if rows := w.savedHennas(t); len(rows) != 1 || rows[0] != (henna.Row{Slot: 1, SymbolID: 1}) {
		t.Fatalf("saved hennas = %+v, want symbol 1 in slot 1", rows)
	}
}
