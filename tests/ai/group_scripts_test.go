package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/follower"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPCs of the group script tests, typed as in the datapack: a golem
// crafter and his golem, and the two of Mammon.
const (
	telsonID             = int32(31592)
	dorothyID            = int32(31593)
	merchantOfMammonID   = int32(31113)
	blacksmithOfMammonID = int32(31126)
)

// listedDraws is a script random source that serves listed draws in order
// and records each bound asked, so a test pins both what a script draws
// and when.
type listedDraws struct {
	mu     sync.Mutex
	draws  []int
	bounds []string
}

// draw serves the next listed draw for a draw of [0, n); one past the list,
// or outside [0, n), is recorded as such and served as 0.
func (d *listedDraws) draw(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.draws) == 0 {
		d.bounds = append(d.bounds, fmt.Sprintf("%d:unlisted", n))
		return 0
	}
	v := d.draws[0]
	d.draws = d.draws[1:]
	if v >= n {
		d.bounds = append(d.bounds, fmt.Sprintf("%d:%d out of range", n, v))
		return 0
	}
	d.bounds = append(d.bounds, fmt.Sprintf("%d:%d", n, v))
	return v
}

// take returns the draws served since the last call, as "<bound>:<value>".
func (d *listedDraws) take() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.bounds
	d.bounds = nil
	return out
}

// left is how many listed draws were not served.
func (d *listedDraws) left() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.draws)
}

// bootGroupScript boots one character in the world with the script path of
// the catalog bound over kinds, the NPC templates the script spawns from,
// and draws as its random source.
func bootGroupScript(t *testing.T, path string, ctor func() script.Script, kinds map[int32]script.NPCKind,
	templates []*npc.Template, draws *listedDraws,
) (*gameservertest.Server, location.Location) {
	t.Helper()
	catalog := script.Catalog{path: ctor}
	return bootFolkDesires(t, catalog,
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: path}}, catalog),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithScriptRand(draws.draw),
	)
}

