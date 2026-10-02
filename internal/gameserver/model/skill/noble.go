package skill

// nobleSkillIDs are the skills noblesse status grants, each at level 1.
var nobleSkillIDs = [...]ID{325, 326, 327, 1323, 1324, 1325, 1326, 1327}

// NobleSkills returns the skills noblesse status grants.
func NobleSkills() []Ref {
	refs := make([]Ref, len(nobleSkillIDs))
	for i, id := range nobleSkillIDs {
		refs[i] = Ref{ID: id, Level: 1}
	}
	return refs
}
