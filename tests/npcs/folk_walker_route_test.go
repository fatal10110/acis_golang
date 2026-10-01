package npcs

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// nodeDelay is the pause the delayed fixture route nodes hold.
const nodeDelay = 2 * time.Second

// groundGeo is passable geodata whose ground lies at z everywhere.
type groundGeo struct {
	gameservertest.Geo
	z int
}

func (g groundGeo) Height(int, int, int) int16 { return int16(g.z) }

// blockedNode is route reachability that finds no way to node for the next
// left checks, and every other node reachable.
type blockedNode struct {
	node location.Location
	left atomic.Int32
}

func (p *blockedNode) CanMove(_, target location.Location) bool {
	return target != p.node || p.left.Load() <= 0
}

func (p *blockedNode) HasPath(_, target location.Location) bool {
	if target != p.node || p.left.Load() <= 0 {
		return true
	}
	p.left.Add(-1)
	return false
}

// walkerRoute is the fixture walker alias's route over nodes.
func walkerRoute(nodes ...route.WalkerLocation) route.WalkerRoutes {
	return route.WalkerRoutes{walkerAlias: {walkerAlias: nodes}}
}

// npcFrame is one frame shown about the NPC.
type npcFrame struct {
	opcode byte
	raw    []byte
}

// body reads the frame past its object id.
func (fr npcFrame) body() *wire.Reader {
	r := wire.NewReader(fr.raw[1:])
	r.ReadInt32()
	return r
}

// readNpcFramesUntil reads frames, keeping those about f, until done holds
// for one of them; on the driven clock each wait moves the clock forward.
func readNpcFramesUntil(t *testing.T, c *testsupport.ScriptedClient, f *npc.Folk, what string, done func(npcFrame) bool) []npcFrame {
	t.Helper()
	var kept []npcFrame
	for range 400 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		if len(frame) < 5 {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != f.ObjectID() {
			continue
		}
		fr := npcFrame{opcode: frame[0], raw: frame}
		kept = append(kept, fr)
		if done(fr) {
			return kept
		}
	}
	t.Fatalf("%s not shown; frames about the NPC = %#x", what, npcOpcodes(kept))
	return nil
}

func npcOpcodes(frames []npcFrame) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.opcode)
	}
	return out
}

func readLocation(r *wire.Reader) location.Location {
	return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
}

// moveLeg decodes a MoveToLocation frame's destination and origin.
func moveLeg(fr npcFrame) (dest, origin location.Location) {
	r := fr.body()
	return readLocation(r), readLocation(r)
}

// isMoveTo reports whether fr shows a walk to dest.
func isMoveTo(fr npcFrame, dest location.Location) bool {
	if fr.opcode != serverpackets.OpcodeMoveToLocation {
		return false
	}
	got, _ := moveLeg(fr)
	return got == dest
}

// TestRouteWalkerFolkDelayedNodeSpeaksAndWaits pins a route node holding a
// chat line, a social action and a delay: on reaching it the walker says
// the line, then plays the action, and stays put until the walker task
// ticks after the delay has run out, when it walks on to the next node.
func TestRouteWalkerFolkDelayedNodeSpeaksAndWaits(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 300, Y: w.at.Y, Z: w.at.Z}
	routes := walkerRoute(
		route.WalkerLocation{Location: a, DelayMillis: int(nodeDelay / time.Millisecond), NPCStringID: 1010205, SocialID: 3},
		route.WalkerLocation{Location: b},
	)
	at := location.Location{X: w.at.X + 150, Y: w.at.Y, Z: w.at.Z}
	f, walker := w.srv.SpawnRouteFolkNPCAt(t, walkerTemplate(), at, routes, true)

	frames := readNpcFramesUntil(t, w.c, f, "social action at the delayed node", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeSocialAction
	})
	want := []byte{serverpackets.OpcodeNPCInfo, serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeNpcSay, serverpackets.OpcodeSocialAction}
	if got := npcOpcodes(frames); string(got) != string(want) {
		t.Fatalf("frames about the walker = %#x, want %#x", got, want)
	}
	say := frames[2].body()
	if sayType, npcID, text := say.ReadInt32(), say.ReadInt32(), say.ReadString(); sayType != serverpackets.SayTypeAll || npcID != 1_000_000+walkerID || text != "Where has he gone?" {
		t.Fatalf("NpcSay = type %d npc %d %q, want type %d npc %d %q", sayType, npcID, text, serverpackets.SayTypeAll, 1_000_000+walkerID, "Where has he gone?")
	}
	if id := frames[3].body().ReadInt32(); id != 3 {
		t.Fatalf("SocialAction = %d, want the node's 3", id)
	}
	if folkAt(f) != a || f.IsMoving() {
		t.Fatalf("walker at %v moving=%v, want standing on %v", folkAt(f), f.IsMoving(), a)
	}

	// A tick inside the delay leaves the walker standing.
	walker.Tick()
	if dests, _ := folkMoves(drainFrames(t, w.c), f); len(dests) != 0 || f.IsMoving() {
		t.Fatalf("walker walked to %v inside its node delay, want it standing", dests)
	}

	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	frames = readNpcFramesUntil(t, w.c, f, "walk to the next node", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeMoveToLocation
	})
	if dest, origin := moveLeg(frames[len(frames)-1]); dest != b || origin != a {
		t.Fatalf("leg after the delay = %v from %v, want %v from %v", dest, origin, b, a)
	}
}

