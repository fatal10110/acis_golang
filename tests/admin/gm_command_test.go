package admin

import (
	"bytes"
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The character window's world: the "Wardens" clan, led by the offline
// "Boss", counts the game master "Admin" as a member and keeps adena and
// potions in its warehouse; a second clan is named "Player", like the
// clanless online player the views are taken of.
const (
	altGClanID      = 300001
	altGNamesakeID  = 300002
	altGBossID      = 910001
	altGBoss2ID     = 910002
	altGClanAdena   = 5000
	altGPotionID    = 20
	altGPlayerAdena = 700
)

type altGWorld struct {
	srv      *gameservertest.Server
	gm       *testsupport.ScriptedClient
	gmID     int32
	player   *testsupport.ScriptedClient
	playerID int32
	// entry is what the player was sent entering the world, the windows
	// the views must repeat.
	entry [][]byte
}

// bootAltG boots "Admin" at level, a member of the seeded clan, and the
// online "Player", carrying adena and keeping adena and potions in its
// warehouse.
func bootAltG(t *testing.T, level int) *altGWorld {
	t.Helper()
	srv, gmID := bootAdmin(t, level, gameservertest.WithClanSeed(func(db *sql.DB) { seedAltGClans(t, db) }))
	w := &altGWorld{srv: srv, gm: srv.Client, gmID: gmID}
	enterWorld(t, w.gm)

	w.playerID = srv.SeedCharacterFor(t, "player2", "Player", 1, 0).ID
	srv.GiveItem(t, w.playerID, item.AdenaID, 300)
	execAltG(t, srv.DB, "INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, ?, 'WAREHOUSE', 0), (?, ?, ?, ?, 'WAREHOUSE', 0)",
		w.playerID, 1900000101, item.AdenaID, altGPlayerAdena, w.playerID, 1900000102, altGPotionID, 4)
	w.player = srv.DialClient(t, "player2", 1)
	w.entry = enterWorldFrames(t, w.player)
	drain(t, w.gm)
	return w
}

func seedAltGClans(t *testing.T, db *sql.DB) {
	t.Helper()
	execAltG(t, db, "INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('boss', ?, 'Boss', 40, ?, 1), ('boss2', ?, 'Boss2', 40, ?, 1)",
		altGBossID, altGClanID, altGBoss2ID, altGNamesakeID)
	execAltG(t, db, "UPDATE characters SET clanid = ?, power_grade = 6 WHERE char_name = 'Admin'", altGClanID)
	execAltG(t, db, "INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Wardens', 1, ?), (?, 'Player', 1, ?)",
		altGClanID, altGBossID, altGNamesakeID, altGBoss2ID)
	execAltG(t, db, "INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, ?, 'CLANWH', 0), (?, ?, ?, ?, 'CLANWH', 0)",
		altGClanID, 1900000001, item.AdenaID, altGClanAdena, altGClanID, 1900000002, altGPotionID, 3)
}

func execAltG(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func encodeGMCommand(name string, command int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGMCommand)
	w.WriteString(name)
	w.WriteInt32(command)
	return w.Bytes()
}

// altG sends the character window request from c and returns its answer.
func altG(t *testing.T, c *testsupport.ScriptedClient, name string, command int32) [][]byte {
	t.Helper()
	return exchange(t, c, encodeGMCommand(name, command))
}

func requireOpcodes(t *testing.T, label string, frames [][]byte, want ...byte) {
	t.Helper()
	if got := testsupport.FrameOpcodes(frames); !slices.Equal(got, want) {
		t.Fatalf("%s: opcodes = %#x, want %#x", label, got, want)
	}
}

// lastFrame returns the last frame with opcode op.
func lastFrame(t *testing.T, frames [][]byte, op byte) []byte {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == op {
			return frames[i]
		}
	}
	t.Fatalf("no %#x frame among %#x", op, testsupport.FrameOpcodes(frames))
	return nil
}

