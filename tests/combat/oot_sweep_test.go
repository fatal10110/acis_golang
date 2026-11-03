package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestHostileFirstStaleHateSweepRuns100msAfterTheArrival pins the
// out-of-territory stale-hate sweep's first run (NpcAI.onEvtArrived ->
// ThreadPool.scheduleAtFixedRate(..., 100, 10000)): a monster whose threat
// entry is already 90 s old when it walks out of its territory keeps that
// hate 99 ms after the arrival and loses it at 100 ms, on its own timer,
// with no AI cycle in between.
func TestHostileFirstStaleHateSweepRuns100msAfterTheArrival(t *testing.T) {
	t.Parallel()
	srv, hostile, _, _ := scriptDesireMonster(t, 150, 0)
	if !srv.DrivesClock() {
		t.Skip("aging the threat entry 90 s and timing the first sweep need the driven clock")
	}
	victim := livePlayer(t, srv, srv.SoleObjectID(t)).(attackable.Combatant)
	hostile.AddDamageHate(victim, 0, 100)
	srv.Advance(t, 90*time.Second)

	x, y, z := hostile.Position()
	out := location.Location{X: x, Y: y + 300, Z: z}
	if !hostile.AI().AddMoveToDesire(out, 1_000) {
		t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", out)
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	walkUntilArrived(t, srv, hostile)
	if hostile.InTerritory() {
		t.Fatal("the walk ended inside the monster's territory, want outside")
	}

	srv.Advance(t, 99*time.Millisecond)
	if got := hostile.AggroHate(victim); got != 100 {
		t.Fatalf("hate 99 ms after the arrival = %v, want 100 kept", got)
	}
	srv.Advance(t, time.Millisecond)
	if got := hostile.AggroHate(victim); got != 0 {
		t.Fatalf("hate 100 ms after the arrival = %v, want 0 (stale hate stopped)", got)
	}
}
