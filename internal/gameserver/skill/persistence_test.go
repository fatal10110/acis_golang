package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSetKnownSkillAttachesPassiveStatsAndReplacesOnRelearn(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
		{ID: 134, Level: 2, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 15},
		}},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 134, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got, want := ch.PAtk(), base+7; got != want {
		t.Fatalf("PAtk() after learning level 1 = %v, want %v", got, want)
	}

	if err := p.SetKnownSkill(context.Background(), ch, 134, 2); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got, want := ch.PAtk(), base+15; got != want {
		t.Fatalf("PAtk() after relearning at level 2 = %v, want %v (level 1's bonus must be dropped)", got, want)
	}
}

func TestSetKnownSkillDoesNotApplyStatsForNonPassiveOrUnloadedSkill(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 60, Level: 1, Activation: modelskill.ActivationToggle},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 60, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after learning a toggle skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(60) != 1 {
		t.Fatalf("SkillLevel(60) = %d, want 1", ch.SkillLevel(60))
	}

	if err := p.SetKnownSkill(context.Background(), ch, 999, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after learning an unloaded skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(999) != 1 {
		t.Fatalf("SkillLevel(999) = %d, want 1", ch.SkillLevel(999))
	}
}

func TestSetKnownSkillDropsPassiveStatsWhenSkillIsRemoved(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 134, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if err := p.SetKnownSkill(context.Background(), ch, 134, 0); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}

	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after removing the passive skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(134) != 0 {
		t.Fatalf("SkillLevel(134) = %d, want 0", ch.SkillLevel(134))
	}
}

func TestRestoreKnownSkillsAttachesPassiveStats(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
	})
	p := NewPersistence(nil, table, fakeSkillLevelStore{levels: player.SkillLevels{134: 1, 9999: 1}})
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.Restore(context.Background(), ch); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if got, want := ch.PAtk(), base+7; got != want {
		t.Fatalf("PAtk() after restoring a passive skill = %v, want %v", got, want)
	}
	if ch.SkillLevel(9999) != 0 {
		t.Fatalf("stale unloaded skill level = %d, want 0 (not restored)", ch.SkillLevel(9999))
	}
}

type fakeSkillLevelStore struct {
	levels player.SkillLevels
}

func (s fakeSkillLevelStore) ListKnownSkills(context.Context, int32, int32) (player.SkillLevels, error) {
	return s.levels, nil
}
