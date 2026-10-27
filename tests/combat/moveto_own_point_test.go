package combat

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

// The template ids of the own-point walk scenario: the monster its behavior
// is bound to, and the unbound one the hook attacks.
const (
	ownPointWalker  = int32(20040)
	ownPointMachine = int32(20041)
)

// TestHostileMoveToOnItsOwnPointWalksAZeroLengthMove pins a monster taking
// up a walk to the point it stands on, under a behavior whose move-finished
// hook queues a light attack on a second monster while the walker hates no
// one. The walk step broadcasts a zero-length MoveToLocation (origin ==
// destination) and opens no hook; the next move tick arrives and opens
// move-finished. That hook's attack desire runs desire selection in place,
// which takes the still queued, heavier walk up again on the same point: a
// second zero-length broadcast, and the arrival then drops the walk's
// desire. The tick after arrives again and opens move-finished a second
// time; the walker now hates the machine, so no selection runs and nothing
// is broadcast.
func TestHostileMoveToOnItsOwnPointWalksAZeroLengthMove(t *testing.T) {
	t.Parallel()
	var (
		mu      sync.Mutex
		hooks   []string
		machine *npc.Hostile
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ownPointWalker}, Hooks: script.Hooks{
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
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{ownPointWalker: script.KindHostile, ownPointMachine: script.KindHostile},
			[]script.Listing{{Path: "ai.MoveToOwnPoint"}},
			script.Catalog{"ai.MoveToOwnPoint": behavior},
		),
	)
	// On the wall clock a walk's arrival timer and the move tick race within
	// one interval, so which tick arrives is not pinned there.
	if !srv.DrivesClock() {
		t.Skip("pinning the arrival tick by tick needs the driven clock")
	}
	c := srv.Client
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	at := location.Location{X: x + 400, Y: y, Z: z}
	walker := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(ownPointWalker, nil), at, at)
	far := location.Location{X: x + 600, Y: y, Z: z}
	m := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(ownPointMachine, nil), far, far)
	mu.Lock()
	machine = m
	mu.Unlock()
	srv.AI.Add(walker)
	id := walker.ObjectID()
	drainUntilQuiet(t, c)
	runAICycle(t, srv) // the first AI cycle: desire selection runs from the next one
	drainUntilQuiet(t, c)

	wx, wy, wz := walker.Position()
	here := location.Location{X: wx, Y: wy, Z: wz}
	if !walker.AI().AddMoveToDesire(here, 1e6) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	walk := &ai.Desire{Kind: ai.IntentionMoveTo, Location: here}
	point := func(f []byte, at int) location.Location {
		v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[at+4*i:]))) }
		return location.Location{X: v(0), Y: v(1), Z: v(2)}
	}
	assertZeroLengthMove := func(what string, want int) {
		t.Helper()
		var moves [][]byte
		for _, f := range c.ReadQueued() {
			if len(f) >= 29 && f[0] == serverpackets.OpcodeMoveToLocation && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
				moves = append(moves, f)
			}
		}
		if len(moves) != want {
			t.Fatalf("MoveToLocation frames %s = %d, want %d", what, len(moves), want)
		}
		for _, mv := range moves {
			if dest, origin := point(mv, 5), point(mv, 17); dest != here || origin != here {
				t.Fatalf("MoveToLocation %s = %+v -> %+v, want the zero-length move on %+v", what, origin, dest, here)
			}
		}
	}

	runAICycle(t, srv)
	assertZeroLengthMove("on the walk step", 1)
	if got := take(); len(got) != 0 {
		t.Fatalf("hooks on the walk step = %q, want none before the move tick", got)
	}
	if got := walker.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the walk step = %v, want %v", got, ai.IntentionMoveTo)
	}
	if !walker.Move().Moving() || !walker.AI().Desires().Has(walk) {
		t.Fatalf("moving = %v, walk queued = %v after the walk step; want both", walker.Move().Moving(), walker.AI().Desires().Has(walk))
	}

	finished := fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", here.X, here.Y, here.Z)
	srv.TickPositions()
	assertZeroLengthMove("on the first arrival", 1)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the first move tick = %q, want %q", got, finished)
	}
	if walker.AI().Desires().Has(walk) {
		t.Fatal("the walk's desire is still queued after its arrival")
	}
	if !walker.Move().Moving() {
		t.Fatal("not moving after the first arrival, want the walk taken up again")
	}

	srv.TickPositions()
	assertZeroLengthMove("on the second arrival", 0)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the second move tick = %q, want %q", got, finished)
	}
	if walker.Move().Moving() {
		t.Fatal("still moving after the second arrival")
	}
	if !walker.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: m}) {
		t.Fatal("the hook's attack desire on the machine is gone")
	}
}
