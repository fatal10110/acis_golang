package pet

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// ---- from feed_test.go ----
func TestNextFed(t *testing.T) {
	tests := []struct{ current, consume, want int }{
		{100, 10, 90},
		{5, 10, 0},  // floors at zero, never negative
		{10, 10, 0}, // exact consumption also floors at zero
	}
	for _, tt := range tests {
		if got := NextFed(tt.current, tt.consume); got != tt.want {
			t.Errorf("NextFed(%d, %d) = %d, want %d", tt.current, tt.consume, got, tt.want)
		}
	}
}

func TestBelowShare(t *testing.T) {
	tests := []struct {
		fed, maxMeal int
		share        float64
		want         bool
	}{
		{50, 100, 0.55, true},
		{60, 100, 0.55, false},
		// 100*0.55 is not exactly 55 in float64 (it rounds up very
		// slightly), so 55 does land below it. That's the same IEEE-754
		// double arithmetic the specified formula is defined in terms of.
		{55, 100, 0.55, true},
	}
	for _, tt := range tests {
		if got := BelowShare(tt.fed, tt.maxMeal, tt.share); got != tt.want {
			t.Errorf("BelowShare(%d, %d, %v) = %v, want %v", tt.fed, tt.maxMeal, tt.share, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		fed, maxMeal int
		want         StarvationTier
	}{
		{0, 1000, StarvationSevere},
		{50, 1000, StarvationMinor}, // < 10% of 1000
		{99, 1000, StarvationMinor}, // just under the 10% line
		{100, 1000, StarvationNone}, // exactly 10% is not below it
		{500, 1000, StarvationNone},
	}
	for _, tt := range tests {
		if got := Classify(tt.fed, tt.maxMeal); got != tt.want {
			t.Errorf("Classify(%d, %d) = %v, want %v", tt.fed, tt.maxMeal, got, tt.want)
		}
	}
}

func TestStarvationTierLeaveChancePercent(t *testing.T) {
	tests := []struct {
		tier StarvationTier
		want int
	}{
		{StarvationNone, 0},
		{StarvationMinor, 3},
		{StarvationSevere, 30},
	}
	for _, tt := range tests {
		if got := tt.tier.LeaveChancePercent(); got != tt.want {
			t.Errorf("%v.LeaveChancePercent() = %d, want %d", tt.tier, got, tt.want)
		}
	}
}

// ---- from level_test.go ----
func TestRoundInt64_NegativeHalves(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want int64
	}{
		{-0.5, 0},
		{-1.5, -1},
	} {
		if got := roundInt64(tt.in); got != tt.want {
			t.Errorf("roundInt64(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestBabyPetSkillLevel(t *testing.T) {
	tests := []struct {
		petLevel, want int
	}{
		{9, 1},
		{45, 4},
		{70, 7},
		{84, 9},
		{150, 12}, // fixed cap of 12, independent of any skill's own max
	}
	for _, tt := range tests {
		if got := BabyPetSkillLevel(tt.petLevel); got != tt.want {
			t.Errorf("BabyPetSkillLevel(%d) = %d, want %d", tt.petLevel, got, tt.want)
		}
	}
}

// ---- from pet_test.go ----
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
	if got := ScaledExpGain(12077, -1, 0.5, 3.0); got != 0 {
		t.Errorf("ScaledExpGain(negative half) = %d, want 0", got)
	}
}

func TestConfigFromPropertiesLoadsPetRatesAndInventoryLimits(t *testing.T) {
	serverProps, err := config.ParseString(`
PetXpRate = 1.75
SinEaterXpRate = 3.25
`)
	if err != nil {
		t.Fatalf("ParseString(server): %v", err)
	}
	playersProps, err := config.ParseString(`
MaximumSlotsForPet = 21
WeightLimit = 2.5
`)
	if err != nil {
		t.Fatalf("ParseString(players): %v", err)
	}

	cfg, err := ConfigFromProperties(serverProps, playersProps)
	if err != nil {
		t.Fatalf("ConfigFromProperties() error = %v", err)
	}

	if got := cfg.ScaledExpGain(12077, 1000); got != 1750 {
		t.Errorf("Configured ScaledExpGain(ordinary pet) = %d, want 1750", got)
	}
	if got := cfg.ScaledExpGain(12564, 1000); got != 3250 {
		t.Errorf("Configured ScaledExpGain(sin eater) = %d, want 3250", got)
	}
	if cfg.InventorySlots != 21 {
		t.Errorf("Inventory slots = %d, want 21", cfg.InventorySlots)
	}
	if weight := int(cfg.BaseWeightLimit(43)); weight != 136275 {
		t.Errorf("Weight limit = %d, want %d", weight, 136275)
	}
}

func TestConfigFromPropertiesUsesReferenceDefaults(t *testing.T) {
	cfg, err := ConfigFromProperties(nil, nil)
	if err != nil {
		t.Fatalf("ConfigFromProperties(nil, nil) error = %v", err)
	}

	if got := cfg.ScaledExpGain(12077, 1000); got != 1000 {
		t.Errorf("default ordinary pet rate = %d, want 1000", got)
	}
	if got := cfg.ScaledExpGain(12564, 1000); got != 1000 {
		t.Errorf("default sin eater rate = %d, want 1000", got)
	}
	if cfg.InventorySlots != 12 {
		t.Errorf("default pet inventory slots = %d, want 12", cfg.InventorySlots)
	}
	if weight := int(cfg.BaseWeightLimit(43)); weight != 54510 {
		t.Errorf("default weight limit = %d, want %d", weight, 54510)
	}
}
