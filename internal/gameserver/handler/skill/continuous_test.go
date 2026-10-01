package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestContinuousRegistryHasAllHandledTypes(t *testing.T) {
	registry := NewDefaultRegistry()
	for _, typ := range []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF", "FUSION",
	} {
		if _, ok := registry.Handler(typ); !ok {
			t.Errorf("continuous handler missing registered skill type %q", typ)
		}
	}
}

func TestContinuousDebuffSkipsInvulnerableTargetWithoutAttackFailed(t *testing.T) {
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.invul = true
	result := continuousHandler{}.UseResult(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType: "DEBUFF", Debuff: true, Offensive: true,
			IgnoreResists: true, BaseLandRate: 100,
			Effects: buffEffect(),
		},
		Targets: []Actor{target},
	})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 (land roll succeeded; apply refused)", result.AttackFailed)
	}
	if got := len(target.list.All()); got != 0 {
		t.Fatalf("invulnerable target received %d effects, want 0", got)
	}
}

func TestContinuousBuffLandsOnInvulnerableTarget(t *testing.T) {
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.invul = true
	continuousHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []Actor{target},
	})
	if got := len(target.list.All()); got != 1 {
		t.Fatalf("buff on invulnerable target landed %d effects, want 1", got)
	}
}

func TestContinuousDebuffSkipsWhenCasterCannotGiveDamage(t *testing.T) {
	caster := newContinuousFake(1)
	caster.denyDamage = true
	target := newContinuousFake(2)
	result := continuousHandler{}.UseResult(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType: "DEBUFF", Debuff: true,
			IgnoreResists: true, BaseLandRate: 100,
			Effects: buffEffect(),
		},
		Targets: []Actor{target},
	})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 (land roll succeeded; apply refused)", result.AttackFailed)
	}
	if got := len(target.list.All()); got != 0 {
		t.Fatalf("denied-damage caster landed %d effects, want 0", got)
	}
}

// TestContinuousRollsLethalAfterLandingRoll pins the lethal strike the
// continuous handler rolls on every creature target after the landing roll:
// a failed landing still rolls it, ATTACK_FAILED is reported before the
// lethal, a reflected cast rolls it against the caster, and the effect-id
// substitute skill supplies the lethal chances.
func TestContinuousRollsLethalAfterLandingRoll(t *testing.T) {
	sureLethal := formulas.LethalInput{AttackerLevel: 40, TargetLevel: 40, LethalMul: 1}
	fear := modelskill.Definition{SkillType: "FEAR", Offensive: true, LethalChance2: 100}

	t.Run("failed landing", func(t *testing.T) {
		target := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{hp: 500}, Skill: fear, Targets: []Actor{target}})
		if result.AttackFailed != 1 || target.hp != 1 {
			t.Fatalf("AttackFailed = %d, target HP = %v; want 1 and a lethal strike to 1 HP", result.AttackFailed, target.hp)
		}
		if len(result.Messages) != 2 {
			t.Fatalf("messages = %#v, want ATTACK_FAILED then the lethal", result.Messages)
		}
		if _, ok := result.Messages[0].(AttackFailedMessage); !ok {
			t.Fatalf("first message = %T, want AttackFailedMessage", result.Messages[0])
		}
		if _, ok := result.Messages[1].(Lethal); !ok {
			t.Fatalf("second message = %T, want Lethal", result.Messages[1])
		}
	})

	t.Run("reflected onto caster", func(t *testing.T) {
		caster := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		target := &skillTarget{hp: 500, reflects: true, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: fear, Targets: []Actor{target}})
		if caster.hp != 1 || target.hp != 500 {
			t.Fatalf("caster HP = %v, target HP = %v; want the reflected lethal on the caster only", caster.hp, target.hp)
		}
		if len(result.Lethals) != 1 {
			t.Fatalf("lethals = %+v, want one", result.Lethals)
		}
	})

	t.Run("raid related target", func(t *testing.T) {
		target := &skillTarget{hp: 500, raidRelated: true, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{hp: 500}, Skill: fear, Targets: []Actor{target}})
		if target.hp != 500 || len(result.Lethals) != 0 {
			t.Fatalf("raid target HP = %v, lethals = %+v; want untouched", target.hp, result.Lethals)
		}
	})

	t.Run("effect-id substitute supplies the chances", func(t *testing.T) {
		defs := continuousDefinitions{{ID: 7, Level: 1}: {ID: 7, Level: 1, SkillType: "DEBUFF", Debuff: true, LethalChance2: 100}}
		target := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		continuousHandler{defs: defs}.UseResult(Cast{
			Caster:  &skillTarget{hp: 500},
			Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, EffectID: 7},
			Targets: []Actor{target},
		})
		if target.hp != 1 {
			t.Fatalf("target HP = %v, want the substitute skill's lethal strike to 1 HP", target.hp)
		}
	})
}

// TestContinuousPassesBlessedShotAndShieldToTemplateLanding pins the inputs
// the continuous handler hands each per-template landing roll: the blessed
// spiritshot sampled at cast start, and the shield outcome it resolved for
// an offensive or debuff skill (no block for anything else).
func TestContinuousPassesBlessedShotAndShieldToTemplateLanding(t *testing.T) {
	for _, tc := range []struct {
		name       string
		def        modelskill.Definition
		bss        bool
		shield     formulas.ShieldDefense
		want       templateLanding
		wantRolls  int
		wantLanded int
	}{
		{
			name:   "debuff with blessed shot and shield block",
			def:    modelskill.Definition{ID: 1, SkillType: "DEBUFF", Debuff: true, Offensive: true, Magic: true},
			bss:    true,
			shield: formulas.ShieldSuccess,
			want:   templateLanding{bss: true, shield: formulas.ShieldSuccess}, wantRolls: 1, wantLanded: 1,
		},
		{
			name:   "debuff without blessed shot",
			def:    modelskill.Definition{ID: 1, SkillType: "DEBUFF", Debuff: true, Magic: true},
			shield: formulas.ShieldSuccess,
			want:   templateLanding{shield: formulas.ShieldSuccess}, wantRolls: 1, wantLanded: 1,
		},
		{
			name:   "buff never rolls the shield",
			def:    modelskill.Definition{ID: 1, SkillType: "BUFF", Magic: true},
			bss:    true,
			shield: formulas.ShieldSuccess,
			want:   templateLanding{bss: true, shield: formulas.ShieldFailed}, wantRolls: 0, wantLanded: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := &bssCasterFake{bss: tc.bss}
			target := newDisablerFake(2)
			target.shield = tc.shield
			def := tc.def
			def.Effects = rolledStun()
			continuousHandler{}.UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

			if target.shieldRolls != tc.wantRolls {
				t.Fatalf("shield rolls = %d, want %d", target.shieldRolls, tc.wantRolls)
			}
			if len(target.templateLandings) != 1 || target.templateLandings[0] != tc.want {
				t.Fatalf("template landing inputs = %+v, want [%+v]", target.templateLandings, tc.want)
			}
			if got := len(target.list.All()); got != tc.wantLanded {
				t.Fatalf("landed effects = %d, want %d", got, tc.wantLanded)
			}
		})
	}
}
