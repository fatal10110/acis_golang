package pets

import (
	"encoding/binary"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The monsters whose attacked hooks jump their attacker onto them, as
// PartyLeaderCoupleCaptain pulls the creature in its slot to itself
// (offset 0) and Q620 drops each party member near a point (offset 80).
const (
	pullerID      = int32(20082)
	scattererID   = int32(20083)
	scatterOffset = 80
)

// jumpGeo is open terrain that records the walks a scattered jump checks
// and stops the walk toward stop halfway.
type jumpGeo struct {
	gameservertest.Geo
	mu    sync.Mutex
	stop  location.Location
	walks [][6]int
}

func (g *jumpGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	g.mu.Lock()
	defer g.mu.Unlock()
	if tx != g.stop.X || ty != g.stop.Y {
		return location.Location{X: tx, Y: ty, Z: tz}
	}
	g.walks = append(g.walks, [6]int{ox, oy, oz, tx, ty, tz})
	return location.Location{X: ox + (tx-ox)/2, Y: oy + (ty-oy)/2, Z: tz}
}

// scriptDraws answers the script random source from a list and records
// each bound it was asked for.
type scriptDraws struct {
	mu     sync.Mutex
	next   []int
	bounds []int
}

func (d *scriptDraws) rand(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bounds = append(d.bounds, n)
	if len(d.next) == 0 {
		return 0
	}
	v := d.next[0]
	d.next = d.next[1:]
	return v
}

// jumpWorld is the owner in the world beside a puller and a scatterer.
type jumpWorld struct {
	*petWorld
	geo       *jumpGeo
	draws     *scriptDraws
	puller    *npc.Hostile
	scatterer *npc.Hostile
}

func bootJumpers(t *testing.T) *jumpWorld {
	t.Helper()
	geo := &jumpGeo{}
	draws := &scriptDraws{}
	catalog := script.Catalog{
		"ai.Puller": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{pullerID}, Hooks: script.Hooks{
				OnAttacked: func(_ *script.Script, e script.Attacked) { e.Attacker.InstantTeleportTo(e.NPC) },
			}}
		},
		"ai.Scatterer": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{scattererID}, Hooks: script.Hooks{
				OnAttacked: func(s *script.Script, e script.Attacked) {
					s.InstantTeleportNear(e.Attacker, e.NPC, scatterOffset)
				},
			}}
		},
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithGeo(geo),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{pullerID: script.KindHostile, scattererID: script.KindHostile},
			[]script.Listing{{Path: "ai.Puller"}, {Path: "ai.Scatterer"}}, catalog),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	spawn := func(id int32, at location.Location) *npc.Hostile {
		tmpl := gameservertest.MovingHostileTemplate("Monster")
		tmpl.ID, tmpl.TemplateID = int(id), int(id)
		return h.srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
	}
	w := &jumpWorld{
		petWorld: h, geo: geo, draws: draws,
		puller:    spawn(pullerID, location.Location{X: x + 400, Y: y, Z: z}),
		scatterer: spawn(scattererID, location.Location{X: x - 400, Y: y + 100, Z: z}),
	}
	drainUntilQuiet(t, h.client)
	return w
}

// hitFrom lands a hit on target from attacker, on the owner's queue, where
// both the owner's and its summon's work runs, and returns the frames the
// owner was sent.
func (w *jumpWorld) hitFrom(t *testing.T, target *npc.Hostile, attacker attackable.Combatant) [][]byte {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.ownerID).Post(func() {
		defer close(done)
		target.TakeDamage(10, attacker)
	}) {
		t.Fatal("the owner's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	return drainFrames(t, w.client)
}

// fastTeleports returns the destinations of the fast TeleportToLocation
// frames for id among frames, failing on a slow one.
func fastTeleports(t *testing.T, frames [][]byte, id int32) []location.Location {
	t.Helper()
	var out []location.Location
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[5+4*i:]))) }
		if v(3) != 1 {
			t.Fatalf("TeleportToLocation for %d is not the fast one", id)
		}
		out = append(out, location.Location{X: v(0), Y: v(1), Z: v(2)})
	}
	return out
}

func positionOf(o interface{ Position() (int, int, int) }) location.Location {
	x, y, z := o.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// TestScriptInstantTeleportOfASummon (Creature.instantTeleportTo,
// SummonAI.onEvtTeleported): a script pulls the summon in its attacker
// slot onto the NPC through the creature handle. The owner sees one fast
// TeleportToLocation for the summon, the summon stands on the NPC's point,
// and it follows its owner again.
func TestScriptInstantTeleportOfASummon(t *testing.T) {
	t.Parallel()
	w := bootJumpers(t)
	wolf, _ := w.spawnWolf(t)
	drainUntilQuiet(t, w.client)
	pullTo := positionOf(w.puller)

	frames := w.hitFrom(t, w.puller, wolf)
	if got := fastTeleports(t, frames, wolf.ObjectID()); !slices.Equal(got, []location.Location{pullTo}) {
		t.Fatalf("summon jumps = %v, want one to %v", got, pullTo)
	}
	if got := positionOf(wolf); got != pullTo {
		t.Fatalf("summon stands at %v, want %v", got, pullTo)
	}
	if !wolf.FollowActive() || wolf.Intent() != summon.IntentFollowOwner {
		t.Fatalf("summon follow = %v intent = %v, want following its owner", wolf.FollowActive(), wolf.Intent())
	}
}

// TestScriptInstantTeleportWithRandomOffset (Creature.instantTeleportTo
// with a random offset, Q620_FourGoblets): the script random source draws
// the x offset, then the y one, each Rnd.get(-80, 80); the jump walks from
// the point toward the drawn one on the geodata and lands where the walk
// stops. The player sees one fast TeleportToLocation there and stands
// there.
func TestScriptInstantTeleportWithRandomOffset(t *testing.T) {
	t.Parallel()
	w := bootJumpers(t)
	at := positionOf(w.scatterer)
	w.draws.mu.Lock()
	w.draws.next = []int{100, 30}
	w.draws.bounds = nil
	w.draws.mu.Unlock()
	drawn := location.Location{X: at.X + 20, Y: at.Y - 50, Z: at.Z}
	w.geo.mu.Lock()
	w.geo.stop = drawn
	w.geo.mu.Unlock()

	owner, ok := w.srv.State.Object(w.ownerID)
	if !ok {
		t.Fatal("the owner is not in the world")
	}
	frames := w.hitFrom(t, w.scatterer, owner.(attackable.Combatant))

	w.draws.mu.Lock()
	bounds := w.draws.bounds
	w.draws.mu.Unlock()
	if want := []int{2*scatterOffset + 1, 2*scatterOffset + 1}; !slices.Equal(bounds, want) {
		t.Fatalf("script draws = %v, want %v", bounds, want)
	}
	w.geo.mu.Lock()
	walks := w.geo.walks
	w.geo.mu.Unlock()
	if want := [][6]int{{at.X, at.Y, at.Z, drawn.X, drawn.Y, at.Z}}; !slices.Equal(walks, want) {
		t.Fatalf("geodata walks = %v, want %v", walks, want)
	}
	landing := location.Location{X: at.X + 10, Y: at.Y - 25, Z: at.Z}
	if got := fastTeleports(t, frames, w.ownerID); !slices.Equal(got, []location.Location{landing}) {
		t.Fatalf("player jumps = %v, want one to %v", got, landing)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.ownerID); (location.Location{X: x, Y: y, Z: z}) != landing {
		t.Fatalf("player stands at %d,%d,%d, want %v", x, y, z, landing)
	}
}
