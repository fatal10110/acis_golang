package clan

import (
	"context"
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Skill is one skill a clan has learnt, at the level it knows it.
type Skill struct {
	ID    int
	Level int
}

// SkillRow is one clan_skills row.
type SkillRow struct {
	ClanID int32
	Skill
}

// KeepSkills drops the clan skill rows known reports false for, as a clan
// skill whose definition is not loaded is skipped at restore.
func (s *Snapshot) KeepSkills(known func(Skill) bool) {
	kept := s.Skills[:0]
	for _, r := range s.Skills {
		if known(r.Skill) {
			kept = append(kept, r)
		}
	}
	s.Skills = kept
}

// Skills returns the clan's skills in ascending id order.
func (cl *Clan) Skills() []Skill {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.skillsLocked()
}

func (cl *Clan) skillsLocked() []Skill {
	out := make([]Skill, 0, len(cl.skills))
	for id, level := range cl.skills {
		out = append(out, Skill{ID: id, Level: level})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (cl *Clan) skillLevelsLocked() modelskill.SkillLevels {
	out := make(modelskill.SkillLevels, len(cl.skills))
	for id, level := range cl.skills {
		out[modelskill.ID(id)] = level
	}
	return out
}

// Reputation is the clan's reputation score.
func (cl *Clan) Reputation() int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.reputation
}

// LearnableSkills returns the skill tree nodes the clan can learn now: each
// skill it does not know at level 1, or the level after the one it knows,
// once the clan reaches the node's level.
func (cl *Clan) LearnableSkills(trees *modelskill.Trees) []modelskill.ClanSkill {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return trees.ClanSkillsFor(cl.level, cl.skillLevelsLocked())
}

// LearnableSkill returns the skill tree node for skill id at level when the
// clan can learn it now.
func (cl *Clan) LearnableSkill(trees *modelskill.Trees, id, level int) (modelskill.ClanSkill, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return trees.ClanSkillFor(cl.level, cl.skillLevelsLocked(), modelskill.ID(id), level)
}

// MinPledgeClass reports the clan rank a member needs to hold a clan skill;
// ok is false when the skill's definition is not loaded.
type MinPledgeClass func(Skill) (class int, ok bool)

// SkillsFor returns the clan's skills a member of pledgeClass holds, the
// member fighting in the Olympiad or not. ok is false when it holds none
// at all: while the clan's reputation is 0 or less, or in the Olympiad.
// Otherwise it holds each skill whose minimum rank it reaches.
func (cl *Clan) SkillsFor(pledgeClass int, olympiad bool, minClass MinPledgeClass) (skills []Skill, ok bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	if cl.reputation <= 0 || olympiad {
		return nil, false
	}
	return cl.reachedLocked(pledgeClass, minClass), true
}

// ReachedSkills returns the clan's skills whose minimum rank pledgeClass
// reaches, whatever the clan's reputation.
func (cl *Clan) ReachedSkills(pledgeClass int, minClass MinPledgeClass) []Skill {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.reachedLocked(pledgeClass, minClass)
}

func (cl *Clan) reachedLocked(pledgeClass int, minClass MinPledgeClass) []Skill {
	var out []Skill
	for _, sk := range cl.skillsLocked() {
		if class, ok := minClass(sk); ok && class <= pledgeClass {
			out = append(out, sk)
		}
	}
	return out
}

// GivesSkill reports whether a member of pledgeClass, in the Olympiad or
// not, is given sk as the clan learns it.
func (cl *Clan) GivesSkill(pledgeClass int, olympiad bool, sk Skill, minClass MinPledgeClass) bool {
	if cl.Reputation() <= 0 || olympiad {
		return false
	}
	class, ok := minClass(sk)
	return ok && class <= pledgeClass
}

// SkillOffer returns the tree node c's clan learns skill id at level from,
// when c leads its clan and the clan can learn that level now. needsItem
// reports whether learning it takes the node's item, which it does only
// when the item is required and the node names one.
func (s *Service) SkillOffer(c *player.Character, trees *modelskill.Trees, id, level int) (node modelskill.ClanSkill, needsItem, ok bool) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return modelskill.ClanSkill{}, false, false
	}
	node, ok = cl.LearnableSkill(trees, id, level)
	return node, ok && s.cfg.LifeCrystalNeeded && node.ItemID != 0, ok
}

// Clan skill learning outcomes, in the order they happen.
type (
	// SkillUnavailable refuses silently: the player does not lead a clan,
	// or the skill is not the clan's next learnable level of it.
	SkillUnavailable struct{}
	// SkillLowReputation refuses a skill the clan's reputation does not
	// cover.
	SkillLowReputation struct{}
	// SkillMissingItem refuses a skill whose item the leader does not
	// carry.
	SkillMissingItem struct{}
	// SkillLearned reports the skill the clan now knows. Refresh is true
	// when its price took the reputation to 0 or below: the members' clan
	// skills were just turned off, so none is given the new one.
	SkillLearned struct {
		Skill   Skill
		Refresh bool
	}
)

// learnRefusalLocked returns the node for skill id at level when the clan
// can learn it now and its reputation covers the cost; otherwise the
// refusal, SkillUnavailable or SkillLowReputation. cl.mu is held.
func (cl *Clan) learnRefusalLocked(trees *modelskill.Trees, id, level int) (modelskill.ClanSkill, any) {
	node, status := trees.CheckClanSkillLearn(cl.level, cl.reputation, cl.skillLevelsLocked(), modelskill.ID(id), level)
	switch status {
	case modelskill.LearnUnavailable:
		return node, SkillUnavailable{}
	case modelskill.LearnNeedsCost:
		return node, SkillLowReputation{}
	}
	return node, nil
}

// LearnSkill has c's clan learn skill id at level, the next level the
// clan can learn of it, for its reputation cost and, when the item is
// required, one of its item paid through payItem. Only the clan's leader
// may. The reputation check is repeated once the item is paid, under the
// same hold of the clan's lock that takes the cost and adds the skill.
func (s *Service) LearnSkill(c *player.Character, trees *modelskill.Trees, id, level int, payItem func(itemID int32) bool) []any {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return []any{SkillUnavailable{}}
	}
	cl.mu.RLock()
	node, refusal := cl.learnRefusalLocked(trees, id, level)
	cl.mu.RUnlock()
	if refusal != nil {
		return []any{refusal}
	}
	if s.cfg.LifeCrystalNeeded && !payItem(node.ItemID) {
		return []any{SkillMissingItem{}}
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if _, refusal := cl.learnRefusalLocked(trees, id, level); refusal != nil {
		return []any{refusal}
	}
	var notices []any
	change, changed := s.addReputationLocked(cl, -node.Cost)
	if changed {
		notices = append(notices, change)
	}
	notices = append(notices, ReputationDeducted{Points: node.Cost})
	sk := Skill{ID: id, Level: level}
	if cl.skills == nil {
		cl.skills = map[int]int{}
	}
	cl.skills[id] = level
	clanID := cl.id
	s.write(clanID, "store clan skill", func(ctx context.Context, st Store) error { return st.SaveSkill(ctx, clanID, sk) })
	return append(notices, SkillLearned{Skill: sk, Refresh: changed && change.Crossed != 0})
}
