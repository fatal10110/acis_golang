package ai

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestFolkMoveToItsStandingPositionFinishesInPlace pins a civilian NPC
// taking up a walk to its spawn point while it stands there facing its
// spawn heading: the walk step opens move-finished and drops the desire
// without a move. Turned away, the same point is a zero-length walk that
// opens no hook on the walk step.
func TestFolkMoveToItsStandingPositionFinishesInPlace(t *testing.T) {
	t.Parallel()
	var (
		mu    sync.Mutex
		hooks []string
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				mu.Lock()
				defer mu.Unlock()
				hooks = append(hooks, fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", e.X, e.Y, e.Z))
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
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkMoveToPosition": behavior})
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	srv.ReadQueued(t, srv.Client)
	cycle(t, srv) // the first AI cycle: desire selection runs from the next one

	n := script.NPCOf(f)
	home := n.SpawnLocation()
	here := location.Location{X: home.X, Y: home.Y, Z: home.Z}
	if x, y, z := f.Position(); (location.Location{X: x, Y: y, Z: z}) != here || f.Heading() != home.Heading {
		t.Fatalf("folk at %d,%d,%d heading %d, want its spawn point %+v", x, y, z, f.Heading(), home)
	}
	walk := &ai.Desire{Kind: ai.IntentionMoveTo, Location: here}

	n.AddMoveToDesire(home, 1e6)
	if moves := framesOf(cycle(t, srv), serverpackets.OpcodeMoveToLocation, f.ObjectID()); len(moves) != 0 {
		t.Fatalf("MoveToLocation frames on the spawn point = %d, want none", len(moves))
	}
	finished := fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", here.X, here.Y, here.Z)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the walk step = %q, want %q", got, finished)
	}
	if f.Desires().Has(walk) || f.IsMoving() {
		t.Fatalf("walk queued = %v, moving = %v after finishing in place; want neither", f.Desires().Has(walk), f.IsMoving())
	}

	n.SetHeading(home.Heading + 1000)
	n.AddMoveToDesire(home, 1e6)
	moves := framesOf(cycle(t, srv), serverpackets.OpcodeMoveToLocation, f.ObjectID())
	if len(moves) != 1 {
		t.Fatalf("MoveToLocation frames turned away = %d, want the zero-length walk", len(moves))
	}
	if dest, origin := moveDestination(moves[0]), moveOrigin(moves[0]); dest != here || origin != here {
		t.Fatalf("MoveToLocation turned away = %+v -> %+v, want the zero-length move on %+v", origin, dest, here)
	}
	if got := take(); len(got) != 0 {
		t.Fatalf("hooks on the turned walk step = %q, want none before the move tick", got)
	}
	if !f.Desires().Has(walk) {
		t.Fatal("the turned walk's desire is gone before its arrival")
	}
}
