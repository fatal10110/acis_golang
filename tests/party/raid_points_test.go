package party

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// raidBossID is the npc id of the fixture raid boss.
const raidBossID = 25001

// raidBoss is a level-40 raid boss: each player credited with its kill
// earns 40/2 - 5 to 40/2 + 5 points.
func raidBoss(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "RaidBoss", Level: 40, HPMax: 2000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

func (g *group) spawnBesideLeader(t *testing.T, tmpl *npc.Template) *npc.Hostile {
	t.Helper()
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	h := g.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)
	return h
}

// storedRaidPoints returns the character_raid_points rows of objectID,
// keyed by boss.
func storedRaidPoints(t *testing.T, db *sql.DB, objectID int32) map[int32]int32 {
	t.Helper()
	rows, err := db.Query("SELECT boss_id, points FROM character_raid_points WHERE char_id = ?", objectID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]int32{}
	for rows.Next() {
		var boss, points int32
		if err := rows.Scan(&boss, &points); err != nil {
			t.Fatal(err)
		}
		out[boss] = points
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func encodeRequestGetBossRecord(bossID int32) []byte {
	return encodeExtendedInt(clientpackets.OpcodeRequestGetBossRecord, bossID)
}

// bossRecord sends RequestGetBossRecord for p and returns the answer.
func (g *group) bossRecord(t *testing.T, i int) []byte {
	t.Helper()
	c := g.players[i].c
	c.Send(encodeRequestGetBossRecord(0))
	for _, f := range drainFrames(t, c) {
		if f[0] == serverpackets.OpcodeExtended && wire.NewReader(f[1:]).ReadUint16() == serverpackets.OpcodeExGetBossRecord {
			return f
		}
	}
	t.Fatalf("%s got no ExGetBossRecord", g.players[i].name)
	return nil
}

// raidAnnouncement asserts frames show the boss dying, then the raid
// success message, then its sound.
func raidAnnouncement(t *testing.T, name string, frames [][]byte) {
	t.Helper()
	die, msg, sound := -1, -1, -1
	for i, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeDie && die < 0:
			die = i
		case f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == serverpackets.SystemMessageRaidWasSuccessful:
			if msg >= 0 {
				t.Fatalf("%s got RAID_WAS_SUCCESSFUL twice", name)
			}
			msg = i
		case f[0] == serverpackets.OpcodePlaySound:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			if r.ReadString() == serverpackets.SoundRaidWasSuccessful {
				sound = i
			}
		}
	}
	if die < 0 || msg <= die || sound != msg+1 {
		t.Fatalf("%s: Die at %d, RAID_WAS_SUCCESSFUL at %d, its sound at %d; want them in that order, the sound right after the message", name, die, msg, sound)
	}
}

// TestRaidBossPartyKillCreditsEveryMember pins a raid boss killed by a
// party member: everyone watching sees the death, the raid success message
// and its sound; every party member, even one far from the fight, earns
// its own 15 to 25 points for the boss, stored and listed in its record;
// the partyless player who watched earns nothing.
func TestRaidBossPartyKillCreditsEveryMember(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 30}, {"Far", 45}, {"Outsider", 40}})
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	obj, _ := g.srv.State.Player(g.players[2].id)
	far, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Far is %T", obj)
	}
	far.TeleportTo(x+3000, y, z, 0)
	boss := g.spawnBesideLeader(t, raidBoss(raidBossID))

	if !boss.TakeDamage(5000, g.combatant(t, 1)) {
		t.Fatal("member's hit did not kill the boss")
	}
	for _, i := range []int{0, 1, 3} {
		raidAnnouncement(t, g.players[i].name, drainFrames(t, g.players[i].c))
	}
	g.quiet(t)

	g.srv.FlushPersistence(t)
	for i, p := range g.players {
		stored := storedRaidPoints(t, g.srv.DB, p.id)
		rec := g.srv.RaidPoints.Record(p.id)
		if i == 3 {
			if len(stored) != 0 || rec.Found {
				t.Fatalf("Outsider earned raid points: stored %v, record %+v", stored, rec)
			}
			continue
		}
		points, ok := stored[raidBossID]
		if !ok || len(stored) != 1 || points < 15 || points > 25 {
			t.Fatalf("%s stored raid points %v, want one row for %d worth 15..25", p.name, stored, raidBossID)
		}
		want := raidpoint.Record{Total: points, Entries: []raidpoint.Entry{{BossID: raidBossID, Points: points}}, Found: true}
		if rec.Total != want.Total || len(rec.Entries) != 1 || rec.Entries[0] != want.Entries[0] {
			t.Fatalf("%s record = %+v, want %+v", p.name, rec, want)
		}
	}

	// The member asks for its record: its rank among the three, its total
	// and its one boss.
	frame := g.bossRecord(t, 1)
	r := wire.NewReader(frame[3:])
	rank, total, n := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	gotBoss, gotPoints, zero := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	rec := g.srv.RaidPoints.Record(g.players[1].id)
	if rank < 1 || rank > 3 || rank != rec.Rank || total != rec.Total || n != 1 || gotBoss != raidBossID || gotPoints != rec.Total || zero != 0 || r.Remaining() != 0 {
		t.Fatalf("ExGetBossRecord = rank %d total %d n %d [%d %d %d] (%d left), want rank %d total %d and one entry", rank, total, n, gotBoss, gotPoints, zero, r.Remaining(), rec.Rank, rec.Total)
	}
}

