package skill

// heroSkillOrder is the skills hero status grants, each at level 1.
var heroSkillOrder = [...]ID{395, 396, 1374, 1375, 1376}

// HeroSkills returns the skills hero status grants.
func HeroSkills() []Ref {
	refs := make([]Ref, len(heroSkillOrder))
	for i, id := range heroSkillOrder {
		refs[i] = Ref{ID: id, Level: 1}
	}
	return refs
}
