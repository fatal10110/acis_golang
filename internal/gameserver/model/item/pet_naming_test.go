package item

import "testing"

func TestSetCustomType2PersistsChangedValue(t *testing.T) {
	inst := &Instance{CustomType2: 0}
	var persisted int
	inst.SetPersistNotifier(func(*Instance) { persisted++ })

	if !inst.SetCustomType2(1) {
		t.Fatal("SetCustomType2() = false, want true")
	}
	if got := inst.Snapshot().CustomType2; got != 1 {
		t.Fatalf("CustomType2 = %d, want 1", got)
	}
	if persisted != 1 {
		t.Fatalf("persistence calls = %d, want 1", persisted)
	}
	if inst.SetCustomType2(1) {
		t.Fatal("SetCustomType2(same value) = true, want false")
	}
}