// TestRouteWalkerFolkFacesSpawnHeadingAtHome pins the walker's heading: it
// turns toward each walk it starts, keeps the heading of its last walk
// standing on a route node away from its spawn point, and faces its spawn
// heading again once it arrives back on its spawn point.
func TestRouteWalkerFolkFacesSpawnHeadingAtHome(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	const spawnHeading = 40000
	home := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	a := location.Location{X: home.X, Y: home.Y + 60, Z: home.Z}
	delay := int(nodeDelay / time.Millisecond)
	f, walker := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: walkerTemplate(),
		At:       home,
		Heading:  spawnHeading,
		Routes:   walkerRoute(route.WalkerLocation{Location: home, DelayMillis: delay}, route.WalkerLocation{Location: a, DelayMillis: delay}),
		WalkMode: true,
	})
	standingOn := func(at location.Location, heading int) func() bool {
		return func() bool { return !f.IsMoving() && folkAt(f) == at && f.Heading() == heading }
	}

	// The first walk is to the spawn point it stands on, which turns it to
	// heading 0 until it arrives.
	w.srv.AdvanceUntil(t, "walker faces its spawn heading on its spawn point", standingOn(home, spawnHeading))

	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	north := home.HeadingTo(a)
	w.srv.AdvanceUntil(t, "walker stands on the far node facing the way it walked", standingOn(a, north))

	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	w.srv.AdvanceUntil(t, "walker walks off the far node", func() bool { return f.IsMoving() })
	if got, want := f.Heading(), a.HeadingTo(home); got != want {
		t.Fatalf("walking home heading = %d, want %d toward its spawn point", got, want)
	}
	w.srv.AdvanceUntil(t, "walker faces its spawn heading back on its spawn point", standingOn(home, spawnHeading))
}

// TestRouteWalkerFolkStuckTeleportsToFirstNode pins a walker whose way to
// its first route node stays blocked: each blocked attempt turns it back,
// and on the arrival after the tenth it jumps to the first node, grounded,
// with no stop shown since it is standing, then walks the route from there.
// The jump clears the streak, so the next arrival walks on rather than
// jumping again.
func TestRouteWalkerFolkStuckTeleportsToFirstNode(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	ground := w.at.Z
	node := func(dy, dz int) location.Location {
		return location.Location{X: w.at.X, Y: w.at.Y + dy, Z: ground + dz}
	}
	// The first node lies above the ground: the jump lands on the ground.
	a, b, c, d := node(200, 300), node(120, 0), node(100, 0), node(60, 0)
	path := &blockedNode{node: a}
	path.left.Store(10)
	f, _ := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: walkerTemplate(),
		At:       node(50, 0),
		Routes: walkerRoute(
			route.WalkerLocation{Location: a}, route.WalkerLocation{Location: b},
			route.WalkerLocation{Location: c}, route.WalkerLocation{Location: d},
		),
		Geo:  groundGeo{z: ground},
		Path: path,
	})

	frames := readNpcFramesUntil(t, w.c, f, "teleport to the first node", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeTeleportToLocation
	})
	for _, fr := range frames {
		if fr.opcode == serverpackets.OpcodeStopMove {
			t.Fatalf("frames about the walker = %#x, want no StopMove for a walker that stands", npcOpcodes(frames))
		}
	}
	if left := path.left.Load(); left != 0 {
		t.Fatalf("walker teleported with %d blocked attempts left, want after all 10", left)
	}
	landed := location.Location{X: a.X, Y: a.Y, Z: ground}
	if to := readLocation(frames[len(frames)-1].body()); to != landed {
		t.Fatalf("teleport to %v, want the first node on the ground %v", to, landed)
	}

	frames = readNpcFramesUntil(t, w.c, f, "walk on past the second node", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeTeleportToLocation || isMoveTo(fr, c)
	})
	var legs [][2]location.Location
	for _, fr := range frames {
		if fr.opcode == serverpackets.OpcodeTeleportToLocation {
			t.Fatalf("walker teleported again after the jump (frames %#x), want its streak cleared", npcOpcodes(frames))
		}
		if fr.opcode == serverpackets.OpcodeMoveToLocation {
			dest, origin := moveLeg(fr)
			legs = append(legs, [2]location.Location{dest, origin})
		}
	}
	want := [][2]location.Location{{b, landed}, {c, b}}
	if len(legs) != len(want) || legs[0] != want[0] || legs[1] != want[1] {
		t.Fatalf("legs after the jump (to, from) = %v, want %v", legs, want)
	}
}
