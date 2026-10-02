package player

import (
	"math"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestCharacterSkillLandingRateFromEffectPower pins the generic skill
// landing rate of shipped debuffs (level 1 of each, definitions copied from
// aCis_datapack/data/xml/skills) cast by a level 1 character (M.Atk 53) at
// a level 1 character (M.Def 57, CON 43, MEN 25).
//
// Expected rates are the reference landing formula worked by hand:
// clamp(effectPower * statMod * vuln * mAtkMod * lvlMod, 1, 99), with
// effectPower and the resisted type from the skill's effect templates, else
// its effectPower/effectType, else its power and skill type; statMod
// 2 - sqrt(MEN_BONUS[25] = 1.28) for magic MEN-resisted types and
// 2 - sqrt(CON_BONUS[43] = 1.58) for STUN; mAtkMod sqrt(53) / 57 * 11;
// lvlMod 1 + 0.005 * (magicLvl + lvlDepend - 1).
func TestCharacterSkillLandingRateFromEffectPower(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	if caster.MAtk() != 53 || target.MDef() != 57 {
		t.Fatalf("fixture M.Atk/M.Def = %v/%v, want 53/57", caster.MAtk(), target.MDef())
	}

	tests := []struct {
		name     string
		def      modelskill.Definition
		wantBase float64
		wantRate float64
	}{
		{
			name: "4164 Paralysis (power 15)",
			def: modelskill.Definition{
				ID: 4164, Level: 1, SkillType: "PARALYZE", Power: 15, Magic: true, Debuff: true,
				MagicLevel: 40, LevelDepend: 1,
				Effects: []modelskill.EffectTemplate{{Name: "Paralyze", Time: 120, EffectPower: -1}},
			},
			wantBase: 15,
			wantRate: 21.966591260048144,
		},
		{
			name: "4053 Decrease P.Atk (power 80, clamped)",
			def: modelskill.Definition{
				ID: 4053, Level: 1, SkillType: "DEBUFF", Power: 80, Magic: true, Debuff: true,
				MagicLevel: 40, LevelDepend: 2,
				Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 120, EffectPower: -1}},
			},
			wantBase: 80,
			wantRate: 99,
		},
		{
			name: "105 Freezing Strike (MDAM, template effectPower 60 DEBUFF)",
			def: modelskill.Definition{
				ID: 105, Level: 1, SkillType: "MDAM", Power: 26, Magic: true, Debuff: true,
				MagicLevel: 34, LevelDepend: 1, Element: modelskill.ElementWater,
				Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, EffectPower: 60, EffectPowerSet: true, EffectType: "DEBUFF"}},
			},
			wantBase: 60,
			wantRate: 85.66970591418776,
		},
		{
			name: "100 Stun Attack (PDAM, template effectPower 50 STUN)",
			def: modelskill.Definition{
				ID: 100, Level: 1, SkillType: "PDAM", Power: 30, Debuff: true,
				MagicLevel: 18, LevelDepend: 1,
				Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 9, EffectPower: 50, EffectPowerSet: true, EffectType: "STUN"}},
			},
			wantBase: 50,
			wantRate: 40.49456225962788,
		},
	}
	for _, tt := range tests {
		in, ok := target.SkillSuccessInput(caster, tt.def, false, formulas.ShieldFailed)
		if !ok {
			t.Fatalf("%s: SkillSuccessInput ok = false", tt.name)
		}
		if in.BaseChance != tt.wantBase {
			t.Errorf("%s: BaseChance = %v, want %v", tt.name, in.BaseChance, tt.wantBase)
		}
		if got := formulas.SkillSuccessRate(in); math.Abs(got-tt.wantRate) > 1e-9 {
			t.Errorf("%s: landing rate = %v, want %v", tt.name, got, tt.wantRate)
		}
	}
}

// TestCharacterEffectLandingKeepsTemplateType asserts a template's own
// landing roll resists as that template's type, not the skill's first
// typed template: the second (STUN) template of a skill whose first
// template is DEBUFF resists through CON (2 - sqrt(1.58)) at its own
// power 30.
func TestCharacterEffectLandingKeepsTemplateType(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	stun := modelskill.EffectTemplate{Name: "Stun", EffectPower: 30, EffectPowerSet: true, EffectType: "STUN"}
	def := modelskill.Definition{
		SkillType: "PDAM", Power: 30,
		Effects: []modelskill.EffectTemplate{
			{Name: "Debuff", EffectPower: 60, EffectPowerSet: true, EffectType: "DEBUFF"},
			stun,
		},
	}

	in, ok := target.EffectSuccessInput(caster, def, stun, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("EffectSuccessInput ok = false")
	}
	if in.BaseChance != 30 || in.IgnoreResists {
		t.Fatalf("EffectSuccessInput = %+v, want base 30 with resists", in)
	}
	if want := 0.7430194910023464; math.Abs(in.StatModifier-want) > 1e-12 {
		t.Fatalf("StatModifier = %v, want %v (STUN)", in.StatModifier, want)
	}
}
