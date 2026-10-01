package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// TestDeadFolkTickBeforeAbortCastDoesNotIdle follows Npc.doDie, which
// resets the NPC's lifetime and takes it off the AI task at once, so a dead
// NPC is never ticked: a hit civilian NPC holding no desire that dies while
// an AI tick is already queued ahead of its death's reset runs that tick as
// a corpse, and the tick must not switch the corpse to its walk stance.
func TestDeadFolkTickBeforeAbortCastDoesNotIdle(t *testing.T) {
	r := newFolkCastRig(t, 50)
	if r.f.Instance.Template.Undying {
		t.Fatal("rig template is undying, want a mortal NPC")
	}
	attacker := newFolkAttacker(t)

	if r.f.TakeDamage(10, attacker) {
		t.Fatal("first hit killed the NPC")
	}
	if !r.f.Running() {
		t.Fatal("hit NPC not running")
	}
	if !r.f.TakeDamage(1_000_000, attacker) {
		t.Fatal("lethal hit reported no kill")
	}
	// The death's AbortCast reset is posted, not run: the tick runs first.
	if r.f.cast.lifeTime == 0 {
		t.Fatal("lifetime already reset before the queued AbortCast ran")
	}
	before := len(r.events.events)

	r.tick(t)

	for _, ev := range r.events.events[before:] {
		switch ev.(type) {
		case event.MoveTypeChanged, event.NPCInfoChanged:
			t.Fatalf("dead NPC's tick emitted %T, want nothing", ev)
		}
	}
	if !r.f.Running() {
		t.Fatal("dead NPC's tick switched it to walk stance")
	}

	r.in.Run()
	if r.aiTask.removes != 1 {
		t.Fatalf("AI task removals after the death's reset = %d, want 1", r.aiTask.removes)
	}
}
