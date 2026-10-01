package clan

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Seeded clan and member ids: far above anything the id factory hands out
// during a test.
const (
	subunitClanID int32 = 0x7f000001
	squireID      int32 = 0x7f100001
)

// Level 0 to 3 costs 680000 SP, 3150000 adena and a Blood Mark.
const (
	toLevel3SP    = 680000
	toLevel3Adena = 3150000
	bloodMarkID   = 1419
)

// warConfig is the shipped clans.properties with one member enough to go
// to war.
func warConfig() clan.Config {
	cfg := clan.DefaultConfig()
	cfg.MembersForWar = 1
	return cfg
}

// warLevels is a level table whose level 10 is the shipped one: 48229 to
// 71201 experience, 8.875% lost at death.
func warLevels(t *testing.T) *player.LevelTable {
	t.Helper()
	levels := map[int]player.Level{
		10: {RequiredExpToLevelUp: 48229, ExpLossAtDeath: 8.875, KarmaModifier: 2.285526643},
		11: {RequiredExpToLevelUp: 71201},
		12: {RequiredExpToLevelUp: 1_000_000_000},
	}
	for lvl := 1; lvl < 10; lvl++ {
		levels[lvl] = player.Level{RequiredExpToLevelUp: int64(lvl - 1)}
	}
	table, err := player.NewLevelTable(levels)
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// subunitPages extends the clan dialog pages with the pages linking the
// sub-unit commands.
func subunitPages(t *testing.T) map[string]string {
	t.Helper()
	pages := clanPages(t)
	key := "villagemaster/" + strconv.Itoa(masterID) + ".htm"
	for _, name := range []string{"9000-12a.htm", "9000-12b.htm", "9000-13a.htm", "9000-13c.htm", "9000-14a.htm", "9000-15.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "Clan", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages[key] += string(data)
	}
	return pages
}

// bootWarWorld boots the founder and a rival (account player2), both level
// 10 with what raising a clan to level 3 costs, both in the world beside the
// village master. The rival's experience is set mid-level 10.
func bootWarWorld(t *testing.T, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 10, toLevel3SP),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(clanPages(t)),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithClanConfig(warConfig()),
		clanLevelItems(),
		gameservertest.WithLevels(warLevels(t)),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Rival", 10, toLevel3SP).ID
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET exp = 60000 WHERE obj_Id = ?`, w.memberID); err != nil {
		t.Fatalf("seed exp: %v", err)
	}
	w.member = srv.DialClient(t, "player2", 1)
	for _, id := range []int32{w.leaderID, w.memberID} {
		srv.GiveItem(t, id, item.AdenaID, toLevel3Adena)
		srv.GiveItem(t, id, bloodMarkID, 1)
	}
	w.enter(t)
	return w
}

// enter brings both players into the world and spawns the master beside
// the founder.
func (w *clanWorld) enter(t *testing.T) {
	t.Helper()
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := w.srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.master = w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMaster", masterID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
}

// masterCommandBy has c talk to the master and send command, returning
// c's answer.
func (w *clanWorld) masterCommandBy(t *testing.T, c *testsupport.ScriptedClient, command string) [][]byte {
	t.Helper()
	c.Send(encodeAction(w.master.ObjectID(), w.at))
	drainFrames(t, c)
	c.Send(encodeAction(w.master.ObjectID(), w.at))
	if _, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talking to the village master opened no page")
	}
	c.Send(encodeBypass("npc_" + strconv.Itoa(int(w.master.ObjectID())) + "_" + command))
	return drainFrames(t, c)
}

// raiseToLevel3 has c, a clan leader, raise its clan from 0 to 3.
func (w *clanWorld) raiseToLevel3(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 3 {
		frames := w.masterCommandBy(t, c, "increase_clan_level")
		if _, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate); !ok {
			t.Fatalf("level-up answer = %x", opcodes(frames))
		}
	}
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
}

// seedSubunitClan seeds the founder's level-7 clan "Knights" with 20000
// reputation and an offline main-clan member "Squire", plus extra
// statements.
func seedSubunitClan(t *testing.T, extra ...string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		stmts := append([]string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
				SELECT ` + itoa(subunitClanID) + `, 'Knights', 7, 20000, obj_Id FROM characters WHERE char_name = 'Founder'`,
			`UPDATE characters SET clanid = ` + itoa(subunitClanID) + `, power_grade = 0 WHERE char_name = 'Founder'`,
			`INSERT INTO characters (account_name, obj_Id, char_name, level, classid, clanid, power_grade, sex, race)
				VALUES ('squire', ` + itoa(squireID) + `, 'Squire', 20, 0, ` + itoa(subunitClanID) + `, 6, 0, 0)`,
		}, extra...)
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("seed clan: %v", err)
			}
		}
	})
}

