package skill

// SiegeSkills returns the skills a clan leader of a siege-level clan holds:
// Seal of Ruler (246) and Build Headquarters (247), and for a noble Build
// Advanced Headquarters (326), each at level 1.
func SiegeSkills(noble bool) []Ref {
	refs := []Ref{{ID: 246, Level: 1}, {ID: 247, Level: 1}}
	if noble {
		refs = append(refs, Ref{ID: 326, Level: 1})
	}
	return refs
}
