package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestGroundClickAnswersActionFailedAwayFromEntrances pins
// MoveBackwardToLocation's boat-entrance probe (Playable.tryToPassBoatEntrance,
// Playable.java:826-845): a ground click ashore that knows no boat, or whose
// walk crosses no entrance of the dock a boat it knows serves, is answered
// ActionFailed ahead of its walk. A click across the entrance walks there
// with no ActionFailed (TestBoardingNeedsLeave).
func TestGroundClickAnswersActionFailedAwayFromEntrances(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		at   location.Location
	}{
		{name: "no boat in sight", at: offset(runeShore, 20_000, 0)},
		{name: "boat in sight, no entrance crossed", at: runeShore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, me, _ := bootPassenger(t, tc.at, 0)
			target := offset(tc.at, 300, 0)
			c.Send(encodeMoveBackward(target, tc.at))
			assertLog(t, "ground click", passengerLog(t, srv, c), af, move(me, target, tc.at))
		})
	}
}
