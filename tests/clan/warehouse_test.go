package clan

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: WarehouseKeeper.onBypassFeedback (WarehouseKeeper.java:82-123):
// WithdrawC needs SP_WAREHOUSE_SEARCH (YOU_DO_NOT_HAVE_THE_RIGHT_TO_USE_CLAN_WAREHOUSE)
// then a clan of level 1 or higher (ONLY_LEVEL_1_CLAN_OR_HIGHER_CAN_USE_WAREHOUSE),
// sets the clan warehouse active and answers NO_ITEM_DEPOSITED_IN_WH when it
// is empty, else WarehouseWithdrawList(2) then ActionFailed; DepositC needs
// the level only, starts the item-list hold and sends WarehouseDepositList(2)
// of the tradable, depositable items then ActionFailed. RequestBypassToServer
// adds its closing ActionFailed. SendWarehouseWithdrawList.java:76-87: from a
// ClanWarehouse, MembersCanWithdrawFromClanWH lets a member holding
// SP_WAREHOUSE_SEARCH withdraw (silent otherwise); without it only the leader
// may (ONLY_CLAN_LEADER_CAN_RETRIEVE_ITEMS_FROM_CLAN_WAREHOUSE).
// ClanWarehouse.java: owner the clan id, location CLANWH, capacity
// MaximumWarehouseSlotsForClan. Clan.java:256 restores it from its rows.

const (
	clanKeeperID = 30005
	levelUpSP    = 30000
	levelUpAdena = 650000
	whAdena      = 1000
	whFee        = 30
	potionID     = 20
	swordID      = 30
	soulshotID   = 1463
)

// whClanWorld is a clanWorld at a warehouse keeper both members talked to.
type whClanWorld struct {
	*clanWorld
	clanID int32
	keeper *npc.Folk
	// items holds each member's seeded stacks by template id.
	leaderItems, memberItems map[int32]int32
}

// bootClanWarehouse boots the founder carrying the clan level-up price plus
// leaderItems and the recruit carrying memberItems, founds a clan, has the
// recruit join and spawns a warehouse keeper. The clan is left at level 0.
func bootClanWarehouse(t *testing.T, leaderItems, memberItems map[int32]int32, extra ...gameservertest.Option) *whClanWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 10, levelUpSP),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(clanPages(t)),
		keeperPage(t),
		gameservertest.WithReuseDelays(0, 0),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &whClanWorld{
		clanWorld:   &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)},
		leaderItems: map[int32]int32{},
		memberItems: map[int32]int32{},
	}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 1, 0).ID
	w.member = srv.DialClient(t, "player2", 1)
	w.leaderItems[item.AdenaID] = srv.GiveItem(t, w.leaderID, item.AdenaID, levelUpAdena+whAdena)
	for id, count := range leaderItems {
		w.leaderItems[id] = srv.GiveItem(t, w.leaderID, id, count)
	}
	for id, count := range memberItems {
		w.memberItems[id] = srv.GiveItem(t, w.memberID, id, count)
	}
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.master = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMaster", masterID), location.Location{X: x + 30, Y: y, Z: z})
	w.keeper = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("WarehouseKeeper", clanKeeperID), location.Location{X: x - 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.clanID = w.found(t, "Keepers")
	w.recruit(t)
	return w
}

// keeperPage serves the keeper's datapack page, whose links name the
// warehouse commands.
func keeperPage(t *testing.T) gameservertest.Option {
	t.Helper()
	page, err := os.ReadFile(datapack.Path(t, "data", "html", "warehouse", "30005.htm"))
	if err != nil {
		t.Fatal(err)
	}
	return gameservertest.WithHTMLPages(map[string]string{"warehouse/30005.htm": string(page)})
}

