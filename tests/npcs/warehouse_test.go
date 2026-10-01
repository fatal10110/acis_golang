package npcs

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: WarehouseKeeper.onBypassFeedback (WarehouseKeeper.java:48-175):
// WithdrawP sets the private warehouse active and answers
// NO_ITEM_DEPOSITED_IN_WH when it is empty, else WarehouseWithdrawList(1)
// then ActionFailed; DepositP answers ActionFailed, starts the item-list
// hold and sends WarehouseDepositList(1) of the available, depositable
// items; WithdrawC/DepositC refuse a clanless player with
// YOU_DO_NOT_HAVE_THE_RIGHT_TO_USE_CLAN_WAREHOUSE /
// ONLY_LEVEL_1_CLAN_OR_HIGHER_CAN_USE_WAREHOUSE; WithdrawF and DepositF do
// nothing while AllowFreight is off, else answer ActionFailed, then
// NO_ITEM_DEPOSITED_IN_WH or WarehouseWithdrawList(4) of the own freight,
// and CHARACTER_DOES_NOT_EXIST or PackageToList of the account's other
// characters. RequestBypassToServer adds its closing ActionFailed.
// SendWarehouseDepositList.java:44-131 charges 30 adena a row once every
// row passes checkItemManipulation and the slots fit
// (YOU_HAVE_EXCEEDED_QUANTITY_THAT_CAN_BE_INPUTTED, YOU_NOT_ENOUGH_ADENA);
// SendWarehouseWithdrawList.java:44-136 checks SLOTS_FULL then
// WEIGHT_LIMIT_EXCEEDED; RequestPackageSend.java:45-146 sends to an account
// character's freight for FreightPrice a row behind allowTransaction
// (YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT), skipping rows checkItemManipulation
// refuses with their fee still charged. aCis revision in the outer repo.

const (
	keeperID      = 30005
	whAdena       = 1000
	depositFee    = 30
	freightPrice  = 1000
	freightAdena  = 5000
	quantityLimit = serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted
)

func warehousePages() map[string]string {
	pages := dialogPages()
	pages["warehouse/30005.htm"] = "<html><body>Keeper</body></html>"
	return pages
}

// whWorld is a folkWorld at a warehouse keeper, with the seeded stacks by
// template id.
type whWorld struct {
	*folkWorld
	keeper *npc.Folk
	items  map[int32]int32
}

// bootKeeper boots with the warehouse pages, seeds adena plus each
// {template, count} stack, enters the world, spawns a keeper in reach and
// talks to it.
func bootKeeper(t *testing.T, adena int32, stacks [][2]int32, extra ...gameservertest.Option) *whWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Keeper", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(warehousePages()),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &whWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	w.items[item.AdenaID] = srv.GiveItem(t, w.player, item.AdenaID, adena)
	for _, s := range stacks {
		w.items[s[0]] = srv.GiveItem(t, w.player, s[0], s[1])
	}
	w.enter(t)
	return w
}

// enter enters the world on w.c, spawns the keeper beside the player and
// opens the page admitting every dialog command.
func (w *whWorld) enter(t *testing.T) {
	t.Helper()
	w.c.Send(encodeRequestGameStart(0))
	enterFrom(t, w.srv, w.c)
	x, y, z := w.srv.PlayerPosition(t, w.player)
	w.at.X, w.at.Y, w.at.Z = x, y, z
	w.keeper = w.spawnFolk(t, folkTemplate("WarehouseKeeper", keeperID), 50)
	w.talkTo(t, w.keeper)
	w.openAnyNpcPage(t)
}

func (w *whWorld) command(t *testing.T, command string) [][]byte {
	t.Helper()
	return w.bypass(t, npcCommand(w.keeper, command))
}

func (w *whWorld) held(t *testing.T, objectID int32) int {
	t.Helper()
	inst := w.srv.PlayerInventory(t, w.player).ItemByObjectID(objectID)
	if inst == nil {
		return 0
	}
	return inst.CountValue()
}

func (w *whWorld) adena(t *testing.T) int {
	t.Helper()
	return w.srv.PlayerInventory(t, w.player).Adena()
}

// savedRow is one persisted item row.
type savedRow struct {
	templateID int32
	count      int
	loc        item.Location
}

