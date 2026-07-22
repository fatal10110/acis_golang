package pet

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

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
	slots, weight := cfg.InventoryLimits(43)
	if slots != 21 {
		t.Errorf("Inventory slots = %d, want 21", slots)
	}
	if want := int(34500 * statbonus.CONBonus[43] * 2.5); weight != want {
		t.Errorf("Weight limit = %d, want %d", weight, want)
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
	slots, weight := cfg.InventoryLimits(43)
	if slots != 12 {
		t.Errorf("default pet inventory slots = %d, want 12", slots)
	}
	if want := int(34500 * statbonus.CONBonus[43]); weight != want {
		t.Errorf("default weight limit = %d, want %d", weight, want)
	}
}