// raiseLevel raises the clan to level 1 at the village master.
func (w *whClanWorld) raiseLevel(t *testing.T) {
	t.Helper()
	w.talkToMaster(t)
	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Contains(ids, serverpackets.SystemMessageClanLevelIncreased) {
		t.Fatalf("level-up messages = %v", ids)
	}
	drainFrames(t, w.member)
}

// talkToKeeper has c select and talk to the keeper, opening its page.
func (w *whClanWorld) talkToKeeper(t *testing.T, c *testsupport.ScriptedClient, objectID int32) {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, objectID)
	at := location.Location{X: x, Y: y, Z: z}
	c.Send(encodeAction(w.keeper.ObjectID(), at))
	drainFrames(t, c)
	c.Send(encodeAction(w.keeper.ObjectID(), at))
	if _, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talking to the warehouse keeper opened no page")
	}
	// The other member sees c target and walk to the keeper.
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
}

func (w *whClanWorld) keeperCommand(t *testing.T, c *testsupport.ScriptedClient, command string) [][]byte {
	t.Helper()
	c.Send(encodeBypass("npc_" + itoa(w.keeper.ObjectID()) + "_" + command))
	return drainFrames(t, c)
}

// clanRows flushes item writes and returns the clan's CLANWH rows: count
// by template id, and how many rows hold each template.
func (w *whClanWorld) clanRows(t *testing.T) (counts, rows map[int32]int) {
	t.Helper()
	counts, rows = map[int32]int{}, map[int32]int{}
	for _, r := range w.savedRows(t, w.clanID) {
		if r.Location == item.LocationClanWarehouse {
			counts[r.TemplateID] += r.Count
			rows[r.TemplateID]++
		}
	}
	return counts, rows
}

func (w *whClanWorld) savedRows(t *testing.T, ownerID int32) []*item.Instance {
	t.Helper()
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items of %d: %v", ownerID, err)
	}
	return rows
}

func (w *whClanWorld) held(t *testing.T, ownerID, templateID int32) int {
	t.Helper()
	inst := w.srv.PlayerInventory(t, ownerID).ItemByTemplateID(templateID)
	if inst == nil {
		return 0
	}
	return inst.CountValue()
}

type whRow struct{ objectID, count int32 }

func encodeWarehouseRows(opcode byte, rows ...whRow) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

// warehouseList decodes a WarehouseDepositList or WarehouseWithdrawList:
// its warehouse type and its rows, count by object id and template by
// object id.
func warehouseList(t *testing.T, frame []byte, opcode byte) (whType uint16, counts, templates map[int32]int32) {
	t.Helper()
	if frame[0] != opcode {
		t.Fatalf("opcode = %#x, want %#x", frame[0], opcode)
	}
	r := wire.NewReader(frame[1:])
	whType = r.ReadUint16()
	r.ReadInt32() // adena
	counts, templates = map[int32]int32{}, map[int32]int32{}
	for n := int(r.ReadUint16()); n > 0; n-- {
		r.ReadUint16() // type1
		objectID, itemID, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		r.ReadUint16() // type2
		r.ReadUint16() // custom type 1
		r.ReadInt32()  // body part
		r.ReadUint16() // enchant
		r.ReadUint16() // custom type 2
		r.ReadUint16()
		r.ReadInt32() // object id again
		r.ReadInt64() // augmentation
		counts[objectID], templates[objectID] = count, itemID
	}
	if r.Remaining() != 0 || r.Err() != nil {
		t.Fatalf("warehouse list has %d trailing bytes (err %v)", r.Remaining(), r.Err())
	}
	return whType, counts, templates
}

func requireAnswer(t *testing.T, what string, frames [][]byte, want ...byte) {
	t.Helper()
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("%s = %x, want %x", what, got, want)
	}
}

// requireRefusal checks frames are the system message msg, then the
// bypass's closing ActionFailed.
func requireRefusal(t *testing.T, what string, frames [][]byte, msg int) {
	t.Helper()
	requireAnswer(t, what, frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed)
	if id, _ := sysMsg(t, frames[0]); id != msg {
		t.Fatalf("%s message = %d, want %d", what, id, msg)
	}
}