// saved flushes item writes and returns ownerID's persisted rows by object
// id.
func saved(t *testing.T, srv *gameservertest.Server, ownerID int32) map[int32]savedRow {
	t.Helper()
	srv.FlushItems(t)
	rows, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	out := map[int32]savedRow{}
	for _, row := range rows {
		out[row.ObjectID] = savedRow{templateID: row.TemplateID, count: row.Count, loc: row.Location}
	}
	return out
}

// rowsAt returns the template id -> count of ownerID's rows at loc.
func rowsAt(rows map[int32]savedRow, loc item.Location) map[int32]int {
	out := map[int32]int{}
	for _, r := range rows {
		if r.loc == loc {
			out[r.templateID] += r.count
		}
	}
	return out
}

type whRow struct{ objectID, count int32 }

func encodeItemRows(opcode byte, rows []whRow) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

func encodeDeposit(rows ...whRow) []byte {
	return encodeItemRows(clientpackets.OpcodeSendWarehouseDeposit, rows)
}

func encodeWithdraw(rows ...whRow) []byte {
	return encodeItemRows(clientpackets.OpcodeSendWarehouseWithdraw, rows)
}

func encodePackageSend(objectID int32, rows ...whRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageSend)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

func encodePackageItemList(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// listRow is one item a warehouse or package window lists.
type listRow struct{ itemID, count int32 }

// readListItem reads one item row: augmentation marks the warehouse lists'
// trailing 8 bytes.
func readListItem(r *wire.Reader, augmentation bool) (int32, listRow) {
	r.ReadUint16() // type1
	objectID, itemID, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	r.ReadUint16() // type2
	r.ReadUint16() // custom type 1
	r.ReadInt32()  // body part
	r.ReadUint16() // enchant
	r.ReadUint16() // custom type 2
	r.ReadUint16()
	if again := r.ReadInt32(); again != objectID {
		return -1, listRow{}
	}
	if augmentation {
		r.ReadInt64()
	}
	return objectID, listRow{itemID: itemID, count: count}
}

// decodeWarehouseList reads a WarehouseDepositList or WarehouseWithdrawList
// frame: its warehouse type, the adena shown and each row in order.
func decodeWarehouseList(t *testing.T, frame []byte, opcode byte) (uint16, int32, []int32, map[int32]listRow) {
	t.Helper()
	if frame[0] != opcode {
		t.Fatalf("opcode = %#x, want %#x", frame[0], opcode)
	}
	r := wire.NewReader(frame[1:])
	whType, adena := r.ReadUint16(), r.ReadInt32()
	var order []int32
	rows := map[int32]listRow{}
	for n := int(r.ReadUint16()); n > 0; n-- {
		objectID, row := readListItem(r, true)
		order = append(order, objectID)
		rows[objectID] = row
	}
	if r.Remaining() != 0 || r.Err() != nil {
		t.Fatalf("warehouse list has %d trailing bytes (err %v)", r.Remaining(), r.Err())
	}
	return whType, adena, order, rows
}

func requireOpcodes(t *testing.T, what string, frames [][]byte, want ...byte) {
	t.Helper()
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("%s = %x, want %x", what, got, want)
	}
}

// requireMessage checks frames are exactly the system message msg, then
// the closing ActionFailed when release.
func requireMessage(t *testing.T, what string, frames [][]byte, msg int, release bool) {
	t.Helper()
	want := []byte{serverpackets.OpcodeSystemMessage}
	if release {
		want = append(want, serverpackets.OpcodeActionFailed)
	}
	requireOpcodes(t, what, frames, want...)
	if got := systemMessageID(frames[0]); got != int32(msg) {
		t.Fatalf("%s message = %d, want %d", what, got, msg)
	}
}

