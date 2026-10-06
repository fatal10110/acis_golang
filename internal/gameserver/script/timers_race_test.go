package script

import (
	"testing"
	"time"
)

// racingNPC is a timer NPC whose next Dead call first runs before: the
// start reads Dead to choose the home queue, so before runs between that
// choice and the registration.
type racingNPC struct {
	*timerNPCFake
	before func()
}

func (n *racingNPC) Dead() bool {
	if f := n.before; f != nil {
		n.before = nil
		f()
	}
	return n.timerNPCFake.Dead()
}

// TestNPCQueueTimerStartRacingDetachIsRemoved: a behavior timer on the
// NPC's queue that is also bound to a player, whose start registered after
// that player's detach removal, does not stay bound to the departed player.
func TestNPCQueueTimerStartRacingDetachIsRemoved(t *testing.T) {
	w, _ := homeWorld(t)
	behavior := w.script("Behavior")
	p1 := w.plf["p1"]
	n := &NPC{self: &racingNPC{timerNPCFake: w.fakes["npc1"], before: func() {
		p1.detaching.Store(true)
		w.r.timers.playerDetached(p1)
	}}}
	if !behavior.StartTimerAtFixedRate("tick", n, w.player("p1"), time.Second, time.Second) {
		t.Fatal("start refused")
	}
	if behavior.HasTimer("tick", n, w.player("p1")) {
		t.Fatal("a start racing the player's detach left its timer bound to the departed player")
	}
	w.advanceTo(3000)
	if got := w.take(); len(got) != 0 {
		t.Fatalf("fired %q after the detach", got)
	}
}

// TestPlayerQueueBehaviorTimerStartRacingDecayIsRemoved: a behavior timer
// bound to a dead NPC runs on its player's queue; a start that registered
// after the NPC's decay removal is removed too.
func TestPlayerQueueBehaviorTimerStartRacingDecayIsRemoved(t *testing.T) {
	w, _ := homeWorld(t)
	behavior := w.script("Behavior")
	f := w.fakes["npc1"]
	f.dead.Store(true)
	n := &NPC{self: &racingNPC{timerNPCFake: f, before: func() {
		f.decayed.Store(true)
		w.r.timers.npcDecayed(f.scratch)
	}}}
	if !behavior.StartTimer("corpse", n, w.player("p1"), time.Second) {
		t.Fatal("start refused")
	}
	if behavior.HasTimer("corpse", n, w.player("p1")) {
		t.Fatal("a start racing the NPC's decay left its timer registered")
	}
	w.advanceTo(2000)
	if got := w.take(); len(got) != 0 {
		t.Fatalf("fired %q after the decay", got)
	}
}

// TestNPCQueueTimerStartedAfterDetachStays: a behavior timer started for a
// player that is already leaving is a start after the departure: nothing
// removes it and it fires on the NPC's queue.
func TestNPCQueueTimerStartedAfterDetachStays(t *testing.T) {
	w, ran := homeWorld(t)
	behavior := w.script("Behavior")
	w.plf["p1"].detaching.Store(true)
	w.r.timers.playerDetached(w.plf["p1"])
	if !behavior.StartTimer("late", w.npc("npc1"), w.player("p1"), time.Second) {
		t.Fatal("start refused")
	}
	w.advanceTo(1000)
	if len(*ran) != 1 || (*ran)[0] != "late on npc1" {
		t.Fatalf("hooks ran %v, want the late timer on npc1", *ran)
	}
}
