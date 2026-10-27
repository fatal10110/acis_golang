package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestAlarmDeviceWithoutItsHallMaker (#4278): an alarm dying in hall 1, 2
// or 3 whose captain, self-destruct or guard maker is missing makes no
// draw, so it stays active: no shout, no maker event, every door closed.
func TestAlarmDeviceWithoutItsHallMaker(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hall  int32
		maker string
	}{
		{1, "godard32_2515_07m1"},
		{2, "godard32_2515_09m1"},
		{3, "godard32_2515_06m1"},
	} {
		t.Run(fmt.Sprintf("hall %d", tc.hall), func(t *testing.T) {
			t.Parallel()
			draws := &ind1Draws{}
			// A draw of 0 would de-activate the alarm in any hall.
			draws.set(0, 0)
			w := bootTombHalls(t, draws, tc.maker)
			w.makers.take()
			alarm, _ := w.spawnSeen(t, fhAlarmID, location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
			w.send(t, alarm, 10025, tc.hall)
			w.srv.ReadQueued(t, w.srv.Client)
			w.onQueue(t, alarm, func() { script.NPCOf(alarm).DoDie(nil) })
			w.srv.Advance(t, 3*time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)

			draws.mu.Lock()
			left := len(draws.queue)
			draws.mu.Unlock()
			if left != 1 {
				t.Fatalf("draws left after its death = %d, want 1 (no draw)", left)
			}
			if got := shouts(t, frames, alarm.ObjectID()); len(got) != 0 {
				t.Fatalf("shouts as it died = %q, want none", got)
			}
			if got := w.makers.take(); len(got) != 0 {
				t.Fatalf("maker events as it died = %q, want none", got)
			}
			if got, want := w.alarmDoorsOpen(t), make([]bool, len(fhAlarmDoors)); !slices.Equal(got, want) {
				t.Fatalf("doors open after its death = %v, want %v", got, want)
			}
		})
	}
}
