package pet

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

func TestExpForLevel(t *testing.T) {
	data := &npc.PetData{Levels: map[int]npc.PetLevelStats{
		10: {MaxExp: 1000},
	}}

	if got, ok := ExpForLevel(data, 10); !ok || got != 1000 {
		t.Errorf("ExpForLevel(10) = (%d, %v), want (1000, true)", got, ok)
	}
	if got, ok := ExpForLevel(data, 11); ok || got != 0 {
		t.Errorf("ExpForLevel(11) = (%d, %v), want (0, false)", got, ok)
	}
	if got, ok := ExpForLevel(nil, 10); ok || got != 0 {
		t.Errorf("ExpForLevel(nil, 10) = (%d, %v), want (0, false)", got, ok)
	}
}

// Expected values below were computed independently from the specified
// formula (percentLost = -0.07*level + 6.5; round half away from zero), not
// copied from this package's own implementation.
func TestDeathPenaltyExpLoss(t *testing.T) {
	tests := []struct {
		name  string
		level int
		cur   int64
		next  int64
		want  int64
	}{
		{"level 44", 44, 800000, 1000000, 6840},
		{"level 10", 10, 1000, 2500, 87},
		{"level 1", 1, 0, 136, 9},
		// Past level ~93 the formula goes negative: a death at this level
		// grants exp instead of costing it. That's an intentional artifact
		// of the specified linear formula, preserved as-is.
		{"level 99 goes negative", 99, 5000000, 5200000, -860},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &npc.PetData{Levels: map[int]npc.PetLevelStats{
				tt.level:     {MaxExp: tt.cur},
				tt.level + 1: {MaxExp: tt.next},
			}}
			if got := DeathPenaltyExpLoss(data, tt.level); got != tt.want {
				t.Errorf("DeathPenaltyExpLoss(level=%d) = %d, want %d", tt.level, got, tt.want)
			}
		})
	}
}

func TestDeathPenaltyExpLoss_MissingRow(t *testing.T) {
	data := &npc.PetData{Levels: map[int]npc.PetLevelStats{
		5: {MaxExp: 100},
	}}
	if got := DeathPenaltyExpLoss(data, 5); got != 0 {
		t.Errorf("DeathPenaltyExpLoss with no next-level row = %d, want 0", got)
	}
	if got := DeathPenaltyExpLoss(data, 4); got != 0 {
		t.Errorf("DeathPenaltyExpLoss with no current-level row = %d, want 0", got)
	}
}

func TestRestoreExp(t *testing.T) {
	tests := []struct {
		name           string
		expBeforeDeath int64
		currentExp     int64
		restorePercent float64
		want           int64
	}{
		{"half restore", 100000, 93160, 50, 3420},
		{"33 percent restore", 50000, 40000, 33, 3300},
		{"nothing lost", 100000, 100000, 100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RestoreExp(tt.expBeforeDeath, tt.currentExp, tt.restorePercent); got != tt.want {
				t.Errorf("RestoreExp() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSkillLevel(t *testing.T) {
	tests := []struct {
		petLevel, maxSkillLevel, want int
	}{
		{45, 12, 5},
		{9, 12, 1},
		{70, 12, 8},
		{84, 12, 10},
		{150, 10, 10}, // clamps to the skill's own max
	}
	for _, tt := range tests {
		if got := SkillLevel(tt.petLevel, tt.maxSkillLevel); got != tt.want {
			t.Errorf("SkillLevel(%d, %d) = %d, want %d", tt.petLevel, tt.maxSkillLevel, got, tt.want)
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