// TestPrivateWarehouseDepositAndWithdraw walks the private warehouse: an
// empty one refuses withdrawal; the deposit window lists what may be
// stored (not the worn tunic); a deposit charges 30 adena a row and moves
// the rows, splitting the potion stack; the withdrawal window lists them
// and a withdrawal brings them back, merging the potions. Every move is
// written.
func TestPrivateWarehouseDepositAndWithdraw(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}, {tunicID, 1}})
	w.c.Send(encodeUseItem(w.items[tunicID]))
	drainUntilQuiet(t, w.c)
	sword, potion := w.items[swordID], w.items[potionID]

	requireMessage(t, "WithdrawP of an empty warehouse", w.command(t, "WithdrawP"), serverpackets.SystemMessageNoItemDepositedInWH, true)

	frames := w.command(t, "DepositP")
	requireOpcodes(t, "DepositP", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
	whType, adena, _, rows := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseDepositList)
	if whType != 1 || adena != whAdena {
		t.Fatalf("deposit list type %d adena %d, want 1 and %d", whType, adena, whAdena)
	}
	want := map[int32]listRow{sword: {swordID, 1}, potion: {potionID, 10}, w.items[item.AdenaID]: {item.AdenaID, whAdena}}
	if len(rows) != len(want) {
		t.Fatalf("deposit list = %v, want %v", rows, want)
	}
	for objectID, row := range want {
		if rows[objectID] != row {
			t.Fatalf("deposit list row %d = %+v, want %+v", objectID, rows[objectID], row)
		}
	}

	w.c.Send(encodeDeposit(whRow{sword, 1}, whRow{potion, 4}))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("deposit answered %x, want no message", opcodes(frames))
	}
	if got := w.adena(t); got != whAdena-2*depositFee {
		t.Fatalf("adena after the deposit = %d, want %d", got, whAdena-2*depositFee)
	}
	if s, p := w.held(t, sword), w.held(t, potion); s != 0 || p != 6 {
		t.Fatalf("after the deposit: sword %d potions %d, want 0 and 6", s, p)
	}
	w.srv.InventoryUpdates.Tick()
	if frames := drainFrames(t, w.c); !containsOpcode(frames, serverpackets.OpcodeInventoryUpdate) {
		t.Fatalf("after the deposit = %x, want an InventoryUpdate", opcodes(frames))
	}
	db := saved(t, w.srv, w.player)
	if got := rowsAt(db, item.LocationWarehouse); got[swordID] != 1 || got[potionID] != 4 || len(got) != 2 {
		t.Fatalf("saved warehouse rows = %v, want the sword and 4 potions", got)
	}
	if db[sword].loc != item.LocationWarehouse {
		t.Fatalf("saved sword row at %v, want moved whole to the warehouse", db[sword].loc)
	}
	if got := rowsAt(db, item.LocationInventory); got[potionID] != 6 || got[item.AdenaID] != whAdena-2*depositFee {
		t.Fatalf("saved inventory rows = %v, want 6 potions and %d adena", got, whAdena-2*depositFee)
	}

	frames = w.command(t, "WithdrawP")
	requireOpcodes(t, "WithdrawP", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	whType, adena, _, rows = decodeWarehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList)
	if whType != 1 || adena != whAdena-2*depositFee || len(rows) != 2 || rows[sword] != (listRow{swordID, 1}) {
		t.Fatalf("withdraw list = type %d adena %d %v, want 1, %d, the sword and 4 potions", whType, adena, rows, whAdena-2*depositFee)
	}
	var stored int32
	for objectID, row := range rows {
		if row == (listRow{potionID, 4}) {
			stored = objectID
		}
	}
	if stored == 0 {
		t.Fatalf("withdraw list = %v, want 4 potions", rows)
	}

	w.c.Send(encodeWithdraw(whRow{stored, 4}, whRow{sword, 1}))
	drainUntilQuiet(t, w.c)
	if s, p := w.held(t, sword), w.held(t, potion); s != 1 || p != 10 {
		t.Fatalf("after the withdrawal: sword %d potions %d, want 1 and 10", s, p)
	}
	db = saved(t, w.srv, w.player)
	if got := rowsAt(db, item.LocationWarehouse); len(got) != 0 {
		t.Fatalf("saved warehouse rows = %v, want none", got)
	}
	if got := rowsAt(db, item.LocationInventory); got[potionID] != 10 || got[swordID] != 1 {
		t.Fatalf("saved inventory rows = %v, want the sword and 10 potions", got)
	}
	requireMessage(t, "WithdrawP of the emptied warehouse", w.command(t, "WithdrawP"), serverpackets.SystemMessageNoItemDepositedInWH, true)
}

