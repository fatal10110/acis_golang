package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestClickOnBoatDoesNothing pins that a boat cannot be acted on: a click,
// shift-click or forced attack on it selects nothing and answers nothing,
// not even ActionFailed.
func TestClickOnBoatDoesNothing(t *testing.T) {
	t.Parallel()
	itinerary, _ := runeRoundTrip()
	srv, _ := bootAt(t, runeDock, itinerary)
	c := srv.Client
	b := srv.Boats.Boats()[0].ObjectID()

	testsupport.SyncBarrier(t, c, func() {
		c.Send(encodeAction(b, false))
		c.Send(encodeAction(b, true))
		c.Send(encodeAttackRequest(b))
		c.Send(encodeRequestManorList())
	}, serverpackets.OpcodeExtended)
}
