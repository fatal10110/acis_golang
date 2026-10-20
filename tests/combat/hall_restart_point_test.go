package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Reference: RestartPointData.getLocationToTeleport (RestartPointData.java
// 323-329): in the town tail, after the Seal of Strife point and before the
// nearest town, a player standing on the battlefield of a flag war hall
// siege restarts at the restart point the siege keeps for its clan
// (FlagWar.getClanRestartPoint); a clan with none goes to town.

// hallRestartPoint is the restart point Devastated Castle's siege keeps for
// the restarter's clan.
var hallRestartPoint = location.Location{X: 700, Y: -700, Z: -300}

// TestHallSiegeClanRestartPoint restarts a dead attacker to town on
// Devastated Castle's battlefield, its siege keeping a restart point for
// the attacker's clan or for another clan only; once the siege ended, its
// battlefield is off and the clan's point is no longer read.
func TestHallSiegeClanRestartPoint(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		clanID int32
		ended  bool
		want   location.Location
	}{
		{"clan with a restart point", restartClanID, false, hallRestartPoint},
		{"clan without one", siegeRivalClanID, false, townRestartPoint},
		{"battlefield off", restartClanID, true, townRestartPoint},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, _ := bootSiegeRestart(t, siegeRestarter{hall: true, side: "ATTACKER"})
			s, ok := srv.Halls.Siege(siegeHallID)
			if !ok {
				t.Fatal("Devastated Castle has no siege")
			}
			s.SetClanRestartPoint(tt.clanID, hallRestartPoint)
			if tt.ended {
				if !s.End() || s.Battlefield().Active() {
					t.Fatal("Devastated Castle's siege did not end with its battlefield off")
				}
				drainUntilQuiet(t, srv.Client)
			}
			c := srv.Client
			c.Send(encodeRequestRestartPoint(0))
			if at := readRestartTeleport(t, c, objID); !nearAny(at, []location.Location{tt.want}) {
				t.Fatalf("restart destination = %+v, want within scatter of %+v", at, tt.want)
			}
		})
	}
}