// TestWarehouseRestoredAtLogin seeds a warehouse row and a freight row
// before login: the private warehouse and the own freight list them.
func TestWarehouseRestoredAtLogin(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Keeper", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(warehousePages()),
		noBypassReuse,
	)
	w := &whWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	stored, shipped := srv.NewObjectID(), srv.NewObjectID()
	for _, inst := range []item.Instance{
		{ObjectID: stored, TemplateID: potionID, OwnerID: w.player, Count: 7, Location: item.LocationWarehouse},
		{ObjectID: shipped, TemplateID: swordID, OwnerID: w.player, Count: 1, Location: item.LocationFreight},
	} {
		if err := srv.Items.Create(context.Background(), w.player, inst); err != nil {
			t.Fatalf("seed item: %v", err)
		}
	}
	w.enter(t)

	frames := w.command(t, "WithdrawP")
	requireOpcodes(t, "WithdrawP", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	if _, _, _, rows := decodeWarehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList); len(rows) != 1 || rows[stored] != (listRow{potionID, 7}) {
		t.Fatalf("warehouse = %v, want the 7 seeded potions", rows)
	}
	frames = w.command(t, "WithdrawF")
	requireOpcodes(t, "WithdrawF", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed)
	whType, _, _, rows := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseWithdrawList)
	if whType != 4 || len(rows) != 1 || rows[shipped] != (listRow{swordID, 1}) {
		t.Fatalf("freight = type %d %v, want 4 and the seeded sword", whType, rows)
	}
	w.c.Send(encodeWithdraw(whRow{shipped, 1}))
	drainUntilQuiet(t, w.c)
	if got := w.held(t, shipped); got != 1 {
		t.Fatalf("sword held after the freight withdrawal = %d, want 1", got)
	}
	if got := saved(t, srv, w.player)[shipped].loc; got != item.LocationInventory {
		t.Fatalf("saved sword row at %v, want the inventory", got)
	}
}

// TestWarehouseDepositRefusals pins the deposit's refusals: rows over the
// warehouse size, a fee over the adena, a row naming an item not held, and
// a request with no keeper in reach each change nothing; only the first
// two answer.
func TestWarehouseDepositRefusals(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, 50, [][2]int32{{swordID, 1}, {potionID, 10}},
		gameservertest.WithStorageSlots(player.StorageSlots{WarehouseNoDwarf: 1, WarehouseDwarf: 1, Freight: 20, PrivateStoreNoDwarf: 4, PrivateStoreDwarf: 5, DwarfRecipe: 50, CommonRecipe: 50}))
	sword, potion := w.items[swordID], w.items[potionID]
	unchanged := func(what string) {
		t.Helper()
		if s, p, a := w.held(t, sword), w.held(t, potion), w.adena(t); s != 1 || p != 10 || a != 50 {
			t.Fatalf("%s: sword %d potions %d adena %d, want 1, 10 and 50", what, s, p, a)
		}
	}

	w.c.Send(encodeDeposit(whRow{sword, 1}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("deposit before a warehouse is opened answered %x, want nothing", opcodes(frames))
	}
	unchanged("deposit before a warehouse is opened")

	w.command(t, "DepositP")
	w.c.Send(encodeDeposit(whRow{sword, 1}, whRow{potion, 1}))
	requireMessage(t, "deposit over the warehouse size", drainFrames(t, w.c), int(quantityLimit), false)
	unchanged("deposit over the warehouse size")

	// 25 of the 50 adena deposited leaves 25 for a fee of 30.
	w.c.Send(encodeDeposit(whRow{w.items[item.AdenaID], 25}))
	requireMessage(t, "deposit short of the fee", drainFrames(t, w.c), serverpackets.SystemMessageYouNotEnoughAdena, false)
	unchanged("deposit short of the fee")

	w.c.Send(encodeDeposit(whRow{potion, 11}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("deposit of more than held answered %x, want nothing", opcodes(frames))
	}
	unchanged("deposit of more than held")

	far := w.spawnFolk(t, folkTemplate("WarehouseKeeper", keeperID), 1000)
	w.selectFolk(t, far)
	w.c.Send(encodeDeposit(whRow{potion, 1}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("deposit at a keeper out of reach answered %x, want nothing", opcodes(frames))
	}
	unchanged("deposit at a keeper out of reach")

	w.selectFolk(t, w.keeper)
	w.c.Send(encodeDeposit(whRow{potion, 1}))
	drainUntilQuiet(t, w.c)
	if p, a := w.held(t, potion), w.adena(t); p != 9 || a != 50-depositFee {
		t.Fatalf("deposit at the keeper: potions %d adena %d, want 9 and %d", p, a, 50-depositFee)
	}
}

// TestWarehouseWithdrawRefusals pins the withdrawal's refusals: a row
// naming more than stored answers nothing, and rows over the free
// inventory slots SLOTS_FULL. Nothing leaves the warehouse.
func TestWarehouseWithdrawRefusals(t *testing.T) {
	t.Parallel()
	catalog := gameservertest.ItemTemplates().All()
	for _, tmpl := range catalog {
		if tmpl.ID == swordID {
			tmpl.Weight = 1000
		}
	}
	w := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}}, gameservertest.WithItemTemplates(item.NewTable(catalog)))
	w.command(t, "DepositP")
	w.c.Send(encodeDeposit(whRow{w.items[swordID], 1}, whRow{w.items[potionID], 10}))
	drainUntilQuiet(t, w.c)
	w.command(t, "WithdrawP")
	potion := w.items[potionID]
	w.c.Send(encodeWithdraw(whRow{potion, 11}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("withdrawal of more than stored answered %x, want nothing", opcodes(frames))
	}
	w.srv.SetInventorySlotLimit(t, w.player, 1) // adena takes the one slot
	w.c.Send(encodeWithdraw(whRow{potion, 10}))
	requireMessage(t, "withdrawal into a full inventory", drainFrames(t, w.c), serverpackets.SystemMessageSlotsFull, false)
	if got := w.held(t, potion); got != 0 {
		t.Fatalf("potions held after the refused withdrawal = %d, want 0", got)
	}
}

