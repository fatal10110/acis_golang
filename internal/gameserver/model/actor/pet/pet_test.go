package pet

import "testing"

func TestIsMountable(t *testing.T) {
	tests := []struct {
		npcID int
		want  bool
	}{
		{12526, true},
		{12527, true},
		{12528, true},
		{12621, true},
		{12077, false}, // an ordinary pet npc id
		{0, false},
	}
	for _, tt := range tests {
		if got := IsMountable(tt.npcID); got != tt.want {
			t.Errorf("IsMountable(%d) = %v, want %v", tt.npcID, got, tt.want)
		}
	}
}

func TestTracksOwnerLevel(t *testing.T) {
	if !TracksOwnerLevel(12564) {
		t.Errorf("TracksOwnerLevel(12564) = false, want true")
	}
	if TracksOwnerLevel(12077) {
		t.Errorf("TracksOwnerLevel(12077) = true, want false")
	}
}

func TestInitialLevel(t *testing.T) {
	if got := InitialLevel(12077, 20, 55); got != 20 {
		t.Errorf("InitialLevel(ordinary pet) = %d, want template level 20", got)
	}
	if got := InitialLevel(12564, 20, 55); got != 55 {
		t.Errorf("InitialLevel(owner-tracking pet) = %d, want owner level 55", got)
	}
}

func TestScaledExpGain(t *testing.T) {
	if got := ScaledExpGain(12077, 1000, 1.5, 3.0); got != 1500 {
		t.Errorf("ScaledExpGain(ordinary pet) = %d, want 1500", got)
	}
	if got := ScaledExpGain(12564, 1000, 1.5, 3.0); got != 3000 {
		t.Errorf("ScaledExpGain(owner-tracking pet) = %d, want 3000", got)
	}
}
