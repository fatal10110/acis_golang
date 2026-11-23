package siege

import "testing"

// TestUnopposed pins #5146: Unopposed runs join only while the two clans
// stand on no opposite sides of a siege, and runs it with the engine's
// lock held, so no registration lands between the check and the join.
func TestUnopposed(t *testing.T) {
	w := twoArtifactWorld(t)
	ran := false
	if w.e.Unopposed(lords, rivals, func() { ran = true }) || ran {
		t.Fatal("the owner and an attacker were not opposed")
	}
	if !w.e.Unopposed(rivals, others, func() {
		ran = true
		if w.e.mu.TryLock() {
			w.e.mu.Unlock()
			t.Error("join ran without the engine's lock")
		}
	}) || !ran {
		t.Fatal("two attackers were opposed")
	}
}
