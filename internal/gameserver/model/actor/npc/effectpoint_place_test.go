package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// discoverHook is a world observer that runs onDiscover when an object comes
// into its sight.
type discoverHook struct {
	world.Presence
	onDiscover func(world.Tracked)
}

func (h *discoverHook) ObjectID() int32            { return 99 }
func (h *discoverHook) Kind() actor.Kind           { return actor.KindNPC }
func (h *discoverHook) Discover(obj world.Tracked) { h.onDiscover(obj) }
func (h *discoverHook) Forget(world.Tracked)       {}

// TestEffectPointRemovedWhileSpawningLeavesNoZoneOccupant drives the removal
// of a signet point into the gap between its world spawn (on the caster's
// queue) and its zone entry: the removal, run on another goroutine as the
// point's own queue would, waits for the spawn to finish, so it takes the
// point out of the zone it just entered and leaves no ghost occupant.
func TestEffectPointRemovedWhileSpawningLeavesNoZoneOccupant(t *testing.T) {
	ix, water := waterIndex(t)
	w := world.New()
	q := sim.NewInline(time.Unix(0, 0)).NewQueue("point")
	ep, err := NewEffectPoint(7, &Template{ID: 13018, Type: "EffectPoint", RunSpeed: 20, WalkSpeed: 20}, 1, q, nil)
	if err != nil {
		t.Fatal(err)
	}
	ep.Attach(Runtime{World: w})
	ep.SetZones(ix)

	removed := make(chan struct{})
	hook := &discoverHook{onDiscover: func(obj world.Tracked) {
		if obj != world.Tracked(ep) {
			return
		}
		// The point is in the world but not yet in its zones.
		go func() {
			ep.Despawn()
			close(removed)
		}()
		select {
		case <-removed:
		case <-time.After(50 * time.Millisecond):
		}
	}}
	w.Spawn(hook, 20, 20, 0, 0)

	ep.Spawn(10, 10, 0, 0)
	select {
	case <-removed:
	case <-time.After(5 * time.Second):
		t.Fatal("removal never finished")
	}

	if got := len(water.Core().Occupants()); got != 0 {
		t.Fatalf("water occupants after the point was removed = %d, want 0", got)
	}
	if _, ok := w.Object(ep.ObjectID()); ok {
		t.Fatal("removed point is still in the world")
	}
}