// npcInfoOf returns the object id and position of the one NpcInfo of the
// template npcID among frames.
func npcInfoOf(t *testing.T, frames [][]byte, npcID int32) (int32, location.Location) {
	t.Helper()
	var obj int32
	var at location.Location
	found := 0
	for _, f := range frames {
		if len(f) < 25 || f[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		id, tmpl := r.ReadInt32(), r.ReadInt32()
		r.ReadInt32() // attackable
		if tmpl != 1_000_000+npcID {
			continue
		}
		obj, at = id, location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		found++
	}
	if found != 1 {
		t.Fatalf("NpcInfo frames of NPC %d = %d, want 1", npcID, found)
	}
	return obj, at
}

// movesWithin runs up to three AI cycles and returns the MoveToLocation
// destinations of obj they sent, stopping at the first cycle that sent one.
func movesWithin(t *testing.T, srv *gameservertest.Server, obj int32) []location.Location {
	t.Helper()
	for range 3 {
		var out []location.Location
		for _, f := range framesOf(cycle(t, srv), serverpackets.OpcodeMoveToLocation, obj) {
			out = append(out, moveDestination(f))
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// TestFollowerComesWithItsMasterAndWalksBesideIt plays
// FollowerMovingAroundMaster for a golem crafter: as he appears his golem
// appears where he stands, and from five seconds on, every five seconds,
// the golem walks to a point drawn within 50 of him on each axis.
func TestFollowerComesWithItsMasterAndWalksBesideIt(t *testing.T) {
	t.Parallel()
	draws := &listedDraws{draws: []int{100, 0, 0, 100}}
	golem := gameservertest.FolkTemplate("MutedFolk", int(dorothyID))
	golem.CanMove = true
	srv, at := bootGroupScript(t, "script.ai.group.FollowerMovingAroundMaster", follower.New,
		map[int32]script.NPCKind{telsonID: script.KindFolk, dorothyID: script.KindFolk},
		[]*npc.Template{golem}, draws)

	masterAt := location.Location{X: at.X + 100, Y: at.Y, Z: at.Z}
	master := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(telsonID)), masterAt)
	srv.ReadQueued(t, srv.Client)

	srv.RunCreated(t, master)
	// A script spawn stands 20 above the ground below the point (no
	// geodata here: the point itself).
	golemObj, golemAt := npcInfoOf(t, srv.ReadQueued(t, srv.Client), dorothyID)
	if want := (location.Location{X: masterAt.X, Y: masterAt.Y, Z: masterAt.Z + 20}); golemAt != want {
		t.Fatalf("the golem appeared at %+v, want where its master stands, %+v", golemAt, want)
	}
	if got := draws.take(); len(got) != 0 {
		t.Fatalf("draws as the master appeared = %q, want none", got)
	}

	srv.Advance(t, 4999*time.Millisecond)
	if moves := movesWithin(t, srv, golemObj); len(moves) != 0 || len(draws.take()) != 0 {
		t.Fatalf("the golem moved before five seconds: %v", moves)
	}

	srv.Advance(t, time.Millisecond)
	if got, want := draws.take(), []string{"101:100", "101:0"}; !slices.Equal(got, want) {
		t.Fatalf("draws at five seconds = %q, want the x then the y offset %q", got, want)
	}
	want := location.Location{X: masterAt.X + 50, Y: masterAt.Y - 50, Z: masterAt.Z}
	if moves := movesWithin(t, srv, golemObj); !slices.Equal(moves, []location.Location{want}) {
		t.Fatalf("the golem's walk = %v, want one to %+v", moves, want)
	}
	// The golem runs the 71 to its point well inside a second.
	walked := walkFor(t, srv, time.Second)

	srv.Advance(t, 4999*time.Millisecond-walked)
	if got := draws.take(); len(got) != 0 {
		t.Fatalf("draws before ten seconds = %q, want none", got)
	}
	srv.Advance(t, time.Millisecond)
	if got, want := draws.take(), []string{"101:0", "101:100"}; !slices.Equal(got, want) {
		t.Fatalf("draws at ten seconds = %q, want %q", got, want)
	}
	want = location.Location{X: masterAt.X - 50, Y: masterAt.Y + 50, Z: masterAt.Z}
	if moves := movesWithin(t, srv, golemObj); !slices.Equal(moves, []location.Location{want}) {
		t.Fatalf("the golem's second walk = %v, want one to %+v", moves, want)
	}
	if n := draws.left(); n != 0 {
		t.Fatalf("%d listed draws never drawn", n)
	}
}

// walkFor runs the position ticks of about d and returns the time they
// took.
func walkFor(t *testing.T, srv *gameservertest.Server, d time.Duration) time.Duration {
	t.Helper()
	n := int(d / move.PositionUpdateInterval)
	for range n {
		srv.TickPositions()
	}
	srv.ReadQueued(t, srv.Client)
	return time.Duration(n) * move.PositionUpdateInterval
}

// teleportTo reads a TeleportToLocation frame's object and destination.
func teleportTo(f []byte) (int32, location.Location) {
	v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[1+4*i:]))) }
	return int32(v(0)), location.Location{X: v(1), Y: v(2), Z: v(3)}
}

