package clan

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The raid point reset suite's clans and stored characters. The shipped
// RaidPointReset entry (MONTHLY_WEEK, TUE-1 00:00:00) booted at resetBoot
// is first due at resetDue (the schedule.calendar golden).
const (
	lowlingsClanID int32 = 0x7f000051
	squireCharID   int32 = 0x7f100051
	lowlingCharID  int32 = 0x7f100052
	lonerCharID    int32 = 0x7f100053
	raidBossID     int32 = 25001
)

var (
	resetBoot = time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	resetDue  = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

// bootRaidPointReset boots the founder, in the world, leading Knights
// (level 5, reputation 1000) with the stored member Squire; Lowlings
// (level 4) with the stored member Lowling; and the clanless Loner. The
// raid point reset runs from resetBoot.
func bootRaidPointReset(t *testing.T, extra ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedStatements(t, db,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT `+itoa(titleClanID)+`, 'Knights', 5, 1000, obj_Id FROM characters WHERE char_name = 'Founder'`,
				`UPDATE characters SET clanid = `+itoa(titleClanID)+`, power_grade = 0 WHERE char_name = 'Founder'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (`+itoa(lowlingsClanID)+`, 'Lowlings', 4, `+itoa(lowlingCharID)+`)`,
				`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES
					('filler', `+itoa(squireCharID)+`, 'Squire', 40, `+itoa(titleClanID)+`, 6),
					('filler', `+itoa(lowlingCharID)+`, 'Lowling', 40, `+itoa(lowlingsClanID)+`, 0),
					('filler', `+itoa(lonerCharID)+`, 'Loner', 40, 0, 0)`,
			)
		}),
		shippedTask(t, "task.RaidPointReset", scripttask.RaidPointReset),
		gameservertest.WithScheduledTasks(resetBoot),
	}, extra...)...)
	startInWorld(t, srv.Client)
	drainFrames(t, srv.Client)
	return srv
}

func countRows(t *testing.T, srv *gameservertest.Server, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := srv.DB.QueryRowContext(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// TestRaidPointResetRewardsClans ranks the founder first, Lowling second,
// Squire third and Loner fourth. At the first monthly reset, not before,
// Knights gains 1250 + 700 for its first and third places; Lowlings, below
// level 5, and the clanless Loner earn nothing. The founder sees its clan's
// new score, every raid point is forgotten and character_raid_points is
// emptied.
func TestRaidPointResetRewardsClans(t *testing.T) {
	for _, exec := range executors {
		t.Run(exec.name, func(t *testing.T) { raidPointResetRewardsClans(t, exec.opts...) })
	}
}

// executors are the actor queue executors the reset's scenarios run on:
// the suite's inline queues and the production pool.
var executors = []struct {
	name string
	opts []gameservertest.Option
}{
	{"inline", nil},
	{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
}

func raidPointResetRewardsClans(t *testing.T, extra ...gameservertest.Option) {
	srv := bootRaidPointReset(t, extra...)
	founderID := srv.SoleObjectID(t)
	srv.RaidPoints.Add(founderID, raidBossID, 60)
	srv.RaidPoints.Add(lowlingCharID, raidBossID, 50)
	srv.RaidPoints.Add(squireCharID, raidBossID, 40)
	srv.RaidPoints.Add(lonerCharID, raidBossID, 30)

	srv.ScheduleClock.Advance(resetDue.Sub(srv.ScheduleClock.Now()) - time.Second)
	srv.Settle(t)
	srv.FlushPersistence(t)
	if got := countRows(t, srv, `SELECT COUNT(*) FROM character_raid_points`); got != 4 {
		t.Fatalf("raid point rows before the reset = %d, want 4", got)
	}
	if frames := drainFrames(t, srv.Client); len(frames) != 0 {
		t.Fatalf("the founder got %x before the reset, want nothing", opcodes(frames))
	}

	srv.ScheduleClock.Advance(time.Second)
	srv.Settle(t)
	srv.FlushPersistence(t)

	frames := drainFrames(t, srv.Client)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodePledgeShowInfoUpdate {
		t.Fatalf("the founder got %x at the reset, want one PledgeShowInfoUpdate", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	for range 6 { // clan id, crest, level, castle, hall, rank
		r.ReadInt32()
	}
	if got := r.ReadInt32(); got != 2950 {
		t.Errorf("shown reputation = %d, want 2950", got)
	}
	if got := countRows(t, srv, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, titleClanID); got != 2950 {
		t.Errorf("stored Knights reputation = %d, want 2950", got)
	}
	if got := countRows(t, srv, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, lowlingsClanID); got != 0 {
		t.Errorf("stored Lowlings reputation = %d, want 0", got)
	}
	if got := countRows(t, srv, `SELECT COUNT(*) FROM character_raid_points`); got != 0 {
		t.Errorf("raid point rows after the reset = %d, want 0", got)
	}
	for _, id := range []int32{founderID, lowlingCharID, squireCharID, lonerCharID} {
		if rec := srv.RaidPoints.Record(id); rec.Found {
			t.Errorf("player %d still has raid points %+v after the reset", id, rec)
		}
	}
}

// TestRaidPointResetWipeRacesAward awards Loner raid points from another
// goroutine while the reset runs. Whichever side of the wipe each award
// falls on, the stored points end equal to the points held: a total
// queued before the wipe never lands after it.
func TestRaidPointResetWipeRacesAward(t *testing.T) {
	for _, exec := range executors {
		t.Run(exec.name, func(t *testing.T) { raidPointResetWipeRacesAward(t, exec.opts...) })
	}
}

func raidPointResetWipeRacesAward(t *testing.T, extra ...gameservertest.Option) {
	srv := bootRaidPointReset(t, extra...)
	srv.RaidPoints.Add(lonerCharID, raidBossID, 30)
	srv.ScheduleClock.Advance(resetDue.Sub(srv.ScheduleClock.Now()) - time.Second)
	srv.Settle(t)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 200 {
			srv.RaidPoints.Add(lonerCharID, raidBossID, 1)
		}
	})
	srv.ScheduleClock.Advance(time.Second)
	wg.Wait()
	srv.Settle(t)
	srv.FlushPersistence(t)

	rec := srv.RaidPoints.Record(lonerCharID)
	stored := countRows(t, srv, `SELECT COUNT(*) FROM character_raid_points WHERE char_id = ?`, lonerCharID)
	switch {
	case !rec.Found && stored != 0:
		t.Fatalf("Loner holds no raid points but %d rows are stored", stored)
	case rec.Found:
		if got := countRows(t, srv, `SELECT COALESCE(SUM(points), 0) FROM character_raid_points WHERE char_id = ?`, lonerCharID); got != int64(rec.Total) {
			t.Fatalf("stored raid points = %d, want the %d held", got, rec.Total)
		}
	}
}