// warehouseView decodes a GMViewWarehouseWithdrawList's owner, adena and
// item count.
func warehouseView(frame []byte) (owner string, adena int32, count uint16) {
	r := wire.NewReader(frame[1:])
	owner = r.ReadString()
	adena = r.ReadInt32()
	return owner, adena, r.ReadUint16()
}

// rest returns what r has left to read.
func rest(r *wire.Reader) []byte { return r.ReadBytes(r.Remaining()) }

// TestGMCommandShowsEachView drives every view of the character window
// (Alt+G) from an administrator: each answers the game master, never the
// player viewed, with the windows the player itself was sent; a name
// matches whatever its case.
func TestGMCommandShowsEachView(t *testing.T) {
	w := bootAltG(t, adminLevel)

	// 1: the status view, then the dyes, which repeat the player's own
	// HennaInfo under the view's opcode.
	frames := altG(t, w.gm, "Player", 1)
	requireOpcodes(t, "status", frames, serverpackets.OpcodeGMViewCharacterInfo, serverpackets.OpcodeGMHennaInfo)
	r := wire.NewReader(frames[0][1:])
	r.ReadBytes(16) // x, y, z, heading
	if id, name := r.ReadInt32(), r.ReadString(); id != w.playerID || name != "Player" {
		t.Fatalf("status view of %d %q, want %d Player", id, name, w.playerID)
	}
	if own := lastFrame(t, w.entry, serverpackets.OpcodeHennaInfo); !bytes.Equal(frames[1][1:], own[1:]) {
		t.Fatalf("GMHennaInfo % x, want the player's HennaInfo % x", frames[1][1:], own[1:])
	}
	requireOpcodes(t, "viewed player", exchange(t, w.player, encodeManorBarrier()))

	// 2: the clan view of a clan member; a clanless player shows nothing.
	frames = altG(t, w.gm, "Admin", 2)
	requireOpcodes(t, "clan", frames, serverpackets.OpcodeGMViewPledgeInfo)
	r = wire.NewReader(frames[0][1:])
	viewed, clanID := r.ReadString(), r.ReadInt32()
	r.ReadInt32()
	if pledge, leader := r.ReadString(), r.ReadString(); viewed != "Admin" || clanID != altGClanID || pledge != "Wardens" || leader != "Boss" {
		t.Fatalf("clan view = %q %d %q %q, want Admin %d Wardens Boss", viewed, clanID, pledge, leader, altGClanID)
	}
	requireOpcodes(t, "clanless", altG(t, w.gm, "Player", 2))

	// 3: the skills, the player's own SkillList under its name.
	frames = altG(t, w.gm, "player", 3)
	requireOpcodes(t, "skills", frames, serverpackets.OpcodeGMViewSkillInfo)
	r = wire.NewReader(frames[0][1:])
	if name := r.ReadString(); name != "Player" {
		t.Fatalf("skills view of %q, want Player", name)
	}
	if own := lastFrame(t, w.entry, serverpackets.OpcodeSkillList); !bytes.Equal(rest(r), own[1:]) {
		t.Fatalf("skills % x, want the player's SkillList % x", rest(r), own[1:])
	}

	// 4: the quest log, the player's own QuestList under its name.
	frames = altG(t, w.gm, "PLAYER", 4)
	requireOpcodes(t, "quests", frames, serverpackets.OpcodeGMViewQuestList)
	r = wire.NewReader(frames[0][1:])
	if name := r.ReadString(); name != "Player" {
		t.Fatalf("quests view of %q, want Player", name)
	}
	if own := lastFrame(t, w.entry, serverpackets.OpcodeQuestList); !bytes.Equal(rest(r), own[1:]) {
		t.Fatalf("quests % x, want the player's QuestList % x", rest(r), own[1:])
	}

	// 5: the inventory, the player's own ItemList under its name and slot
	// limit, then the dyes.
	frames = altG(t, w.gm, "Player", 5)
	requireOpcodes(t, "inventory", frames, serverpackets.OpcodeGMViewItemList, serverpackets.OpcodeGMHennaInfo)
	r = wire.NewReader(frames[0][1:])
	if name, limit, show := r.ReadString(), r.ReadInt32(), r.ReadUint16(); name != "Player" || limit <= 0 || show != 1 {
		t.Fatalf("inventory view = %q limit %d show %d, want Player, a limit, 1", name, limit, show)
	}
	if own := lastFrame(t, w.entry, serverpackets.OpcodeItemList); !bytes.Equal(rest(r), own[3:]) {
		t.Fatalf("inventory % x, want the player's ItemList % x", rest(r), own[3:])
	}

	// 6: the player's warehouse; the player wins over the clan it is the
	// namesake of.
	frames = altG(t, w.gm, "Player", 6)
	requireOpcodes(t, "warehouse", frames, serverpackets.OpcodeGMViewWarehouseWithdrawList)
	if owner, adena, count := warehouseView(frames[0]); owner != "Player" || adena != altGPlayerAdena || count != 2 {
		t.Fatalf("warehouse view = %q %d adena %d items, want Player %d adena 2 items", owner, adena, count, altGPlayerAdena)
	}

	// 6 by a clan's name: its warehouse, under its leader's name.
	frames = altG(t, w.gm, "wardens", 6)
	requireOpcodes(t, "clan warehouse", frames, serverpackets.OpcodeGMViewWarehouseWithdrawList)
	if owner, adena, count := warehouseView(frames[0]); owner != "Boss" || adena != altGClanAdena || count != 2 {
		t.Fatalf("clan warehouse view = %q %d adena %d items, want Boss %d adena 2 items", owner, adena, count, altGClanAdena)
	}
	requireOpcodes(t, "viewed player", exchange(t, w.player, encodeManorBarrier()))
}

