package character

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestRecommendationUpdateHoldsNoQueue holds character_recommends under a
// write lock across the daily refresh, so its stored half waits on the
// database with every persistence lane paused: the start returns to the
// schedule's queue at once, both online players' queues still send their
// refreshed UserInfo while the lock lasts, and the stored-only Sleeper keeps
// its counters until the lock is released, when the stored refresh lands.
// Were the stored refresh made on the schedule's queue, the clock advance
// would wait on the lock until the refresh's own timeout gave up on it.
func TestRecommendationUpdateHoldsNoQueue(t *testing.T) {
	w := bootRecommendationTask(t, gameservertest.WithCapturedLog())
	clock := w.srv.ScheduleClock
	clock.Advance(recommendationTaskDue.Sub(clock.Now()) - time.Second)
	w.srv.Settle(t)

	ctx := context.Background()
	lock, err := w.srv.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `LOCK TABLES character_recommends WRITE`); err != nil {
		t.Fatal(err)
	}
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			if _, err := lock.ExecContext(ctx, `UNLOCK TABLES`); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer unlock()

	resume := w.srv.ExpectEveryLanePaused(t)
	clock.Advance(time.Second)
	// The stored half starts with the table reset: once it waits on the
	// lock, the online half has posted every UserInfo.
	awaitStatementOnLock(t, w, "TRUNCATE character_recommends%")
	for name, c := range map[string]*testsupport.ScriptedClient{"Giver": w.giver, "Taker": w.taker} {
		if frame := c.Read(); frame[0] != serverpackets.OpcodeUserInfo {
			t.Fatalf("%s got opcode %#x under the lock, want UserInfo (%#x)", name, frame[0], serverpackets.OpcodeUserInfo)
		}
	}
	if have, left := recommendationColumns(t, w.srv, w.sleeperID); have != 10 || left != 0 {
		t.Fatalf("stored Sleeper under the lock rec_have %d rec_left %d, want 10 and 0", have, left)
	}

	unlock()
	w.srv.Settle(t)
	resume()
	w.srv.FlushPersistence(t)
	if log := w.srv.LogText(); strings.Contains(log, "scheduled refresh") {
		t.Fatalf("the refresh failed:\n%s", log)
	}
	if have, left := recommendationColumns(t, w.srv, w.sleeperID); have != 7 || left != 9 {
		t.Fatalf("stored Sleeper after the lock rec_have %d rec_left %d, want 7 and 9", have, left)
	}
	var rows int
	if err := w.srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("character_recommends rows = %d, %v; want 0", rows, err)
	}
}

// awaitStatementOnLock returns once a statement of the test's database
// matching the LIKE pattern info waits on a table lock, failing the test if
// none ever does.
func awaitStatementOnLock(t *testing.T, w *recommendationTaskWorld, info string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no %q statement waited on the table lock", info)
		}
		var n int
		if err := w.srv.DB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE ?`, info).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
	}
}
