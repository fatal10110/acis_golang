package combat

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// moveFramesOf returns the MoveToLocation and TeleportToLocation frames
// about id among frames.
func moveFramesOf(frames [][]byte, id int32) (moves, teleports [][]byte) {
	for _, f := range frames {
		if len(f) < 5 || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		switch f[0] {
		case serverpackets.OpcodeMoveToLocation:
			moves = append(moves, f)
		case serverpackets.OpcodeTeleportToLocation:
			teleports = append(teleports, f)
		}
	}
	return moves, teleports
}

// framePoint reads the x, y, z triple at offset at of a frame.
func framePoint(f []byte, at int) location.Location {
	v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[at+4*i:]))) }
	return location.Location{X: v(0), Y: v(1), Z: v(2)}
}

// bootMoveToWalker boots a world with one monster of ownPointWalker, bound
// to behavior when it is set, standing 400 east of the character; the
// first AI cycle has run, so desire selection runs from the next one.
func bootMoveToWalker(t *testing.T, behavior func() script.Script) (*gameservertest.Server, *testsupport.ScriptedClient, *npc.Hostile) {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
	}
	if behavior != nil {
		opts = append(opts, gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{ownPointWalker: script.KindHostile},
			[]script.Listing{{Path: "ai.MoveToWalk"}},
			script.Catalog{"ai.MoveToWalk": behavior},
		))
	}
	srv := gameservertest.Boot(t, opts...)
	c := srv.Client
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	at := location.Location{X: x + 400, Y: y, Z: z}
	walker := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(ownPointWalker, nil), at, at)
	srv.AI.Add(walker)
	drainUntilQuiet(t, c)
	runAICycle(t, srv)
	drainUntilQuiet(t, c)
	return srv, c, walker
}

// TestHostileMoveToAtTheGeoFailLimitWalksAndKeepsTheCount pins a monster
// whose failed path-finding streak reached the limit that sends a return
// home to its spawn point by teleport: a move-to desire still walks to its
// point with the plain path-finding move, and a straight walk leaves the
// streak the scripts read untouched.
func TestHostileMoveToAtTheGeoFailLimitWalksAndKeepsTheCount(t *testing.T) {
	t.Parallel()
	srv, c, walker := bootMoveToWalker(t, nil)
	for range move.HomeGeoFailLimit {
		walker.AddGeoPathFailCount()
	}
	x, y, z := walker.Position()
	dest := location.Location{X: x + 200, Y: y, Z: z}
	if !walker.AI().AddMoveToDesire(dest, 1e6) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	runAICycle(t, srv)
	moves, teleports := moveFramesOf(c.ReadQueued(), walker.ObjectID())
	if len(teleports) != 0 {
		t.Fatalf("TeleportToLocation frames = %d, want none for a move-to", len(teleports))
	}
	if len(moves) != 1 {
		t.Fatalf("MoveToLocation frames = %d, want 1", len(moves))
	}
	if got := framePoint(moves[0], 5); got != dest {
		t.Fatalf("MoveToLocation destination = %+v, want %+v", got, dest)
	}
	if !walker.Move().Moving() {
		t.Fatal("not moving after the walk step")
	}
	if got := walker.GeoPathFailCount(); got != move.HomeGeoFailLimit {
		t.Fatalf("GeoPathFailCount() after the walk step = %d, want %d kept", got, move.HomeGeoFailLimit)
	}
}

// TestHostileMoveToItsStandingPositionFinishesInPlace pins a monster
// taking up a walk to its spawn point while it stands there facing its
// spawn heading: the walk step opens move-finished and drops the desire
// without a move. Turned away, the same point is a zero-length walk that
// opens no hook on the walk step.
func TestHostileMoveToItsStandingPositionFinishesInPlace(t *testing.T) {
	t.Parallel()
	var (
		mu    sync.Mutex
		hooks []string
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ownPointWalker}, Hooks: script.Hooks{
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
	srv, c, walker := bootMoveToWalker(t, behavior)
	n := script.NPCOf(walker)
	home := n.SpawnLocation()
	here := location.Location{X: home.X, Y: home.Y, Z: home.Z}
	if x, y, z := walker.Position(); (location.Location{X: x, Y: y, Z: z}) != here || walker.Heading() != home.Heading {
		t.Fatalf("walker at %d,%d,%d heading %d, want its spawn point %+v", x, y, z, walker.Heading(), home)
	}
	walk := &ai.Desire{Kind: ai.IntentionMoveTo, Location: here}

	n.AddMoveToDesire(home, 1e6)
	runAICycle(t, srv)
	moves, _ := moveFramesOf(c.ReadQueued(), walker.ObjectID())
	if len(moves) != 0 {
		t.Fatalf("MoveToLocation frames on the spawn point = %d, want none", len(moves))
	}
	finished := fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", here.X, here.Y, here.Z)
	if got := take(); !slices.Equal(got, []string{finished}) {
		t.Fatalf("hooks on the walk step = %q, want %q", got, finished)
	}
	if walker.AI().Desires().Has(walk) || walker.Move().Moving() {
		t.Fatalf("walk queued = %v, moving = %v after finishing in place; want neither", walker.AI().Desires().Has(walk), walker.Move().Moving())
	}

	n.SetHeading(home.Heading + 1000)
	n.AddMoveToDesire(home, 1e6)
	runAICycle(t, srv)
	moves, _ = moveFramesOf(c.ReadQueued(), walker.ObjectID())
	if len(moves) != 1 {
		t.Fatalf("MoveToLocation frames turned away = %d, want the zero-length walk", len(moves))
	}
	if dest, origin := framePoint(moves[0], 5), framePoint(moves[0], 17); dest != here || origin != here {
		t.Fatalf("MoveToLocation turned away = %+v -> %+v, want the zero-length move on %+v", origin, dest, here)
	}
	if got := take(); len(got) != 0 {
		t.Fatalf("hooks on the turned walk step = %q, want none before the move tick", got)
	}
	if !walker.AI().Desires().Has(walk) {
		t.Fatal("the turned walk's desire is gone before its arrival")
	}
}
