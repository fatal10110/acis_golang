package npcs

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: ClanHallManagerNpc.onBypassFeedback (ClanHallManagerNpc.java:
// 95-156, 1108-1140): every command first passes the owner gate; then
// banish_foreigner (CHP_RIGHT_TO_DISMISS) opens banish-list.htm for "list"
// and, for "banish", ClanHall.banishForeigners then banish.htm, and any
// other choice throws on the unset page (nothing more is sent);
// manage_vault (SP_WAREHOUSE_SEARCH) shows vault.htm with %rent% the lease
// and %date% paidUntil as dd-MM-yyyy HH:mm; door (CHP_ENTRY_EXIT_RIGHTS)
// opens or closes every gate (Residence.openDoors / closeDoors) before
// door-open.htm / door-close.htm, else door.htm; WithdrawC
// (SP_WAREHOUSE_SEARCH, its refusal page followed by
// YOU_DO_NOT_HAVE_THE_RIGHT_TO_USE_CLAN_WAREHOUSE) and DepositC (no
// privilege; tempInventoryDisable) refuse a level 0 clan with
// ONLY_LEVEL_1_CLAN_OR_HIGHER_CAN_USE_WAREHOUSE, else send the clan
// WarehouseWithdrawList / WarehouseDepositList, then ActionFailed.
// showChatWindow (1146-1154) sends ActionFailed, then chamberlain.htm to a
// member of the owning clan and chamberlain-no.htm to anyone else.
// Moonstone Hall (22): lease 500000, gates gludio_castle_agit_001_001 and
// gludio_castle_agit_001_002, banish point (-15872, 123824, -3116).

// Clan privileges, as clan_privs stores them.
const (
	privWarehouseSearch = 8
	privHallEntryExit   = 1024
	privHallDismiss     = 8192
)

const (
	moonstoneGateA = 24_220_001
	moonstoneGateB = 24_220_002
)

var moonstoneBanish = location.Location{X: -15872, Y: 123824, Z: -3116}

// moonstoneGates are two closed doors named after Moonstone Hall's gates,
// beside the players' spawn point so the talker sees their status.
func moonstoneGates() gameservertest.Option {
	gate := func(id int, name string, x int) *door.Template {
		return &door.Template{
			ID: id, Name: name, Kind: door.KindDoor, Level: 1,
			Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
			Coordinates: []location.Point{
				{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
			},
			HP: 100, PDef: 10, MDef: 10, Height: 32, OpenKind: door.OpenNPC,
		}
	}
	return gameservertest.WithDoors(
		gate(moonstoneGateA, "gludio_castle_agit_001_001", -60),
		gate(moonstoneGateB, "GLUDIO_CASTLE_AGIT_001_002", 80),
	)
}

// gateStates returns, in order, each DoorStatusUpdate among frames as its
// door id and whether it reports the door open, and the index of the last.
func gateStates(frames [][]byte) (ids []int32, open []bool, last int) {
	last = -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeDoorStatusUpdate || len(f) < 21 {
			continue
		}
		ids = append(ids, int32(binary.LittleEndian.Uint32(f[17:])))
		open = append(open, binary.LittleEndian.Uint32(f[5:]) == 0)
		last = i
	}
	return ids, open, last
}

func pageIndex(frames [][]byte) int {
	return slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage })
}

// TestClanHallManagerChatWindow: talking to the manager releases the client
// before its page: the main page for a member of the owning clan, the
// refusal page for anyone else.
func TestClanHallManagerChatWindow(t *testing.T) {
	t.Parallel()
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owner", true: "foreign"}[foreign], func(t *testing.T) {
			t.Parallel()
			w := bootHallManager(t, hallManagerSetup{foreign: foreign})
			w.selectFolk(t, w.manager)
			frames := w.talk(t, w.manager, false)
			want := []byte{
				serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
				serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage,
			}
			if got := interactOrder(frames); !bytes.Equal(got, want) {
				t.Fatalf("talk = % x, want % x", got, want)
			}
			file := "chamberlain.htm"
			if foreign {
				file = "chamberlain-no.htm"
			}
			if got := pageIn(t, frames); got != fill(managerPage(t, file), "%objectId%", w.oid()) {
				t.Fatalf("page =\n%s\nwant %s", got, file)
			}
		})
	}
}

