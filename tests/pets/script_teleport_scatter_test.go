package pets

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonScatter is the random offset the sender scatters the summon with.
const summonScatter = 200

// TestScriptTeleportNearScattersASummon (Creature.teleportTo with a random
// offset on a summon): the script random source draws the x offset, then
// the y one, each in [-200, 200]; the teleport walks from the point toward
// the drawn one on the geodata and lands where the walk stops. The owner
// sees one ordinary TeleportToLocation for the summon there, the summon
// stands there and follows its owner again.
func TestScriptTeleportNearScattersASummon(t *testing.T) {
	t.Parallel()
	geo := &jumpGeo{}
	draws := &scriptDraws{}
	var mu sync.Mutex
	var dest location.Location
	catalog := script.Catalog{
		"ai.ScatterSender": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{senderID}, Hooks: script.Hooks{
				OnAttacked: func(s *script.Script, e script.Attacked) {
					p, ok := e.Attacker.(*script.Player)
					if !ok {
						return
					}
					if sm := p.ActiveSummon(); sm != nil {
						mu.Lock()
						to := dest
						mu.Unlock()
						s.TeleportNear(sm, script.Loc{X: to.X, Y: to.Y, Z: to.Z}, summonScatter)
					}
				},
			}}
		},
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithGeo(geo),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{senderID: script.KindHostile},
			[]script.Listing{{Path: "ai.ScatterSender"}}, catalog),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(senderID), int(senderID)
	sender := h.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x - 400, Y: y, Z: z})
	drainUntilQuiet(t, h.client)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	to := location.Location{X: x + 600, Y: y + 50, Z: z}
	toward := location.Location{X: to.X + 100, Y: to.Y - 150, Z: to.Z}
	mu.Lock()
	dest = to
	mu.Unlock()
	draws.mu.Lock()
	draws.next, draws.bounds = []int{300, 50}, nil
	draws.mu.Unlock()
	geo.mu.Lock()
	geo.stop = toward
	geo.mu.Unlock()

	owner, ok := h.srv.State.Object(h.ownerID)
	if !ok {
		t.Fatal("the owner is not in the world")
	}
	done := make(chan struct{})
	if !h.srv.PlayerQueue(t, h.ownerID).Post(func() {
		defer close(done)
		sender.TakeDamage(10, owner.(attackable.Combatant))
	}) {
		t.Fatal("the owner's queue is closed")
	}
	<-done
	h.srv.Settle(t)
	frames := drainFrames(t, h.client)

	draws.mu.Lock()
	bounds := draws.bounds
	draws.mu.Unlock()
	if want := []int{2*summonScatter + 1, 2*summonScatter + 1}; !slices.Equal(bounds, want) {
		t.Fatalf("script draws = %v, want %v", bounds, want)
	}
	geo.mu.Lock()
	walks := geo.walks
	geo.mu.Unlock()
	if want := [][6]int{{to.X, to.Y, to.Z, toward.X, toward.Y, to.Z}}; !slices.Equal(walks, want) {
		t.Fatalf("geodata walks = %v, want %v", walks, want)
	}
	landing := location.Location{X: to.X + 50, Y: to.Y - 75, Z: to.Z}
	got, fast := teleports(frames, wolf.ObjectID())
	if !slices.Equal(got, []location.Location{landing}) || !slices.Equal(fast, []bool{false}) {
		t.Fatalf("summon teleports = %v fast %v, want one ordinary to %v", got, fast, landing)
	}
	if got := positionOf(wolf); got != landing {
		t.Fatalf("summon stands at %v, want %v", got, landing)
	}
	if !wolf.FollowActive() || wolf.Intent() != summon.IntentFollowOwner {
		t.Fatalf("summon follow = %v intent = %v, want following its owner", wolf.FollowActive(), wolf.Intent())
	}
}