// TestRaidBossSoloKillCreditsKillerAlone credits a partyless killer only.
func TestRaidBossSoloKillCreditsKillerAlone(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 40}, {"Watcher", 40}})
	boss := g.spawnBesideLeader(t, raidBoss(raidBossID))
	if !boss.TakeDamage(5000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the boss")
	}
	raidAnnouncement(t, "Watcher", drainFrames(t, g.players[1].c))
	g.quiet(t)
	g.srv.FlushPersistence(t)
	if got := g.srv.RaidPoints.Record(g.players[0].id); got.Total < 15 || got.Total > 25 || got.Rank != 1 {
		t.Fatalf("Leader record = %+v, want rank 1 and 15..25 points", got)
	}
	if stored := storedRaidPoints(t, g.srv.DB, g.players[1].id); len(stored) != 0 {
		t.Fatalf("Watcher stored raid points %v, want none", stored)
	}
}

// TestMonsterKillEarnsNoRaidPoints: an ordinary monster neither announces
// a raid success nor pays raid points.
func TestMonsterKillEarnsNoRaidPoints(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 40}})
	monster := g.spawnBesideLeader(t, partyRewardMonster())
	if !monster.TakeDamage(5000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	if frames := drainFrames(t, g.players[0].c); hasStaticMessage(frames, serverpackets.SystemMessageRaidWasSuccessful) {
		t.Fatal("a monster kill showed RAID_WAS_SUCCESSFUL")
	}
	g.srv.FlushPersistence(t)
	if rec := g.srv.RaidPoints.Record(g.players[0].id); rec.Found {
		t.Fatalf("record = %+v after a monster kill, want none", rec)
	}
}

// readRaidPointsGolden reads testdata/raid_points.golden: the reference's
// ExGetBossRecord for players 1 to 4 of RaidPointProbe's scenario, then
// for the same players once the stored rows are read back at a restart.
func readRaidPointsGolden(t *testing.T) (live, restarted map[int][]byte) {
	t.Helper()
	f, err := os.Open("testdata/raid_points.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	live, restarted = map[int][]byte{}, map[int][]byte{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<16)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		into := live
		if fields[0] == "restart" {
			into, fields = restarted, fields[1:]
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		into[n] = b
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return live, restarted
}

// TestBossRecordMatchesReference replays RaidPointProbe's scenario: player
// 1 restored from stored rows, then points added to players 1 to 4. Each
// player's ExGetBossRecord, bosses in the reference record's order
// included, matches the reference byte for byte, before and after the
// stored rows are read back.
func TestBossRecordMatchesReference(t *testing.T) {
	stored := [][2]int32{{25001, 10}, {25002, 20}, {25016, 5}, {25017, 7}, {25033, 1}, {29001, 30}}
	g := bootGroup(t, []seat{{"First", 40}, {"Second", 40}, {"Third", 40}, {"Fourth", 40}},
		gameservertest.WithBossSeed(func(db *sql.DB) {
			var id int32
			if err := db.QueryRow("SELECT obj_Id FROM characters WHERE char_name = 'First'").Scan(&id); err != nil {
				t.Fatal(err)
			}
			for _, row := range stored {
				if _, err := db.Exec("INSERT INTO character_raid_points (char_id, boss_id, points) VALUES (?, ?, ?)", id, row[0], row[1]); err != nil {
					t.Fatal(err)
				}
			}
		}))
	id := func(n int) int32 { return g.players[n-1].id }
	points := g.srv.RaidPoints
	for _, a := range [][2]int32{{25003, 4}, {25001, 3}, {25004, 2}, {25005, 2}, {25006, 2}, {25007, 2}, {25018, 1}, {25019, 1}} {
		points.Add(id(1), a[0], a[1])
	}
	for boss := int32(25008); boss <= 25136; boss += 16 {
		points.Add(id(2), boss, 5)
	}
	points.Add(id(3), 25001, 0)
	points.Add(id(4), 25001, -1)

	live, restarted := readRaidPointsGolden(t)
	for n := 1; n <= 4; n++ {
		if got := g.bossRecord(t, n-1); !bytes.Equal(got, live[n]) {
			t.Errorf("player %d ExGetBossRecord = %x\nwant %x", n, got, live[n])
		}
	}

	// A restart reads back what was stored.
	g.srv.FlushPersistence(t)
	again := raidpoint.New(gamesql.NewRaidPointStore(g.srv.DB), nil, zerolog.Nop())
	if err := again.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 4; n++ {
		rec := again.Record(id(n))
		entries := make([]serverpackets.BossRecordEntry, len(rec.Entries))
		for i, e := range rec.Entries {
			entries[i] = serverpackets.BossRecordEntry{BossID: e.BossID, Points: e.Points}
		}
		frame := serverpackets.FrameExGetBossRecord(rec.Rank, rec.Total, entries, rec.Found)
		got := append([]byte(nil), frame.Bytes()[2:]...)
		frame.Release()
		if !bytes.Equal(got, restarted[n]) {
			t.Errorf("player %d after restart = %x\nwant %x", n, got, restarted[n])
		}
	}
}
