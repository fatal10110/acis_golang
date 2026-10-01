package player

import (
	"slices"
	"testing"
)

// TestAvailableSubclassesFollowsTheBaseClassesRules pins the subclass set
// of each rule: an exclusive group, the elf and dark elf exclusion, a third
// profession using its second profession's set, and a base below the
// second profession holding none.
func TestAvailableSubclassesFollowsTheBaseClassesRules(t *testing.T) {
	gladiator := []int{3, 5, 6, 8, 9, 12, 13, 14, 16, 17, 20, 21, 23, 24, 27, 28, 30, 33, 34, 36, 37, 40, 41, 43, 46, 48, 52, 55}
	paladin := []int{2, 3, 8, 9, 12, 13, 14, 16, 17, 21, 23, 24, 27, 28, 30, 34, 36, 37, 40, 41, 43, 46, 48, 52, 55}
	for _, tc := range []struct {
		name string
		base int
		want []int
	}{
		// A human in no exclusive group: every second profession but its
		// own, Overlord and Warsmith.
		{"Gladiator", 2, gladiator},
		// Paladin, Dark Avenger, Temple Knight and Shillien Knight exclude
		// each other.
		{"Paladin", 5, paladin},
		// An elf takes no dark elf profession; Plains Walker no Treasure
		// Hunter or Abyss Walker either.
		{"Plains Walker", 23, []int{2, 3, 5, 6, 9, 12, 13, 14, 16, 17, 20, 21, 24, 27, 28, 30, 46, 48, 52, 55}},
		// A dark elf takes no elven profession; Spellhowler no Sorcerer
		// either.
		{"Spellhowler", 40, []int{2, 3, 5, 6, 8, 9, 13, 14, 16, 17, 33, 34, 36, 37, 41, 43, 46, 48, 52, 55}},
		// An elf outside every group still takes no dark elf profession.
		{"Sword Singer", 21, []int{2, 3, 5, 6, 8, 9, 12, 13, 14, 16, 17, 20, 23, 24, 27, 28, 30, 46, 48, 52, 55}},
		// A third profession takes its second profession's set.
		{"Duelist", 88, gladiator},
		{"Phoenix Knight", 90, paladin},
		// Below the second profession there is no subclass.
		{"Warrior", 1, nil},
		{"Human Fighter", 0, nil},
		{"unknown", 200, nil},
	} {
		if got := AvailableSubclasses(tc.base); !slices.Equal(got, tc.want) {
			t.Errorf("%s: AvailableSubclasses(%d) = %v, want %v", tc.name, tc.base, got, tc.want)
		}
	}
}

func TestClassEqualsOrChildOf(t *testing.T) {
	for _, tc := range []struct {
		id, ancestor int
		want         bool
	}{
		{2, 2, true},
		{88, 2, true}, // Duelist upgrades from Gladiator
		{88, 1, true}, // and from Warrior
		{2, 88, false},
		{3, 2, false},
		{200, 2, false},
	} {
		if got := ClassEqualsOrChildOf(tc.id, tc.ancestor); got != tc.want {
			t.Errorf("ClassEqualsOrChildOf(%d, %d) = %v, want %v", tc.id, tc.ancestor, got, tc.want)
		}
	}
}
