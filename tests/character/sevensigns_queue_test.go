package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSevenSignsPeriodChangeOnItsQueue pins the period change run as steps
// on the Seven Signs queue, each waiting step continuing there: the order
// stays announce, save, expel, sky, re-arm. As the competition ends with
// Dawn leading, a Dusk member in a dungeon hears the change announced, is
// sent to town and out of the dungeon, then sees the new sky; the stored
// status row carries the results period stamped with the calendar's clock,
// and the next change is armed.
func TestSevenSignsPeriodChangeOnItsQueue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 24, 18, 0, 0, 0, time.Local)
	w := bootSevenSigns(t, sevenSignsSetup{status: dawnLeads(sevensigns.Competition), cabal: sevensigns.Dusk, dungeon: true},
		gameservertest.WithRestartPoints(escapeTown(dungeonTown)),
		gameservertest.WithSevenSignsClock(func() time.Time { return now }))
	w.enter(t)

	change := w.change
	w.change = nil
	change()
	frames := readUntilQuiet(w.srv.Client)

	sound := firstOpcode(frames, serverpackets.OpcodePlaySound)
	ended := firstOpcode(frames, serverpackets.OpcodeSystemMessage)
	teleport, expelled := teleportedToTown(t, w, frames)
	sky := firstOpcode(frames, serverpackets.OpcodeSSQInfo)
	if sound != 0 || ended != 1 || !expelled || teleport < ended || sky != len(frames)-1 || teleport > sky {
		t.Fatalf("period change frames = %x, want the sound and end message, then the expulsion, then the closing SSQInfo", opcodes(frames))
	}
	if w.online(t).In7sDungeon() {
		t.Fatal("still in the dungeon after the period change")
	}

	var period string
	var date int64
	if err := w.srv.DB.QueryRow(`SELECT active_period, date FROM seven_signs_status WHERE id = 0`).Scan(&period, &date); err != nil {
		t.Fatalf("read seven_signs_status: %v", err)
	}
	if period != "RESULTS" || date != now.UnixMilli() {
		t.Fatalf("status row = (%s, %d), want (RESULTS, %d)", period, date, now.UnixMilli())
	}
	if w.change == nil {
		t.Fatal("the next period change was not armed")
	}
}