// setRecruitPrivileges sets the recruits' rank 6 privileges to privs.
func (w *whClanWorld) setRecruitPrivileges(t *testing.T, privs clan.Privilege) {
	t.Helper()
	w.leader.Send(encodeRequestPledgePower(clan.MemberPowerGrade, 2, int32(privs)))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
}

// TestClanWarehouseSharedByMembers walks two members through the clan
// warehouse: a level 0 clan refuses both commands; at level 1 the deposit
// window lists only tradable items; both members deposit at once into the
// one shared warehouse, which persists every row at CLANWH under the clan
// id; a privileged member may list but not withdraw while only the leader
// may, and the leader's withdrawal is written back.
func TestClanWarehouseSharedByMembers(t *testing.T) {
	w := bootClanWarehouse(t,
		map[int32]int32{potionID: 10, swordID: 1, soulshotID: 5},
		map[int32]int32{item.AdenaID: whAdena, potionID: 6})

	w.talkToKeeper(t, w.leader, w.leaderID)
	w.talkToKeeper(t, w.member, w.memberID)
	requireRefusal(t, "level 0 WithdrawC", w.keeperCommand(t, w.leader, "WithdrawC"), serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse)
	requireRefusal(t, "level 0 DepositC", w.keeperCommand(t, w.leader, "DepositC"), serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse)
	requireRefusal(t, "unprivileged WithdrawC", w.keeperCommand(t, w.member, "WithdrawC"), serverpackets.SystemMessageNoRightToUseClanWarehouse)

	w.raiseLevel(t)
	w.talkToKeeper(t, w.leader, w.leaderID)
	requireRefusal(t, "empty WithdrawC", w.keeperCommand(t, w.leader, "WithdrawC"), serverpackets.SystemMessageNoItemDepositedInWH)
	requireRefusal(t, "unprivileged WithdrawC at level 1", w.keeperCommand(t, w.member, "WithdrawC"), serverpackets.SystemMessageNoRightToUseClanWarehouse)

	frames := w.keeperCommand(t, w.leader, "DepositC")
	requireAnswer(t, "leader DepositC", frames, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	whType, listed, _ := warehouseList(t, frames[0], serverpackets.OpcodeWarehouseDepositList)
	sword, potion, shot := w.leaderItems[swordID], w.leaderItems[potionID], w.leaderItems[soulshotID]
	if whType != 2 || len(listed) != 3 || listed[sword] != 1 || listed[potion] != 10 || listed[w.leaderItems[item.AdenaID]] != whAdena {
		t.Fatalf("leader deposit list = type %d %v, want type 2 with the sword, 10 potions and %d adena", whType, listed, whAdena)
	}
	if _, ok := listed[shot]; ok {
		t.Fatal("deposit list offers the non-tradable soulshots")
	}
	frames = w.keeperCommand(t, w.member, "DepositC")
	requireAnswer(t, "member DepositC", frames, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)

	// Both members deposit into the one warehouse at once.
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{sword, 1}, whRow{potion, 4}))
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.memberItems[potionID], 6}))
	for _, c := range []*testsupport.ScriptedClient{w.leader, w.member} {
		if ids := messages(t, drainFrames(t, c)); len(ids) != 0 {
			t.Fatalf("deposit answered messages %v, want none", ids)
		}
	}
	if got := w.held(t, w.leaderID, item.AdenaID); got != whAdena-2*whFee {
		t.Fatalf("leader adena = %d, want %d", got, whAdena-2*whFee)
	}
	if got := w.held(t, w.memberID, item.AdenaID); got != whAdena-whFee {
		t.Fatalf("member adena = %d, want %d", got, whAdena-whFee)
	}
	if got := w.held(t, w.memberID, potionID); got != 0 {
		t.Fatalf("member potions = %d, want 0", got)
	}
	counts, rows := w.clanRows(t)
	if counts[potionID] != 10 || counts[swordID] != 1 || len(counts) != 2 || rows[potionID] != 1 {
		t.Fatalf("clan warehouse rows = %v (rows %v), want one stack of 10 potions and the sword", counts, rows)
	}

	// A member with the warehouse-search privilege sees the warehouse but
	// only the leader may take from it.
	w.setRecruitPrivileges(t, clan.PrivWarehouseSearch)
	frames = w.keeperCommand(t, w.member, "WithdrawC")
	requireAnswer(t, "privileged WithdrawC", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	whType, listed, templates := warehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList)
	var stored int32
	for objectID, id := range templates {
		if id == potionID {
			stored = objectID
		}
	}
	if whType != 2 || len(listed) != 2 || listed[sword] != 1 || listed[stored] != 10 {
		t.Fatalf("withdraw list = type %d %v, want type 2 with the sword and 10 potions", whType, listed)
	}
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 2}))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageOnlyClanLeaderCanRetrieveItemsFromClanWarehouse}) {
		t.Fatalf("member withdrawal messages = %v, want ONLY_CLAN_LEADER_CAN_RETRIEVE_ITEMS_FROM_CLAN_WAREHOUSE", ids)
	}
	if got := w.held(t, w.memberID, potionID); got != 0 {
		t.Fatalf("member potions after the refusal = %d, want 0", got)
	}

	frames = w.keeperCommand(t, w.leader, "WithdrawC")
	requireAnswer(t, "leader WithdrawC", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{sword, 1}, whRow{stored, 3}))
	if ids := messages(t, drainFrames(t, w.leader)); len(ids) != 0 {
		t.Fatalf("leader withdrawal messages = %v, want none", ids)
	}
	if s, p := w.held(t, w.leaderID, swordID), w.held(t, w.leaderID, potionID); s != 1 || p != 9 {
		t.Fatalf("leader after the withdrawal: sword %d potions %d, want 1 and 9", s, p)
	}
	counts, _ = w.clanRows(t)
	if counts[potionID] != 7 || len(counts) != 1 {
		t.Fatalf("clan warehouse rows after the withdrawal = %v, want 7 potions", counts)
	}
	leaderRows := map[int32]int{}
	for _, r := range w.savedRows(t, w.leaderID) {
		if r.Location == item.LocationInventory {
			leaderRows[r.TemplateID] += r.Count
		}
	}
	if leaderRows[swordID] != 1 || leaderRows[potionID] != 9 {
		t.Fatalf("leader inventory rows = %v, want the sword and 9 potions", leaderRows)
	}
}