// TestClanHallManagerDoors: "door" shows the doors page; "door open" opens
// both gates, then shows its page, and "door close" closes them again. A
// member without the entry rights is refused and the gates stay shut.
func TestClanHallManagerDoors(t *testing.T) {
	t.Parallel()
	t.Run("entry rights", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallEntryExit, extra: []gameservertest.Option{moonstoneGates()}})
		frames := w.run(t, "door")
		if got := pageIn(t, frames); got != fill(managerPage(t, "door.htm"), "%objectId%", w.oid()) {
			t.Fatalf("door page =\n%s", got)
		}
		if ids, _, _ := gateStates(frames); len(ids) != 0 {
			t.Fatalf("door moved gates %v", ids)
		}
		for _, c := range []struct {
			command, page string
			open          bool
		}{{"door open", "door-open.htm", true}, {"door close", "door-close.htm", false}} {
			frames := w.run(t, c.command)
			ids, open, last := gateStates(frames)
			if !slices.Equal(ids, []int32{moonstoneGateA, moonstoneGateB}) || open[0] != c.open || open[1] != c.open {
				t.Fatalf("%s: gate updates %v open %v, want both gates open=%v", c.command, ids, open, c.open)
			}
			if page := pageIndex(frames); page < last {
				t.Fatalf("%s: page at %d before the last gate update at %d (opcodes % x)", c.command, page, last, opcodes(frames))
			}
			if got := pageIn(t, frames); got != fill(managerPage(t, c.page), "%objectId%", w.oid()) {
				t.Fatalf("%s page =\n%s", c.command, got)
			}
			if got := frames[len(frames)-1][0]; got != serverpackets.OpcodeActionFailed {
				t.Fatalf("%s: last frame %#x, want ActionFailed", c.command, got)
			}
		}
	})
	t.Run("no right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallDismiss, extra: []gameservertest.Option{moonstoneGates()}})
		frames := w.run(t, "door open")
		if got := pageIn(t, frames); got != managerPage(t, "not_authorized.htm") {
			t.Fatalf("refused door page =\n%s", got)
		}
		if ids, _, _ := gateStates(frames); len(ids) != 0 {
			t.Fatalf("refused door moved gates %v", ids)
		}
	})
}

// TestClanHallManagerBanishForeigners: "banish_foreigner list" shows the
// banishment page; "banish_foreigner banish" throws the bystander, of no
// clan, out to the hall's banish point and shows its page; any other
// choice answers nothing at all. A member without the right to dismiss is
// refused and nobody moves.
func TestClanHallManagerBanishForeigners(t *testing.T) {
	t.Parallel()
	t.Run("right to dismiss", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallDismiss})
		if got := w.page(t, "banish_foreigner list"); got != fill(managerPage(t, "banish-list.htm"), "%objectId%", w.oid()) {
			t.Fatalf("list page =\n%s", got)
		}
		if got := w.run(t, "banish_foreigner"); len(got) != 0 {
			t.Fatalf("banish_foreigner alone = % x, want nothing", opcodes(got))
		}
		if got := w.page(t, "banish_foreigner banish"); got != fill(managerPage(t, "banish.htm"), "%objectId%", w.oid()) {
			t.Fatalf("banish page =\n%s", got)
		}
		teleport, ok := firstOpcode(drainFrames(t, w.bystander), serverpackets.OpcodeTeleportToLocation)
		if !ok {
			t.Fatal("bystander was not thrown out")
		}
		x := int(int32(binary.LittleEndian.Uint32(teleport[5:])))
		y := int(int32(binary.LittleEndian.Uint32(teleport[9:])))
		if abs(x-moonstoneBanish.X) > clanHallBanishOffset || abs(y-moonstoneBanish.Y) > clanHallBanishOffset {
			t.Fatalf("bystander sent to (%d, %d), want within %d of %+v", x, y, clanHallBanishOffset, moonstoneBanish)
		}
		if px, py, _ := w.srv.PlayerPosition(t, w.player); px != w.at.X || py != w.at.Y {
			t.Fatalf("talker moved to (%d, %d)", px, py)
		}
	})
	t.Run("no right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallEntryExit})
		if got := w.page(t, "banish_foreigner banish"); got != managerPage(t, "not_authorized.htm") {
			t.Fatalf("refused banish page =\n%s", got)
		}
		if _, ok := firstOpcode(drainFrames(t, w.bystander), serverpackets.OpcodeTeleportToLocation); ok {
			t.Fatal("refused banishment threw the bystander out")
		}
	})
}