// TestWarehouseWithdrawOverWeight pins WEIGHT_LIMIT_EXCEEDED: with a
// weight limit of 0, the stored sword weighs more than the player may
// carry, so nothing leaves the warehouse.
func TestWarehouseWithdrawOverWeight(t *testing.T) {
	t.Parallel()
	catalog := gameservertest.ItemTemplates().All()
	for _, tmpl := range catalog {
		if tmpl.ID == swordID {
			tmpl.Weight = 1000
		}
	}
	heavy := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}},
		gameservertest.WithItemTemplates(item.NewTable(catalog)), gameservertest.WithWeightLimitMultiplier(0))
	heavy.command(t, "DepositP")
	heavy.c.Send(encodeDeposit(whRow{heavy.items[swordID], 1}, whRow{heavy.items[potionID], 10}))
	drainUntilQuiet(t, heavy.c)
	heavy.command(t, "WithdrawP")
	heavy.c.Send(encodeWithdraw(whRow{heavy.items[potionID], 10}, whRow{heavy.items[swordID], 1}))
	requireMessage(t, "withdrawal over the weight limit", drainFrames(t, heavy.c), serverpackets.SystemMessageWeightLimitExceeded, false)
	if s, p := heavy.held(t, heavy.items[swordID]), heavy.held(t, heavy.items[potionID]); s != 0 || p != 0 {
		t.Fatalf("after the overweight withdrawal: sword %d potions %d, want both stored", s, p)
	}
}

// TestClanWarehouseRefusesClanlessPlayer pins the clan commands for a
// player in no clan.
func TestClanWarehouseRefusesClanlessPlayer(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, nil)
	requireMessage(t, "WithdrawC", w.command(t, "WithdrawC"), serverpackets.SystemMessageNoRightToUseClanWarehouse, true)
	requireMessage(t, "DepositC", w.command(t, "DepositC"), serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse, true)
}

// TestFreightCommandsWithoutFreight pins the freight commands of a player
// alone on the account with nothing shipped.
func TestFreightCommandsWithoutFreight(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, nil)
	frames := w.command(t, "WithdrawF")
	requireOpcodes(t, "WithdrawF of an empty freight", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed)
	if got := systemMessageID(frames[1]); got != serverpackets.SystemMessageNoItemDepositedInWH {
		t.Fatalf("WithdrawF message = %d, want NO_ITEM_DEPOSITED_IN_WH", got)
	}
	frames = w.command(t, "DepositF")
	requireOpcodes(t, "DepositF alone on the account", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed)
	if got := systemMessageID(frames[1]); got != serverpackets.SystemMessageCharacterDoesNotExist {
		t.Fatalf("DepositF message = %d, want CHARACTER_DOES_NOT_EXIST", got)
	}
}

