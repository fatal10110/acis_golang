package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type reviveFakeCaster struct{ wit float64 }

func (c reviveFakeCaster) WITBonus() float64 { return c.wit }

type reviveFakeTarget struct{ percent float64 }

func (t *reviveFakeTarget) Revive(percent float64) { t.percent = percent }

func TestResurrectRevivesEveryTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := reviveFakeCaster{wit: 1.5}
	a := &reviveFakeTarget{}
	b := &reviveFakeTarget{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []any{a, b, "not revivable"},
	}) {
		t.Fatal("Use() returned false for RESURRECT")
	}

	want := formulas.RevivePower(1.5, 40)
	if a.percent != want || b.percent != want {
		t.Fatalf("revive percent = %v/%v, want %v", a.percent, b.percent, want)
	}
}

func TestResurrectWithoutCasterInterfaceIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "RESURRECT"},
		Targets: []any{a},
	})
	if a.percent != 0 {
		t.Fatalf("revive percent = %v, want unchanged 0", a.percent)
	}
}
