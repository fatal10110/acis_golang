package clan

import (
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The ladder suite's other clans, with no members.
const (
	ladderRivalsID int32 = 0x7f000041
	ladderOthersID int32 = 0x7f000042
)

// pledgeListRank opens c's clan window and returns the rank its main
// clan's list shows.
func pledgeListRank(t *testing.T, c *testsupport.ScriptedClient) int32 {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	list, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatal("the clan window request got no member list")
	}
	r := wire.NewReader(list[1:])
	r.ReadInt32() // sub-unit list
	r.ReadInt32() // clan id
	r.ReadInt32() // pledge type
	r.ReadString()
	r.ReadString()
	for range 4 { // crest, level, castle, hall
		r.ReadInt32()
	}
	rank := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("decode PledgeShowMemberListAll: %v", err)
	}
	return rank
}

// TestClanLadderRefreshRunsDaily boots at 00:04 with Knights (reputation
// 100), Rivals (300) and Others (200), ranked 3, 1 and 2. Knights then
// climbs to 500 and Rivals falls to 0. The ranks hold until the shipped
// daily 00:05:00 refresh, which ranks Knights first, Others second and
// leaves Rivals off the ladder; the founder is sent nothing and sees the
// new rank in its next clan window.
func TestClanLadderRefreshRunsDaily(t *testing.T) {
	boot := time.Date(2026, 10, 6, 0, 4, 0, 0, time.UTC)
	due := time.Date(2026, 10, 6, 0, 5, 0, 0, time.UTC)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedStatements(t, db,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT `+itoa(titleClanID)+`, 'Knights', 5, 100, obj_Id FROM characters WHERE char_name = 'Founder'`,
				`UPDATE characters SET clanid = `+itoa(titleClanID)+`, power_grade = 0 WHERE char_name = 'Founder'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score) VALUES
					(`+itoa(ladderRivalsID)+`, 'Rivals', 5, 300), (`+itoa(ladderOthersID)+`, 'Others', 5, 200)`,
			)
		}),
		shippedTask(t, "task.ClanLadderRefresh", scripttask.ClanLadderRefresh),
		gameservertest.WithScheduledTasks(boot),
	)
	startInWorld(t, srv.Client)
	drainFrames(t, srv.Client)
	if got := pledgeListRank(t, srv.Client); got != 3 {
		t.Fatalf("Knights rank at boot = %d, want 3", got)
	}

	table := srv.Clans.Table()
	knights, _ := table.Get(titleClanID)
	rivals, _ := table.Get(ladderRivalsID)
	if _, ok := srv.Clans.AddReputation(knights, 400); !ok {
		t.Fatal("Knights gained no reputation")
	}
	if _, ok := srv.Clans.TakeReputation(rivals, 300); !ok {
		t.Fatal("Rivals lost no reputation")
	}

	srv.ScheduleClock.Advance(due.Sub(srv.ScheduleClock.Now()) - time.Second)
	srv.Settle(t)
	if got := pledgeListRank(t, srv.Client); got != 3 {
		t.Fatalf("Knights rank before the refresh = %d, want it still 3", got)
	}

	srv.ScheduleClock.Advance(time.Second)
	srv.Settle(t)
	if frames := drainFrames(t, srv.Client); len(frames) != 0 {
		t.Fatalf("the refresh sent the founder %x, want nothing", opcodes(frames))
	}
	if got := pledgeListRank(t, srv.Client); got != 1 {
		t.Errorf("Knights rank after the refresh = %d, want 1", got)
	}
	others, _ := table.Get(ladderOthersID)
	if got := others.Info().Rank; got != 2 {
		t.Errorf("Others rank after the refresh = %d, want 2", got)
	}
	if got := rivals.Info().Rank; got != 0 {
		t.Errorf("Rivals rank after the refresh = %d, want 0", got)
	}
}