// TestFreightCommandsWithFreightOff pins WithdrawF and DepositF while the
// freight service is off: only the closing ActionFailed.
func TestFreightCommandsWithFreightOff(t *testing.T) {
	t.Parallel()
	off := bootKeeper(t, whAdena, nil, gameservertest.WithFreight(network.FreightConfig{Allow: false, RegionBased: true, Price: freightPrice}))
	requireOpcodes(t, "WithdrawF with freight off", off.command(t, "WithdrawF"), serverpackets.OpcodeActionFailed)
	requireOpcodes(t, "DepositF with freight off", off.command(t, "DepositF"), serverpackets.OpcodeActionFailed)
}

// freightWorld is a sender in the world at a keeper, with a second
// character on its account to ship to.
type freightWorld struct {
	*whWorld
	receiver *player.Character
}

func bootFreight(t *testing.T, adena int32, stacks [][2]int32, extra ...gameservertest.Option) *freightWorld {
	t.Helper()
	return bootFreightPrepared(t, nil, adena, stacks, extra...)
}

// bootFreightPrepared is bootFreight running prepare, when set, once both
// characters exist and before the sender enters the world.
func bootFreightPrepared(t *testing.T, prepare func(srv *gameservertest.Server, sender int32, receiver *player.Character), adena int32, stacks [][2]int32, extra ...gameservertest.Option) *freightWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Sender", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(warehousePages()),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &whWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}, items: map[int32]int32{}}
	receiver := srv.SeedCharacter(t, "Receiver", playerLevel, 0)
	w.items[item.AdenaID] = srv.GiveItem(t, w.player, item.AdenaID, adena)
	for _, s := range stacks {
		w.items[s[0]] = srv.GiveItem(t, w.player, s[0], s[1])
	}
	if prepare != nil {
		prepare(srv, w.player, receiver)
	}
	w.enter(t)
	return &freightWorld{whWorld: w, receiver: receiver}
}

// TestPackageSendShipsToAccountCharacter walks a package: DepositF lists
// the account's other character, the sendable list offers the carried
// tradable items, and a package charges FreightPrice a row and moves the
// rows into that character's freight, which it withdraws at its next
// login.
func TestPackageSendShipsToAccountCharacter(t *testing.T) {
	t.Parallel()
	w := bootFreight(t, freightAdena, [][2]int32{{swordID, 1}, {potionID, 10}, {soulshotID, 5}})
	sword, potion := w.items[swordID], w.items[potionID]

	frames := w.command(t, "DepositF")
	requireOpcodes(t, "DepositF", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodePackageToList, serverpackets.OpcodeActionFailed)
	r := wire.NewReader(frames[1][1:])
	if n, id, name := r.ReadInt32(), r.ReadInt32(), r.ReadString(); n != 1 || id != w.receiver.ID || name != "Receiver" {
		t.Fatalf("PackageToList = %d [%d %q], want the receiver", n, id, name)
	}

	w.c.Send(encodePackageItemList(w.receiver.ID))
	frames = drainFrames(t, w.c)
	requireOpcodes(t, "RequestPackageSendableItemList", frames, serverpackets.OpcodePackageSendableList)
	r = wire.NewReader(frames[0][1:])
	if target, adena := r.ReadInt32(), r.ReadInt32(); target != w.receiver.ID || adena != freightAdena {
		t.Fatalf("PackageSendableList target %d adena %d, want %d and %d", target, adena, w.receiver.ID, freightAdena)
	}
	var offered []int32
	for n := r.ReadInt32(); n > 0; n-- {
		objectID, _ := readListItem(r, false)
		offered = append(offered, objectID)
	}
	for _, want := range []int32{sword, potion, w.items[item.AdenaID]} {
		if !slices.Contains(offered, want) {
			t.Fatalf("PackageSendableList offers %v, want %d among them", offered, want)
		}
	}

	w.c.Send(encodePackageSend(w.receiver.ID, whRow{sword, 1}, whRow{potion, 3}))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("package answered %x, want no message", opcodes(frames))
	}
	if s, p, a := w.held(t, sword), w.held(t, potion), w.adena(t); s != 0 || p != 7 || a != freightAdena-2*freightPrice {
		t.Fatalf("after the package: sword %d potions %d adena %d, want 0, 7 and %d", s, p, a, freightAdena-2*freightPrice)
	}
	if got := rowsAt(saved(t, w.srv, w.receiver.ID), item.LocationFreight); got[swordID] != 1 || got[potionID] != 3 || len(got) != 2 {
		t.Fatalf("receiver's saved freight = %v, want the sword and 3 potions", got)
	}

	w.c.Send(encodePackageSend(w.receiver.ID, whRow{potion, 1}, whRow{potion, 1}, whRow{potion, 1}, whRow{potion, 1}))
	requireMessage(t, "package short of the fee", drainFrames(t, w.c), serverpackets.SystemMessageYouNotEnoughAdena, false)

	// The receiver logs in on the account, once the sender's session has
	// left, and collects its freight.
	rw := w.loginReceiver(t)
	c := rw.c
	frames = rw.command(t, "WithdrawF")
	requireOpcodes(t, "receiver's WithdrawF", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed)
	_, _, _, rows := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseWithdrawList)
	if len(rows) != 2 || rows[sword] != (listRow{swordID, 1}) {
		t.Fatalf("receiver's freight = %v, want the sword and 3 potions", rows)
	}
	moves := make([]whRow, 0, 2)
	for objectID, row := range rows {
		moves = append(moves, whRow{objectID, row.count})
	}
	c.Send(encodeWithdraw(moves...))
	drainUntilQuiet(t, c)
	if got := rowsAt(saved(t, w.srv, w.receiver.ID), item.LocationInventory); got[swordID] != 1 || got[potionID] != 3 {
		t.Fatalf("receiver's saved inventory = %v, want the sword and 3 potions", got)
	}
}

