package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// moveOrigin reads a MoveToLocation frame's origin.
func moveOrigin(f []byte) location.Location {
	v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[17+4*i:]))) }
	return location.Location{X: v(0), Y: v(1), Z: v(2)}
}

// TestFolkMoveToOnItsOwnPointWalksAZeroLengthMove pins a civilian NPC
// taking up a walk to the point it stands on, under a behavior whose
// move-finished hook queues a light attack on a second NPC while the NPC
// hates no one (the Doctor Chaos machine arrival). The walk step broadcasts
// a zero-length MoveToLocation (origin == destination) and opens no hook;
// the next move tick arrives and opens move-finished. That hook's attack
// desire runs desire selection in place, which takes the still queued,
// heavier walk up again on the same point: a second zero-length broadcast,
// and the arrival then drops the walk's desire. The tick after arrives
// again and opens move-finished a second time; the NPC now hates the
// machine, so no selection runs and nothing is broadcast.
func TestFolkMoveToOnItsOwnPointWalksAZeroLengthMove(t *testing.T) {
	t.Parallel()
	var (
		mu      sync.Mutex
		hooks   []string
		machine *npc.Folk
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				mu.Lock()
				hooks = append(hooks, fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", e.X, e.Y, e.Z))
				target := machine
				mu.Unlock()
				e.NPC.AddAttackDesire(script.NPCOf(target), 10)
			},
		}}
	}
	take := func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := hooks
		hooks = nil
		return got
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkMoveToOwnPoint": behavior})
	// On the wall clock a walk's arrival timer and the move tick race within
	// one interval, so which tick arrives is not pinned there.
	if !srv.DrivesClock() {
		t.Skip("pinning the arrival tick by tick needs the driven clock")
	}
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	m := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(folkOfferingID)), location.Location{X: at.X + 200, Y: at.Y, Z: at.Z})
	mu.Lock()
	machine = m
	mu.Unlock()
	srv.ReadQueued(t, srv.Client)
	cycle(t, srv) // the first AI cycle: desire selection runs from the next one

	x, y, z := f.Position()
	here := location.Location{X: x, Y: y, Z: z}
	if !f.AddMoveToDesire(here, 1e6) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	walk := &ai.Desire{Kind: ai.IntentionMoveTo, Location: here}
	assertZeroLengthMove := func(what string, frames [][]byte, want int) {
		t.Helper()
		moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, f.ObjectID())
		if len(moves) != want {
			t.Fatalf("MoveToLocation frames %s = %d, want %d", what, len(moves), want)
		}
		for _, mv := range moves {
			if dest, origin := moveDestination(mv), moveOrigin(mv); dest != here || origin != here {
				t.Fatalf("MoveToLocation %s = %+v -> %+v, want the zero-length move on %+v", what, origin, dest, here)
			}
		}
	}

	assertZeroLengthMove("on the walk step", cycle(t, srv), 1)
	if got := take(); len(got) != 0 {
		t.Fatalf("hooks on the walk step = %q, want none before the move tick", got)
	}
	if !f.IsMoving() || !f.Desires().Has(walk) {
		t.Fatalf("moving = %v, walk queued = %v after the walk step; want both", f.IsMoving(), f.Desires().Has(walk))
	}

	finished := fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", here.X, here.Y, here.Z)
	srv.TickPositions()
	assertZeroLengthMove("on the first arrival", srv.ReadQueued(t, srv.Client), 1)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the first move tick = %q, want %q", got, finished)
	}
	if f.Desires().Has(walk) {
		t.Fatal("the walk's desire is still queued after its arrival")
	}
	if !f.IsMoving() {
		t.Fatal("not moving after the first arrival, want the walk taken up again")
	}

	srv.TickPositions()
	assertZeroLengthMove("on the second arrival", srv.ReadQueued(t, srv.Client), 0)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the second move tick = %q, want %q", got, finished)
	}
	if f.IsMoving() {
		t.Fatal("still moving after the second arrival")
	}
	if !f.Desires().Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: m}) {
		t.Fatal("the hook's attack desire on the machine is gone")
	}
}
