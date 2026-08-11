package player

import "testing"

func TestCharacterMountWyvernTracksControlItemAndNotifies(t *testing.T) {
	c := &Character{}
	if !c.Mount(12621, 77) {
		t.Fatal("Mount() = false, want true")
	}
	if got := c.MountType(); got != 2 {
		t.Fatalf("MountType() = %d, want 2", got)
	}
	if got := c.MountNPCID(); got != 12621 {
		t.Fatalf("MountNPCID() = %d, want 12621", got)
	}
	if got := c.MountObjectID(); got != 77 {
		t.Fatalf("MountObjectID() = %d, want 77", got)
	}
}