// TestClanWarehouseMembersMayWithdraw turns MembersCanWithdrawFromClanWH on:
// a member holding the warehouse-search privilege withdraws, and once the
// privilege is taken away its next withdrawal from the open window is
// dropped without an answer.
func TestClanWarehouseMembersMayWithdraw(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.MembersCanWithdrawFromWarehouse = true
	w := bootClanWarehouse(t, map[int32]int32{potionID: 10}, nil, gameservertest.WithClanConfig(cfg))
	w.raiseLevel(t)
	w.talkToKeeper(t, w.leader, w.leaderID)
	w.talkToKeeper(t, w.member, w.memberID)
	w.keeperCommand(t, w.leader, "DepositC")
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.leaderItems[potionID], 10}))
	drainFrames(t, w.leader)

	w.setRecruitPrivileges(t, clan.PrivWarehouseSearch)
	frames := w.keeperCommand(t, w.member, "WithdrawC")
	requireAnswer(t, "member WithdrawC", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{w.leaderItems[potionID], 4}))
	if ids := messages(t, drainFrames(t, w.member)); len(ids) != 0 {
		t.Fatalf("member withdrawal messages = %v, want none", ids)
	}
	if got := w.held(t, w.memberID, potionID); got != 4 {
		t.Fatalf("member potions = %d, want 4", got)
	}

	w.setRecruitPrivileges(t, clan.PrivNone)
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{w.leaderItems[potionID], 2}))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("unprivileged withdrawal = %x, want nothing", opcodes(frames))
	}
	if got := w.held(t, w.memberID, potionID); got != 4 {
		t.Fatalf("member potions after the refusal = %d, want 4", got)
	}
	if counts, _ := w.clanRows(t); counts[potionID] != 6 {
		t.Fatalf("clan warehouse rows = %v, want 6 potions", counts)
	}
}

