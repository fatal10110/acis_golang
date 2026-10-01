package clan

import (
	"context"
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func (f *fakeStore) SaveSkill(context.Context, int32, Skill) error { return nil }

// vitalityTree is Clan Vitality's first two levels, as clanSkills.xml lists
// them: 500 reputation and one item 8166 each, from clan level 5.
func vitalityTree() *modelskill.Trees {
	return &modelskill.Trees{Clan: []modelskill.ClanSkill{
		{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166},
		{ID: 370, Level: 2, MinLevel: 5, Cost: 500, ItemID: 8166},
	}}
}

// TestSkillsForFollowsReputationAndRank gives a member the clan skills its
// rank reaches while the reputation is above 0 and it is not in the
// Olympiad; at 0 reputation it holds none at all.
func TestSkillsForFollowsReputationAndRank(t *testing.T) {
	_, cl, _, _ := orderedClan(t, 1, 0)
	cl.skills = map[int]int{370: 1, 371: 2}
	minClass := func(sk Skill) (int, bool) {
		switch sk.ID {
		case 370:
			return 2, true
		case 371:
			return 5, true
		}
		return 0, false
	}

	if got, ok := cl.SkillsFor(2, false, minClass); !ok || !slices.Equal(got, []Skill{{ID: 370, Level: 1}}) {
		t.Fatalf("rank 2 = %v %v, want Clan Vitality only", got, ok)
	}
	if got, ok := cl.SkillsFor(5, false, minClass); !ok || len(got) != 2 {
		t.Fatalf("rank 5 = %v %v, want both skills", got, ok)
	}
	if _, ok := cl.SkillsFor(8, true, minClass); ok {
		t.Fatal("an Olympiad fighter was given clan skills")
	}
	cl.reputation = 0
	if _, ok := cl.SkillsFor(8, false, minClass); ok {
		t.Fatal("a member was given clan skills at 0 reputation")
	}
	if cl.GivesSkill(8, false, Skill{ID: 370, Level: 1}, minClass) {
		t.Fatal("a learnt skill was given at 0 reputation")
	}
}

// TestLearnSkillRechecksReputationAfterPayment drops the clan's reputation
// below the price while the leader's item is being paid: the clan learns
// nothing and keeps its score.
func TestLearnSkillRechecksReputationAfterPayment(t *testing.T) {
	s, cl, _, _ := orderedClan(t, 600, 0)
	leader := inClan(orderLeaderID, "Leader", orderClanID)
	pay := func(itemID int32) bool {
		if itemID != 8166 {
			t.Fatalf("paid item %d, want 8166", itemID)
		}
		cl.mu.Lock()
		cl.reputation = 100
		cl.mu.Unlock()
		return true
	}

	got := s.LearnSkill(leader, vitalityTree(), 370, 1, pay)
	if len(got) != 1 || got[0] != (SkillLowReputation{}) {
		t.Fatalf("notices = %#v, want SkillLowReputation", got)
	}
	if skills := cl.Skills(); len(skills) != 0 || cl.Reputation() != 100 {
		t.Fatalf("clan = skills %v reputation %d, want none and 100", skills, cl.Reputation())
	}
}

// TestLearnSkillTakesReputationToZero learns Clan Vitality with exactly
// its price: the score crosses to 0, the deduction is reported, and the
// skill is learnt with the members' refresh pending.
func TestLearnSkillTakesReputationToZero(t *testing.T) {
	s, cl, store, writes := orderedClan(t, 500, 1)
	leader := inClan(orderLeaderID, "Leader", orderClanID)

	got := s.LearnSkill(leader, vitalityTree(), 370, 1, func(int32) bool { return true })
	want := []any{
		ReputationChanged{Score: 0, Crossed: -1},
		ReputationDeducted{Points: 500},
		SkillLearned{Skill: Skill{ID: 370, Level: 1}, Refresh: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("notices = %#v, want %#v", got, want)
	}
	writes.drain()
	if skills := cl.Skills(); !slices.Equal(skills, []Skill{{ID: 370, Level: 1}}) || store.clans[orderClanID].reputation != 0 {
		t.Fatalf("clan skills %v, stored reputation %d; want Clan Vitality 1 and 0", skills, store.clans[orderClanID].reputation)
	}
	if got := s.LearnSkill(leader, vitalityTree(), 370, 1, func(int32) bool { return true }); got[0] != (SkillUnavailable{}) {
		t.Fatalf("relearning level 1 = %#v, want SkillUnavailable", got)
	}
	member := inClan(orderLeaderID+1, "Member0", orderClanID)
	if got := s.LearnSkill(member, vitalityTree(), 370, 2, func(int32) bool { return true }); got[0] != (SkillUnavailable{}) {
		t.Fatalf("a member's learn = %#v, want SkillUnavailable", got)
	}
}
