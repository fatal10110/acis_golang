package script

import "testing"

// ClassParent answers ClassId.getParent: the class a class upgrades from,
// none (-1) for a starting class; an id that is no class panics.
func TestClassParent(t *testing.T) {
	for _, tc := range []struct{ class, parent int32 }{
		{90, 5},   // Phoenix Knight from Paladin
		{5, 4},    // Paladin from Human Knight
		{4, 0},    // Human Knight from Human Fighter
		{0, -1},   // Human Fighter starts
		{118, 57}, // Maestro from Warsmith
	} {
		if got := ClassParent(tc.class); got != tc.parent {
			t.Errorf("ClassParent(%d) = %d, want %d", tc.class, got, tc.parent)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("ClassParent of no class returned")
		}
	}()
	ClassParent(200)
}