// loginReceiver takes the account over on a second client, waits for the
// sender's session to leave, enters the receiver and talks to a keeper
// beside it.
func (w *freightWorld) loginReceiver(t *testing.T) *whWorld {
	t.Helper()
	c := w.srv.DialClient(t, w.srv.Account(), 2)
	w.srv.AdvanceUntil(t, "sender out of the world", func() bool {
		_, ok := w.srv.State.Player(w.player)
		return !ok
	})
	chars, err := w.srv.Chars.ListByAccount(context.Background(), w.srv.Account())
	if err != nil {
		t.Fatalf("list characters: %v", err)
	}
	slot := slices.IndexFunc(chars, func(ch *player.Character) bool { return ch.ID == w.receiver.ID })
	rw := &whWorld{folkWorld: &folkWorld{srv: w.srv, c: c, player: w.receiver.ID}, items: map[int32]int32{}}
	c.Send(encodeRequestGameStart(int32(slot)))
	enterFrom(t, w.srv, c)
	x, y, z := w.srv.PlayerPosition(t, rw.player)
	rw.at.X, rw.at.Y, rw.at.Z = x, y, z
	rw.keeper = rw.spawnFolk(t, folkTemplate("WarehouseKeeper", keeperID), 60)
	rw.talkTo(t, rw.keeper)
	rw.openAnyNpcPage(t)
	return rw
}

// enterFrom finishes a character selection on c and drains the enter
// burst. It waits for the server to finish handling EnterWorld first: on
// the wall clock (the real pool) the login reads the character's rows
// before it sends the burst's first frame, and a quiet spell then can end
// the drain before the player is in the world.
func enterFrom(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) {
	t.Helper()
	for _, want := range []byte{serverpackets.OpcodeSSQInfo, serverpackets.OpcodeCharSelected} {
		if reply := c.Read(); reply[0] != want {
			t.Fatalf("opcode = %#x, want %#x", reply[0], want)
		}
	}
	enterWorld(t, srv, c)
}

