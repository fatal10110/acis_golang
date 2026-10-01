package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	// walkerID is Leandro, a reference walking id: walk stance.
	walkerID    = 31357
	walkerAlias = "scribe_leandro"
	walkerPage  = `<html><body>Leandro is busy.</body></html>`
)

// walkerTemplate is the fixture civilian template under the route alias.
func walkerTemplate() *npc.Template {
	tmpl := folkTemplate("Folk", walkerID)
	tmpl.Alias = walkerAlias
	tmpl.CanMove = true
	return tmpl
}

// walkerRoutes is a two-node route under the walker alias, neither node
// holding a delay, chat line or social action.
func walkerRoutes(a, b location.Location) route.WalkerRoutes {
	return route.WalkerRoutes{walkerAlias: {walkerAlias: {{Location: a}, {Location: b}}}}
}

// spawnWalker spawns the fixture walker at at on the a-b route.
func (w *folkWorld) spawnWalker(t *testing.T, at, a, b location.Location) *npc.Folk {
	t.Helper()
	f, _ := w.srv.SpawnRouteFolkNPCAt(t, walkerTemplate(), at, walkerRoutes(a, b), true)
	return f
}

// folkMoves decodes the MoveToLocation frames f is shown with: each
// destination and origin.
func folkMoves(frames [][]byte, f *npc.Folk) (dests, origins []location.Location) {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != f.ObjectID() {
			continue
		}
		dest := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		origin := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		dests, origins = append(dests, dest), append(origins, origin)
	}
	return dests, origins
}

func folkAt(f *npc.Folk) location.Location {
	x, y, z := f.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// TestRouteWalkerFolkWalksItsRoute pins a route-walking civilian NPC: shown
// in walk stance, it heads for its nearest route node the moment it spawns,
// at its walk speed, and on reaching it walks on to the next node, every
// leg broadcast to the players watching it.
func TestRouteWalkerFolkWalksItsRoute(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	at := location.Location{X: w.at.X + 150, Y: w.at.Y, Z: w.at.Z}
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	f := w.spawnWalker(t, at, a, b)

	// Reading only the two frames the spawn sends keeps the driven clock
	// where the walk started.
	frames := [][]byte{w.c.Read(), w.c.Read()}
	if got := opcodes(frames); got[0] != serverpackets.OpcodeNPCInfo || got[1] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("spawn frames = %#x, want NpcInfo then MoveToLocation", got)
	}
	if f.Running() {
		t.Fatal("walker shown running, want walk stance")
	}
	dests, origins := folkMoves(frames, f)
	if len(dests) != 1 || dests[0] != a || origins[0] != at {
		t.Fatalf("first leg = %v from %v, want one leg to nearest node %v from %v", dests, origins, a, at)
	}

	// 50 units at the fixture walk speed (50 * 0.84 DEX bonus = 42/s) take
	// 12 position updates; at its run speed (100.8/s) it would arrive in 5.
	// The wall clock cannot hold the walk at 1.1s.
	if w.srv.DrivesClock() {
		w.srv.Advance(t, 1100*time.Millisecond)
		if !f.IsMoving() || folkAt(f) == a {
			t.Fatalf("walker at %v after 1.1s, want still walking to %v", folkAt(f), a)
		}
	}
	w.srv.AdvanceUntil(t, "walker reaches its first node", func() bool { return folkAt(f) == a })

	dests, origins = folkMoves(drainFrames(t, w.c), f)
	if len(dests) != 1 || dests[0] != b || origins[0] != a {
		t.Fatalf("second leg = %v from %v, want one leg to %v from %v", dests, origins, b, a)
	}
	if !f.IsMoving() {
		t.Fatal("walker stopped at its first node, want it walking on")
	}
}

// TestInteractWithWalkingFolkStopsPlayer pins the interact with a civilian
// NPC under way on its route: the player's StopMove is shown instead of the
// MoveToPawn that faces a standing NPC, and the NPC still talks.
func TestInteractWithWalkingFolkStopsPlayer(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{"default/31357.htm": walkerPage})
	at := location.Location{X: w.at.X + 20, Y: w.at.Y + 10, Z: w.at.Z}
	a := location.Location{X: w.at.X + 20, Y: w.at.Y + 160, Z: w.at.Z}
	b := location.Location{X: w.at.X + 20, Y: w.at.Y - 160, Z: w.at.Z}
	f := w.spawnWalker(t, at, a, b)
	drainFrames(t, w.c)
	if !f.IsMoving() {
		t.Fatal("walker is not walking its route")
	}

	w.selectFolk(t, f)
	frames := w.talk(t, f, false)

	order := interactOrder(frames)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeStopMove, serverpackets.OpcodeSocialAction, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if string(order) != string(want) {
		t.Fatalf("interact frames = %#x, want %#x", order, want)
	}
	stop, _ := firstOpcode(frames, serverpackets.OpcodeStopMove)
	r := wire.NewReader(stop[1:])
	if id := r.ReadInt32(); id != w.player {
		t.Fatalf("StopMove for object %d, want the player %d", id, w.player)
	}
	html, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if objectID, page, _ := htmlMessage(t, html); objectID != f.ObjectID() || page != wantChatPage(walkerPage, f) {
		t.Fatalf("chat window = object %d %q, want %d %q", objectID, page, f.ObjectID(), wantChatPage(walkerPage, f))
	}
}
