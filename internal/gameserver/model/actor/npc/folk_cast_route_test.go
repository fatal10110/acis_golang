package npc

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// recordingFolkRoute records a route walker leaving and resuming its route.
type recordingFolkRoute struct{ leaves, resumes int }

func (r *recordingFolkRoute) LeaveRoute(task.WalkerActor)        { r.leaves++ }
func (r *recordingFolkRoute) ResumeRoute(task.WalkerActor) error { r.resumes++; return nil }

// fallingTarget is a target a test can kill.
type fallingTarget struct {
	*hostileTarget
	dead bool
}

func (t *fallingTarget) AlikeDead() bool { return t.dead }

// TestFolkOffRouteWalkerWaitsOutTeleportBeforeResuming pins the off-route
// mark: a route walker whose last cast desire goes away on a tick its AI
// sits out (a teleport under way) resumes its route on the first tick
// after the teleport, and never idles meanwhile (NpcAI.runAI: its route
// desire never leaves the queue).
func TestFolkOffRouteWalkerWaitsOutTeleportBeforeResuming(t *testing.T) {
	inst, err := NewInstance(1, &Template{ID: 31226, TemplateID: 31226, Type: "Folk", Level: 70, HPMax: 2444, CanMove: true})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst, false)
	if err != nil {
		t.Fatal(err)
	}
	in := sim.NewInline(time.Unix(0, 0))
	q := in.NewQueue("folk")
	aiTask, route := &recordingFolkAI{}, &recordingFolkRoute{}
	w := world.New()
	if err := f.Attach(FolkRuntime{World: w, Queue: q, Sink: &recordingSink{}, AI: aiTask, LOS: &switchLOS{}}); err != nil {
		t.Fatal(err)
	}
	control := &scriptedFolkControl{f: f}
	// canCast refuses, so the walker acts on the desire without a cast in
	// flight and keeps it.
	castAI := &scriptedFolkCastAI{control: control, canDesire: true, castRange: 100}
	f.SetCaster(control, castAI)
	if _, err := f.EnableMovement(FolkMovement{Geo: hostileGeo{}, Queue: q, Route: route}); err != nil {
		t.Fatal(err)
	}
	target := &fallingTarget{hostileTarget: &hostileTarget{id: 2, playable: true}}
	w.Spawn(f, 100, 0, 0, 0)
	w.Spawn(target.hostileTarget, 160, 0, 0, 0)

	tick := func() {
		t.Helper()
		if err := f.TickThink(); err != nil {
			t.Fatalf("TickThink() = %v", err)
		}
	}
	// The AI acts on nothing on the NPC's first tick.
	tick()
	f.AddCastDesire(target, modelskill.Ref{ID: 4380, Level: 1}, 2*routeDesireWeight)
	in.Run()

	tick()
	if route.leaves != 1 || !f.cast.offRoute {
		t.Fatalf("leaves = %d, offRoute = %v after acting on the desire, want 1, true", route.leaves, f.cast.offRoute)
	}

	// The target dies while a teleport is under way: the desire is pruned,
	// but the AI acts on nothing this tick.
	f.motion.teleporting.Store(true)
	target.dead = true
	tick()
	if got := f.cast.desires.Len(); got != 0 {
		t.Fatalf("desires = %d, want the dead target's desire dropped", got)
	}
	if route.resumes != 0 {
		t.Fatalf("resumes = %d during the teleport, want 0", route.resumes)
	}
	if !f.Running() {
		t.Fatal("walker idled to its walk stance off its route")
	}

	f.motion.teleporting.Store(false)
	tick()
	if route.resumes != 1 || f.cast.offRoute {
		t.Fatalf("resumes = %d, offRoute = %v after the teleport, want 1, false", route.resumes, f.cast.offRoute)
	}
	if aiTask.removes != 0 {
		t.Fatalf("AI task removes = %d once back on the route, want the walker kept on the task", aiTask.removes)
	}
}
