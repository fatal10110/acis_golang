package combat

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSiegeAttackerRevivedDuringWaitStays: a dead attacker inside the siege
// asks to restart to town, and a healer standing just outside the
// battlefield resurrects it while the attacker respawn delay runs. Once the
// delay has passed the waiting restart does nothing: no TeleportToLocation,
// and the attacker stays where it was revived.
func TestSiegeAttackerRevivedDuringWaitStays(t *testing.T) {
	t.Parallel()
	// The battlefield spans x -1000..300; the attacker, sent at login to the
	// town restart point on it, lies near its east edge and the healer
	// stands in cast range outside it.
	srv, objID, _ := bootSiegeRestart(t, siegeRestarter{side: "ATTACKER", fieldEast: 300},
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{resurrectSkill()})))
	victim := srv.Client
	healer := srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	seedKnownSkill(t, srv, healer.ID, resurrectSkillID, 1)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = 550, y = 400, z = -300 WHERE obj_Id = ?", healer.ID); err != nil {
		t.Fatalf("place healer: %v", err)
	}
	hc := srv.DialClient(t, "healer", 1)
	startInWorld(t, hc)
	drainUntilQuiet(t, victim)

	victim.Send(encodeRequestRestartPoint(0))
	srv.Settle(t)
	selectPlayerTarget(t, hc, objID)
	drainUntilQuiet(t, hc)
	hc.Send(encodeRequestMagicSkillUse(resurrectSkillID, false, false))
	srv.Settle(t)
	srv.Advance(t, 600*time.Millisecond)
	if offer := indexOfOpcode(readQuiet(victim), 0, serverpackets.OpcodeConfirmDlg); offer < 0 {
		t.Fatal("attacker never got the resurrection offer")
	}
	victim.Send(encodeDlgAnswer(resurrectionDlgID, 1, 0))
	srv.Settle(t)
	if srv.PlayerDead(t, objID) {
		t.Fatal("accepted resurrection left the attacker dead")
	}
	revivedAt := siegeRestarterAt(t, srv, objID)
	if !nearAny(revivedAt, []location.Location{townRestartPoint}) {
		t.Fatalf("revived at %+v, want where it lay, by %+v", revivedAt, townRestartPoint)
	}
	drainUntilQuiet(t, victim)

	srv.Advance(t, siegeRespawn)
	for _, frame := range srv.ReadQueued(t, victim) {
		if frame[0] == serverpackets.OpcodeTeleportToLocation {
			t.Fatal("the waiting restart teleported the revived attacker")
		}
	}
	if at := siegeRestarterAt(t, srv, objID); at != revivedAt {
		t.Fatalf("attacker moved from %+v to %+v after the delay", revivedAt, at)
	}
}

// siegeRestarterAt is objID's position in the world.
func siegeRestarterAt(t *testing.T, srv *gameservertest.Server, objID int32) location.Location {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	x, y, z := obj.(interface{ Position() (int, int, int) }).Position()
	return location.Location{X: x, Y: y, Z: z}
}
