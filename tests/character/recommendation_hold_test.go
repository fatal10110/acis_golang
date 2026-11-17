package character

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestRecommendationUpdateHoldsNoQueue holds a persistence lane across the
// daily refresh, so its stored half waits on the hold: the start returns to
// the schedule's queue at once, both online players are still sent their
// refreshed UserInfo while the hold lasts, and the stored-only Sleeper keeps
// its counters until the hold is released, when the stored refresh lands.
// Were the stored refresh made on the schedule's queue, the clock advance
// would wait on the hold until the refresh's own timeout gave up on it.
func TestRecommendationUpdateHoldsNoQueue(t *testing.T) {
	w := bootRecommendationTask(t, gameservertest.WithCapturedLog())
	clock := w.srv.ScheduleClock
	clock.Advance(recommendationTaskDue.Sub(clock.Now()) - time.Second)
	w.srv.Settle(t)

	release := w.srv.HoldPersistenceLane(t, w.sleeperID)
	resume := w.srv.ExpectEveryLanePaused(t)
	clock.Advance(time.Second)
	for name, c := range map[string]*testsupport.ScriptedClient{"Giver": w.giver, "Taker": w.taker} {
		if frame := c.Read(); frame[0] != serverpackets.OpcodeUserInfo {
			t.Fatalf("%s got opcode %#x under the hold, want UserInfo (%#x)", name, frame[0], serverpackets.OpcodeUserInfo)
		}
	}
	if have, left := recommendationColumns(t, w.srv, w.sleeperID); have != 10 || left != 0 {
		t.Fatalf("stored Sleeper under the hold rec_have %d rec_left %d, want 10 and 0", have, left)
	}

	release()
	w.srv.Settle(t)
	resume()
	w.srv.FlushPersistence(t)
	if log := w.srv.LogText(); strings.Contains(log, "scheduled refresh") {
		t.Fatalf("the refresh failed:\n%s", log)
	}
	if have, left := recommendationColumns(t, w.srv, w.sleeperID); have != 7 || left != 9 {
		t.Fatalf("stored Sleeper after the hold rec_have %d rec_left %d, want 7 and 9", have, left)
	}
	var rows int
	if err := w.srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("character_recommends rows = %d, %v; want 0", rows, err)
	}
}
