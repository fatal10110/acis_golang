package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// waterIndex is a zone index holding one water zone around the origin.
func waterIndex(t *testing.T) (*zone.Index, *zone.Water) {
	t.Helper()
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	water := zone.NewWater(1, form)
	ix := zone.NewIndex()
	ix.Add(water)
	return ix, water
}

func assertViewEvents(t *testing.T, when string, sink *recordingSink, want ...event.NPCInfoChanged) {
	t.Helper()
	if len(sink.events) != len(want) {
		t.Fatalf("%s: events = %v, want %v", when, sink.events, want)
	}
	for i, ev := range sink.events {
		if ev != want[i] {
			t.Fatalf("%s: events = %v, want %v", when, sink.events, want)
		}
	}
	sink.events = nil
}

// TestFixedNPCsCrossWaterWithTheirView pins the water crossing of the NPCs
// that never move, a placed decoration and a signet effect point
// (WaterZone.onEnter and onExit, WaterZone.java:28-59): spawning in water
// enters the zone and swims, showing observers the stationary view of an
// NPC whose move speed is 0 and the full one otherwise; despawning leaves
// the zone, showing it again on dry land.
func TestFixedNPCsCrossWaterWithTheirView(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runSpeed   float64
		stationary bool
	}{
		{"still", 0, true},
		{"able to move", 20, false},
	} {
		t.Run("decoration "+tc.name, func(t *testing.T) {
			ix, water := waterIndex(t)
			inst, err := NewInstance(7, &Template{ID: 13006, Type: "ChristmasTree", RunSpeed: tc.runSpeed, WalkSpeed: tc.runSpeed})
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewDecoration(inst, "")
			if err != nil {
				t.Fatal(err)
			}
			sink := &recordingSink{}
			d.Attach(DecorationRuntime{World: world.New(), Zones: ix, Sink: sink})
			d.Spawn(10, 10, 0, 0)
			if !water.Core().Inside(d.zones) || !d.InsideZone(zone.FlagWater) {
				t.Fatal("decoration spawned in water is not in it")
			}
			if got := d.NPCInfoSnapshot().MoveType; got != int(move.MoveSwim) {
				t.Fatalf("decoration in water shows move type %d, want %d (swim)", got, move.MoveSwim)
			}
			assertViewEvents(t, "spawned in water", sink, event.NPCInfoChanged{ServerObject: tc.stationary})
			d.Despawn()
			if water.Core().Inside(d.zones) || d.InsideZone(zone.FlagWater) {
				t.Fatal("despawned decoration is still in the water")
			}
			assertViewEvents(t, "despawned", sink, event.NPCInfoChanged{ServerObject: tc.stationary})
		})
		t.Run("effect point "+tc.name, func(t *testing.T) {
			ix, water := waterIndex(t)
			q := sim.NewInline(time.Unix(0, 0)).NewQueue("point")
			ep, err := NewEffectPoint(7, &Template{ID: 13018, Type: "EffectPoint", RunSpeed: tc.runSpeed, WalkSpeed: tc.runSpeed}, 1, q, nil)
			if err != nil {
				t.Fatal(err)
			}
			sink := &recordingSink{}
			ep.Attach(Runtime{World: world.New(), Sink: sink})
			ep.SetZones(ix)
			ep.Spawn(10, 10, 0, 0)
			if !water.Core().Inside(ep.zones) || !ep.InsideZone(zone.FlagWater) {
				t.Fatal("effect point spawned in water is not in it")
			}
			assertViewEvents(t, "spawned in water", sink, event.NPCInfoChanged{ServerObject: tc.stationary})
			ep.Despawn()
			if water.Core().Inside(ep.zones) || ep.InsideZone(zone.FlagWater) {
				t.Fatal("despawned effect point is still in the water")
			}
			assertViewEvents(t, "despawned", sink, event.NPCInfoChanged{ServerObject: tc.stationary})
		})
	}
}
