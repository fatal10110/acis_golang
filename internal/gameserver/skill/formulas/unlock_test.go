package formulas

import "testing"

func TestDoorUnlockSpecialSucceeds(t *testing.T) {
	if !DoorUnlockSpecialSucceeds(50, 49) {
		t.Error("DoorUnlockSpecialSucceeds(50, 49) = false, want true")
	}
	if DoorUnlockSpecialSucceeds(50, 50) {
		t.Error("DoorUnlockSpecialSucceeds(50, 50) = true, want false")
	}
}

func TestDoorUnlockRate(t *testing.T) {
	tests := []struct {
		level int
		want  int
	}{
		{0, 0}, {1, 30}, {2, 50}, {3, 75}, {4, 100}, {100, 100},
	}
	for _, tt := range tests {
		if got := DoorUnlockRate(tt.level); got != tt.want {
			t.Errorf("DoorUnlockRate(%d) = %d, want %d", tt.level, got, tt.want)
		}
	}
}

func TestDoorUnlockSucceeds(t *testing.T) {
	if !DoorUnlockSucceeds(2, 49) {
		t.Error("DoorUnlockSucceeds(2, 49) = false, want true")
	}
	if DoorUnlockSucceeds(2, 50) {
		t.Error("DoorUnlockSucceeds(2, 50) = true, want false")
	}
	if DoorUnlockSucceeds(0, 0) {
		t.Error("DoorUnlockSucceeds(0, 0) = true, want false")
	}
}

func TestChestUnlockDeluxeKeyRate(t *testing.T) {
	tests := []struct {
		name              string
		level, skillLevel int
		regularKey        bool
		want              int
	}{
		{"deluxe key, exact match", 100, 10, false, 100},
		{"regular key, exact match", 100, 10, true, 60},
		{"mismatch goes negative uncapped", 125, 8, false, -60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChestUnlockDeluxeKeyRate(tt.level, tt.skillLevel, tt.regularKey)
			if got != tt.want {
				t.Errorf("ChestUnlockDeluxeKeyRate() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestChestUnlockRate(t *testing.T) {
	tests := []struct {
		name              string
		level, skillLevel int
		wantChance        int
		wantDefinite      bool
		wantSucceeds      bool
	}{
		{"above 60, too low", 70, 9, 0, true, false},
		{"above 60, at threshold", 70, 10, 30, false, false},
		{"above 60, exactly at cap", 70, 14, 50, false, false},
		{"above 60, capped", 70, 20, 50, false, false},
		{"above 40, too low", 50, 5, 0, true, false},
		{"above 40, at threshold", 50, 6, 10, false, false},
		{"above 30, too low", 35, 2, 0, true, false},
		{"above 30, definite success", 35, 13, 0, true, true},
		{"above 30, capped", 35, 12, 50, false, false},
		{"base bracket, definite success", 20, 11, 0, true, true},
		{"base bracket, capped", 20, 10, 50, false, false},
		{"base bracket, uncapped", 20, 0, 35, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chance, definite, succeeds := ChestUnlockRate(tt.level, tt.skillLevel)
			if chance != tt.wantChance || definite != tt.wantDefinite || succeeds != tt.wantSucceeds {
				t.Errorf("ChestUnlockRate(%d, %d) = (%d, %v, %v), want (%d, %v, %v)",
					tt.level, tt.skillLevel, chance, definite, succeeds, tt.wantChance, tt.wantDefinite, tt.wantSucceeds)
			}
		})
	}
}