// TestMammonMovesElsewhereEveryHalfHour plays RandomTeleport for the
// Blacksmith and the Merchant of Mammon: the Blacksmith shouts a drawn line
// as he appears; each moves half an hour after appearing, then every half
// hour, to a drawn place of its list, drawing again while the place is
// where it last moved to (the first place, before any move), and the
// Blacksmith shouts again where he arrives.
func TestMammonMovesElsewhereEveryHalfHour(t *testing.T) {
	t.Parallel()
	draws := &listedDraws{draws: []int{
		1,          // the Blacksmith's shout as he appears
		0, 0, 3, 2, // his first move: the first place twice, then the fourth; his shout
		7,       // the Merchant's first move: the last place
		3, 5, 0, // the Blacksmith's second move: where he is, then the sixth; his shout
		7, 2, // the Merchant's second move: where he is, then the third
	}}
	srv, at := bootGroupScript(t, "script.ai.group.RandomTeleport", randomteleport.New,
		map[int32]script.NPCKind{blacksmithOfMammonID: script.KindFolk, merchantOfMammonID: script.KindFolk},
		nil, draws)

	bom := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("SignsPriest", int(blacksmithOfMammonID)),
		location.Location{X: at.X + 100, Y: at.Y, Z: at.Z})
	srv.ReadQueued(t, srv.Client)
	srv.RunCreated(t, bom)
	says := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeNpcSay, bom.ObjectID())
	if len(says) != 1 {
		t.Fatalf("NpcSay frames as the Blacksmith appeared = %d, want 1", len(says))
	}
	r := wire.NewReader(says[0][5:])
	if typ, id, text := r.ReadInt32(), r.ReadInt32(), r.ReadString(); typ != serverpackets.SayTypeShout ||
		id != 1_000_000+blacksmithOfMammonID || text != "Rulers of the seal! I have some excellent weapons to show you!" {
		t.Fatalf("the Blacksmith's line = type %d npc %d %q, want his second line shouted", typ, id, text)
	}
	if got, want := draws.take(), []string{"3:1"}; !slices.Equal(got, want) {
		t.Fatalf("draws as the Blacksmith appeared = %q, want %q", got, want)
	}

	srv.Advance(t, time.Minute)
	mom := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("SignsPriest", int(merchantOfMammonID)),
		location.Location{X: at.X - 100, Y: at.Y, Z: at.Z})
	srv.RunCreated(t, mom)
	srv.ReadQueued(t, srv.Client)
	if got := draws.take(); len(got) != 0 {
		t.Fatalf("draws as the Merchant appeared = %q, want none", got)
	}

	// moves checks that the half hour from appearing has drawn wantDraws
	// and moved obj to dest. Next to the character, the character sees it
	// go, in one TeleportToLocation to dest, and hears nothing it says
	// where it arrives; far away, the character gets nothing at all.
	moves := func(step string, obj int32, wantDraws []string, dest location.Location, near bool, pos func() (int, int, int)) {
		t.Helper()
		if got := draws.take(); !slices.Equal(got, wantDraws) {
			t.Fatalf("%s: draws = %q, want %q", step, got, wantDraws)
		}
		var jumps []location.Location
		for _, f := range srv.ReadQueued(t, srv.Client) {
			switch f[0] {
			case serverpackets.OpcodeTeleportToLocation:
				id, to := teleportTo(f)
				if id != obj {
					t.Fatalf("%s: TeleportToLocation of object %d, want only the NPC's", step, id)
				}
				jumps = append(jumps, to)
			case serverpackets.OpcodeNpcSay:
				t.Fatalf("%s: the character heard the NPC where it arrived", step)
			}
		}
		switch {
		case near && (len(jumps) != 1 || jumps[0].X != dest.X || jumps[0].Y != dest.Y):
			t.Fatalf("%s: TeleportToLocation = %v, want one to %+v", step, jumps, dest)
		case !near && len(jumps) != 0:
			t.Fatalf("%s: TeleportToLocation = %v, want none from far away", step, jumps)
		}
		if x, y, _ := pos(); x != dest.X || y != dest.Y {
			t.Fatalf("%s: the NPC stands at %d,%d, want %d,%d", step, x, y, dest.X, dest.Y)
		}
	}

	srv.Advance(t, 29*time.Minute-time.Millisecond)
	if got := draws.take(); len(got) != 0 {
		t.Fatalf("draws before the half hour = %q, want none", got)
	}
	srv.Advance(t, time.Millisecond)
	moves("the Blacksmith's first move", bom.ObjectID(), []string{"6:0", "6:0", "6:3", "3:2"},
		location.Location{X: 46288, Y: 170096}, true, bom.Position)

	srv.Advance(t, time.Minute)
	moves("the Merchant's first move", mom.ObjectID(), []string{"8:7"},
		location.Location{X: 172384, Y: -17823}, true, mom.Position)

	srv.Advance(t, 29*time.Minute)
	moves("the Blacksmith's second move", bom.ObjectID(), []string{"6:3", "6:5", "3:0"},
		location.Location{X: 140480, Y: 79472}, false, bom.Position)

	srv.Advance(t, time.Minute)
	moves("the Merchant's second move", mom.ObjectID(), []string{"8:7", "8:2"},
		location.Location{X: 83136, Y: 208992}, false, mom.Position)
	if n := draws.left(); n != 0 {
		t.Fatalf("%d listed draws never drawn", n)
	}
}