// TestGMCommandSilences sends the requests the character window answers
// with nothing: a name nobody carries, an empty one, a clan's name for a
// view other than the warehouse, and a view number outside 1-6.
func TestGMCommandSilences(t *testing.T) {
	w := bootAltG(t, adminLevel)
	for _, tc := range []struct {
		label   string
		name    string
		command int32
	}{
		{"unknown name", "Nobody", 1},
		{"unknown warehouse", "Nobody", 6},
		{"empty name", "", 6},
		{"clan status", "Wardens", 1},
		{"clan inventory", "Wardens", 5},
		{"view 0", "Player", 0},
		{"view 7", "Player", 7},
	} {
		requireOpcodes(t, tc.label, altG(t, w.gm, tc.name, tc.command))
	}
	requireOpcodes(t, "viewed player", exchange(t, w.player, encodeManorBarrier()))
}

// TestGMCommandRefusesWithoutAltG answers nothing to a player who is not a
// game master, nor to one of the shipped levels that open the character
// window without being a game master's.
func TestGMCommandRefusesWithoutAltG(t *testing.T) {
	w := bootAltG(t, userLevel)
	helper, _ := addPlayer(t, w.srv, "player3", "Helper", 2)
	drain(t, w.gm)
	drain(t, w.player)
	for command := int32(1); command <= 6; command++ {
		requireOpcodes(t, "user", altG(t, w.gm, "Player", command))
		requireOpcodes(t, "test GM", altG(t, helper, "Player", command))
	}
	requireOpcodes(t, "test GM clan warehouse", altG(t, helper, "Wardens", 6))
	requireOpcodes(t, "viewed player", exchange(t, w.player, encodeManorBarrier()))
}

// TestGMCommandRefusesGMWithoutAltG answers nothing to a game master whose
// access level keeps the character window closed.
func TestGMCommandRefusesGMWithoutAltG(t *testing.T) {
	const level = 9
	data, err := admin.NewData([]admin.AccessLevel{
		{Level: userLevel, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77"},
		{Level: level, Name: "Watcher", NameColor: "FFFFFF", TitleColor: "FFFFFF", IsGM: true},
	}, nil)
	if err != nil {
		t.Fatalf("new admin data: %v", err)
	}
	srv, _ := bootAdmin(t, level, gameservertest.WithAdmin(data))
	enterWorld(t, srv.Client)
	for command := int32(1); command <= 6; command++ {
		requireOpcodes(t, "game master without Alt+G", altG(t, srv.Client, "Admin", command))
	}
}
