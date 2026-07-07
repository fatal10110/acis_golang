package npc

import "testing"

func TestRaceBySecondarySkillID(t *testing.T) {
	if got := RaceBySecondarySkillID(4290); got != RaceUndead {
		t.Fatalf("RaceBySecondarySkillID(4290) = %v, want RaceUndead", got)
	}
	if got := RaceBySecondarySkillID(4302); got != RaceFairy {
		t.Fatalf("RaceBySecondarySkillID(4302) = %v, want RaceFairy", got)
	}
	if got := RaceBySecondarySkillID(4416); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(4416) = %v, want RaceDummy (not a secondary marker)", got)
	}
	if got := RaceBySecondarySkillID(1); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(1) = %v, want RaceDummy", got)
	}
}

func TestRaceByOrdinal(t *testing.T) {
	got, ok := RaceByOrdinal(13)
	if !ok || got != RaceFairy {
		t.Fatalf("RaceByOrdinal(13) = %v, %v, want RaceFairy, true", got, ok)
	}
	if _, ok := RaceByOrdinal(-1); ok {
		t.Fatal("RaceByOrdinal(-1) ok = true, want false")
	}
	if _, ok := RaceByOrdinal(len(raceNames)); ok {
		t.Fatal("RaceByOrdinal(len) ok = true, want false")
	}
}
