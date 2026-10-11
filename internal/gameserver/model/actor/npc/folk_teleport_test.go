package npc

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestFolkTeleportWithoutMovement (Creature.teleportTo): a civilian NPC
// with no movement runtime jumps to the target as given, with no
// grounding; its observers see one jump, and it leaves the world grid at
// its old position and re-enters it at the target, so the objects around
// the target know it and the ones it left do not.
func TestFolkTeleportWithoutMovement(t *testing.T) {
	inst, err := NewInstance(1, &Template{ID: 31677, TemplateID: 31677, Type: "Folk", Level: 70, HPMax: 2444})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New()
	rec := &event.Recorder{}
	if err := f.Attach(FolkRuntime{World: w, Queue: sim.NewInline(time.Unix(0, 0)).NewQueue("folk"), Sink: rec}); err != nil {
		t.Fatal(err)
	}
	w.Spawn(f, 100, 0, 0, 0)
	before := &hostileTarget{id: 2}
	w.Spawn(before, 200, 0, 0, 0)
	target := location.Location{X: 40000, Y: 40000, Z: -1234}
	after := &hostileTarget{id: 3}
	w.Spawn(after, target.X+100, target.Y, target.Z, 0)
	if f.motion != nil {
		t.Fatal("the NPC has a movement runtime; the test needs one without")
	}
	if known := w.AppendKnown(nil, f); !slices.Contains(known, world.Tracked(before)) || slices.Contains(known, world.Tracked(after)) {
		t.Fatalf("before the teleport the NPC knows %v, want the object at its spawn only", trackedIDs(known))
	}

	f.TeleportTo(target)

	if got := f.location(); got.X != target.X || got.Y != target.Y || got.Z != target.Z {
		t.Errorf("position after the teleport = %+v, want %+v", got, target)
	}
	var jumps []event.Teleported
	for _, ev := range rec.Events() {
		if tp, ok := ev.(event.Teleported); ok {
			jumps = append(jumps, tp)
		}
	}
	if want := []event.Teleported{{To: target}}; !slices.Equal(jumps, want) {
		t.Errorf("Teleported events = %+v, want %+v", jumps, want)
	}
	if known := w.AppendKnown(nil, f); slices.Contains(known, world.Tracked(before)) || !slices.Contains(known, world.Tracked(after)) {
		t.Errorf("after the teleport the NPC knows %v, want the object at the target only", trackedIDs(known))
	}
}

func trackedIDs(ts []world.Tracked) []int32 {
	out := make([]int32, 0, len(ts))
	for _, tr := range ts {
		out = append(out, tr.ObjectID())
	}
	return out
}
