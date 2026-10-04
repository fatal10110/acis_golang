package clan

import "context"

// SetSkill has cl know sk at its level, stored, replacing the level it
// knew before. It reports false, changing nothing, when cl already knows
// that very level.
func (s *Service) SetSkill(cl *Clan, sk Skill) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if level, ok := cl.skills[sk.ID]; ok && level == sk.Level {
		return false
	}
	if cl.skills == nil {
		cl.skills = map[int]int{}
	}
	cl.skills[sk.ID] = sk.Level
	clanID := cl.id
	s.write(clanID, "store clan skill", func(ctx context.Context, st Store) error { return st.SaveSkill(ctx, clanID, sk) })
	return true
}

// RaiseSkills has cl know each of skills it does not know, or knows at a
// lower level, at that skill's level, all stored in one write. It reports
// false, changing nothing, when cl already knows every one of them at its
// level or above.
func (s *Service) RaiseSkills(cl *Clan, skills []Skill) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	var raised []Skill
	for _, sk := range skills {
		if level, ok := cl.skills[sk.ID]; !ok || level < sk.Level {
			raised = append(raised, sk)
		}
	}
	if len(raised) == 0 {
		return false
	}
	if cl.skills == nil {
		cl.skills = map[int]int{}
	}
	for _, sk := range raised {
		cl.skills[sk.ID] = sk.Level
	}
	clanID := cl.id
	s.write(clanID, "store clan skills", func(ctx context.Context, st Store) error {
		for _, sk := range raised {
			if err := st.SaveSkill(ctx, clanID, sk); err != nil {
				return err
			}
		}
		return nil
	})
	return true
}

// RemoveSkill has cl forget skill id, its row deleted. It reports false
// when cl does not know it.
func (s *Service) RemoveSkill(cl *Clan, id int) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if _, ok := cl.skills[id]; !ok {
		return false
	}
	delete(cl.skills, id)
	clanID := cl.id
	s.write(clanID, "remove clan skill", func(ctx context.Context, st Store) error { return st.RemoveSkill(ctx, clanID, id) })
	return true
}

// RemoveAllSkills has cl forget every skill it knows, its rows deleted,
// and returns them in ascending id order. It reports false when cl knows
// none.
func (s *Service) RemoveAllSkills(cl *Clan) ([]Skill, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if len(cl.skills) == 0 {
		return nil, false
	}
	removed := cl.skillsLocked()
	clear(cl.skills)
	clanID := cl.id
	s.write(clanID, "remove clan skills", func(ctx context.Context, st Store) error { return st.RemoveAllSkills(ctx, clanID) })
	return removed, true
}
