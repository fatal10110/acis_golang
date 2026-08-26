package lifecycle

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func assertSystemMessageID(t *testing.T, frame []byte, want int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(want) {
		t.Fatalf("system message id = %d, want %d", id, want)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("system message params = %d, want 0", params)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func statusRow(t *testing.T, db *sql.DB) (cycle int, period string, date int64) {
	t.Helper()
	if err := db.QueryRow(`SELECT current_cycle, active_period, date FROM seven_signs_status WHERE id = 0`).Scan(&cycle, &period, &date); err != nil {
		t.Fatalf("read seven_signs_status: %v", err)
	}
	return cycle, period, date
}

// The entry burst announces the active Seven Signs period right after the
// welcome message, and a calendar with no overdue change leaves the status
// row untouched.
func TestEnterWorldBurstCarriesCurrentPeriodMessage(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))

	frames := startInWorld(t, srv.Client)
	assertSystemMessageID(t, frames[4], serverpackets.SystemMessageWelcomeToLineage)
	assertSystemMessageID(t, frames[5], serverpackets.SystemMessageCompetitionPeriodBegun)

	cycle, period, date := statusRow(t, srv.DB)
	if cycle != 1 || period != "COMPETITION" || date != 0 {
		t.Fatalf("untouched status row = (%d, %s, %d), want (1, COMPETITION, 0)", cycle, period, date)
	}
}

// A period that ended while the server was down is caught up during boot:
// the status row advances and is re-stamped, and a client entering afterwards
// hears the new period's announcement.
func TestBootCatchesUpOverduePeriodAndReportsItOnEntry(t *testing.T) {
	staleSave := time.Now().Add(-time.Hour)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load status row: found=%v err=%v", found, err)
			}
			row.Cycle = 7
			row.Period = sevensigns.Results
			row.LastSave = staleSave
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed status row: %v", err)
			}
		}),
	)

	cycle, period, date := statusRow(t, srv.DB)
	deadline := time.Now().Add(2 * time.Second)
	for (period != "SEAL_VALIDATION" || cycle != 7 || date <= staleSave.UnixMilli()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		cycle, period, date = statusRow(t, srv.DB)
	}
	if period != "SEAL_VALIDATION" || cycle != 7 || date <= staleSave.UnixMilli() {
		t.Fatalf("status row after boot catch-up = (%d, %s, %d), want (7, SEAL_VALIDATION, fresh date)", cycle, period, date)
	}

	frames := startInWorld(t, srv.Client)
	assertSystemMessageID(t, frames[4], serverpackets.SystemMessageWelcomeToLineage)
	assertSystemMessageID(t, frames[5], serverpackets.SystemMessageValidationPeriodBegun)
}
