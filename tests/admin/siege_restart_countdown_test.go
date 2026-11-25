package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// TestAdminSiegeRestartRunsOneCountdown pins the parity exception of
// #5016: //siege end stops the ended siege's clock, so a //siege start
// right after runs one countdown. Up to 25 minutes before the new siege's
// end the attacking Raider hears the hour left and the 30 minutes left
// once each.
func TestAdminSiegeRestartRunsOneCountdown(t *testing.T) {
	t.Parallel()
	w := bootSiegeAdmin(t)
	if !w.srv.DrivesClock() {
		t.Skip("the siege clock is waited out on the test clock")
	}
	gludio := w.srv.Sieges.All()[0]
	w.target(t, w.raiderID)
	for _, args := range []string{"attack gludio_castle", "start gludio_castle", "end gludio_castle", "attack gludio_castle", "start gludio_castle"} {
		w.siege(t, args)
	}
	if !gludio.InProgress() {
		t.Fatal("siege not under way after end and start")
	}
	drain(t, w.raider)

	w.srv.Advance(t, siege.DefaultConfig().Length-25*time.Minute)
	told := map[int]int{}
	for _, f := range w.raiderFrames(t) {
		if id := systemMessageID(f); id >= 0 {
			told[id]++
		}
	}
	if told[siege.MsgHoursUntilConclusion] != 1 || told[siege.MsgMinutesUntilConclusion] != 1 {
		t.Fatalf("the Raider was told %v, want the hour left (%d) and the 30 minutes left (%d) once each",
			told, siege.MsgHoursUntilConclusion, siege.MsgMinutesUntilConclusion)
	}
	if !gludio.InProgress() {
		t.Fatal("siege ended early")
	}
}
