package npc

import "testing"

// TestRaceEnumNameCoversEveryRace pins one enum name per race, in the
// reference NpcRace declaration order.
func TestRaceEnumNameCoversEveryRace(t *testing.T) {
	if len(raceEnumNames) != len(raceNames) {
		t.Fatalf("%d enum names for %d races", len(raceEnumNames), len(raceNames))
	}
	for r, want := range map[Race]string{
		RaceDummy: "DUMMY", RaceMagicCreature: "MAGIC_CREATURE", RaceFairy: "FAIRIE",
		RaceElf: "ELVE", RaceDarkElf: "DARKELVE", RaceDwarf: "DWARVE", RaceMercenary: "MERCENARIE",
		RaceUnknownCreature: "UNKNOWN_CREATURE",
	} {
		if got := r.EnumName(); got != want {
			t.Errorf("%v.EnumName() = %q, want %q", r, got, want)
		}
	}
	if got := Race(len(raceNames)).EnumName(); got != "" {
		t.Errorf("EnumName past the last race = %q, want empty", got)
	}
}
