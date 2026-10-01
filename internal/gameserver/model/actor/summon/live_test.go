package summon

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestNewServitorAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	stats := CombatStats{MaxHP: 1000, CON: 40}
	base := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	passive := modelskill.Ref{ID: 99, Level: 1}
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	got := mustServitor(t, ServitorConfig{
		ObjectID:  2,
		Level:     44,
		Stats:     stats,
		Roll:      zeroSummonRoll,
		Passives:  []modelskill.Ref{passive},
		SkillDefs: table,
	})

	wantMax := base.MaxHPValue() + 250
	if got.MaxHPValue() != wantMax {
		t.Fatalf("MaxHPValue() = %v, want %v (base %v + 250 add after CON mul)", got.MaxHPValue(), wantMax, base.MaxHPValue())
	}
	if got.HP() != wantMax {
		t.Fatalf("HP() = %v, want %v (seed after funcs attach)", got.HP(), wantMax)
	}
}

func TestNewPetAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	stats := CombatStats{MaxHP: 500, CON: 40}
	base := mustPet(t, PetConfig{ObjectID: 1, Level: 40, Stats: stats, Roll: zeroSummonRoll})

	passive := modelskill.Ref{ID: 99, Level: 1}
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	got := mustPet(t, PetConfig{
		ObjectID:  2,
		Level:     40,
		Stats:     stats,
		Roll:      zeroSummonRoll,
		Passives:  []modelskill.Ref{passive},
		SkillDefs: table,
	})

	wantMax := base.MaxHPValue() + 250
	if got.MaxHPValue() != wantMax {
		t.Fatalf("MaxHPValue() = %v, want %v", got.MaxHPValue(), wantMax)
	}
	if got.HP() != wantMax {
		t.Fatalf("HP() = %v, want %v", got.HP(), wantMax)
	}
}

func TestNewServitorFailsOnTemplatePassiveBuildError(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:    99,
		Level: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "notAStat", Value: 1}},
	}})
	_, err := NewServitor(ServitorConfig{
		ObjectID:  1,
		Passives:  []modelskill.Ref{{ID: 99, Level: 1}},
		SkillDefs: table,
	})
	if err == nil {
		t.Fatal("NewServitor() error = nil, want template-passive build error")
	}
}
