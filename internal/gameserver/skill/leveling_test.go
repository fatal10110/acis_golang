package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recordingSkillLevelStore records every character_skills write and delete
// GiveSkills makes, so a test can tell a free level entitlement (memory
// only) from a correction to something the character really learned
// (persisted).
type recordingSkillLevelStore struct {
	known   player.SkillLevels
	written []writtenSkill
	deleted []int
}

type writtenSkill struct {
	skillID int
	level   int
}

func (s *recordingSkillLevelStore) ListKnownSkills(context.Context, int32, int32) (player.SkillLevels, error) {
	return s.known, nil
}

func (s *recordingSkillLevelStore) SetKnownSkill(_ context.Context, _ int32, _ int32, skillID, level int) error {
	s.written = append(s.written, writtenSkill{skillID: skillID, level: level})
	return nil
}

func (s *recordingSkillLevelStore) DeleteKnownSkill(_ context.Context, _ int32, _ int32, skillID int) error {
	s.deleted = append(s.deleted, skillID)
	return nil
}

func newLevelingPersistence(store *recordingSkillLevelStore) *Persistence {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1}, {ID: 3, Level: 2}, {ID: 3, Level: 3},
		{ID: 194, Level: 1},
		{ID: 239, Level: 1},
		{ID: 249, Level: 1}, {ID: 249, Level: 2},
	})
	return NewPersistence(nil, table, store)
}

// TestGiveSkillsGrantsFreeSkillsWithoutPersisting pins the free half of the
// refresh: the level's zero-cost grants land on the character, but leave no
// character_skills row behind, because the next level change re-derives them.
func TestGiveSkillsGrantsFreeSkillsWithoutPersisting(t *testing.T) {
	store := &recordingSkillLevelStore{}
	p := newLevelingPersistence(store)
	c := &player.Character{ID: 1, CharLevel: 10}
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
	}}

	if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
		t.Fatalf("GiveSkills() error: %v", err)
	}
	if got := c.SkillLevel(249); got != 2 {
		t.Errorf("SkillLevel(249) = %d, want 2 (highest free grant the level unlocks)", got)
	}
	if got := c.SkillLevel(3); got != 0 {
		t.Errorf("SkillLevel(3) = %d, want 0 (a bought skill is never handed over)", got)
	}
	if len(store.written) != 0 || len(store.deleted) != 0 {
		t.Errorf("persisted writes = %v, deletes = %v; want none for free grants", store.written, store.deleted)
	}
}

func TestRewardSkillsGrantsAllAvailableSkillsWithSelectivePersistence(t *testing.T) {
	store := &recordingSkillLevelStore{}
	p := newLevelingPersistence(store)
	c := &player.Character{ID: 1, CharLevel: 10}
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
	}}

	if err := p.RewardSkills(context.Background(), c, tmpl); err != nil {
		t.Fatalf("RewardSkills() error: %v", err)
	}
	if got := c.SkillLevel(249); got != 2 {
		t.Errorf("SkillLevel(249) = %d, want 2", got)
	}
	if got := c.SkillLevel(3); got != 1 {
		t.Errorf("SkillLevel(3) = %d, want 1", got)
	}
	if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 1}) {
		t.Errorf("persisted writes = %v, want [{3 1}]", store.written)
	}
}

// TestGiveSkillsDropsLuckyAtMaxLevel pins the newbie Lucky skill going away
// at level 10, without a persisted delete: it was never persisted either.
func TestGiveSkillsDropsLuckyAtMaxLevel(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
	}}

	t.Run("below the cutoff it is granted", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 9}
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(194); got != 1 {
			t.Errorf("SkillLevel(194) at level 9 = %d, want 1", got)
		}
	})

	t.Run("at the cutoff it is taken away", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 10}
		c.SetSkillLevel(194, 1)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(194); got != 0 {
			t.Errorf("SkillLevel(194) at level 10 = %d, want 0", got)
		}
		if len(store.deleted) != 0 {
			t.Errorf("persisted deletes = %v, want none", store.deleted)
		}
	})
}

// TestGiveSkillsCorrectsSkillsTheLevelNoLongerSupports pins the half of the
// refresh a level loss depends on, and that those corrections do reach
// character_skills.
func TestGiveSkillsCorrectsSkillsTheLevelNoLongerSupports(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
		{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
	}}

	t.Run("downgrades a skill held above the level's grant", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 11}
		c.SetSkillLevel(3, 3)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		// Level 11 reaches the MinLevel-20 grant through the nine-level
		// slack, but not the level-3 one at MinLevel 40.
		if got := c.SkillLevel(3); got != 2 {
			t.Errorf("SkillLevel(3) = %d, want 2", got)
		}
		if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 2}) {
			t.Errorf("persisted writes = %v, want [{3 2}]", store.written)
		}
	})

	t.Run("drops a skill the level cannot hold at all", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 1}
		c.SetSkillLevel(3, 1)
		// Every grant for skill 3 here sits beyond level 1's nine-level
		// slack, so the character may not hold it at any level.
		highOnly := &player.Template{Skills: []player.SkillGrant{
			{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
			{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
		}}
		if err := p.GiveSkills(context.Background(), c, highOnly); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 0 {
			t.Errorf("SkillLevel(3) = %d, want 0", got)
		}
		if len(store.deleted) != 1 || store.deleted[0] != 3 {
			t.Errorf("persisted deletes = %v, want [3]", store.deleted)
		}
	})

	t.Run("leaves skills the profession never grants alone", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 1}
		// 4267 comes from equipment state, not the profession line.
		c.SetSkillLevel(4267, 1)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(4267); got != 1 {
			t.Errorf("SkillLevel(4267) = %d, want 1 (untouched)", got)
		}
		if len(store.written) != 0 || len(store.deleted) != 0 {
			t.Errorf("persisted writes = %v, deletes = %v; want none", store.written, store.deleted)
		}
	})
}

// TestGiveSkillsKeepsEnchantOnlyAtHighLevel pins the enchanted-skill branch:
// a skill levelled past the top of its table keeps that enchant only from
// level 76 up, and only while the profession already grants it at full table
// level.
func TestGiveSkillsKeepsEnchantOnlyAtHighLevel(t *testing.T) {
	// Skill 3's table tops out at level 3; 101 is an enchant route level.
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
	}}

	t.Run("kept at 76 and above", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 76}
		c.SetSkillLevel(3, 101)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 101 {
			t.Errorf("SkillLevel(3) at level 76 = %d, want 101", got)
		}
	})

	t.Run("pulled back below 76", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 75}
		c.SetSkillLevel(3, 101)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 3 {
			t.Errorf("SkillLevel(3) at level 75 = %d, want 3", got)
		}
		if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 3}) {
			t.Errorf("persisted writes = %v, want [{3 3}]", store.written)
		}
	})
}
