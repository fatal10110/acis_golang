package manager

import (
	"testing"
	"time"
)

// scriptSlots counts the slots of privates a script created.
func (w *scriptRespawnWorld) scriptSlots() int {
	w.npcs.mu.Lock()
	defer w.npcs.mu.Unlock()
	n := 0
	for _, slot := range w.npcs.slot {
		if slot.scripted {
			n++
		}
	}
	return n
}

// TestScriptPrivateOfDeadMasterDropsItsSlotAtDecay pins that a private a
// script created, which dies after its master, leaves no slot behind once
// its corpse decays: nothing can bring a dead master's private back
// (MinionSpawn.doRespawn), so a kept slot would stay for good and every
// kill of the master would add more.
func TestScriptPrivateOfDeadMasterDropsItsSlotAtDecay(t *testing.T) {
	w := newScriptRespawnWorld(t)
	private := w.private(t)
	w.master.Kill(nil)
	if !w.master.Dead() {
		t.Fatal("the master did not die")
	}
	if private.Master() != nil {
		t.Fatal("the dying master kept its private's master link")
	}
	private.Kill(nil)
	w.decay(t, private)
	if got := w.scriptSlots(); got != 0 {
		t.Fatalf("script slots after the masterless private decayed = %d, want none", got)
	}
	w.pass(24 * time.Hour)
	if got := w.livePrivates(); got != 0 {
		t.Fatalf("privates after the masterless private decayed = %d, want none", got)
	}
}

// TestScriptPrivateOfLiveMasterKeepsItsSlotAtDecay pins the other side: a
// private that decays while its master lives keeps its slot, for the
// master's script to ask it back.
func TestScriptPrivateOfLiveMasterKeepsItsSlotAtDecay(t *testing.T) {
	w := newScriptRespawnWorld(t)
	private := w.private(t)
	private.Kill(nil)
	w.decay(t, private)
	if got := w.scriptSlots(); got != 1 {
		t.Fatalf("script slots after the private of a live master decayed = %d, want 1", got)
	}
	if err := w.npcs.ScheduleRespawn(private, time.Second); err != nil {
		t.Fatalf("respawn request = %v", err)
	}
	w.pass(time.Second)
	if got := w.livePrivates(); got != 1 {
		t.Fatalf("privates after the request = %d, want the private back", got)
	}
}