// TestPackageSendGates pins the package's item gates: a row naming the
// selected enchant scroll is left out with its fee still charged, and a
// worn item is taken off and shipped.
func TestPackageSendGates(t *testing.T) {
	t.Parallel()
	catalog := gameservertest.ItemTemplates().All()
	for _, tmpl := range catalog {
		if tmpl.ID == scrollID {
			tmpl.Tradable, tmpl.Depositable = true, true
		}
	}
	w := bootFreight(t, freightAdena, [][2]int32{{scrollID, 1}, {swordID, 1}}, gameservertest.WithItemTemplates(item.NewTable(catalog)))
	scroll, sword := w.items[scrollID], w.items[swordID]
	w.c.Send(encodeUseItem(scroll))
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeUseItem(sword))
	drainUntilQuiet(t, w.c)

	w.c.Send(encodePackageSend(w.receiver.ID, whRow{scroll, 1}, whRow{sword, 1}))
	drainUntilQuiet(t, w.c)
	if s, sw, a := w.held(t, scroll), w.held(t, sword), w.adena(t); s != 1 || sw != 0 || a != freightAdena-2*freightPrice {
		t.Fatalf("after the package: scroll %d sword %d adena %d, want 1, 0 and %d", s, sw, a, freightAdena-2*freightPrice)
	}
	if got := w.srv.PlayerInventory(t, w.player).ItemAt(itemcontainer.RHand); got != nil {
		t.Fatalf("right hand holds %d after shipping the worn sword, want empty", got.ObjectID)
	}
}

// TestPackageSendRefusedWithoutTransactionRight pins the access gate: a
// character whose access level forbids transactions is refused a package
// with YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT, while its private warehouse
// still takes a deposit.
func TestPackageSendRefusedWithoutTransactionRight(t *testing.T) {
	t.Parallel()
	levels, err := admin.NewData([]admin.AccessLevel{{Level: 0, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", GiveDamage: true}}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	locked := bootFreight(t, freightAdena, [][2]int32{{potionID, 5}}, gameservertest.WithAdmin(levels))
	potion := locked.items[potionID]
	locked.c.Send(encodePackageSend(locked.receiver.ID, whRow{potion, 1}))
	requireMessage(t, "package without the transaction right", drainFrames(t, locked.c), serverpackets.SystemMessageNotAuthorizedToDoThat, false)
	locked.command(t, "DepositP")
	locked.c.Send(encodeDeposit(whRow{potion, 1}))
	drainUntilQuiet(t, locked.c)
	if got := locked.held(t, potion); got != 4 {
		t.Fatalf("potions after the private deposit = %d, want 4", got)
	}
}

// TestSelectionWaitsForAccountSessionToLeave holds a sender in the world
// after its account is taken over: selecting the other character of the
// account answers nothing until the sender's session has left, since that
// session holds the character's freight. Once it has, the selection goes
// through.
func TestSelectionWaitsForAccountSessionToLeave(t *testing.T) {
	t.Parallel()
	w := bootFreight(t, freightAdena, nil, gameservertest.WithReuseDelays(0, 0), gameservertest.WithRealPool())
	queue := w.srv.PlayerQueue(t, w.player)
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(letGo) // a failure below must not leave the queue held at shutdown
	if !queue.Post(func() { close(held); <-release }) {
		t.Fatal("post to the sender's queue: queue closed")
	}
	<-held

	c := w.srv.DialClient(t, w.srv.Account(), 2)
	chars, err := w.srv.Chars.ListByAccount(context.Background(), w.srv.Account())
	if err != nil {
		t.Fatalf("list characters: %v", err)
	}
	slot := int32(slices.IndexFunc(chars, func(ch *player.Character) bool { return ch.ID == w.receiver.ID }))
	c.Send(encodeRequestGameStart(slot))
	// The sender's queue is held, so Settle would wait on it; the selection
	// itself runs on the connection, and once it is handled its answer, if
	// any, is already on the way.
	w.srv.AwaitHandled(t)
	if frames := drainFrames(t, c); len(frames) != 0 {
		t.Fatalf("selection with the sender still in the world answered %x, want nothing", opcodes(frames))
	}

	letGo()
	w.srv.AdvanceUntil(t, "sender out of the world", func() bool {
		_, ok := w.srv.State.Player(w.player)
		return !ok
	})
	c.Send(encodeRequestGameStart(slot))
	enterFrom(t, w.srv, c)
	if _, ok := w.srv.State.Player(w.receiver.ID); !ok {
		t.Fatal("receiver not in the world after its selection")
	}
}
