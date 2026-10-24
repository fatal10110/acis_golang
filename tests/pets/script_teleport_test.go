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

// senderID is the monster whose attacked hook sends its attacker's summon
// to a point, as WarriorHereticPriestAggressive sends the summon of a
// player it teleports after it.
const senderID = int32(20086)

// groundDrop is how far below the probed height the ground lies at the
// summon's destination.
const groundDrop = 40

// sinkGeo is open terrain whose ground lies groundDrop below the probe at
// one point.
type sinkGeo struct {
	gameservertest.Geo
	mu   sync.Mutex
	sink location.Location
}

func (g *sinkGeo) Height(x, y, z int) int16 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if x == g.sink.X && y == g.sink.Y {
		return int16(z - groundDrop)
	}
	return g.Geo.Height(x, y, z)
}

// sendWorld is the owner in the world beside the sender, with a watcher
// beside the point the sender sends the summon to.
type sendWorld struct {
	*petWorld
	seen    *sightings
	sender  *npc.Hostile
	watcher *npc.Hostile
	dest    location.Location
}

func bootSender(t *testing.T) *sendWorld {
	t.Helper()
	geo := &sinkGeo{}
	seen := &sightings{calls: map[[2]int32]int{}}
	var mu sync.Mutex
	var dest location.Location
	catalog := script.Catalog{
		"ai.Sender": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{senderID}, Hooks: script.Hooks{
				OnAttacked: func(_ *script.Script, e script.Attacked) {
					p, ok := e.Attacker.(*script.Player)
					if !ok {
						return
					}
					if s := p.ActiveSummon(); s != nil {
						mu.Lock()
						to := dest
						mu.Unlock()
						script.TeleportTo(s, script.Loc{X: to.X, Y: to.Y, Z: to.Z})
					}
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
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithGeo(geo),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{
			senderID: script.KindHostile, watcherID: script.KindHostile,
		}, []script.Listing{{Path: "ai.Sender"}, {Path: "ai.Watcher"}}, catalog),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	spawn := func(id int32, at location.Location) *npc.Hostile {
		tmpl := gameservertest.MovingHostileTemplate("Monster")
		tmpl.ID, tmpl.TemplateID = int(id), int(id)
		return h.srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
	}
	w := &sendWorld{
		petWorld: h, seen: seen,
		sender:  spawn(senderID, location.Location{X: x - 400, Y: y, Z: z}),
		watcher: spawn(watcherID, location.Location{X: x + 1300, Y: y + 100, Z: z}),
		dest:    location.Location{X: x + 1200, Y: y + 50, Z: z},
	}
	geo.mu.Lock()
	geo.sink = w.dest
	geo.mu.Unlock()
	mu.Lock()
	dest = w.dest
	mu.Unlock()
	drainUntilQuiet(t, h.client)
	return w
}

// teleports returns the destinations and fast flags of the
// TeleportToLocation frames for id among frames.
func teleports(frames [][]byte, id int32) (out []location.Location, fast []bool) {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[5+4*i:]))) }
		out = append(out, location.Location{X: v(0), Y: v(1), Z: v(2)})
		fast = append(fast, v(3) == 1)
	}
	return out, fast
}

// TestScriptTeleportOfTheActiveSummon (Creature.teleportTo on a summon,
// Summon.onTeleported, SummonAI.onEvtTeleported, forceSeeCreature): a
// script sends the summon of the player that hit its NPC to a point
// through the handle ActiveSummon returns. The summon drops its target,
// the owner sees one TeleportToLocation for it, the ordinary one, with the
// point's Z snapped to the ground, the summon stands there and follows its
// owner again, and the NPC beside the landing that sees it runs its
// see-creature hook for it once. The owner stays where it was.
func TestScriptTeleportOfTheActiveSummon(t *testing.T) {
	t.Parallel()
	w := bootSender(t)
	wolf, _ := w.spawnWolf(t)
	drainUntilQuiet(t, w.client)
	w.seen.take(w.watcher.ObjectID(), wolf.ObjectID())
	wolf.SetTarget(w.sender)
	ox, oy, oz := w.srv.PlayerPosition(t, w.ownerID)

	owner, ok := w.srv.State.Object(w.ownerID)
	if !ok {
		t.Fatal("the owner is not in the world")
	}
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.ownerID).Post(func() {
		defer close(done)
		w.sender.TakeDamage(10, owner.(attackable.Combatant))
	}) {
		t.Fatal("the owner's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	frames := drainFrames(t, w.client)

	landing := location.Location{X: w.dest.X, Y: w.dest.Y, Z: w.dest.Z - groundDrop}
	got, fast := teleports(frames, wolf.ObjectID())
	if !slices.Equal(got, []location.Location{landing}) || !slices.Equal(fast, []bool{false}) {
		t.Fatalf("summon teleports = %v fast %v, want one ordinary to %v", got, fast, landing)
	}
	if got := positionOf(wolf); got != landing {
		t.Fatalf("summon stands at %v, want %v", got, landing)
	}
	if target := wolf.CurrentTarget(); target != nil {
		t.Fatalf("summon target = %v, want none after the teleport", target)
	}
	if !wolf.FollowActive() || wolf.Intent() != summon.IntentFollowOwner {
		t.Fatalf("summon follow = %v intent = %v, want following its owner", wolf.FollowActive(), wolf.Intent())
	}
	if wolf.Teleporting() {
		t.Fatal("summon still marked teleporting after it landed")
	}
	if got := w.seen.take(w.watcher.ObjectID(), wolf.ObjectID()); got != 1 {
		t.Fatalf("see-creature calls of the NPC beside the landing = %d, want 1", got)
	}
	if got, _ := teleports(frames, w.ownerID); len(got) != 0 {
		t.Fatalf("owner teleports = %v, want none", got)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.ownerID); x != ox || y != oy || z != oz {
		t.Fatalf("owner at %d,%d,%d, want it kept at %d,%d,%d", x, y, z, ox, oy, oz)
	}
}
