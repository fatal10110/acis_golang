package script

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

var (
	classGroupNames      = []string{"@attacker_group", "@fighter_group", "@mage_group", "@wizard_group", "@cleric_group", "@subjob_group_knight"}
	classOccupationNames = []string{"@fighter", "@mage", "@swordsinger", "@elven_wizard"}
)

// TestPlayerClassGroupsAndOccupations checks InClassGroup and SameOccupation
// for every class id against testdata/class_groups.golden, a dump of the
// reference ClassId enum (id is the enum ordinal): the groups each class is
// in and the occupation it is.
func TestPlayerClassGroupsAndOccupations(t *testing.T) {
	raw, err := os.ReadFile("testdata/class_groups.golden")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Fatalf("golden row %q: want 4 fields", line)
		}
		id, err := strconv.Atoi(f[0])
		if err != nil || id != rows {
			t.Fatalf("golden row %q: id is not the next ordinal %d", line, rows)
		}
		rows++
		inGroup := splitSet(t, f[2], classGroupNames)
		isOcc := splitSet(t, f[3], classOccupationNames)

		p := playerAt(id, 40)
		for _, g := range classGroupNames {
			if got := p.InClassGroup(g); got != inGroup[g] {
				t.Errorf("class %d %s: InClassGroup(%q) = %v, want %v", id, f[1], g, got, inGroup[g])
			}
		}
		for _, o := range classOccupationNames {
			if got := p.SameOccupation(o); got != isOcc[o] {
				t.Errorf("class %d %s: SameOccupation(%q) = %v, want %v", id, f[1], o, got, isOcc[o])
			}
		}
	}
	if rows != 119 {
		t.Fatalf("golden has %d classes, want 119 (ids 0..118)", rows)
	}
}

// splitSet reads a comma list ("-" for none) of names from the vocabulary.
func splitSet(t *testing.T, field string, vocabulary []string) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	if field == "-" {
		return set
	}
	for _, name := range strings.Split(field, ",") {
		known := false
		for _, v := range vocabulary {
			known = known || v == name
		}
		if !known {
			t.Fatalf("golden names %q, which the test does not exercise", name)
		}
		set[name] = true
	}
	return set
}

// An unknown name, an empty one and a name of the other kind hold no class.
func TestPlayerClassGroupUnknownNames(t *testing.T) {
	for _, class := range []int{0, 10, 21, 26, 88, 118} {
		p := playerAt(class, 40)
		for _, name := range []string{"", "@unknown_group", "@FIGHTER_GROUP", "fighter_group", "@fighter", "@mage", "@race_human"} {
			if p.InClassGroup(name) {
				t.Errorf("class %d: InClassGroup(%q) = true", class, name)
			}
		}
		for _, name := range []string{"", "@unknown", "@FIGHTER", "fighter", "@fighter_group", "@mage_group", "@race_human"} {
			if p.SameOccupation(name) {
				t.Errorf("class %d: SameOccupation(%q) = true", class, name)
			}
		}
	}
}
