package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type persistenceTestGeo struct{}

func (persistenceTestGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (persistenceTestGeo) Height(_, _, _ int) int16          { return 0 }
func (persistenceTestGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (persistenceTestGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

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

func TestApplyTransientPassiveSkillReplacesStatsWithoutLearningSkill(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 5076, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7}}},
		{ID: 5076, Level: 2, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 15}}},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.ApplyTransientPassiveSkill(ch, 5076, 0, 1); err != nil {
		t.Fatalf("ApplyTransientPassiveSkill() level 1 error: %v", err)
	}
	if got, want := ch.PAtk(), base+7; got != want {
		t.Fatalf("PAtk() at transient level 1 = %v, want %v", got, want)
	}
	if got := ch.SkillLevel(5076); got != 0 {
		t.Fatalf("SkillLevel(5076) = %d, want 0 for transient passive", got)
	}

	if err := p.ApplyTransientPassiveSkill(ch, 5076, 1, 2); err != nil {
		t.Fatalf("ApplyTransientPassiveSkill() level 2 error: %v", err)
	}
	if got, want := ch.PAtk(), base+15; got != want {
		t.Fatalf("PAtk() at transient level 2 = %v, want %v", got, want)
	}

	if err := p.ApplyTransientPassiveSkill(ch, 5076, 2, 0); err != nil {
		t.Fatalf("ApplyTransientPassiveSkill() removal error: %v", err)
	}
	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after transient removal = %v, want %v", got, base)
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