// TestClanWarehouseCapacity fills a clan warehouse of two slots: a deposit
// needing a third slot is refused, a stack merging into one it holds is not.
func TestClanWarehouseCapacity(t *testing.T) {
	slots := player.DefaultStorageSlots
	slots.ClanWarehouse = 2
	w := bootClanWarehouse(t, map[int32]int32{potionID: 10, swordID: 1}, nil, gameservertest.WithStorageSlots(slots))
	w.raiseLevel(t)
	w.talkToKeeper(t, w.leader, w.leaderID)
	w.keeperCommand(t, w.leader, "DepositC")
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.leaderItems[potionID], 5}, whRow{w.leaderItems[swordID], 1}))
	drainFrames(t, w.leader)

	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.leaderItems[item.AdenaID], 1}))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted}) {
		t.Fatalf("deposit into a full clan warehouse = %v, want YOU_HAVE_EXCEEDED_QUANTITY_THAT_CAN_BE_INPUTTED", ids)
	}
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.leaderItems[potionID], 5}))
	if ids := messages(t, drainFrames(t, w.leader)); len(ids) != 0 {
		t.Fatalf("merging deposit messages = %v, want none", ids)
	}
	if counts, _ := w.clanRows(t); counts[potionID] != 10 || counts[swordID] != 1 || len(counts) != 2 {
		t.Fatalf("clan warehouse rows = %v, want 10 potions and the sword", counts)
	}
}

// TestClanWarehouseRestoredFromRows stores CLANWH rows under a clan before
// any member opens its warehouse: the leader's withdrawal window lists
// them and a withdrawal moves the row into the leader's inventory.
func TestClanWarehouseRestoredFromRows(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0, seedClan(t, clanSeed{level: 1}), keeperPage(t))
	stored := w.srv.GiveItem(t, seededClanID, potionID, 7)
	if _, err := w.srv.DB.ExecContext(context.Background(), `UPDATE items SET loc = 'CLANWH' WHERE object_id = ?`, stored); err != nil {
		t.Fatal(err)
	}
	keeper := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("WarehouseKeeper", clanKeeperID), location.Location{X: w.at.X - 30, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.leader)
	ww := &whClanWorld{clanWorld: w, clanID: seededClanID, keeper: keeper}
	ww.talkToKeeper(t, w.leader, w.leaderID)

	frames := ww.keeperCommand(t, w.leader, "WithdrawC")
	requireAnswer(t, "WithdrawC", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	if whType, listed, _ := warehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList); whType != 2 || len(listed) != 1 || listed[stored] != 7 {
		t.Fatalf("withdraw list = type %d %v, want type 2 with the 7 stored potions", whType, listed)
	}
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 7}))
	drainFrames(t, w.leader)
	if got := ww.held(t, w.leaderID, potionID); got != 7 {
		t.Fatalf("leader potions = %d, want 7", got)
	}
	if counts, _ := ww.clanRows(t); len(counts) != 0 {
		t.Fatalf("clan warehouse rows = %v, want none", counts)
	}
	for _, r := range ww.savedRows(t, w.leaderID) {
		if r.ObjectID == stored && r.Location != item.LocationInventory {
			t.Fatalf("withdrawn row at %v, want INVENTORY", r.Location)
		}
	}
}
