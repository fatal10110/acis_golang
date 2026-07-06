package player

import "testing"

func TestClassRace(t *testing.T) {
	tests := []struct {
		classID int
		want    Race
	}{
		{0, RaceHuman},    // Human Fighter (root)
		{9, RaceHuman},    // human fighter line, 2nd tier
		{93, RaceHuman},   // human fighter line, 3rd tier, multiple parent hops to root
		{10, RaceHuman},   // Human Mystic (root)
		{18, RaceElf},     // Elven Fighter (root)
		{25, RaceElf},     // Elven Mystic (root)
		{31, RaceDarkElf}, // Dark Fighter (root)
		{38, RaceDarkElf}, // Dark Mystic (root)
		{44, RaceOrc},     // Orc Fighter (root)
		{49, RaceOrc},     // Orc Mystic (root)
		{53, RaceDwarf},   // Dwarven Fighter (root)
		{57, RaceDwarf},   // Warsmith, dwarf line, 2nd tier
		{118, RaceDwarf},  // 3rd tier id parented under the dwarf line
	}
	for _, tt := range tests {
		got, ok := ClassRace(tt.classID)
		if !ok {
			t.Errorf("ClassRace(%d) reported unknown, want %v", tt.classID, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("ClassRace(%d) = %v, want %v", tt.classID, got, tt.want)
		}
	}
}

func TestClassRace_UnknownID(t *testing.T) {
	if _, ok := ClassRace(9999); ok {
		t.Error("ClassRace(9999) reported known, want unknown")
	}
	// 58-87 are reserved dummy ids with no profession behind them.
	if _, ok := ClassRace(70); ok {
		t.Error("ClassRace(70) reported known, want unknown")
	}
}
