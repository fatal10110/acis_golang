package item

import "testing"

func TestShotItemIDRanges(t *testing.T) {
	tests := []struct {
		name        string
		itemID      int32
		fishingShot bool
		summonShot  bool
	}{
		{"below fishing shots", 6534, false, false},
		{"first fishing shot", 6535, true, false},
		{"last fishing shot", 6540, true, false},
		{"after fishing shots", 6541, false, false},
		{"first summon shot", 6645, false, true},
		{"last summon shot", 6647, false, true},
		{"after summon shots", 6648, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFishingShotID(tt.itemID); got != tt.fishingShot {
				t.Fatalf("IsFishingShotID(%d) = %v, want %v", tt.itemID, got, tt.fishingShot)
			}
			if got := IsSummonShotID(tt.itemID); got != tt.summonShot {
				t.Fatalf("IsSummonShotID(%d) = %v, want %v", tt.itemID, got, tt.summonShot)
			}
		})
	}
}