// clanHallBanishOffset is the random offset of a player thrown out of a
// hall's grounds.
const clanHallBanishOffset = 20

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TestClanHallManagerVault: manage_vault shows the hall's lease and the
// date its lease is paid until; a member without the warehouse privilege
// is refused.
func TestClanHallManagerVault(t *testing.T) {
	t.Parallel()
	t.Run("warehouse right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privWarehouseSearch})
		v, ok := w.srv.Halls.View(hallMoonstone)
		if !ok || v.PaidUntil == 0 {
			t.Fatalf("Moonstone Hall view = %+v", v)
		}
		want := fill(managerPage(t, "vault.htm"), "%rent%", "500000",
			"%date%", time.UnixMilli(v.PaidUntil).Format("02-01-2006 15:04"), "%objectId%", w.oid())
		if got := w.page(t, "manage_vault"); got != want {
			t.Fatalf("vault page =\n%s\nwant\n%s", got, want)
		}
	})
	t.Run("no right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallDismiss})
		if got := w.page(t, "manage_vault"); got != managerPage(t, "not_authorized.htm") {
			t.Fatalf("refused vault page =\n%s", got)
		}
	})
}

// TestClanHallManagerClanWarehouse: WithdrawC lists the clan warehouse,
// even empty, and DepositC the talker's depositable items, each then the
// manager's ActionFailed and the dispatcher's. A member without the
// warehouse privilege is refused a withdrawal with the refusal page and
// message but may deposit; a level 0 clan is told it cannot use a
// warehouse.
func TestClanHallManagerClanWarehouse(t *testing.T) {
	t.Parallel()
	list := func(opcode byte) []byte {
		return []byte{opcode, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	}
	t.Run("leader", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{adena: 1000})
		if got := opcodes(w.run(t, "WithdrawC")); !bytes.Equal(got, list(serverpackets.OpcodeWarehouseWithdrawList)) {
			t.Fatalf("WithdrawC = % x", got)
		}
		frames := w.run(t, "DepositC")
		if got := opcodes(frames); !bytes.Equal(got, list(serverpackets.OpcodeWarehouseDepositList)) {
			t.Fatalf("DepositC = % x", got)
		}
		if whType := binary.LittleEndian.Uint16(frames[0][1:]); whType != 2 {
			t.Fatalf("deposit window type = %d, want the clan's (2)", whType)
		}
	})
	t.Run("no warehouse right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: privHallDismiss})
		frames := w.run(t, "WithdrawC")
		want := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}
		if got := opcodes(frames); !bytes.Equal(got, want) {
			t.Fatalf("refused WithdrawC = % x, want % x", got, want)
		}
		if got := pageIn(t, frames); got != managerPage(t, "not_authorized.htm") {
			t.Fatalf("refused WithdrawC page =\n%s", got)
		}
		wantMessages(t, "refused WithdrawC", frames, serverpackets.SystemMessageNoRightToUseClanWarehouse)
		if got := opcodes(w.run(t, "DepositC")); !bytes.Equal(got, list(serverpackets.OpcodeWarehouseDepositList)) {
			t.Fatalf("DepositC without the right = % x", got)
		}
	})
	t.Run("level 0 clan", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{levelZero: true})
		for _, command := range []string{"WithdrawC", "DepositC"} {
			frames := w.run(t, command)
			if got := opcodes(frames); !bytes.Equal(got, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
				t.Fatalf("%s at a level 0 clan = % x", command, got)
			}
			wantMessages(t, command, frames, serverpackets.SystemMessageOnlyLevel1ClanOrHigherCanUseWarehouse)
		}
	})
}
