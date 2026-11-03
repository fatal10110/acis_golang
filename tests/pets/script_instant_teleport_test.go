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
// (offset 0) and Q620 drops each party member near a point (offset 80);
// the monsters whose see-creature hooks count what they are told of; and a
// civilian NPC that fights.
const (
	pullerID      = int32(20082)
	scattererID   = int32(20083)
	watcherID     = int32(20084)
	fighterID     = int32(30085)
	scatterOffset = 80
)

// sightings counts, per watching NPC, the see-creature calls for each
// creature; hooks run on other queues, so it is guarded by mu.
type sightings struct {
	mu    sync.Mutex
	calls map[[2]int32]int
}

func (s *sightings) add(npcID, creatureID int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[[2]int32{npcID, creatureID}]++
}

// take returns the calls npcID had for creatureID since the last take.
func (s *sightings) take(npcID, creatureID int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]int32{npcID, creatureID}
	n := s.calls[k]
	delete(s.calls, k)
	return n
}

// pulls records the attackers the puller's hook pulled, once the jump call
// returned to it.
type pulls struct {
	mu  sync.Mutex
	ids []int32
}

func (p *pulls) add(id int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ids = append(p.ids, id)
}

func (p *pulls) take() []int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := p.ids
	p.ids = nil
	return ids
}

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

// jumpWorld is the owner in the world beside a puller, a scatterer and a
// watcher next to the puller, with two more pullers and a watcher between
// them three regions away.
type jumpWorld struct {
	*petWorld
	geo        *jumpGeo
	draws      *scriptDraws
	seen       *sightings
	pulled     *pulls
	puller     *npc.Hostile
	scatterer  *npc.Hostile
	watcher    *npc.Hostile
	farPullers [2]*npc.Hostile
	farWatcher *npc.Hostile
}

// farAway is far enough from the owner to lie outside the 3x3 block of
// regions around it.
const farAway = 6000

func bootJumpers(t *testing.T) *jumpWorld {
	t.Helper()
	geo := &jumpGeo{}
	draws := &scriptDraws{}
	seen := &sightings{calls: map[[2]int32]int{}}
	pulled := &pulls{}
	catalog := script.Catalog{
		"ai.Puller": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{pullerID}, Hooks: script.Hooks{
				OnAttacked: func(_ *script.Script, e script.Attacked) {
					e.Attacker.InstantTeleportTo(e.NPC)
					pulled.add(e.Attacker.ObjectID())
				},
			}}
		},
		"ai.Watcher": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{watcherID}, Hooks: script.Hooks{
				OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
					seen.add(e.NPC.ObjectID(), e.Creature.ObjectID())
				},
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
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{
			pullerID: script.KindHostile, scattererID: script.KindHostile, watcherID: script.KindHostile,
		}, []script.Listing{{Path: "ai.Puller"}, {Path: "ai.Scatterer"}, {Path: "ai.Watcher"}}, catalog),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	spawn := func(id int32, at location.Location) *npc.Hostile {
		tmpl := gameservertest.MovingHostileTemplate("Monster")
		tmpl.ID, tmpl.TemplateID = int(id), int(id)
		return h.srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
	}
	w := &jumpWorld{
		petWorld: h, geo: geo, draws: draws, seen: seen, pulled: pulled,
		puller:    spawn(pullerID, location.Location{X: x + 400, Y: y, Z: z}),
		scatterer: spawn(scattererID, location.Location{X: x - 400, Y: y + 100, Z: z}),
		watcher:   spawn(watcherID, location.Location{X: x + 550, Y: y, Z: z}),
		farPullers: [2]*npc.Hostile{
			spawn(pullerID, location.Location{X: x + farAway, Y: y, Z: z}),
			spawn(pullerID, location.Location{X: x + farAway + 300, Y: y, Z: z}),
		},
		farWatcher: spawn(watcherID, location.Location{X: x + farAway + 150, Y: y, Z: z}),
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

// TestScriptInstantTeleportOfASummonTellsTheNPCsAroundItsRegion
// (Creature.instantTeleportTo, forceSeeCreature over the knownlist of the
// region kept): a summon pulled next to a watching NPC is seen by it once.
// Pulled three regions away, it stays in its owner's region, so the NPC
// beside its landing is not told; pulled again before it next moves, the
// NPCs told are still those around that region, not around the point it
// jumped from.
func TestScriptInstantTeleportOfASummonTellsTheNPCsAroundItsRegion(t *testing.T) {
	t.Parallel()
	w := bootJumpers(t)
	wolf, _ := w.spawnWolf(t)
	drainUntilQuiet(t, w.client)
	w.seen.take(w.watcher.ObjectID(), wolf.ObjectID())

	w.hitFrom(t, w.puller, wolf)
	if got := w.pulled.take(); !slices.Equal(got, []int32{wolf.ObjectID()}) {
		t.Fatalf("pulled = %v, want the summon", got)
	}
	if got := w.seen.take(w.watcher.ObjectID(), wolf.ObjectID()); got != 1 {
		t.Fatalf("see-creature calls of the NPC beside the landing = %d, want 1", got)
	}

	for i, puller := range w.farPullers {
		pullTo := positionOf(puller)
		frames := w.hitFrom(t, puller, wolf)
		if got := fastTeleports(t, frames, wolf.ObjectID()); !slices.Equal(got, []location.Location{pullTo}) {
			t.Fatalf("far pull %d: summon jumps = %v, want one to %v", i, got, pullTo)
		}
		if got := positionOf(wolf); got != pullTo {
			t.Fatalf("far pull %d: summon stands at %v, want %v", i, got, pullTo)
		}
		if got := w.seen.take(w.farWatcher.ObjectID(), wolf.ObjectID()); got != 0 {
			t.Fatalf("far pull %d: see-creature calls of the NPC beside the landing = %d, want none outside the kept region's block", i, got)
		}
		if got := w.seen.take(w.watcher.ObjectID(), wolf.ObjectID()); got != 0 {
			t.Fatalf("far pull %d: see-creature calls of the NPC out of sight = %d, want none", i, got)
		}
	}
}

// TestScriptInstantTeleportOfACivilianNPC (Npc.instantTeleportTo): a
// civilian NPC that fights can sit in a script's attacker slot, and the
// script pulls it onto its NPC like any other creature. The player sees
// one fast TeleportToLocation for it, it stands on the point, and the hook
// carries on past the jump.
func TestScriptInstantTeleportOfACivilianNPC(t *testing.T) {
	t.Parallel()
	w := bootJumpers(t)
	x, y, z := w.srv.PlayerPosition(t, w.ownerID)
	tmpl := gameservertest.FolkTemplate("Folk", int(fighterID))
	tmpl.CanMove = true
	folk := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: x + 200, Y: y - 300, Z: z})
	drainUntilQuiet(t, w.client)
	pullTo := positionOf(w.puller)

	done := make(chan struct{})
	if !folk.Queue().Post(func() {
		defer close(done)
		w.puller.TakeDamage(10, folk)
	}) {
		t.Fatal("the civilian NPC's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	frames := drainFrames(t, w.client)

	if got := w.pulled.take(); !slices.Equal(got, []int32{folk.ObjectID()}) {
		t.Fatalf("pulled = %v, want the civilian NPC, with the hook run past the jump", got)
	}
	if got := fastTeleports(t, frames, folk.ObjectID()); !slices.Equal(got, []location.Location{pullTo}) {
		t.Fatalf("civilian NPC jumps = %v, want one to %v", got, pullTo)
	}
	if got := positionOf(folk); got != pullTo {
		t.Fatalf("civilian NPC stands at %v, want %v", got, pullTo)
	}
}
