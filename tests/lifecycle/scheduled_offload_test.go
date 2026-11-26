package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestScheduledSaveHoldsNoQueue holds the Seven Signs status table under a
// write lock across the hourly save: the save's start returns to the
// schedule's queue at once, a character selection is still served while
// the database holds the save, and the status row is written once the lock
// is released. Were the save made on the queue, the start would hold the
// test until the save's own timeout gave up on it.
func TestScheduledSaveHoldsNoQueue(t *testing.T) {
	t.Parallel()
	boot := time.Date(2026, 10, 5, 12, 59, 30, 0, time.UTC)
	hour := time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC)
	srv := gameservertest.Boot(t, append(shippedTask(t, "task.SevenSignsUpdate", scripttask.SevenSignsUpdate, boot),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) { row.Period = sevensigns.Competition }),
	)...)
	ctx := context.Background()
	if _, err := srv.DB.ExecContext(ctx, `UPDATE seven_signs_status SET active_period = 'RESULTS' WHERE id = 0`); err != nil {
		t.Fatal(err)
	}

	lock, err := srv.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `LOCK TABLES seven_signs_status WRITE`); err != nil {
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

	// The start runs on the test goroutine: were the save made on the
	// schedule's queue, this would wait on the lock.
	srv.ScheduleClock.Advance(hour.Sub(srv.ScheduleClock.Now()))
	srv.Client.Send(clientwire.RequestGameStart(0))
	readUntilOpcode(t, srv.Client, serverpackets.OpcodeCharSelected)
	var held string
	if err := lock.QueryRowContext(ctx, `SELECT active_period FROM seven_signs_status WHERE id = 0`).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != "RESULTS" {
		t.Fatalf("status under the lock = %s, want it untouched", held)
	}

	unlock()
	srv.Settle(t)
	if _, period, _ := statusRow(t, srv.DB); period != "COMPETITION" {
		t.Fatalf("status after the lock = %s, want COMPETITION", period)
	}
	if log := srv.LogText(); strings.Contains(log, "scheduled save") {
		t.Fatalf("the save failed:\n%s", log)
	}
}