// bootSubunitWorld boots the founder, leading the seeded clan, and a
// clanless recruit of level recruitLevel, both in the world beside the
// master whose page links the sub-unit commands. It returns the founder's
// login burst too.
func bootSubunitWorld(t *testing.T, recruitLevel int, extra ...gameservertest.Option) (*clanWorld, [][]byte) {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(subunitPages(t)),
		gameservertest.WithReuseDelays(0, 0),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", recruitLevel, 0).ID
	w.member = srv.DialClient(t, "player2", 1)
	burst := startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.master = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMaster", masterID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return w, burst
}

func encodeWarName(opcode byte, name string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(name)
	return w.Bytes()
}

func encodeWarReply(opcode byte, answer int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(answer)
	return w.Bytes()
}

func encodeRequestPledgeWarList(page, tab int32) []byte {
	w := encodeExtended(clientpackets.OpcodeRequestPledgeWarList)
	w.WriteInt32(page)
	w.WriteInt32(tab)
	return w.Bytes()
}

func encodeRequestPledgeReorganizeMember(selected int32, name string, pledgeType int32, swap string) []byte {
	w := encodeExtended(clientpackets.OpcodeRequestPledgeReorganizeMember)
	w.WriteInt32(selected)
	w.WriteString(name)
	w.WriteInt32(pledgeType)
	w.WriteString(swap)
	return w.Bytes()
}

func encodeRequestPledgeSetAcademyMaster(set int32, current, target string) []byte {
	w := encodeExtended(clientpackets.OpcodeRequestPledgeSetAcademyMaster)
	w.WriteInt32(set)
	w.WriteString(current)
	w.WriteString(target)
	return w.Bytes()
}

// extended returns the frames whose extended opcode is sub, in order.
func extended(frames [][]byte, sub uint16) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeExtended && len(f) >= 3 && uint16(f[1])|uint16(f[2])<<8 == sub {
			out = append(out, f)
		}
	}
	return out
}

// warList decodes a PledgeReceiveWarList frame: tab, page, count and the
// clan names.
func warList(t *testing.T, frame []byte) (tab, page, count int32, names []string) {
	t.Helper()
	r := wire.NewReader(frame[3:])
	tab, page, count = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	for r.Remaining() > 0 {
		names = append(names, r.ReadString())
		r.ReadInt32()
		r.ReadInt32()
	}
	return tab, page, count, names
}

// headerAtWar reads the at-war flag, the last field, of a
// PledgeShowInfoUpdate frame.
func headerAtWar(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowInfoUpdate {
		t.Fatalf("opcode = %#x, want PledgeShowInfoUpdate", frame[0])
	}
	return wire.NewReader(frame[len(frame)-4:]).ReadInt32()
}

// unitList decodes the pledge type, unit name, leader name and member names
// of a PledgeShowMemberListAll frame.
func unitList(t *testing.T, frame []byte) (pledgeType int32, name, leader string, members []string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowMemberListAll {
		t.Fatalf("opcode = %#x, want PledgeShowMemberListAll", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	r.ReadInt32()
	pledgeType, name, leader = r.ReadInt32(), r.ReadString(), r.ReadString()
	for range 8 {
		r.ReadInt32()
	}
	r.ReadInt32()
	r.ReadString()
	r.ReadInt32()
	r.ReadInt32()
	n := int(r.ReadInt32())
	for range n {
		members = append(members, r.ReadString())
		for range 6 {
			r.ReadInt32()
		}
	}
	return pledgeType, name, leader, members
}

// unitLists returns the pledge types of the PledgeShowMemberListAll frames
// among frames, in order.
func unitLists(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var out []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodePledgeShowMemberListAll {
			pledgeType, _, _, _ := unitList(t, f)
			out = append(out, pledgeType)
		}
	}
	return out
}
